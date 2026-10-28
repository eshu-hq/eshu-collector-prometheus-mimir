// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

// FloorProbeChain is a throwaway coverage-floor probe (close unmerged). Its
// statements are intentionally uncovered so the race lane's floor check
// fails while every other lane stays green.
func FloorProbeChain(a, b int) int {
	x := a + b
	if x < 0 {
		x = -x
	}
	y := x * 2
	y += a
	y -= b
	if y == 0 {
		return -1
	}
	return x + y
}

var _ = FloorProbeChain
