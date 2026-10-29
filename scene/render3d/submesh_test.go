package render3d

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestRenderSubmeshesShareGeometryAndPreserveMaterialsAndRigidMotion(t *testing.T) {
	for _, gpu := range []bool{false, true} {
		name := "CPU"
		if gpu {
			name = "GPU"
		}
		t.Run(name, func(t *testing.T) {
			base := &meshTestDriver{viewport: render.Rect{Width: 320, Height: 180}}
			var driver render.Driver = base
			if gpu {
				driver = &gpuMeshTestDriver{meshTestDriver: base}
			}
			render.Uninstall()
			if err := render.Install(driver); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(render.Uninstall)
			canvas := graphics.Canvas2DContext(320, 180)
			t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
			world := ecs.NewWorld()
			graphics.BindFrame(world, graphics.CurrentContext())
			Register(world)
			camera := ecs.NewBundle(4)
			camera.Put(scene.NewCamera())
			camera.Put(Camera{})
			camera.Put(scene.PerspectiveProjection3D())
			camera.Put(scene.GlobalTransform{Matrix: mgl32.Ident4()})
			world.Spawn(camera)
			vertices := []render.Vertex3D{
				{Position: [3]float32{-1, -1, 0}, Normal: [3]float32{0, 0, 1}},
				{Position: [3]float32{1, -1, 0}, Normal: [3]float32{0, 0, 1}},
				{Position: [3]float32{0, 1, 0}, Normal: [3]float32{0, 0, 1}},
			}
			mesh, err := graphics.NewMesh(append(vertices, vertices...), []uint32{0, 1, 2, 3, 4, 5})
			if err != nil {
				t.Fatal(err)
			}
			mesh.Submeshes = []graphics.Submesh{
				{FirstIndex: 0, IndexCount: 3, Material: 0, Node: -1},
				{FirstIndex: 3, IndexCount: 3, Material: 1, Node: 0},
			}
			mesh.Skin = make([]graphics.SkinVertex, 6)
			for index := range 3 {
				mesh.Skin[index] = graphics.SkinVertex{Active: true, Weights: [4]float32{1}}
			}
			materials := []graphics.StandardMaterial{
				*graphics.NewStandardMaterial(graphics.ColorRGBA{R: 1, A: 1}),
				*graphics.NewStandardMaterial(graphics.ColorRGBA{B: 1, A: 1}),
			}
			entity := spawnMesh3DForTest(world, mesh, nil, mgl32.Vec3{0, 0, -4})
			if err := world.Insert(entity, Material[graphics.StandardMaterial]{Materials: materials}); err != nil {
				t.Fatal(err)
			}
			if err := world.Insert(entity, Deformation{
				JointMatrices: []mgl32.Mat4{mgl32.Translate3D(0, .5, 0)},
				NodeMatrices:  []mgl32.Mat4{mgl32.Translate3D(.75, 0, 0)},
			}); err != nil {
				t.Fatal(err)
			}
			for _, system := range []ecs.SystemConfig{scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""), SortSystem(""), DrawSystem("")} {
				if err := system.Run(world); err != nil {
					t.Fatalf("%s: %v", system.Name, err)
				}
			}
			if err := canvas.FinishFrame(); err != nil {
				t.Fatal(err)
			}
			if len(base.batches) != 1 || len(base.batches[0].Draws) != 2 {
				t.Fatalf("draw batches: %+v", base.batches)
			}
			batch := base.batches[0]
			for index, draw := range batch.Draws {
				if draw.FirstIndex != uint32(index*3) || draw.IndexCount != 3 {
					t.Fatalf("draw %d index range: %+v", index, draw)
				}
				instance := batch.Instances[draw.FirstInstance]
				color := materials[index].BaseColor
				if instance.BaseColor != [4]float32{color.R, color.G, color.B, color.A} || instance.Model[12] != float32(index)*.75 || instance.Model[14] != -4 {
					t.Fatalf("draw %d material or node transform: %+v", index, instance)
				}
			}
			if gpu {
				if base.meshUploads != 1 || len(batch.Vertices) != 0 || len(batch.Deformations) != 1 || batch.Draws[0].DeformationIndex != batch.Draws[1].DeformationIndex {
					t.Fatal("GPU submeshes duplicated geometry or deformation uploads")
				}
			} else if len(batch.Vertices) != 6 || len(batch.Indices) != 6 || batch.Vertices[0].Position[1] != -.5 || batch.Vertices[3].Position[1] != -1 {
				t.Fatal("CPU submeshes duplicated geometry or deformed rigid vertices")
			}

			state := WorldFor(world)
			view := state.views[state.viewOrder[0]]
			frame := func() {
				if err := extractRender3D(&ecs.SystemContext{World: world}); err != nil {
					panic(err)
				}
				view.instances = view.instances[:0]
				view.draws = view.draws[:0]
				view.vertices = view.vertices[:0]
				view.indices = view.indices[:0]
				view.deformations = view.deformations[:0]
				clear(view.dynamicGeometry)
				clear(view.deformationIndices)
				view.appendPhase(view.extracted, state.white, true)
			}
			for range 5 {
				frame()
			}
			if allocs := testing.AllocsPerRun(100, frame); allocs != 0 {
				t.Fatalf("submesh frame allocations = %g", allocs)
			}
			for _, instance := range view.instances {
				if instance.PickingID() != entity.PickingID() {
					t.Fatal("submesh picking lost its owner entity")
				}
			}
		})
	}
}

