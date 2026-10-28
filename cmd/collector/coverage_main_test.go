// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

// TestRunClaimRejectsUnexpectedArguments pins the CLI surface: the command
// serves one claim on stdio and takes no positional arguments.
func TestRunClaimRejectsUnexpectedArguments(t *testing.T) {
	t.Parallel()

	err := runClaim(context.Background(), []string{"extra"},
		strings.NewReader("{}"), io.Discard, io.Discard, noEnv, fixedNow)
	if err == nil || !strings.Contains(err.Error(), "unexpected argument") {
		t.Errorf("runClaim(extra arg) error = %v, want the argument rejection", err)
	}
}
