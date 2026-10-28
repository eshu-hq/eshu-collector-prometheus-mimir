// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"sync"
	"testing"
)

var seedRaceShared int

// Throwaway race seed (close unmerged): unsynchronized counter trips the
// detector under -race and passes without it.
func TestSeedRaceTripsTheDetector(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				seedRaceShared++
			}
		}()
	}
	wg.Wait()
}
