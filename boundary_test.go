// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestImportBoundaryRejectsCoreAndCrossCollectorImports is the CI gate that
// keeps a standalone collector repository standalone: no Eshu internal
// implementation imports and no cross-collector implementation imports. Only
// the published eshu-sdk modules plus the standard library and yaml are
// allowed. The self-import prefix is derived from go.mod so renaming the
// module can never desync this allowlist.
func TestImportBoundaryRejectsCoreAndCrossCollectorImports(t *testing.T) {
	t.Parallel()

	violations, err := importViolations(".", allowedImportPrefixes(modulePath(t)))
	if err != nil {
		t.Fatalf("importViolations() error = %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("forbidden imports:\n%s", strings.Join(violations, "\n"))
	}
}

func allowedImportPrefixes(module string) []string {
	return []string{
		module,
		"github.com/eshu-hq/eshu-sdk/collector",
		"github.com/eshu-hq/eshu-sdk/factschema",
		"gopkg.in/yaml.v3",
	}
}

// importViolations parses every .go file under root and reports each import
// outside allowedPrefixes. Every directory is scanned, scripts/ and nested
// testdata/ included, so a forbidden import cannot hide in a helper tree.
// Only .git and the top-level testdata/ directory (fixture data, whose .go.txt
// files are not compiled) are skipped.
func importViolations(root string, allowedPrefixes []string) ([]string, error) {
	var violations []string
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			if rel == ".git" || rel == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			allowed := false
			for _, prefix := range allowedPrefixes {
				if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
					allowed = true
					break
				}
			}
			if !allowed && !isStdlib(importPath) {
				violations = append(violations, path+": "+importPath)
			}
		}
		return nil
	}
	if err := filepath.WalkDir(root, walk); err != nil {
		return nil, err
	}
	return violations, nil
}

func isStdlib(importPath string) bool {
	return !strings.Contains(importPath, ".")
}

func modulePath(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "module ") {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "module "))
		}
	}
	t.Fatal("go.mod has no module line")
	return ""
}
