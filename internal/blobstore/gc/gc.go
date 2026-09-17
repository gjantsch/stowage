package gc

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Sweep walks rootDir and removes every file whose name begins with "_tmp_".
// It returns the number of files deleted and the first removal error encountered,
// continuing to delete remaining candidates even when one removal fails.
func Sweep(ctx context.Context, rootDir string) (int, error) {
	var deleted int
	var firstErr error

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "_tmp_") && !strings.HasPrefix(name, "_lock_") {
			return nil
		}
		if rmErr := os.Remove(path); rmErr != nil {
			slog.ErrorContext(ctx, "gc: failed to remove orphan file",
				"path", path,
				"error", rmErr,
			)
			if firstErr == nil {
				firstErr = rmErr
			}
			return nil
		}
		slog.InfoContext(ctx, "gc: removed orphan file",
			"path", path,
		)
		deleted++
		return nil
	})
	if err != nil {
		return deleted, err
	}
	return deleted, firstErr
}
