package project

import (
	"fmt"
	"math"
	"sort"

	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/go-gl/mathgl/mgl32"
)

// ModelRig shares immutable hierarchy and animation data between instances.
// ModelPose owns the per-instance buffers; sampling performs no allocations.
type ModelRig struct {
	nodes      []ModelNode
	joints     []compiledModelJoint
	bind       []scene.Transform
	bindEuler  []mgl32.Vec3
	morphCount int
	order      []int
	clips      []ModelPoseClip
}

type ModelPoseClip struct {
	Name     string
	Duration float32
	rig      *ModelRig
	tracks   []compiledModelAnimationTrack
	targets  []int
}

type ModelPose struct {
	Deformation render3d.Deformation
	rig         *ModelRig
	local       []scene.Transform
	world       []mgl32.Mat4
	euler       []mgl32.Vec3
}

func (b *Bundle) ModelRig(modelID string) (*ModelRig, error) {
	value, _, err := b.modelResourceForSpec(SceneNodeSpec{ID: modelID, Components: SceneComponents{Model: &ResourceComponentSpec{Resource: modelID}}})
	if err != nil {
		return nil, err
	}
	return newModelRig(value.asset)
}

func newModelRig(asset compiledModelResource) (*ModelRig, error) {
	r := &ModelRig{nodes: asset.Nodes, joints: asset.Joints, bind: make([]scene.Transform, len(asset.Nodes)), clips: make([]ModelPoseClip, len(asset.Animations))}
	r.bindEuler = make([]mgl32.Vec3, len(r.nodes))
	r.morphCount = len(asset.MorphTargets)
	paths := make(map[string]int, len(r.nodes))
	visited := make([]uint8, len(r.nodes))
	var visit func(int) error
	visit = func(i int) error {
		if i < 0 || i >= len(r.nodes) || visited[i] == 1 {
			return fmt.Errorf("invalid model node hierarchy")
		}
		if visited[i] == 2 {
			return nil
		}
		visited[i] = 1
		if parent := r.nodes[i].Parent; parent >= 0 {
			if err := visit(int(parent)); err != nil {
				return err
			}
		}
		visited[i] = 2
		r.order = append(r.order, i)
		return nil
	}
	for i, node := range r.nodes {
		paths[node.Path] = i
		t, err := modelTransform(mgl32.Mat4(node.Local))
		if err != nil {
			return nil, fmt.Errorf("node %q: %w", node.Path, err)
		}
		r.bind[i] = t
		q := t.Rotation
		r.bindEuler[i] = mgl32.Vec3(modelQuaternionEuler([4]float32{q.V[0], q.V[1], q.V[2], q.W}))
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	for i, source := range asset.Animations {
		clip := &r.clips[i]
		*clip = ModelPoseClip{Name: source.Name, Duration: source.Duration, rig: r, tracks: source.Tracks, targets: make([]int, len(source.Tracks))}
		for j, track := range source.Tracks {
			target, ok := paths[track.Path]
			if !ok {
				return nil, fmt.Errorf("animation %q node %q not found", source.Name, track.Path)
			}
			clip.targets[j] = target
		}
	}
	return r, nil
}

func (r *ModelRig) Animation(name string) (*ModelPoseClip, error) {
	for i := range r.clips {
		if r.clips[i].Name == name {
			return &r.clips[i], nil
		}
	}
	return nil, fmt.Errorf("model animation %q not found", name)
}

// NodeIndex resolves an exact path or an unambiguous source node name at setup.
func (r *ModelRig) NodeIndex(name string) (int, error) {
	index := -1
	for i, node := range r.nodes {
		if node.Path == name {
			return i, nil
		}
		if node.Name == name {
			if index >= 0 {
				return -1, fmt.Errorf("ambiguous model node %q", name)
			}
			index = i
		}
	}
	if index < 0 {
		return -1, fmt.Errorf("model node %q not found", name)
	}
	return index, nil
}

func (r *ModelRig) NewPose() *ModelPose {
	n := len(r.nodes)
	p := &ModelPose{rig: r, local: make([]scene.Transform, n), world: make([]mgl32.Mat4, n), euler: make([]mgl32.Vec3, n)}
	p.Deformation = render3d.Deformation{MorphWeights: make([]float32, p.rig.morphCount), JointMatrices: make([]mgl32.Mat4, len(r.joints)), InverseBind: make([]mgl32.Mat4, len(r.joints)), NodeMatrices: make([]mgl32.Mat4, n), NodeInverseBind: make([]mgl32.Mat4, n)}
	for i, joint := range r.joints {
		p.Deformation.InverseBind[i] = mgl32.Mat4(joint.InverseBind)
	}
	for i, node := range r.nodes {
		p.Deformation.NodeInverseBind[i] = mgl32.Mat4(node.World).Inv()
	}
	p.Sample(nil, 0, false)
	return p
}

// NodeMatrix returns an animated node in model space for weapon/socket attachments.
func (p *ModelPose) NodeMatrix(index int) mgl32.Mat4 { return p.world[index] }

// Sample resets untracked nodes to their bind pose, then samples this rig's clip.
// Loop wraps positive time; a non-looping clip holds its last keyframe.
func (p *ModelPose) Sample(clip *ModelPoseClip, time float32, loop bool) {
	copy(p.local, p.rig.bind)
	if !finiteFloat32(time) {
		time = 0
	}
	if clip != nil && clip.rig == p.rig {
		if loop && clip.Duration > 0 {
			time = float32(math.Mod(float64(max(0, time)), float64(clip.Duration)))
		} else {
			time = max(0, min(time, clip.Duration))
		}
		copy(p.euler, p.rig.bindEuler)
		for i, track := range clip.tracks {
			node := clip.targets[i]
			value := sampleModelTrack(track, time)
			switch track.Target {
			case "transform.translation":
				if track.Components == 1 {
					p.local[node].Translation[track.Component] = value[0]
				} else {
					p.local[node].Translation = mgl32.Vec3{value[0], value[1], value[2]}
				}
			case "transform.scale":
				if track.Components == 1 {
					p.local[node].Scale[track.Component] = value[0]
				} else {
					p.local[node].Scale = mgl32.Vec3{value[0], value[1], value[2]}
				}
			case "transform.rotation":
				if track.Components == 4 {
					p.local[node].Rotation = animationQuat(value)
				} else {
					if track.Components == 1 {
						p.euler[node][track.Component] = value[0]
					} else {
						p.euler[node] = mgl32.Vec3{value[0], value[1], value[2]}
					}
					r := p.euler[node]
					p.local[node].Rotation = mgl32.QuatRotate(r[2], mgl32.Vec3{0, 0, 1}).Mul(mgl32.QuatRotate(r[1], mgl32.Vec3{0, 1, 0})).Mul(mgl32.QuatRotate(r[0], mgl32.Vec3{1, 0, 0}))
				}
			}
		}
	}
	for _, i := range p.rig.order {
		matrix := p.local[i].LocalMatrix()
		if parent := p.rig.nodes[i].Parent; parent >= 0 {
			matrix = p.world[parent].Mul4(matrix)
		}
		p.world[i] = matrix
		p.Deformation.NodeMatrices[i] = matrix.Mul4(p.Deformation.NodeInverseBind[i])
	}
	for i, joint := range p.rig.joints {
		p.Deformation.JointMatrices[i] = p.world[joint.Node].Mul4(p.Deformation.InverseBind[i])
	}
}

func animationQuat(v [4]float32) mgl32.Quat {
	return mgl32.Quat{W: v[3], V: mgl32.Vec3{v[0], v[1], v[2]}}.Normalize()
}

func sampleModelTrack(track compiledModelAnimationTrack, time float32) [4]float32 {
	keys := track.Keyframes
	if len(keys) == 0 {
		return [4]float32{}
	}
	next := sort.Search(len(keys), func(i int) bool { return keys[i].Time > time })
	if next == 0 {
		return keys[0].Value
	}
	if next == len(keys) {
		return keys[next-1].Value
	}
	a, b := keys[next-1], keys[next]
	if track.Interpolation == "STEP" {
		return a.Value
	}
	dt := b.Time - a.Time
	t := (time - a.Time) / dt
	if track.Components == 4 && track.Target == "transform.rotation" && track.Interpolation != "CUBICSPLINE" {
		q := mgl32.QuatSlerp(animationQuat(a.Value), animationQuat(b.Value), t).Normalize()
		return [4]float32{q.V[0], q.V[1], q.V[2], q.W}
	}
	var result [4]float32
	for i := range int(track.Components) {
		if track.Interpolation == "CUBICSPLINE" {
			t2, t3 := t*t, t*t*t
			result[i] = (2*t3-3*t2+1)*a.Value[i] + (t3-2*t2+t)*dt*a.OutTangent[i] + (-2*t3+3*t2)*b.Value[i] + (t3-t2)*dt*b.InTangent[i]
		} else {
			result[i] = a.Value[i] + (b.Value[i]-a.Value[i])*t
		}
	}
	return result
}

func modelTransform(matrix mgl32.Mat4) (scene.Transform, error) {
	t := scene.TransformFromTranslation(matrix.Col(3).Vec3())
	for i := 0; i < 3; i++ {
		t.Scale[i] = matrix.Col(i).Vec3().Len()
	}
	if matrix.Det() < 0 {
		t.Scale[0] = -t.Scale[0]
	}
	for i := 0; i < 3; i++ {
		if math.Abs(float64(t.Scale[i])) < 1e-12 {
			return t, fmt.Errorf("singular transform")
		}
		for row := 0; row < 3; row++ {
			matrix[i*4+row] /= t.Scale[i]
		}
	}
	t.Rotation = mgl32.Mat4ToQuat(matrix).Normalize()
	return t, nil
}
