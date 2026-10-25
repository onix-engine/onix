package render

import (
	"fmt"
	"testing"
)

var generationPoolBenchmarkLength int

func BenchmarkGenerationPoolColdLifecycle(b *testing.B) {
	for _, count := range []int{1024, 65536} {
		b.Run(fmt.Sprintf("resources=%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				var pool GenerationPool[poolTestResource, uint32]
				for i := range count {
					pool.Insert(uint32(i))
				}
				for i := range count {
					pool.Remove(NewHandle[poolTestResource](uint32(i)+1, 1))
				}
				generationPoolBenchmarkLength = pool.Len()
			}
		})
	}
}
