package brotli

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Functions for clustering similar histograms together. */

type histogramPair struct {
	idx1       uint32
	idx2       uint32
	cost_combo float64
	cost_diff  float64
}

func histogramPairIsLess(p1 *histogramPair, p2 *histogramPair) bool {
	if p1.cost_diff != p2.cost_diff {
		return p1.cost_diff > p2.cost_diff
	}

	return (p1.idx2 - p1.idx1) > (p2.idx2 - p2.idx1)
}

/* Returns entropy reduction of the context map when we combine two clusters. */
func clusterCostDiff(size_a uint, size_b uint) float64 {
	var size_c uint = size_a + size_b
	return float64(size_a)*fastLog2(size_a) + float64(size_b)*fastLog2(size_b) - float64(size_c)*fastLog2(size_c)
}

func histogramReindexInPlace[T any](out []T, symbols []uint32, length uint) uint {
	const invalidIndex = ^uint32(0)
	newIndex := make([]uint32, length)
	for i := range newIndex {
		newIndex[i] = invalidIndex
	}

	var nextIndex uint32
	for i := uint(0); i < length; i++ {
		symbol := symbols[i]
		if newIndex[symbol] == invalidIndex {
			newIndex[symbol] = nextIndex
			nextIndex++
		}
	}

	for i := uint(0); i < length; i++ {
		symbols[i] = newIndex[symbols[i]]
	}

	// Move each selected histogram into its compact target slot without a second buffer.
	for source := uint32(0); source < uint32(length); source++ {
		if newIndex[source] == invalidIndex {
			continue
		}
		current := source
		carry := out[current]
		for {
			target := newIndex[current]
			newIndex[current] = invalidIndex
			if target == invalidIndex {
				break
			}
			if target == source {
				out[target] = carry
				break
			}
			carry, out[target] = out[target], carry
			if newIndex[target] == invalidIndex {
				break
			}
			current = target
		}
	}
	return uint(nextIndex)
}
