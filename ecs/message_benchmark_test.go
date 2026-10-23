package ecs

import (
	"fmt"
	"testing"
)

func BenchmarkMessageIteration(b *testing.B) {
	for _, count := range []int{1, 100, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := NewWorld()
			for value := range count / 2 {
				WriteMessage(world, value)
			}
			AdvanceMessages(world)
			for value := count / 2; value < count; value++ {
				WriteMessage(world, value)
			}
			var reader MessageReader[int]
			b.ReportAllocs()
			for b.Loop() {
				reader.cursor = 0
				for value := range reader.All(world) {
					benchmarkTotal += float32(*value)
				}
			}
		})
	}
}

func BenchmarkMessageDiscard(b *testing.B) {
	for _, count := range []int{1, 100, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := NewWorld()
			for value := range count {
				WriteMessage(world, value)
			}
			var reader MessageReader[int]
			b.ReportAllocs()
			for b.Loop() {
				reader.cursor = 0
				reader.Discard(world)
				benchmarkTotal += float32(reader.cursor)
			}
		})
	}
}

// The consumption helper models a system body outside testing.B.Loop.
func BenchmarkMessageIterationSystem(b *testing.B) {
	for _, count := range []int{1, 100, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := NewWorld()
			for value := range count / 2 {
				WriteMessage(world, value)
			}
			AdvanceMessages(world)
			for value := count / 2; value < count; value++ {
				WriteMessage(world, value)
			}
			var reader MessageReader[int]
			b.ReportAllocs()
			for b.Loop() {
				reader.cursor = 0
				benchmarkTotal += consumeBenchmarkMessages(world, &reader)
			}
		})
	}
}

func consumeBenchmarkMessages(world *World, reader *MessageReader[int]) float32 {
	var total float32
	for value := range reader.All(world) {
		total += float32(*value)
	}
	return total
}
