package e2e

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binary string

// runEnv is like run but also sets extra environment variables.
func runEnv(t *testing.T, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), env...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("unexpected exec error: %v", err)
		}
	}
	return
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "stowage-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "MkdirTemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	binary = filepath.Join(dir, "stowage")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/stowage")
	cmd.Dir = repoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// run executes the stowage binary with the given arguments and returns stdout,
// stderr, and exit code. It never calls t.Fatal on non-zero exit.
func run(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("unexpected exec error: %v", err)
		}
	}
	return
}

// repoRoot returns the module root by walking up from this file's directory.
func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found")
		}
		dir = parent
	}
}

// testMEKHex is a 32-byte key encoded as hex for CLI use.
var testMEKHex = hex.EncodeToString(bytes.Repeat([]byte{0x42}, 32))

// writeFile writes content to a temp file and returns its path.
func writeFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stowage-input-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	f.Close()
	return f.Name()
}

func TestE2E_StoreRetrieve_Identical(t *testing.T) {
	blobRoot := t.TempDir()
	content := bytes.Repeat([]byte("the quick brown fox "), 100)
	inputPath := writeFile(t, content)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	outputPath := filepath.Join(t.TempDir(), "output")

	stdout, stderr, code := run(t,
		"store",
		"-root", blobRoot,
		"-input", inputPath,
		"-manifest", manifestPath,
		"-encryption", "none",
		"-compression", "none",
	)
	if code != 0 {
		t.Fatalf("store failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	stdout, stderr, code = run(t,
		"retrieve",
		"-root", blobRoot,
		"-manifest", manifestPath,
		"-output", outputPath,
	)
	if code != 0 {
		t.Fatalf("retrieve failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "integrity ok") {
		t.Errorf("retrieve stdout missing 'integrity ok': %q", stdout)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile output: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %d bytes, want %d bytes", len(got), len(content))
	}
}

func TestE2E_AllCompressions(t *testing.T) {
	for _, comp := range []string{"none", "gzip", "zstd", "lz4"} {
		t.Run(comp, func(t *testing.T) {
			blobRoot := t.TempDir()
			content := bytes.Repeat([]byte("compression test data "), 200)
			inputPath := writeFile(t, content)
			manifestPath := filepath.Join(t.TempDir(), "manifest.json")
			outputPath := filepath.Join(t.TempDir(), "output")

			stdout, stderr, code := run(t,
				"store",
				"-root", blobRoot,
				"-input", inputPath,
				"-manifest", manifestPath,
				"-compression", comp,
				"-encryption", "none",
			)
			if code != 0 {
				t.Fatalf("store failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}

			stdout, stderr, code = run(t,
				"retrieve",
				"-root", blobRoot,
				"-manifest", manifestPath,
				"-output", outputPath,
			)
			if code != 0 {
				t.Fatalf("retrieve failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}

			got, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if !bytes.Equal(got, content) {
				t.Errorf("content mismatch after %s round-trip", comp)
			}
		})
	}
}

func TestE2E_Encryption_CorrectKey(t *testing.T) {
	blobRoot := t.TempDir()
	content := []byte("secret content that must be encrypted")
	inputPath := writeFile(t, content)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	outputPath := filepath.Join(t.TempDir(), "output")

	stdout, stderr, code := run(t,
		"store",
		"-root", blobRoot,
		"-input", inputPath,
		"-manifest", manifestPath,
		"-encryption", "aes256gcm",
		"-key", testMEKHex,
	)
	if code != 0 {
		t.Fatalf("store failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	stdout, stderr, code = run(t,
		"retrieve",
		"-root", blobRoot,
		"-manifest", manifestPath,
		"-output", outputPath,
		"-key", testMEKHex,
	)
	if code != 0 {
		t.Fatalf("retrieve failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch after encrypted round-trip")
	}
}

func TestE2E_Encryption_WrongKey(t *testing.T) {
	blobRoot := t.TempDir()
	content := []byte("secret content")
	inputPath := writeFile(t, content)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	outputPath := filepath.Join(t.TempDir(), "output")

	_, _, code := run(t,
		"store",
		"-root", blobRoot,
		"-input", inputPath,
		"-manifest", manifestPath,
		"-encryption", "aes256gcm",
		"-key", testMEKHex,
	)
	if code != 0 {
		t.Fatalf("store unexpectedly failed")
	}

	wrongKey := hex.EncodeToString(bytes.Repeat([]byte{0x99}, 32))
	stdout, stderr, code := run(t,
		"retrieve",
		"-root", blobRoot,
		"-manifest", manifestPath,
		"-output", outputPath,
		"-key", wrongKey,
	)
	if code == 0 {
		t.Fatalf("retrieve with wrong key should have failed, but exited 0")
	}
	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "integrity") &&
		!strings.Contains(strings.ToLower(combined), "decrypt") {
		t.Errorf("expected 'integrity' or 'decrypt' in output, got:\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestE2E_LargeFile_StoreEraseEmpty(t *testing.T) {
	blobRoot := t.TempDir()
	// 5 chunks at default 4 MiB chunk size = 20 MiB+ of data
	content := bytes.Repeat([]byte("large file content "), 1024*1024)
	inputPath := writeFile(t, content)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")

	stdout, stderr, code := run(t,
		"store",
		"-root", blobRoot,
		"-input", inputPath,
		"-manifest", manifestPath,
		"-encryption", "none",
		"-compression", "none",
	)
	if code != 0 {
		t.Fatalf("store failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "chunks") {
		t.Errorf("store stdout missing chunk count: %q", stdout)
	}

	stdout, stderr, code = run(t,
		"erase",
		"-root", blobRoot,
		"-manifest", manifestPath,
	)
	if code != 0 {
		t.Fatalf("erase failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "erased:") {
		t.Errorf("erase stdout missing 'erased:': %q", stdout)
	}

	// After erase, no .bin files should remain.
	var binFiles []string
	_ = filepath.Walk(blobRoot, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".bin") {
			binFiles = append(binFiles, path)
		}
		return nil
	})
	if len(binFiles) > 0 {
		t.Errorf("expected no .bin files after erase, found: %v", binFiles)
	}
}

func TestE2E_ManifestFormats(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			blobRoot := t.TempDir()
			content := []byte("manifest format test content")
			inputPath := writeFile(t, content)
			ext := "." + format
			if format == "yaml" {
				ext = ".yaml"
			}
			manifestPath := filepath.Join(t.TempDir(), "manifest"+ext)
			outputPath := filepath.Join(t.TempDir(), "output")

			stdout, stderr, code := run(t,
				"store",
				"-root", blobRoot,
				"-input", inputPath,
				"-manifest", manifestPath,
				"-encryption", "none",
				"-format", format,
			)
			if code != 0 {
				t.Fatalf("store failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}

			data, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("ReadFile manifest: %v", err)
			}
			if len(data) == 0 {
				t.Fatal("manifest file is empty")
			}

			stdout, stderr, code = run(t,
				"retrieve",
				"-root", blobRoot,
				"-manifest", manifestPath,
				"-output", outputPath,
			)
			if code != 0 {
				t.Fatalf("retrieve failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}

			got, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("ReadFile output: %v", err)
			}
			if !bytes.Equal(got, content) {
				t.Errorf("content mismatch for %s format", format)
			}
		})
	}
}

func TestE2E_GC_RemovesOrphans(t *testing.T) {
	blobRoot := t.TempDir()

	// Plant orphan files directly in the root.
	for _, name := range []string{"_tmp_orphan1", "_tmp_orphan2", "_lock_stale"} {
		p := filepath.Join(blobRoot, name)
		if err := os.WriteFile(p, []byte("orphan"), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	stdout, stderr, code := run(t, "gc", "-root", blobRoot)
	if code != 0 {
		t.Fatalf("gc failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "removed 3") {
		t.Errorf("gc stdout: want 'removed 3', got %q", stdout)
	}

	// Orphan files must be gone.
	for _, name := range []string{"_tmp_orphan1", "_tmp_orphan2", "_lock_stale"} {
		p := filepath.Join(blobRoot, name)
		if _, err := os.Stat(p); err == nil {
			t.Errorf("orphan file still present: %s", name)
		}
	}
}

// TestE2E_Init creates a ~/.stowage file via the init subcommand.
func TestE2E_Init(t *testing.T) {
	fakeHome := t.TempDir()

	stdout, stderr, code := runEnv(t, []string{"HOME=" + fakeHome}, "init")
	if code != 0 {
		t.Fatalf("init failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	cfgPath := filepath.Join(fakeHome, ".stowage")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	if !strings.Contains(string(data), "compression:") {
		t.Errorf("config file missing 'compression:' field: %q", string(data))
	}

	// Second init must fail because the file already exists.
	_, _, code = runEnv(t, []string{"HOME=" + fakeHome}, "init")
	if code == 0 {
		t.Error("second init should have failed but exited 0")
	}
}

// TestE2E_ConfigFile_Root verifies that root set in ~/.stowage is used when -root is omitted.
func TestE2E_ConfigFile_Root(t *testing.T) {
	fakeHome := t.TempDir()
	blobRoot := t.TempDir()
	content := []byte("config-file root resolution test")
	inputPath := writeFile(t, content)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	outputPath := filepath.Join(t.TempDir(), "output")

	cfg := fmt.Sprintf("root: %s\ncompression: none\nencryption: none\n", blobRoot)
	if err := os.WriteFile(filepath.Join(fakeHome, ".stowage"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	env := []string{"HOME=" + fakeHome}

	// No -root flag — must be resolved from config.
	stdout, stderr, code := runEnv(t, env, "store",
		"-input", inputPath,
		"-manifest", manifestPath,
	)
	if code != 0 {
		t.Fatalf("store failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	stdout, stderr, code = runEnv(t, env, "retrieve",
		"-manifest", manifestPath,
		"-output", outputPath,
	)
	if code != 0 {
		t.Fatalf("retrieve failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile output: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch")
	}
}

// TestE2E_EnvKey verifies that STOWAGE_KEY is picked up without -key flag.
func TestE2E_EnvKey(t *testing.T) {
	blobRoot := t.TempDir()
	content := []byte("env key test content")
	inputPath := writeFile(t, content)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	outputPath := filepath.Join(t.TempDir(), "output")
	keyEnv := "STOWAGE_KEY=" + testMEKHex

	stdout, stderr, code := runEnv(t, []string{keyEnv},
		"store",
		"-root", blobRoot,
		"-input", inputPath,
		"-manifest", manifestPath,
		"-encryption", "aes256gcm",
	)
	if code != 0 {
		t.Fatalf("store failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	stdout, stderr, code = runEnv(t, []string{keyEnv},
		"retrieve",
		"-root", blobRoot,
		"-manifest", manifestPath,
		"-output", outputPath,
	)
	if code != 0 {
		t.Fatalf("retrieve failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile output: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch after env-key round-trip")
	}
}

// TestE2E_EncryptedFilename verifies that:
//   - store writes EncryptedFilename into the manifest
//   - retrieve auto-resolves output path from the decrypted filename
func TestE2E_EncryptedFilename(t *testing.T) {
	blobRoot := t.TempDir()
	content := []byte("filename encryption test content")

	// Create a file with a distinctive name.
	inputDir := t.TempDir()
	inputPath := filepath.Join(inputDir, "secret-document.txt")
	if err := os.WriteFile(inputPath, content, 0o600); err != nil {
		t.Fatalf("WriteFile input: %v", err)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")

	stdout, stderr, code := run(t,
		"store",
		"-root", blobRoot,
		"-input", inputPath,
		"-manifest", manifestPath,
		"-encryption", "aes256gcm",
		"-key", testMEKHex,
		"-compression", "none",
	)
	if code != 0 {
		t.Fatalf("store failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// Verify the manifest does NOT contain the plaintext filename.
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile manifest: %v", err)
	}
	if strings.Contains(string(manifestData), "secret-document.txt") {
		t.Error("manifest contains plaintext filename — should be encrypted")
	}
	if !strings.Contains(string(manifestData), "encrypted_filename") {
		t.Error("manifest missing 'encrypted_filename' field")
	}

	// Retrieve without -output; the binary should write to the original filename.
	outDir := t.TempDir()
	origCmd := exec.Command(binary, "retrieve",
		"-root", blobRoot,
		"-manifest", manifestPath,
		"-key", testMEKHex,
	)
	origCmd.Dir = outDir
	var outBuf, errBuf bytes.Buffer
	origCmd.Stdout = &outBuf
	origCmd.Stderr = &errBuf
	if err := origCmd.Run(); err != nil {
		t.Fatalf("retrieve failed:\nstdout: %s\nstderr: %s", outBuf.String(), errBuf.String())
	}

	stdout = outBuf.String()
	if !strings.Contains(stdout, "secret-document.txt") {
		t.Errorf("retrieve stdout should mention original filename, got: %q", stdout)
	}

	got, err := os.ReadFile(filepath.Join(outDir, "secret-document.txt"))
	if err != nil {
		t.Fatalf("ReadFile restored: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch after encrypted-filename round-trip")
	}
}

// TestE2E_LogLevel_Silent verifies that -log-level silent suppresses slog output.
func TestE2E_LogLevel_Silent(t *testing.T) {
	blobRoot := t.TempDir()
	content := []byte("log level test")
	inputPath := writeFile(t, content)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")

	stdout, stderr, code := run(t,
		"-log-level", "silent",
		"store",
		"-root", blobRoot,
		"-input", inputPath,
		"-manifest", manifestPath,
		"-encryption", "none",
	)
	if code != 0 {
		t.Fatalf("store failed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	// With silent level, stderr should have no slog lines (level=INFO etc.).
	if strings.Contains(stderr, "level=") {
		t.Errorf("expected no slog output with -log-level silent, got stderr: %q", stderr)
	}
}

// TestE2E_DefaultEncryption_None verifies that omitting -encryption defaults to none (no key required).
func TestE2E_DefaultEncryption_None(t *testing.T) {
	blobRoot := t.TempDir()
	content := []byte("no encryption default test")
	inputPath := writeFile(t, content)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")

	// No -encryption flag, no -key flag, no STOWAGE_KEY env — should succeed.
	stdout, stderr, code := run(t,
		"store",
		"-root", blobRoot,
		"-input", inputPath,
		"-manifest", manifestPath,
	)
	if code != 0 {
		t.Fatalf("store without -encryption should default to none and succeed (exit %d):\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile manifest: %v", err)
	}
	if !strings.Contains(string(data), `"none"`) && !strings.Contains(string(data), "none") {
		t.Errorf("manifest encryption should be 'none', got: %s", data)
	}
}
