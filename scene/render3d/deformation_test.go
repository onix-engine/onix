package render3d

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestInactiveDeformationDoesNotInvalidateComponents(t *testing.T) {
	world := ecs.NewWorld()
	deformations := world.NewSpawner1[Deformation]()
	deformations.Spawn(Deformation{MorphWeights: []float32{.5}})
	context := ecs.SystemContext{World: world}
	version := world.ComponentVersion[Deformation]()
	update := func() {
		if err := updateMeshDeformations3D(&context); err != nil {
			panic(err)
		}
	}
	if allocations := testing.AllocsPerRun(100, update); allocations != 0 {
		t.Fatalf("inactive update allocated %v times", allocations)
	}
	if world.ComponentVersion[Deformation]() != version {
		t.Fatal("inactive deformation marked the column changed")
	}
}

func TestInvalidDeformationDoesNotMutateBuffersAndReleasesQuery(t *testing.T) {
	for _, test := range []struct {
		name        string
		deformation Deformation
		transform   mgl32.Mat4
	}{
		{"joint count", Deformation{JointEntities: []ecs.Entity{1}}, mgl32.Ident4()},
		{"node count", Deformation{NodeEntities: []ecs.Entity{1}}, mgl32.Ident4()},
		{"singular owner", Deformation{JointEntities: []ecs.Entity{1}, InverseBind: []mgl32.Mat4{mgl32.Ident4()}}, mgl32.Scale3D(0, 1, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := ecs.NewWorld()
			deformations := world.NewSpawner2[Deformation, scene.GlobalTransform]()
			entity := deformations.Spawn(test.deformation, scene.GlobalTransform{Matrix: test.transform})
			version := world.ComponentVersion[Deformation]()
			if err := updateMeshDeformations3D(&ecs.SystemContext{World: world}); err == nil {
				t.Fatal("invalid deformation was accepted")
			}
			value := world.GetRef[Deformation](entity)
			if world.ComponentVersion[Deformation]() != version || len(value.JointMatrices) != 0 || len(value.NodeMatrices) != 0 || len(value.gpuJoints) != 0 {
				t.Fatal("validation error mutated deformation buffers")
			}
			if !world.Despawn(entity) {
				t.Fatal("failed update retained the active query")
			}
		})
	}
}
