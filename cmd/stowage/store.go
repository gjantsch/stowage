package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	blobfs "github.com/gjantsch/stowage/internal/blobstore/fs"
	documentengine "github.com/gjantsch/stowage/internal/documentengine"
	"github.com/gjantsch/stowage/internal/manifest"
	pkgdocumentengine "github.com/gjantsch/stowage/pkg/documentengine"
)
func runStore(args []string) error {
	fs := flag.NewFlagSet("store", flag.ContinueOnError)
	input := fs.String("input", "", "path to file to store (required)")
	manifestPath := fs.String("manifest", "", "path to write manifest (required)")
	root := fs.String("root", "", "BlobStore root directory (required)")
	compression := fs.String("compression", "zstd", "compression algorithm: none|gzip|zstd|lz4")
	encryption := fs.String("encryption", "none", "encryption algorithm: none|aes256gcm")
	hashAlgo := fs.String("hash", "blake3", "hash algorithm: sha256|blake3")
	chunkSize := fs.Int64("chunk-size", 4*1024*1024, "chunk size in bytes")
	key := fs.String("key", "", "hex-encoded 32-byte MEK (required when encryption != none)")
	format := fs.String("format", "json", "manifest output format: json|yaml")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: stowage store -root <dir> -input <file> -manifest <file> [flags]

Compress, encrypt, and chunk a file into the BlobStore. Writes a manifest that
describes how to restore the file later.

example:
  stowage store \
    -root /data/blobs \
    -input photo.jpg \
    -manifest photo.json \
    -compression zstd \
    -encryption aes256gcm \
    -key <64-hex-chars> \
    -hash blake3

flags:`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *input == "" {
		return fmt.Errorf("store: -input is required")
	}
	if *manifestPath == "" {
		return fmt.Errorf("store: -manifest is required")
	}
	if *root == "" {
		return fmt.Errorf("store: -root is required")
	}

	var mek []byte
	if manifest.EncryptionType(*encryption) != manifest.EncryptionNone {
		if *key == "" {
			return fmt.Errorf("store: -key is required when encryption is %q", *encryption)
		}
		decoded, err := hex.DecodeString(*key)
		if err != nil {
			return fmt.Errorf("store: -key is not valid hex: %w", err)
		}
		if len(decoded) != 32 {
			return fmt.Errorf("store: -key must decode to exactly 32 bytes, got %d", len(decoded))
		}
		mek = decoded
	}

	cfg := pkgdocumentengine.Config{
		Compression: manifest.CompressionType(*compression),
		Encryption:  manifest.EncryptionType(*encryption),
		HashAlgo:    manifest.HashAlgorithm(*hashAlgo),
		ChunkSize:   *chunkSize,
	}

	blobs := blobfs.NewFS(*root)
	engine, err := documentengine.New(cfg, blobs)
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
