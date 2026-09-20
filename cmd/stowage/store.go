package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	blobfs "github.com/gjantsch/stowage/internal/blobstore/fs"
	documentengine "github.com/gjantsch/stowage/internal/documentengine"
	"github.com/gjantsch/stowage/internal/manifest"
	pkgdocumentengine "github.com/gjantsch/stowage/pkg/documentengine"
)

func runStore(args []string, cfg cliConfig) error {
	fs := flag.NewFlagSet("store", flag.ContinueOnError)
	input := fs.String("input", "", "path to file to store (required)")
	manifestPath := fs.String("manifest", "", "path to write manifest (required)")
	root := fs.String("root", "", "BlobStore root directory")
	compression := fs.String("compression", "", "compression algorithm: none|gzip|zstd|lz4")
	encryption := fs.String("encryption", "", "encryption algorithm: none|aes256gcm")
	hashAlgo := fs.String("hash", "", "hash algorithm: sha256|blake3")
	chunkSize := fs.Int64("chunk-size", 0, "chunk size in bytes")
	key := fs.String("key", "", "hex-encoded 32-byte MEK")
	format := fs.String("format", "", "manifest output format: json|yaml")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: stowage store -input <file> -manifest <file> [flags]

Compress, encrypt, and chunk a file into the BlobStore. Writes a manifest that
describes how to restore the file later.

example:
  stowage store \
    -input photo.jpg \
    -manifest photo.json \
    -compression zstd \
    -encryption aes256gcm \
    -key <64-hex-chars>

flags:`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Apply config defaults for unset flags.
	if *root == "" {
		*root = cfg.Root
	}
	if *compression == "" {
		*compression = cfg.Compression
	}
	if *encryption == "" {
		*encryption = cfg.Encryption
	}
	if *hashAlgo == "" {
		*hashAlgo = cfg.Hash
	}
	if *chunkSize == 0 {
		*chunkSize = cfg.ChunkSize
	}
	if *format == "" {
		*format = cfg.Format
	}

	// Hardcoded fallbacks.
	if *root == "" {
		return fmt.Errorf("store: -root is required (or set 'root' in ~/.stowage)")
	}
	if *input == "" {
		return fmt.Errorf("store: -input is required")
	}
	if *manifestPath == "" {
		return fmt.Errorf("store: -manifest is required")
	}
	if *compression == "" {
		*compression = "zstd"
	}
	if *encryption == "" {
		*encryption = "none"
	}
	if *hashAlgo == "" {
		*hashAlgo = "blake3"
	}
	if *chunkSize == 0 {
		*chunkSize = 4 * 1024 * 1024
	}
	if *format == "" {
		*format = "json"
	}

	mek, err := resolveKey(*key, *encryption)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}

	engineCfg := pkgdocumentengine.Config{
		Compression: manifest.CompressionType(*compression),
		Encryption:  manifest.EncryptionType(*encryption),
		HashAlgo:    manifest.HashAlgorithm(*hashAlgo),
		ChunkSize:   *chunkSize,
	}

	blobs := blobfs.NewFSWithConfig(blobfsConfig(cfg, *root))
	engine, err := documentengine.New(engineCfg, blobs)
	if err != nil {
		return fmt.Errorf("store: init engine: %w", err)
	}

	f, err := os.Open(*input)
	if err != nil {
		return fmt.Errorf("store: open input: %w", err)
	}
	defer f.Close()

	m, err := engine.Store(context.Background(), f, mek)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}

	// Encrypt the original filename when encryption is active.
	if mek != nil {
		enc, err := encryptFilename(filepath.Base(*input), mek)
		if err != nil {
			return fmt.Errorf("store: encrypt filename: %w", err)
		}
		m.EncryptedFilename = enc
	}

	var data []byte
	switch strings.ToLower(*format) {
	case "yaml":
		data, err = m.ToYAML()
	default:
		data, err = m.ToJSON()
	}
	if err != nil {
		return fmt.Errorf("store: marshal manifest: %w", err)
	}

	if err := os.WriteFile(*manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("store: write manifest: %w", err)
	}

	fmt.Printf("stored: %d chunks, %s, %s+%s\n", len(m.Chunks), m.HashAlgorithm, m.Compression, m.Encryption)
	fmt.Printf("manifest written to: %s\n", *manifestPath)
	return nil
}
