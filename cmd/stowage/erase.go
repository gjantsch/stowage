package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	blobfs "github.com/gjantsch/stowage/internal/blobstore/fs"
	documentengine "github.com/gjantsch/stowage/internal/documentengine"
	pkgdocumentengine "github.com/gjantsch/stowage/pkg/documentengine"
)

func runErase(args []string, cfg cliConfig) error {
	fs := flag.NewFlagSet("erase", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "path to manifest file (required)")
	root := fs.String("root", "", "BlobStore root directory")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: stowage erase -manifest <file> [flags]

Remove all BlobStore chunks referenced by a manifest. Each chunk is erased
using LIFO slot removal. The manifest file itself is not deleted.

example:
  stowage erase -manifest photo.json

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
		return fmt.Errorf("erase: -root is required (or set 'root' in ~/.stowage)")
	}
	if *manifestPath == "" {
		return fmt.Errorf("erase: -manifest is required")
	}

	m, err := readManifest(*manifestPath)
	if err != nil {
		return fmt.Errorf("erase: read manifest: %w", err)
	}

	engineCfg := pkgdocumentengine.Config{
		Compression: m.Compression,
		Encryption:  m.Encryption,
		HashAlgo:    m.HashAlgorithm,
	}
	engine, err := documentengine.New(engineCfg, blobfs.NewFSWithConfig(blobfsConfig(cfg, *root)))
	if err != nil {
		return fmt.Errorf("erase: init engine: %w", err)
	}

	if err := engine.Erase(context.Background(), m); err != nil {
		return fmt.Errorf("erase: %w", err)
	}

	fmt.Printf("erased: %d chunks\n", len(m.Chunks))
	return nil
}
