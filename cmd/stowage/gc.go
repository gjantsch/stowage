package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	blobfs "github.com/gjantsch/stowage/internal/blobstore/fs"
)

func runGC(args []string) error {
	fs := flag.NewFlagSet("gc", flag.ContinueOnError)
	root := fs.String("root", "", "BlobStore root directory (required)")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: stowage gc -root <dir>

Scan the BlobStore root directory and remove any orphaned temporary files
(_tmp_*) and stale lock files (_lock_*) left by interrupted operations.

example:
  stowage gc -root /data/blobs

flags:`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *root == "" {
		return fmt.Errorf("gc: -root is required")
	}

	deleted, err := blobfs.NewFS(*root).GC(context.Background())
	if err != nil {
		return fmt.Errorf("gc: %w", err)
	}

	fmt.Printf("gc: removed %d orphan files\n", deleted)
	return nil
}
