package scene

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/go-gl/mathgl/mgl32"
)

func TestPerspectiveProjectionUsesInfiniteReverseZ(t *testing.T) {
	projection := NewPerspectiveProjection().Matrix(16.0 / 9.0)
	near := projection.Mul4x1(mgl32.Vec4{0, 0, -0.1, 1})
	far := projection.Mul4x1(mgl32.Vec4{0, 0, -100000, 1})
	if math.Abs(float64(near.Z()/near.W()-1)) > 1e-5 {
		t.Fatalf("near depth = %f, want 1", near.Z()/near.W())
	}
	if far.Z()/far.W() >= 0.00001 {
		t.Fatalf("far depth = %f, want approximately 0", far.Z()/far.W())
	}
}

func TestSyncGlobalTransformPropagatesHierarchy(t *testing.T) {
	world := ecs.NewWorld()
	parent := ecs.NewBundle(1)
	parent.Put(TransformFromTranslation(mgl32.Vec3{4, 5, 6}))
	parentEntity := world.Spawn(parent)
	child := ecs.NewBundle(2)
	child.Put(TransformFromTranslation(mgl32.Vec3{1, 2, 3}))
	child.Put(Parent{Entity: parentEntity})
	childEntity := world.Spawn(child)

	if err := SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatalf("sync 3D transforms: %v", err)
	}
	global, ok := world.Get[GlobalTransform](childEntity)
	if !ok {
		t.Fatal("expected a global transform")
	}
	if got := global.Matrix.Col(3).Vec3(); got != (mgl32.Vec3{5, 7, 9}) {
		t.Fatalf("global translation = %v, want [5 7 9]", got)
	}
}

func TestTransformLookingAtUsesNegativeZForward(t *testing.T) {
	transform := TransformFromTranslation(mgl32.Vec3{4, 3, 8}).LookingAt(mgl32.Vec3{}, mgl32.Vec3{0, 1, 0})
	viewTarget := transform.LocalMatrix().Inv().Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	if math.Abs(float64(viewTarget.X())) > 1e-5 || math.Abs(float64(viewTarget.Y())) > 1e-5 || viewTarget.Z() >= 0 {
		t.Fatalf("target in view space = %v, want centered on negative Z", viewTarget)
	}
}
