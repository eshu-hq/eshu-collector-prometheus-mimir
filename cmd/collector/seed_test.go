// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	collector "github.com/eshu-hq/eshu-collector-prometheus-mimir"
)

func TestRunSeedWritesCollectorAndManifest(t *testing.T) {
	dest := t.TempDir()
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(),
		[]string{"seed", "--dest", dest, "--image-ref", "ghcr.io/example/coll@sha256:0"},
		nil, &stdout, &stderr, os.Getenv); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	bin := filepath.Join(dest, "collector")
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("seed did not write collector: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("seeded collector is not executable: %v", info.Mode())
	}
	got, err := os.ReadFile(filepath.Join(dest, "manifest.yaml"))
	if err != nil {
		t.Fatalf("seed did not write manifest.yaml: %v", err)
	}
	if string(got) != string(collector.ManifestYAML) {
		t.Fatal("seeded manifest.yaml differs from the embedded manifest")
	}
}

func TestRunSeedRequiresDest(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"seed"},
		nil, &stdout, &stderr, os.Getenv); err == nil {
		t.Fatal("seed without --dest must fail")
	}
}

func TestRunSeedRejectsExtraArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"seed", "--dest", t.TempDir(), "stowaway"},
		nil, &stdout, &stderr, os.Getenv); err == nil {
		t.Fatal("seed with an extra positional argument must fail")
	}
}

func TestRunSeedReseedEnforcesModes(t *testing.T) {
	dest := t.TempDir()
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(),
		[]string{"seed", "--dest", dest, "--image-ref", "ghcr.io/example/coll@sha256:0"},
		nil, &stdout, &stderr, os.Getenv); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	// Corrupt the modes, then re-seed: the enforced 0755/0644 must come back.
	bin := filepath.Join(dest, "collector")
	man := filepath.Join(dest, "manifest.yaml")
	if err := os.Chmod(bin, 0o600); err != nil {
		t.Fatalf("chmod fixture: %v", err)
	}
	if err := os.Chmod(man, 0o600); err != nil {
		t.Fatalf("chmod fixture: %v", err)
	}
	stdout.Reset()
	if err := run(context.Background(),
		[]string{"seed", "--dest", dest},
		nil, &stdout, &stderr, os.Getenv); err != nil {
		t.Fatalf("re-seed failed: %v", err)
	}
	if info, err := os.Stat(bin); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("re-seed left collector mode %v, want 0755", info.Mode())
	}
	if info, err := os.Stat(man); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("re-seed left manifest mode %v, want 0644", info.Mode())
	}
	if got := stdout.String(); got != "seeded "+dest+"\n" {
		t.Fatalf("re-seed summary without --image-ref = %q, want no 'from' suffix", got)
	}
}

func TestRunSeedRejectsBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"seed", "--bogus"},
		nil, &stdout, &stderr, os.Getenv); err == nil {
		t.Fatal("seed with an unknown flag must fail")
	}
}

func TestRunSeedFailsWhenDestIsReadOnly(t *testing.T) {
	dest := t.TempDir()
	if err := os.Chmod(dest, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dest, 0o755) })
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(),
		[]string{"seed", "--dest", dest},
		nil, &stdout, &stderr, os.Getenv); err == nil {
		t.Fatal("seed into a read-only dest must fail")
	}
}

func TestRunSeedFailsWhenManifestPathIsADirectory(t *testing.T) {
	dest := t.TempDir()
	// The collector binary writes first; a directory at the manifest path
	// fails only the manifest write.
	if err := os.Mkdir(filepath.Join(dest, "manifest.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(),
		[]string{"seed", "--dest", dest},
		nil, &stdout, &stderr, os.Getenv); err == nil {
		t.Fatal("seed with a directory at the manifest path must fail")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errTestWrite }

var errTestWrite = errors.New("test writer fails")

func TestStageFromUnreadableSource(t *testing.T) {
	if err := stageFrom(t.TempDir(), t.TempDir(), "x", &bytes.Buffer{}); err == nil {
		t.Fatal("staging from a directory must fail the binary read")
	}
}

func TestStageFromFailingWriter(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip("needs own binary path")
	}
	if err := stageFrom(self, t.TempDir(), "x", failingWriter{}); err == nil {
		t.Fatal("staging with a failing report writer must fail")
	}
}

func TestRunSeedFailsWhenDestIsAFile(t *testing.T) {
	f, err := os.CreateTemp("", "seed-dest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(),
		[]string{"seed", "--dest", f.Name()},
		nil, &stdout, &stderr, os.Getenv); err == nil {
		t.Fatal("seed into a file path must fail")
	}
}
