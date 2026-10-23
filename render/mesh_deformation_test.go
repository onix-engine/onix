package render

import (
	"math"
	"testing"
)

type deformationTestSource struct {
	layouts GenerationPool[MeshResource, meshDeformationLayout]
}

func (s *deformationTestSource) deformationLayout(handle MeshHandle) (meshDeformationLayout, bool) {
	return s.layouts.Get(handle)
}
func (s *deformationTestSource) upload(t testing.TB, descriptor MeshDeformationDescriptor) (MeshHandle, []uint32) {
	t.Helper()
	words, layout, err := packMeshDeformationStatic(nil, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	return s.layouts.Insert(layout), words
}

func TestMeshDeformationSeparatesStaticAndAnimatedStreams(t *testing.T) {
	source := &deformationTestSource{}
	handle, static := source.upload(t, MeshDeformationDescriptor{VertexCount: 1, TargetCount: 1, Skin: []MeshSkinVertex{{Joints: [4]uint32{2}, Weights: [4]float32{1}}}, MorphTargets: []MeshMorphVertex{{Position: [3]float32{1}, Tangent: [3]float32{2}}}})
	identity := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	value := MeshDeformationData{Mesh: handle, JointMatrices: [][16]float32{identity, identity, identity}, MorphWeights: []float32{.5}}
	batch := MeshBatch3D{Draws: []MeshDraw3D{{Mesh: handle, DeformationIndex: 1}}, Deformations: []MeshDeformationData{value}}
	words, err := packMeshDeformations(nil, batch, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(static) != 1+8+9 || len(words) != 8+3*16+1 || words[0] != 1 || words[1] != 9 || words[2] != 8 || words[3] != 56 || words[4] != 1 || words[5] != 1 || words[6] != 3 {
		t.Fatalf("static=%v animated=%v", static, words)
	}
	if math.Float32frombits(static[9]) != 1 || math.Float32frombits(static[15]) != 2 || math.Float32frombits(words[56]) != .5 {
		t.Fatal("deformation streams are invalid")
	}
	value.MorphWeights[0] = .75
	value.JointMatrices[0][12] = 4
	words, err = packMeshDeformations(words, batch, source)
	if err != nil || math.Float32frombits(words[56]) != .75 || math.Float32frombits(words[20]) != 4 {
		t.Fatalf("animated update: %v", err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		words, err = packMeshDeformations(words, batch, source)
		if err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("packing allocated %v times", allocations)
	}
}

func TestMeshDeformationRejectsInvalidStaticStreams(t *testing.T) {
	for _, descriptor := range []MeshDeformationDescriptor{
		{}, {VertexCount: 2, Skin: []MeshSkinVertex{{}}}, {VertexCount: 1, TargetCount: 1},
		{VertexCount: 1, Skin: []MeshSkinVertex{{Weights: [4]float32{-1}}}},
		{VertexCount: 1, Skin: []MeshSkinVertex{{Weights: [4]float32{2}}}},
		{VertexCount: 1, Skin: []MeshSkinVertex{{Weights: [4]float32{float32(math.NaN())}}}},
		{VertexCount: 1, Skin: []MeshSkinVertex{{Joints: [4]uint32{math.MaxUint32}, Weights: [4]float32{1}}}},
		{VertexCount: 1, TargetCount: 1, MorphTargets: []MeshMorphVertex{{Position: [3]float32{float32(math.Inf(1))}}}},
		{VertexCount: 1, TargetCount: 1, MorphTargets: []MeshMorphVertex{{Normal: [3]float32{float32(math.NaN())}}}},
		{VertexCount: 1, TargetCount: 1, MorphTargets: []MeshMorphVertex{{Tangent: [3]float32{float32(math.Inf(-1))}}}},
	} {
		if _, _, err := packMeshDeformationStatic(nil, descriptor); err == nil {
			t.Fatalf("accepted invalid static data: %+v", descriptor)
		}
	}
}

func TestMeshDeformationRejectsInvalidAnimatedDataAndRecovers(t *testing.T) {
	source := &deformationTestSource{}
	handle, _ := source.upload(t, MeshDeformationDescriptor{VertexCount: 1, TargetCount: 1, Skin: []MeshSkinVertex{{Joints: [4]uint32{1}, Weights: [4]float32{1}}}, MorphTargets: []MeshMorphVertex{{}}})
	valid := MeshDeformationData{Mesh: handle, JointMatrices: [][16]float32{{}, {}}, MorphWeights: []float32{.5}}
	for _, bad := range []MeshDeformationData{
		{Mesh: NewHandle[MeshResource](handle.ID(), handle.Generation()+1)},
		{Mesh: handle, JointMatrices: [][16]float32{{}}},
		{Mesh: handle, JointMatrices: [][16]float32{{}, {}}, MorphWeights: []float32{1, 2}},
		{Mesh: handle, JointMatrices: [][16]float32{{}, {}}, MorphWeights: []float32{float32(math.NaN())}},
		{Mesh: handle, JointMatrices: [][16]float32{{float32(math.Inf(1))}, {}}},
	} {
		batch := MeshBatch3D{Deformations: []MeshDeformationData{valid, bad}}
		words, err := packMeshDeformations(make([]uint32, 0, 128), batch, source)
		if err == nil || len(words) != 0 {
			t.Fatal("invalid animated data accepted")
		}
		batch.Deformations[1] = valid
		words, err = packMeshDeformations(words, batch, source)
		if err != nil || len(words) != 82 || words[8+2] != 49 {
			t.Fatalf("recovery: %v %v", words, err)
		}
		batch.Draws = []MeshDraw3D{{Mesh: handle, DeformationIndex: 3}}
		if _, err = packMeshDeformations(words, batch, source); err == nil {
			t.Fatal("invalid index accepted")
		}
		batch.Draws = []MeshDraw3D{{Mesh: NewHandle[MeshResource](handle.ID(), 2), DeformationIndex: 1}}
		if _, err = packMeshDeformations(words, batch, source); err == nil {
			t.Fatal("unrelated mesh accepted")
		}
	}
	source.layouts.Remove(handle)
	if _, err := packMeshDeformations(nil, MeshBatch3D{Deformations: []MeshDeformationData{valid}}, source); err == nil {
		t.Fatal("destroyed mesh accepted")
	}
}

func BenchmarkPackMeshDeformations(b *testing.B) {
	source := &deformationTestSource{}
	descriptor := MeshDeformationDescriptor{VertexCount: 10000, TargetCount: 4, Skin: make([]MeshSkinVertex, 10000), MorphTargets: make([]MeshMorphVertex, 40000)}
	for i := range descriptor.Skin {
		descriptor.Skin[i].Weights[0] = 1
	}
	handle, _ := source.upload(b, descriptor)
	batch := MeshBatch3D{Draws: []MeshDraw3D{{Mesh: handle, DeformationIndex: 1}}, Deformations: []MeshDeformationData{{Mesh: handle, JointMatrices: make([][16]float32, 64), MorphWeights: []float32{.1, .2, .3, .4}}}}
	words, err := packMeshDeformations(nil, batch, source)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(words) * 4))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		words, err = packMeshDeformations(words, batch, source)
		if err != nil {
			b.Fatal(err)
		}
	}
}
