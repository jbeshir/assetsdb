// Command assetsdb builds and manages a local, searchable database of CC0 game-art assets.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jbeshir/assetsdb"
	"github.com/jbeshir/assetsdb/internal/build"
	"github.com/jbeshir/assetsdb/internal/ingest/kaykit"
	"github.com/jbeshir/assetsdb/internal/ingest/kenney"
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stdout)
		return
	}

	switch os.Args[1] {
	case "build":
		if err := runBuild(os.Args[2:]); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "assetsdb build:", err)
			os.Exit(1)
		}
	case "add":
		runAdd(os.Args[2:])
		os.Exit(2)
	case "-h", "--help", "help":
		printUsage(os.Stdout)
	default:
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

// printUsage writes the top-level assetsdb command summary to w.
func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, `assetsdb builds and manages a local database of CC0 game-art assets.

Usage:
  assetsdb build --manifest <path> --out <dir>   fetch and catalog packs from a manifest
  assetsdb add ...                               phase-2 (not yet implemented): ingest a
                                                  locally-generated asset`)
}

// runBuild implements the "build" subcommand: it loads the manifest at --manifest, fetches every
// pack through the kenney and kaykit ingesters, and writes the resulting database to --out.
func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "path to the packs manifest YAML file")
	outDir := fs.String("out", "", "output directory for the built database")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	if *manifestPath == "" || *outDir == "" {
		return errors.New("both --manifest and --out are required")
	}

	specs, err := build.LoadManifest(*manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	ingesters := make(map[string]assetsdb.Ingester)
	for _, ing := range []assetsdb.Ingester{kenney.New(), &kaykit.Ingester{}} {
		ingesters[ing.Name()] = ing
	}

	dp, err := build.Build(context.Background(), specs, ingesters, *outDir, time.Now())
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	fmt.Printf("assetsdb: built %d pack(s), %d item(s) -> %s\n", len(dp.Sources), len(dp.Resources), *outDir)

	return nil
}

// runAdd handles the reserved phase-2 "add" subcommand for ingesting a locally-generated asset
// into generated/. No write path is implemented; it only reports that the subcommand is
// unavailable yet and documents its intended future signature:
// assetsdb add --collection <name> --file <path> --generator <id> --prompt <text> --license <spdx>.
func runAdd(_ []string) {
	_, _ = fmt.Fprintln(os.Stderr, "assetsdb add: not yet implemented (phase 2: generated-asset ingest).")
	_, _ = fmt.Fprintln(os.Stderr, "intended usage: assetsdb add --collection <name> --file <path>"+
		" --generator <id> --prompt <text> --license <spdx>")
}
