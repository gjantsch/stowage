package fs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/gjantsch/stowage/internal/blobstore/gc"
	"github.com/gjantsch/stowage/internal/blobstore/shard"
	"github.com/gjantsch/stowage/internal/blobstore/staging"
	"github.com/gjantsch/stowage/pkg/blobstore"
)

type FS struct {
	RootDir     string
	Deduplicate bool
}

func NewFS(rootDir string) *FS {
	return &FS{RootDir: rootDir}
}

func NewFSWithConfig(cfg blobstore.Config) *FS {
	return &FS{RootDir: cfg.RootDir, Deduplicate: cfg.Deduplicate}
}

func (f *FS) Store(ctx context.Context, r io.Reader) (blobstore.Hash32, error) {
	stager := staging.NewStager(f.RootDir, f.RootDir, f.Deduplicate)
	hash, err := stager.Stage(r, sha256.New())
	if err != nil {
		slog.ErrorContext(ctx, "store: failed",
			"root", f.RootDir,
			"error", err,
		)
		return blobstore.Hash32{}, err
	}
	sh := shard.NewShard(hash, f.RootDir)
	slot, _ := sh.HighestSlot()
	path := sh.FilePathAt(slot)
	info, statErr := os.Stat(path)
	size := int64(-1)
	if statErr == nil {
		size = info.Size()
	}
	slog.InfoContext(ctx, "store: ok",
		"hash", hex.EncodeToString(hash[:]),
		"slot", slot,
		"path", path,
		"size", size,
	)
	return hash, nil
}

func (f *FS) Retrieve(ctx context.Context, hash blobstore.Hash32) (io.ReadCloser, error) {
	// Always open slot 0 — all slots hold identical content.
	path := shard.NewShard(hash, f.RootDir).FilePathAt(0)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.InfoContext(ctx, "retrieve: not found",
				"hash", hex.EncodeToString(hash[:]),
				"path", path,
			)
			return nil, fmt.Errorf("%w", blobstore.ErrNotFound)
		}
		slog.ErrorContext(ctx, "retrieve: failed",
			"hash", hex.EncodeToString(hash[:]),
			"path", path,
			"error", err,
		)
		return nil, err
	}
	info, statErr := file.Stat()
	size := int64(-1)
	if statErr == nil {
		size = info.Size()
	}
	slog.InfoContext(ctx, "retrieve: ok",
		"hash", hex.EncodeToString(hash[:]),
		"path", path,
		"size", size,
	)
	return file, nil
}

func (f *FS) Erase(ctx context.Context, hash blobstore.Hash32) error {
	sh := shard.NewShard(hash, f.RootDir)
	slot, ok := sh.HighestSlot()
	if !ok {
		slog.InfoContext(ctx, "erase: not found",
			"hash", hex.EncodeToString(hash[:]),
		)
		return fmt.Errorf("%w", blobstore.ErrNotFound)
	}
	path := sh.FilePathAt(slot)
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w", blobstore.ErrNotFound)
		}
		slog.ErrorContext(ctx, "erase: failed",
			"hash", hex.EncodeToString(hash[:]),
			"slot", slot,
			"path", path,
			"error", err,
		)
		return err
	}
	slog.InfoContext(ctx, "erase: ok",
		"hash", hex.EncodeToString(hash[:]),
		"slot", slot,
		"path", path,
	)
	return nil
}

func (f *FS) GC(ctx context.Context) (int, error) {
	deleted, err := gc.Sweep(ctx, f.RootDir)
	if err != nil {
		slog.ErrorContext(ctx, "gc: sweep failed",
			"root", f.RootDir,
			"deleted", deleted,
			"error", err,
		)
		return deleted, err
	}
	slog.InfoContext(ctx, "gc: sweep ok",
		"root", f.RootDir,
		"deleted", deleted,
	)
	return deleted, nil
}
