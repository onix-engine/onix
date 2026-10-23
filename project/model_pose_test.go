package project

import (
	"math"
	"strconv"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func poseFixture(t testing.TB) (*ModelRig, *ModelPoseClip) {
	t.Helper()
	nodes := make([]ModelNode, 160)
	for i := range nodes {
		nodes[i] = ModelNode{Name: fmtNode(i), Path: fmtNode(i), Parent: -1, Local: [16]float32(mgl32.Ident4()), World: [16]float32(mgl32.Ident4())}
	}
	nodes[0].Parent = 159
	nodes[0].Local = [16]float32(mgl32.Translate3D(0, 2, 0))
	nodes[0].World = nodes[0].Local
	clips, err := compileAnimationSources([]ModelAnimationSource{{Name: "turn", Duration: 1, Tracks: []ModelAnimationTrackSource{
		{Name: "node159.quaternion", Type: "quaternion", Times: []float32{0, 1}, Values: []float32{0, 0, 0, 1, 0, 0, 1, 0}},
		{Name: "node159.position", Type: "vector", Times: []float32{0, 1}, Values: []float32{0, 0, 0, 2, 0, 0}},
	}}}, nodes)
	if err != nil {
		t.Fatal(err)
	}
	rig, err := newModelRig(compiledModelResource{Nodes: nodes, Joints: []compiledModelJoint{{Node: 0, InverseBind: [16]float32(mgl32.Translate3D(0, -2, 0))}}, Animations: clips})
	if err != nil {
		t.Fatal(err)
	}
	clip, err := rig.Animation("turn")
	if err != nil {
		t.Fatal(err)
	}
	return rig, clip
}

func fmtNode(i int) string { return "node" + strconv.Itoa(i) }

func TestModelPoseQuaternionHierarchyAndIsolation(t *testing.T) {
	rig, clip := poseFixture(t)
	p, other := rig.NewPose(), rig.NewPose()
	p.Sample(clip, .5, false)
	position := p.NodeMatrix(0).Col(3).Vec3()
	if position.Sub(mgl32.Vec3{-1, 0, 0}).Len() > .0001 {
		t.Fatalf("quaternion/node hierarchy: %v", position)
	}
	if other.NodeMatrix(0).Col(3).Vec3() != (mgl32.Vec3{0, 2, 0}) {
		t.Fatal("pose buffers alias")
	}
	if p.Deformation.JointMatrices[0] != p.NodeMatrix(0).Mul4(mgl32.Translate3D(0, -2, 0)) {
		t.Fatal("inverse bind not applied")
	}
	p.Sample(clip, 1.5, true)
	if p.NodeMatrix(0).Col(3).Vec3().Sub(position).Len() > .0001 {
		t.Fatal("loop did not wrap")
	}
	p.Sample(nil, 0, false)
	if p.NodeMatrix(0).Col(3).Vec3() != (mgl32.Vec3{0, 2, 0}) {
		t.Fatal("bind pose not restored")
	}
	if allocations := testing.AllocsPerRun(100, func() { p.Sample(clip, .23, true) }); allocations != 0 {
		t.Fatalf("sample allocations: %g", allocations)
	}
}

func TestModelPoseInterpolation(t *testing.T) {
	track := compiledModelAnimationTrack{Target: "transform.translation", Components: 3, Interpolation: "STEP", Keyframes: []compiledModelAnimationKeyframe{{Time: 0, Value: [4]float32{1}}, {Time: 1, Value: [4]float32{3}}}}
	if sampleModelTrack(track, .9)[0] != 1 {
		t.Fatal("step interpolated")
	}
	track.Interpolation = "CUBICSPLINE"
	track.Keyframes[0].OutTangent[0] = 4
	if got := sampleModelTrack(track, .5)[0]; math.Abs(float64(got-2.5)) > .0001 {
		t.Fatalf("cubic tangent: %v", got)
	}
}

func TestModelAnimationSourceRejectsInvalidBindingsAndKeys(t *testing.T) {
	nodes := []ModelNode{{Name: "root", Path: "root", Parent: -1}}
	good := func() []ModelAnimationSource {
		return []ModelAnimationSource{{Name: "a", Duration: 1, Tracks: []ModelAnimationTrackSource{{Name: "root.position", Type: "vector", Times: []float32{0, 1}, Values: []float32{0, 0, 0, 1, 1, 1}}}}}
	}
	for _, mutate := range []func([]ModelAnimationSource){
		func(c []ModelAnimationSource) { c[0].Tracks[0].Name = "missing.position" },
		func(c []ModelAnimationSource) { c[0].Tracks[0].Values = nil },
		func(c []ModelAnimationSource) { c[0].Tracks[0].Times[1] = 0 },
		func(c []ModelAnimationSource) { c[0].Tracks[0].Values[0] = float32(math.NaN()) },
		func(c []ModelAnimationSource) { c[0].BlendMode = 2501 },
	} {
		clips := good()
		mutate(clips)
		if _, err := compileAnimationSources(clips, nodes); err == nil {
			t.Fatal("invalid animation accepted")
		}
	}
}
