package ecs

import (
	"fmt"
	"slices"
	"testing"
)

// Compare a compact ordered index with the maps used by prepared plans before
// choosing a replacement. Values stay separately owned, as real transitions do.
type transitionBenchmarkIndex[T any] []transitionBenchmarkEntry[T]

type transitionBenchmarkEntry[T any] struct {
	value *T
	id    uint32
}

func (index transitionBenchmarkIndex[T]) get(id uint32) *T {
	if len(index) == 0 {
		return nil
	}
	if offset := uint64(id) - uint64(index[0].id); offset < uint64(len(index)) && index[offset].id == id {
		return index[offset].value
	}
	if len(index) <= 8 {
		for _, entry := range index {
			if entry.id == id {
				return entry.value
			}
			if entry.id > id {
				break
			}
		}
		return nil
	}
	lo, hi := 0, len(index)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if index[mid].id < id {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(index) && index[lo].id == id {
		return index[lo].value
	}
	return nil
}

func (index *transitionBenchmarkIndex[T]) set(id uint32, value *T) {
	entries := *index
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if entries[mid].id < id {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(entries) && entries[lo].id == id {
		entries[lo].value = value
		return
	}
	if entries == nil {
		entries = make(transitionBenchmarkIndex[T], 0, 4)
	}
	*index = slices.Insert(entries, lo, transitionBenchmarkEntry[T]{value: value, id: id})
}

var transitionBenchmarkMapSink map[uint32]*patchTransition
var transitionBenchmarkIndexSink transitionBenchmarkIndex[patchTransition]
var transitionBenchmarkValueSink *patchTransition

func BenchmarkPreparedTransitionIndex(b *testing.B) {
	for _, count := range []int{1, 4, 16, 64} {
		for _, stride := range []uint32{1, 4093} {
			distribution := "Dense"
			if stride != 1 {
				distribution = "Sparse"
			}
			keys := make([]uint32, count)
			values := make([]patchTransition, count)
			for i := range keys {
				keys[i] = uint32(i)*stride + 1
			}
			b.Run(fmt.Sprintf("%d/%s", count, distribution), func(b *testing.B) {
				for _, cold := range []bool{false, true} {
					operation := "Hit"
					if cold {
						operation = "Populate"
					}
					b.Run(operation+"/Map", func(b *testing.B) {
						entries := make(map[uint32]*patchTransition)
						for i, key := range keys {
							entries[key] = &values[i]
						}
						b.ReportAllocs()
						var iteration int
						for b.Loop() {
							if cold {
								entries = make(map[uint32]*patchTransition)
								for i := range keys {
									position := (i * 17) & (count - 1)
									entries[keys[position]] = &values[position]
								}
								transitionBenchmarkMapSink = entries
							} else {
								transitionBenchmarkValueSink = entries[keys[(iteration*17)&(count-1)]]
								iteration++
							}
						}
					})
					b.Run(operation+"/Compact", func(b *testing.B) {
						var entries transitionBenchmarkIndex[patchTransition]
						for i, key := range keys {
							entries.set(key, &values[i])
						}
						b.ReportAllocs()
						var iteration int
						for b.Loop() {
							if cold {
								entries = nil
								for i := range keys {
									position := (i * 17) & (count - 1)
									entries.set(keys[position], &values[position])
								}
								transitionBenchmarkIndexSink = entries
							} else {
								transitionBenchmarkValueSink = entries.get(keys[(iteration*17)&(count-1)])
								iteration++
							}
						}
					})
				}
			})
		}
	}
}
