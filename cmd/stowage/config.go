package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type cliConfig struct {
	Root        string `yaml:"root"`
	Compression string `yaml:"compression"`
	Encryption  string `yaml:"encryption"`
	Hash        string `yaml:"hash"`
	ChunkSize   int64  `yaml:"chunk_size"`
	Format      string `yaml:"format"`
	LogLevel    string `yaml:"log_level"`
	Deduplicate bool   `yaml:"deduplicate"`
}

// loadConfig reads $HOME/.stowage. A missing file is silently ignored.
func loadConfig() (cliConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return cliConfig{}, err
	}
	data, err := os.ReadFile(filepath.Join(home, ".stowage"))
	if errors.Is(err, os.ErrNotExist) {
		return cliConfig{}, nil
	}
	if err != nil {
		return cliConfig{}, err
	}
	var cfg cliConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cliConfig{}, err
	}
	cfg.Root = expandHome(cfg.Root, home)
	return cfg, nil
}

func expandHome(path, home string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
