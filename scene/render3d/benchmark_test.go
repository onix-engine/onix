package render3d

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func BenchmarkRender3DPreparedFrame(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			render.Uninstall()
			if err := render.Install(&meshTestDriver{viewport: render.Rect{Width: 800, Height: 600}}); err != nil {
				b.Fatal(err)
			}
			b.Cleanup(render.Uninstall)
			restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(800, 600))
			b.Cleanup(restore)
			world := ecs.NewWorld()
			Register(world)
			state := renderWorld3D(world)
			b.Cleanup(func() { _ = state.Dispose() })
			cameras := ecs.NewSpawner4[scene.Camera, Camera, scene.Projection, scene.GlobalTransform](world)
			for range 2 {
				cameras.Spawn(scene.NewCamera(), Camera{}, scene.PerspectiveProjection3D(), scene.GlobalTransform{Matrix: mgl32.Ident4()})
			}
			mesh := graphics.CubeMesh(1)
			b.Cleanup(func() { _ = mesh.Dispose() })
			material := graphics.NewStandardMaterial(graphics.White)
			meshes := ecs.NewSpawner3[Mesh, Material[graphics.StandardMaterial], scene.GlobalTransform](world)
			for i := range count {
				meshes.Spawn(Mesh{mesh}, Material[graphics.StandardMaterial]{Material: material},
					scene.GlobalTransform{Matrix: mgl32.Translate3D(float32(i%10)-5, 0, -float32(i/10)-5)})
			}
			context := ecs.SystemContext{World: world}
			frame := func() {
				for _, system := range [...]func(*ecs.SystemContext) error{scene.ExtractCameras, extractRender3D, prepareRenderResources3D, sortRenderPhases3D} {
					if err := system(&context); err != nil {
						b.Fatal(err)
					}
				}
				for _, entity := range state.viewOrder {
					view := state.views[entity]
					view.instances, view.draws = view.instances[:0], view.draws[:0]
					view.appendPhase(view.opaque.items, state.white, false)
				}
			}
			for range 5 {
				frame()
			}
			b.ReportAllocs()
			for b.Loop() {
				frame()
			}
		})
	}
}

func BenchmarkMeshDeformationUpdate(b *testing.B) {
	for _, active := range []bool{false, true} {
		b.Run(fmt.Sprint(active), func(b *testing.B) {
			world := ecs.NewWorld()
			Register(world)
			var joints []ecs.Entity
			var inverse []mgl32.Mat4
			if active {
				spawner := ecs.NewSpawner1[scene.GlobalTransform](world)
				for i := range 8 {
					joints = append(joints, spawner.Spawn(scene.GlobalTransform{Matrix: mgl32.Translate3D(float32(i), 0, 0)}))
					inverse = append(inverse, mgl32.Ident4())
				}
			}
			meshes := ecs.NewSpawner2[Deformation, scene.GlobalTransform](world)
			for range 1000 {
				meshes.Spawn(Deformation{JointEntities: joints, InverseBind: inverse}, scene.GlobalTransform{Matrix: mgl32.Ident4()})
			}
			context := ecs.SystemContext{World: world}
			update := func() {
				if err := updateMeshDeformations3D(&context); err != nil {
					b.Fatal(err)
				}
			}
			update()
			b.ReportAllocs()
			for b.Loop() {
				update()
			}
		})
	}
}
