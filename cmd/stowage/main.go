package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not load ~/.stowage:", err)
	}

	// Global -log-level flag must be parsed before the subcommand.
	logLevel := flag.String("log-level", "", "log verbosity: debug|info|warn|error|silent")
	flag.Parse()

	setupLogging(resolveLogLevel(*logLevel, cfg.LogLevel))

	if flag.NArg() < 1 {
		usage()
		os.Exit(1)
	}

	subArgs := flag.Args()[1:]
	switch flag.Arg(0) {
	case "store":
		err = runStore(subArgs, cfg)
	case "retrieve":
		err = runRetrieve(subArgs, cfg)
	case "erase":
		err = runErase(subArgs, cfg)
	case "gc":
		err = runGC(subArgs, cfg)
	case "init":
		err = runInit(subArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", flag.Arg(0))
		usage()
		os.Exit(1)
	}

	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: stowage [-log-level <level>] <command> [flags]

commands:
  init      create a default ~/.stowage config file
  store     compress, encrypt, chunk and store a file
  retrieve  restore a file from its manifest
  erase     remove all chunks referenced by a manifest
  gc        remove orphaned temp and lock files from the store

global flags:`)
	flag.PrintDefaults()
}

func resolveLogLevel(flagVal, cfgVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if cfgVal != "" {
		return cfgVal
	}
	return "info"
}

func setupLogging(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	case "silent":
		l = slog.Level(100)
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})))
}
