package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	blobfs "github.com/gjantsch/stowage/internal/blobstore/fs"
	documentengine "github.com/gjantsch/stowage/internal/documentengine"
	"github.com/gjantsch/stowage/internal/manifest"
	"github.com/gjantsch/stowage/pkg/blobstore"
	pkgdocumentengine "github.com/gjantsch/stowage/pkg/documentengine"
)

func runRetrieve(args []string, cfg cliConfig) error {
	fs := flag.NewFlagSet("retrieve", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "path to manifest file (required)")
	output := fs.String("output", "", "path to write restored file (defaults to original filename when available)")
	root := fs.String("root", "", "BlobStore root directory")
	key := fs.String("key", "", "hex-encoded 32-byte MEK")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: stowage retrieve -manifest <file> [flags]

Restore a file from BlobStore using its manifest. Verifies the integrity hash
after reassembly and returns an error if the content has been tampered with.

example:
  stowage retrieve \
    -manifest photo.json \
    -output photo_restored.jpg \
    -key <64-hex-chars>

flags:`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *root == "" {
		*root = cfg.Root
	}
	if *root == "" {
		return fmt.Errorf("retrieve: -root is required (or set 'root' in ~/.stowage)")
	}
	if *manifestPath == "" {
		return fmt.Errorf("retrieve: -manifest is required")
	}

	m, err := readManifest(*manifestPath)
	if err != nil {
		return fmt.Errorf("retrieve: read manifest: %w", err)
	}

	mek, err := resolveKey(*key, string(m.Encryption))
	if err != nil {
		return fmt.Errorf("retrieve: %w", err)
	}

	// Resolve output path: flag → decrypted filename → error.
	outputPath := *output
	var originalName string
	if mek != nil && len(m.EncryptedFilename) > 0 {
		originalName, err = decryptFilename([]byte(m.EncryptedFilename), mek)
		if err != nil {
			return fmt.Errorf("retrieve: decrypt filename: %w", err)
		}
	}
	if outputPath == "" {
		if originalName != "" {
			outputPath = originalName
		} else {
			return fmt.Errorf("retrieve: -output is required (manifest has no encrypted filename)")
		}
	}

	engineCfg := pkgdocumentengine.Config{
		Compression: m.Compression,
		Encryption:  m.Encryption,
		HashAlgo:    m.HashAlgorithm,
	}
	engine, err := documentengine.New(engineCfg, blobfs.NewFSWithConfig(blobfsConfig(cfg, *root)))
	if err != nil {
		return fmt.Errorf("retrieve: init engine: %w", err)
	}

	rc, err := engine.Retrieve(context.Background(), m, mek)
	if err != nil {
		return fmt.Errorf("retrieve: %w", err)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		rc.Close()
		return fmt.Errorf("retrieve: create output file: %w", err)
	}

	if _, err := io.Copy(outFile, rc); err != nil {
		outFile.Close()
		rc.Close()
		return fmt.Errorf("retrieve: write output: %w", err)
	}
	if err := outFile.Close(); err != nil {
		rc.Close()
		return fmt.Errorf("retrieve: close output: %w", err)
	}
	if err := rc.Close(); err != nil {
		if errors.Is(err, pkgdocumentengine.ErrIntegrityMismatch) {
			return fmt.Errorf("retrieve: integrity check failed — file may be corrupted: %w", err)
		}
		return fmt.Errorf("retrieve: close reader: %w", err)
	}

	if originalName != "" {
		fmt.Printf("retrieved: %d chunks, integrity ok (original: %s)\n", len(m.Chunks), originalName)
	} else {
		fmt.Printf("retrieved: %d chunks, integrity ok\n", len(m.Chunks))
	}
	return nil
}

// readManifest reads and deserialises a manifest file.
// Files with a .yaml or .yml extension are parsed as YAML; all others as JSON.
func readManifest(path string) (*manifest.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch {
	case len(path) > 5 && (path[len(path)-5:] == ".yaml" || path[len(path)-4:] == ".yml"):
		return manifest.FromYAML(data)
	default:
		return manifest.FromJSON(data)
	}
}

// blobfsConfig builds a blobstore.Config from CLI config and resolved root.
func blobfsConfig(cfg cliConfig, root string) blobstore.Config {
	return blobstore.Config{
		RootDir:     root,
		Deduplicate: cfg.Deduplicate,
	}
}
