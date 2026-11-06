package render3d

import (
	"testing"
	"unsafe"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/reuse"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestSharedSnapshotsKeepPerViewCPUAndGPUChoices(t *testing.T) {
	for _, pickingFirst := range []bool{false, true} {
		name := "GPU-first"
		if pickingFirst {
			name = "CPU-first"
		}
		t.Run(name, func(t *testing.T) {
			base := &meshTestDriver{}
			render.Uninstall()
			if err := render.Install(&gpuMeshTestDriver{meshTestDriver: base}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(render.Uninstall)
			canvas := graphics.Canvas2DContext(800, 600)
			t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
			w := ecs.NewWorld()
			graphics.BindFrame(w, canvas)
			Register(w)
			state := WorldFor(w)
			t.Cleanup(func() { _ = state.Dispose() })
			cameras := w.NewSpawner4[scene.Camera, Camera, scene.Projection, scene.GlobalTransform]()
			first := cameras.Spawn(scene.NewCamera(), Camera{}, scene.PerspectiveProjection3D(), scene.GlobalTransform{Matrix: mgl32.Ident4()})
			second := cameras.Spawn(scene.NewCamera(), Camera{}, scene.PerspectiveProjection3D(), scene.GlobalTransform{Matrix: mgl32.Translate3D(0, 0, 1)})
			picking, normal := second, first
			if pickingFirst {
				picking, normal = first, second
			}
			mesh := graphics.CubeMesh(1)
			t.Cleanup(func() { _ = mesh.Dispose() })
			mesh.MorphTargets = []graphics.MorphTarget{{PositionDelta: make([][3]float32, len(mesh.Vertices))}}
			meshes := w.NewSpawner3[Mesh, Deformation, scene.GlobalTransform]()
			meshes.Spawn(Mesh{Mesh: mesh}, Deformation{MorphWeights: []float32{.5}}, scene.GlobalTransform{Matrix: mgl32.Translate3D(0, 0, -5)})
			lights := w.NewSpawner2[DirectionalLight, scene.GlobalTransform]()
			sun := NewDirectionalLight()
			sun.Shadows = NewShadowSettings()
			lights.Spawn(sun, scene.GlobalTransform{Matrix: mgl32.Ident4()})
			frame := func() {
				if err := scene.ExtractCamerasSystem("").Run(w); err != nil {
					t.Fatal(err)
				}
				for i := range scene.RenderWorldFor(w).Cameras {
					camera := &scene.RenderWorldFor(w).Cameras[i]
					if camera.Entity == picking {
						camera.ViewMode = render.RenderViewPicking
					}
				}
				for _, system := range []ecs.SystemConfig{ExtractSystem(""), PrepareSystem(""), SortSystem("")} {
					if err := system.Run(w); err != nil {
						t.Fatal(err)
					}
				}
			}
			frame()
			cpu, gpu := state.views[picking], state.views[normal]
			if state.snapshotCount != 1 || len(cpu.extracted) != 1 || len(gpu.extracted) != 1 {
				t.Fatal("multiple cameras duplicated or lost the shared mesh payload")
			}
			c, g := cpu.extracted[0], gpu.extracted[0]
			if c.meshSnapshot3D != g.meshSnapshot3D || len(c.vertices) == 0 || len(g.vertices) != 0 || !g.meshHandle.Valid() {
				t.Fatal("CPU preparation changed another view's GPU choice/handle")
			}
			casters := 0
			for _, shadow := range gpu.shadow.views[:gpu.shadow.count] {
				for _, item := range shadow.extracted {
					if item.meshSnapshot3D != g.meshSnapshot3D || len(item.vertices) != 0 {
						t.Fatal("shadow duplicated the shared material or inherited CPU geometry")
					}
					casters++
				}
			}
			if casters == 0 {
				t.Fatal("fixture lost its shadow caster")
			}
			for _, view := range []*renderView3D{cpu, gpu} {
				view.appendPhase(view.opaque.items, nil, false)
				if len(view.draws) != 1 || view.draws[0].Mesh.Valid() != (view == gpu) {
					t.Fatal("shared payload changed CPU/GPU draw submission")
				}
			}
			if n := testing.AllocsPerRun(100, frame); n != 0 {
				t.Fatalf("shared extraction/preparation allocated %g", n)
			}
		})
	}
}

func TestCulledCandidatesDoNotAllocateMaterialSnapshots(t *testing.T) {
	w, _ := memoryPreparationFixture(t, 0)
	cameras := w.NewSpawner4[scene.Camera, Camera, scene.Projection, scene.GlobalTransform]()
	cameras.Spawn(scene.NewCamera(), Camera{}, scene.PerspectiveProjection3D(), scene.GlobalTransform{Matrix: mgl32.Ident4()})
	mesh := graphics.CubeMesh(1)
	t.Cleanup(func() { _ = mesh.Dispose() })
	meshes := w.NewSpawner2[Mesh, scene.GlobalTransform]()
	var first ecs.Entity
	for i := range 4096 {
		e := meshes.Spawn(Mesh{Mesh: mesh}, scene.GlobalTransform{Matrix: mgl32.Translate3D(10000, 0, -5)})
		if i == 0 {
			first = e
		}
	}
	frame := func() {
		for _, system := range []ecs.SystemConfig{scene.ExtractCamerasSystem(""), ExtractSystem("")} {
			if err := system.Run(w); err != nil {
				t.Fatal(err)
			}
		}
	}
	frame()
	state := WorldFor(w)
	if state.snapshotCount != 0 || len(state.snapshotBlocks) != 0 {
		t.Fatal("fully culled meshes allocated large material snapshots")
	}
	w.GetMut[scene.GlobalTransform](first).Matrix[12] = 0
	frame()
	if state.snapshotCount != 1 {
		t.Fatal("first visible mesh did not capture a snapshot")
	}
	w.GetMut[scene.GlobalTransform](first).Matrix[12] = 10000
	for range reuse.RetentionFrames + 1 {
		frame()
	}
	if state.snapshotCount != 0 || len(state.snapshotBlocks) != 0 {
		t.Fatal("inactive shared snapshot blocks were not released")
	}
	t.Logf("shared payload=%d B, view reference=%d B, candidate=%d B", unsafe.Sizeof(meshSnapshot3D{}), unsafe.Sizeof(extractedMesh3D{}), unsafe.Sizeof(extractedMeshCandidate3D{}))
}

func TestSharedSnapshotIncludesShadowCasterOutsideCamera(t *testing.T) {
	w, _ := memoryPreparationFixture(t, 0)
	cameras := w.NewSpawner4[scene.Camera, Camera, scene.Projection, scene.GlobalTransform]()
	for range 2 {
		cameras.Spawn(scene.NewCamera(), Camera{}, scene.PerspectiveProjection3D(), scene.GlobalTransform{Matrix: mgl32.Ident4()})
	}
	mesh := graphics.CubeMesh(1)
	t.Cleanup(func() { _ = mesh.Dispose() })
	meshes := w.NewSpawner2[Mesh, scene.GlobalTransform]()
	meshes.Spawn(Mesh{Mesh: mesh}, scene.GlobalTransform{Matrix: mgl32.Translate3D(0, 0, 5)})
	light := NewPointLight()
	light.Shadows = NewPunctualShadowSettings()
	lights := w.NewSpawner2[PointLight, scene.GlobalTransform]()
	lights.Spawn(light, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	for _, system := range []ecs.SystemConfig{scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem("")} {
		if err := system.Run(w); err != nil {
			t.Fatal(err)
		}
	}
	state := WorldFor(w)
	if state.snapshotCount != 1 {
		t.Fatalf("shadow-only caster captured %d payloads, want 1", state.snapshotCount)
	}
	casters := 0
	for _, entity := range state.viewOrder {
		view := state.views[entity]
		if len(view.extracted) != 0 {
			t.Fatal("fixture's caster is visible to the main camera")
		}
		for _, shadow := range view.shadow.views[:view.shadow.count] {
			for _, item := range shadow.extracted {
				if item.meshSnapshot3D != state.snapshotAt(0) {
					t.Fatal("shadow views duplicated the off-camera caster")
				}
				casters++
			}
		}
	}
	if casters != 2 {
		t.Fatalf("shadow-only caster appeared %d times, want once per camera", casters)
	}
}
