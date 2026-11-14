// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	collector "github.com/eshu-hq/eshu-collector-prometheus-mimir"
)

// runSeed implements the `seed` subcommand used by the component-extension
// host init container: it stages this binary and the embedded component
// manifest into dest so the host can install and enable the component.
// --image-ref is accepted for provenance logging and otherwise unused.
func runSeed(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("seed", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dest := flags.String("dest", "", "runtime directory to seed (required)")
	imageRef := flags.String("image-ref", "", "seed image reference (provenance only)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("seed: unexpected argument %q", flags.Arg(0))
	}
	if *dest == "" {
		return fmt.Errorf("seed: --dest is required")
	}
	if err := os.MkdirAll(*dest, 0o755); err != nil {
		return fmt.Errorf("seed: create dest: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("seed: locate binary: %w", err)
	}
	return stageFrom(self, *dest, *imageRef, stdout)
}

// osChmod is os.Chmod behind a package seam so tests can inject a chmod
// failure that file ownership rules make unproducible otherwise.
var osChmod = os.Chmod

// stageFrom stages the collector at selfPath plus the embedded manifest
// into dest. It exists so tests can drive the copy path with any source,
// including one that fails to read.
func stageFrom(selfPath, dest, imageRef string, stdout io.Writer) error {
	content, err := os.ReadFile(selfPath)
	if err != nil {
		return fmt.Errorf("seed: read binary: %w", err)
	}
	binPath := filepath.Join(dest, "collector")
	if err := os.WriteFile(binPath, content, 0o755); err != nil {
		return fmt.Errorf("seed: write collector: %w", err)
	}
	// os.WriteFile applies the mode only when it creates the file, so a
	// re-seed over files with stale modes must enforce them explicitly.
	if err := osChmod(binPath, 0o755); err != nil {
		return fmt.Errorf("seed: chmod collector: %w", err)
	}
	manPath := filepath.Join(dest, "manifest.yaml")
	if err := os.WriteFile(manPath, collector.ManifestYAML, 0o644); err != nil {
		return fmt.Errorf("seed: write manifest.yaml: %w", err)
	}
	if err := osChmod(manPath, 0o644); err != nil {
		return fmt.Errorf("seed: chmod manifest.yaml: %w", err)
	}
	if imageRef == "" {
		if _, err := fmt.Fprintf(stdout, "seeded %s\n", dest); err != nil {
			return fmt.Errorf("seed: report: %w", err)
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "seeded %s from %s\n", dest, imageRef); err != nil {
		return fmt.Errorf("seed: report: %w", err)
	}
	return nil
}
