// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

// Throwaway vet seed (close unmerged): unreachable code fails go vet while
// go test's built-in vet subset stays green.
func seedRedUnreachable() int {
	return 1
	x := 2
	_ = x
	return x
}

var _ = seedRedUnreachable
