package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	blobfs "github.com/gjantsch/stowage/internal/blobstore/fs"
	documentengine "github.com/gjantsch/stowage/internal/documentengine"
	"github.com/gjantsch/stowage/internal/manifest"
	pkgdocumentengine "github.com/gjantsch/stowage/pkg/documentengine"
)

func runRetrieve(args []string) error {
	fs := flag.NewFlagSet("retrieve", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "path to manifest file (required)")
	output := fs.String("output", "", "path to write restored file (required)")
	root := fs.String("root", "", "BlobStore root directory (required)")
	key := fs.String("key", "", "hex-encoded 32-byte MEK (required when manifest uses encryption)")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: stowage retrieve -root <dir> -manifest <file> -output <file> [flags]

Restore a file from BlobStore using its manifest. Verifies the integrity hash
after reassembly and returns an error if the content has been tampered with.

example:
  stowage retrieve \
    -root /data/blobs \
    -manifest photo.json \
    -output photo_restored.jpg \
    -key <64-hex-chars>

flags:`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *manifestPath == "" {
		return fmt.Errorf("retrieve: -manifest is required")
	}
	if *output == "" {
		return fmt.Errorf("retrieve: -output is required")
	}
	if *root == "" {
		return fmt.Errorf("retrieve: -root is required")
	}

	m, err := readManifest(*manifestPath)
	if err != nil {
		return fmt.Errorf("retrieve: read manifest: %w", err)
	}

	var mek []byte
	if m.Encryption != manifest.EncryptionNone {
		if *key == "" {
			return fmt.Errorf("retrieve: -key is required (manifest uses %q encryption)", m.Encryption)
		}
		decoded, err := hex.DecodeString(*key)
		if err != nil {
			return fmt.Errorf("retrieve: -key is not valid hex: %w", err)
		}
		if len(decoded) != 32 {
			return fmt.Errorf("retrieve: -key must decode to exactly 32 bytes, got %d", len(decoded))
		}
		mek = decoded
	}

	// Build engine from manifest fields so algorithms always match what was stored.
	cfg := pkgdocumentengine.Config{
		Compression: m.Compression,
		Encryption:  m.Encryption,
		HashAlgo:    m.HashAlgorithm,
	}
	blobs := blobfs.NewFS(*root)
	engine, err := documentengine.New(cfg, blobs)
	if err != nil {
		return fmt.Errorf("retrieve: init engine: %w", err)
	}

	rc, err := engine.Retrieve(context.Background(), m, mek)
	if err != nil {
		return fmt.Errorf("retrieve: %w", err)
	}

	out, err := os.Create(*output)
	if err != nil {
		rc.Close()
		return fmt.Errorf("retrieve: create output file: %w", err)
	}

	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		rc.Close()
		return fmt.Errorf("retrieve: write output: %w", err)
	}
	if err := out.Close(); err != nil {
		rc.Close()
		return fmt.Errorf("retrieve: close output: %w", err)
	}

	if err := rc.Close(); err != nil {
		if errors.Is(err, pkgdocumentengine.ErrIntegrityMismatch) {
			return fmt.Errorf("retrieve: integrity check failed — file may be corrupted: %w", err)
		}
		return fmt.Errorf("retrieve: close reader: %w", err)
	}

	fmt.Printf("retrieved: %d chunks, integrity ok\n", len(m.Chunks))
	return nil
}

// readManifest reads and deserialises a manifest file.
// Files with a .yaml or .yml extension are parsed as YAML; all others as JSON.
func readManifest(path string) (*manifest.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
		return manifest.FromYAML(data)
	}
	return manifest.FromJSON(data)
}
