package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "store":
		err = runStore(os.Args[2:])
	case "retrieve":
		err = runRetrieve(os.Args[2:])
	case "erase":
		err = runErase(os.Args[2:])
	case "gc":
		err = runGC(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", os.Args[1])
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
	fmt.Fprintln(os.Stderr, `usage: stowage <command> [flags]

commands:
  store     compress, encrypt, chunk and store a file
  retrieve  restore a file from its manifest
  erase     remove all chunks referenced by a manifest
  gc        remove orphaned temp and lock files from the store`)
}