func TestRigidNodeEntitiesFollowParentsWithoutReplacingManualSkinMatrices(t *testing.T) {
	world := ecs.NewWorld()
	Register(world)
	parent := world.Spawn(ecs.NewBundle(1))
	child := world.Spawn(ecs.NewBundle(2))
	owner := world.Spawn(ecs.NewBundle(2))
	for _, err := range []error{
		world.Insert(parent, scene.TransformFromTranslation(mgl32.Vec3{7, 0, 0})),
		world.Insert(child, scene.TransformFromTranslation(mgl32.Vec3{0, 2, 0})),
		world.Insert(child, scene.Parent{Entity: parent}),
		world.Insert(owner, scene.TransformFromTranslation(mgl32.Vec3{5, 0, 0})),
		world.Insert(owner, Deformation{
			NodeEntities: []ecs.Entity{child}, NodeInverseBind: []mgl32.Mat4{mgl32.Translate3D(0, -2, 0)},
			JointMatrices: []mgl32.Mat4{mgl32.Ident4()},
		}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := scene.SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := UpdateDeformationsSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	got, _ := world.Get[Deformation](owner)
	if len(got.JointMatrices) != 1 || got.JointMatrices[0] != mgl32.Ident4() || len(got.NodeMatrices) != 1 || got.NodeMatrices[0] != mgl32.Translate3D(2, 0, 0) {
		t.Fatalf("node deformation: %+v", got)
	}
}

func TestRigidNodeMotionKeepsStaticGeometry(t *testing.T) {
	canvas := graphics.Canvas2DContext(320, 180)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	world := ecs.NewWorld()
	graphics.BindFrame(world, graphics.CurrentContext())
	Register(world)
	camera := ecs.NewBundle(4)
	camera.Put(scene.NewCamera())
	camera.Put(Camera{})
	camera.Put(scene.PerspectiveProjection3D())
	camera.Put(scene.GlobalTransform{Matrix: mgl32.Ident4()})
	world.Spawn(camera)
	mesh := graphics.CubeMesh(1)
	mesh.Submeshes = []graphics.Submesh{{IndexCount: uint32(len(mesh.Indices)), Node: 0}}
	entity := spawnMesh3DForTest(world, mesh, graphics.NewStandardMaterial(graphics.White), mgl32.Vec3{0, 0, -4})
	if err := world.Insert(entity, Deformation{NodeMatrices: []mgl32.Mat4{mgl32.Translate3D(.5, 0, 0)}}); err != nil {
		t.Fatal(err)
	}
	if err := ExtractSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state := WorldFor(world)
	items := state.views[state.viewOrder[0]].extracted
	if len(items) != 1 || len(items[0].vertices) != 0 || items[0].transform != mgl32.Translate3D(.5, 0, -4) {
		t.Fatal("rigid animation should change only the draw transform")
	}
}
