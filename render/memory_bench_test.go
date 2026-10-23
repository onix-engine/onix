package render

import (
	"fmt"
	"testing"
)

var deformationBenchmarkWord uint32

func BenchmarkMeshDeformationStatic(b *testing.B) {
	for _, vertices := range []int{64, 1024, 10000} {
		b.Run(fmt.Sprintf("vertices=%d", vertices), func(b *testing.B) {
			descriptor := MeshDeformationDescriptor{
				VertexCount: uint32(vertices), TargetCount: 4,
				Skin: make([]MeshSkinVertex, vertices), MorphTargets: make([]MeshMorphVertex, vertices*4),
			}
			for i := range descriptor.Skin {
				descriptor.Skin[i] = MeshSkinVertex{Joints: [4]uint32{3, 7, 11, 15}, Weights: [4]float32{.25, .25, .25, .25}}
			}
			for i := range descriptor.MorphTargets {
				descriptor.MorphTargets[i] = MeshMorphVertex{Position: [3]float32{.1, -.2, .3}, Normal: [3]float32{.01, .02, .03}, Tangent: [3]float32{-.01, -.02, -.03}}
			}
			b.SetBytes(int64((1 + vertices*44) * 4))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				benchmarkStaticDeformationUpload(b, descriptor)
			}
		})
	}
}

// Consume the packed data synchronously, as the native upload entry points do.
// GPU resource creation and the GPU-owned copy are outside this CPU benchmark.
func benchmarkStaticDeformationUpload(b *testing.B, descriptor MeshDeformationDescriptor) {
	scratch := meshDeformationBuffers.get()
	defer meshDeformationBuffers.put(scratch)
	words, layout, err := packMeshDeformationStatic(scratch.data, descriptor)
	scratch.data = words
	if err != nil {
		b.Fatal(err)
	}
	deformationBenchmarkWord = words[len(words)-1] ^ layout.requiredJoints
}

func BenchmarkMeshDeformationAnimated(b *testing.B) {
	for _, deformations := range []int{1, 64} {
		for _, warm := range []bool{false, true} {
			b.Run(fmt.Sprintf("deformations=%d/warm=%t", deformations, warm), func(b *testing.B) {
				var source deformationTestSource
				handle := source.layouts.Insert(meshDeformationLayout{skinOffset: 1, morphOffset: 9, vertexCount: 1024, requiredJoints: 64, targetCount: 4})
				joints := make([][16]float32, 64)
				for i := range joints {
					joints[i] = [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, float32(i), 0, 0, 1}
				}
				batch := MeshBatch3D{Deformations: make([]MeshDeformationData, deformations), Draws: make([]MeshDraw3D, deformations)}
				for i := range batch.Deformations {
					batch.Deformations[i] = MeshDeformationData{Mesh: handle, JointMatrices: joints, MorphWeights: []float32{.1, .2, .3, .4}}
					batch.Draws[i] = MeshDraw3D{Mesh: handle, DeformationIndex: uint32(i + 1)}
				}
				words, err := packMeshDeformations(nil, batch, &source)
				if err != nil {
					b.Fatal(err)
				}
				b.SetBytes(int64(len(words) * 4))
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if !warm {
						words = nil
					}
					words, err = packMeshDeformations(words, batch, &source)
					if err != nil {
						b.Fatal(err)
					}
				}
				deformationBenchmarkWord = words[len(words)-1]
			})
		}
	}
}
