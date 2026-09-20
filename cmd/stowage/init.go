package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const configTemplate = `# stowage configuration
# All fields are optional. Missing fields fall back to built-in defaults.

# BlobStore root directory (required for most commands)
root: ~/stowage-data

# Compression algorithm: none | gzip | zstd | lz4
compression: zstd

# Encryption algorithm: none | aes256gcm
# The encryption key is never stored here. Provide it via:
#   STOWAGE_KEY environment variable, or the -key flag, or an interactive prompt.
encryption: none

# Hash algorithm for chunk identity and integrity verification: sha256 | blake3
hash: blake3

# Chunk size in bytes (default 4 MiB)
chunk_size: 4194304

# Manifest output format: json | yaml
format: json

# Log verbosity: debug | info | warn | error | silent
log_level: info

# Enable BlobStore deduplication (skip writing if slot 0 already exists for a hash)
deduplicate: false
`

func runInit(_ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("init: get home dir: %w", err)
	}
	path := filepath.Join(home, ".stowage")

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("init: %s already exists; remove it first if you want to regenerate", path)
	}

	if err := os.WriteFile(path, []byte(configTemplate), 0o600); err != nil {
		return fmt.Errorf("init: write config: %w", err)
	}

	fmt.Printf("config written to: %s\n", path)
	return nil
}
