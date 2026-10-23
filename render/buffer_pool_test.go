package render

import (
	"math"
	"runtime"
	"slices"
	"sync"
	"testing"
)

func TestBufferPoolBoundsRetentionAndRecoversAfterGC(t *testing.T) {
	pool := bufferPool[uint32]{maxCapacity: 64}
	buffer := pool.get()
	buffer.data = make([]uint32, 32)
	pool.put(buffer)
	buffer = pool.get()
	buffer.data = make([]uint32, 65)
	pool.put(buffer)
	buffer = pool.get()
	if len(buffer.data) != 0 || cap(buffer.data) > pool.maxCapacity {
		t.Fatalf("oversized scratch retained: len/cap = %d/%d", len(buffer.data), cap(buffer.data))
	}
	pool.put(buffer)
	runtime.GC()
	runtime.GC()
	buffer = pool.get()
	defer pool.put(buffer)
	var err error
	buffer.data, _, err = packMeshDeformationStatic(buffer.data, MeshDeformationDescriptor{VertexCount: 1})
	if err != nil || !slices.Equal(buffer.data, []uint32{0}) {
		t.Fatalf("packing after GC: %v, %v", buffer.data, err)
	}
}

func TestStaticDeformationScratchOverwritesAndRecovers(t *testing.T) {
	descriptor := MeshDeformationDescriptor{
		VertexCount: 1, TargetCount: 1,
		Skin:         []MeshSkinVertex{{Joints: [4]uint32{1, 3, 5, 7}, Weights: [4]float32{.1, .2, .3, .4}}},
		MorphTargets: []MeshMorphVertex{{Position: [3]float32{1, 2, 3}, Normal: [3]float32{4, 5, 6}, Tangent: [3]float32{7, 8, 9}}},
	}
	want := []uint32{0, 1, 3, 5, 7}
	for _, value := range []float32{.1, .2, .3, .4, 1, 2, 3, 4, 5, 6, 7, 8, 9} {
		want = append(want, math.Float32bits(value))
	}
	buffer := make([]uint32, 128)
	for i := range buffer {
		buffer[i] = math.MaxUint32
	}
	words, layout, err := packMeshDeformationStatic(buffer, descriptor)
	if err != nil || !slices.Equal(words, want) || &words[0] != &buffer[0] || layout.requiredJoints != 8 {
		t.Fatalf("packing reused data: words=%v layout=%+v err=%v", words, layout, err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		words, _, err = packMeshDeformationStatic(words, descriptor)
		if err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("packing warm scratch allocated %g times", allocations)
	}
	for _, failSkin := range []bool{true, false} {
		if failSkin {
			descriptor.Skin[0].Weights[3] = -1
		} else {
			descriptor.MorphTargets[0].Tangent[2] = float32(math.NaN())
		}
		words, _, err = packMeshDeformationStatic(words, descriptor)
		if err == nil || len(words) != 0 || cap(words) != cap(buffer) {
			t.Fatalf("failed packing lost scratch: len/cap=%d/%d err=%v", len(words), cap(words), err)
		}
		descriptor.Skin[0].Weights[3] = .4
		descriptor.MorphTargets[0].Tangent[2] = 9
		words, _, err = packMeshDeformationStatic(words, descriptor)
		if err != nil || !slices.Equal(words, want) {
			t.Fatalf("recovery produced %v: %v", words, err)
		}
	}
	words, layout, err = packMeshDeformationStatic(words, MeshDeformationDescriptor{VertexCount: 1})
	if err != nil || !slices.Equal(words, []uint32{0}) || layout.skinOffset != 0 || layout.morphOffset != 0 || layout.requiredJoints != 0 {
		t.Fatalf("short upload retained previous data: %v, %+v, %v", words, layout, err)
	}
}

func TestStaticDeformationPoolKeepsConcurrentUploadsIndependent(t *testing.T) {
	var workers sync.WaitGroup
	for worker := range 8 {
		workers.Go(func() {
			descriptor := MeshDeformationDescriptor{VertexCount: 32, TargetCount: 1, MorphTargets: make([]MeshMorphVertex, 32)}
			for i := range descriptor.MorphTargets {
				descriptor.MorphTargets[i].Position[0] = float32(worker + 1)
			}
			for range 100 {
				func() {
					scratch := meshDeformationBuffers.get()
					defer meshDeformationBuffers.put(scratch)
					words, _, err := packMeshDeformationStatic(scratch.data, descriptor)
					scratch.data = words
					if err != nil {
						t.Error(err)
						return
					}
					// Other uploads may run before this upload consumes its words.
					runtime.Gosched()
					for i := range descriptor.MorphTargets {
						if words[1+i*9] != math.Float32bits(float32(worker+1)) {
							t.Error("concurrent upload overwrote borrowed scratch")
							return
						}
					}
				}()
			}
		})
	}
	workers.Wait()
}
