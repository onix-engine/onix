package render3d

import (
	"testing"
	"unsafe"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func memoryPreparationFixture(t testing.TB, count int) (*ecs.World, *renderView3D) {
	t.Helper()
	render.Uninstall()
	if err := render.Install(&meshTestDriver{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(800, 600))
	t.Cleanup(restore)
	w := ecs.NewWorld()
	graphics.BindFrame(w, graphics.CurrentContext())
	Register(w)
	state := WorldFor(w)
	t.Cleanup(func() { _ = state.Dispose() })
	view := state.viewFor(1)
	state.viewOrder = []ecs.Entity{1}
	view.extracted = make([]extractedMesh3D, count)
	for i := range view.extracted {
		view.extracted[i] = extractedMesh3D{entity: ecs.Entity(i + 1), vertices: []render.Vertex3D{{}}, material: graphics.Material3DData{AlphaMode: graphics.AlphaOpaque}}
		if i%2 == 0 {
			view.extracted[i].material.AlphaMode = graphics.AlphaBlend
		}
	}
	return w, view
}

func TestPreparedPhasesShareSnapshots(t *testing.T) {
	w, view := memoryPreparationFixture(t, 8)
	if err := PrepareSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	if len(view.opaque.items) != 4 || len(view.transparent.items) != 4 {
		t.Fatal("material partition lost meshes")
	}
	var seen [9]bool
	for phaseIndex, phase := range []renderPhase3D{view.opaque, view.transparent} {
		for i := range phase.items {
			item := &phase.items[i]
			if item.entity < 1 || item.entity > 8 || seen[item.entity] {
				t.Fatal("partition duplicated or lost a mesh")
			}
			seen[item.entity] = true
			if (item.material.AlphaMode == graphics.AlphaBlend) != (phaseIndex == 1) {
				t.Fatal("mesh was queued into the wrong material phase")
			}
			if phase.items[i].textureHandle != view.extracted[0].textureHandle {
				t.Fatal("phase contains an unprepared mesh")
			}
		}
	}
	if &view.opaque.items[0] != &view.extracted[0] || &view.transparent.items[0] != &view.extracted[4] {
		t.Fatal("render phases duplicate mesh snapshots")
	}
}

func TestExtractionReleasesRemovedMeshReferences(t *testing.T) {
	w, _ := memoryPreparationFixture(t, 0)
	cameras := w.NewSpawner4[scene.Camera, Camera, scene.Projection, scene.GlobalTransform]()
	camera := scene.NewCamera()
	camera.Viewport = scene.CameraViewport{Width: 800, Height: 600}
	entity := cameras.Spawn(camera, Camera{}, scene.PerspectiveProjection3D(), scene.GlobalTransform{Matrix: mgl32.Ident4()})
	mesh := graphics.CubeMesh(1)
	t.Cleanup(func() { _ = mesh.Dispose() })
	meshes := w.NewSpawner2[Mesh, scene.GlobalTransform]()
	first := meshes.Spawn(Mesh{Mesh: mesh}, scene.GlobalTransform{Matrix: mgl32.Translate3D(0, 0, -5)})
	second := meshes.Spawn(Mesh{Mesh: mesh}, scene.GlobalTransform{Matrix: mgl32.Translate3D(0, 0, -6)})
	frame := func() {
		for _, system := range []ecs.SystemConfig{scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem("")} {
			if err := system.Run(w); err != nil {
				t.Fatal(err)
			}
		}
	}
	frame()
	view := WorldFor(w).views[entity]
	if len(view.extracted) != 2 {
		t.Fatal("meshes were not extracted")
	}
	for _, removed := range []ecs.Entity{second, first} {
		w.Despawn(removed)
		frame()
		for _, items := range [][]extractedMesh3D{view.extracted, view.opaque.items, view.transparent.items} {
			for _, item := range items[:cap(items)] {
				if item.entity == removed || len(items) == 0 && item.mesh != nil {
					t.Fatal("unused render snapshot retains a removed mesh")
				}
			}
		}
	}
}

func TestShadowAtlasReleasesRemovedViews(t *testing.T) {
	_, view := memoryPreparationFixture(t, 0)
	var settings [render.MaxMeshLights]extractedShadowLight3D
	settings[0].punctual = NewPunctualShadowSettings()
	settings[0].punctual.Resolution = 64
	view.lights = []render.Light3D{NewPointLight().renderLight(mgl32.Ident4())}
	if err := setupShadowAtlas3D(view, settings, false, 0); err != nil {
		t.Fatal(err)
	}
	if view.shadow.count != 6 || view.shadow.views[5] == nil {
		t.Fatal("point light did not create six shadow views")
	}
	first := view.shadow.views[0]
	view.lights[0] = NewSpotLight().renderLight(mgl32.Ident4())
	if err := setupShadowAtlas3D(view, settings, false, 0); err != nil {
		t.Fatal(err)
	}
	if view.shadow.count != 1 || view.shadow.views[0] != first {
		t.Fatal("shadow atlas did not reuse its remaining view")
	}
	for _, removed := range view.shadow.views[1:] {
		if removed != nil {
			t.Fatal("shadow atlas retains inactive views")
		}
	}
}

func TestPreparationReleasesTemporaryResourceReferences(t *testing.T) {
	w, view := memoryPreparationFixture(t, 1)
	mesh := graphics.CubeMesh(1)
	t.Cleanup(func() { _ = mesh.Dispose() })
	view.extracted[0].mesh, view.extracted[0].vertices = mesh, nil
	meshes := w.FrameScratchMap[preparedMeshHandles3DScratch, *graphics.Mesh, render.MeshHandle]()
	textures := w.FrameScratchMap[preparedTextureHandles3DScratch, *graphics.Texture, render.TextureHandle]()
	if err := PrepareSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	if len(meshes) != 0 || len(textures) != 0 {
		t.Fatal("temporary preparation maps retain resource references")
	}
}

func TestExtractionCleansUpPartiallyInitializedShadows(t *testing.T) {
	w, _ := memoryPreparationFixture(t, 0)
	cameras := w.NewSpawner4[scene.Camera, Camera, scene.Projection, scene.GlobalTransform]()
	cameras.Spawn(scene.NewCamera(), Camera{}, scene.PerspectiveProjection3D(), scene.GlobalTransform{Matrix: mgl32.Ident4()})
	if err := scene.ExtractCamerasSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	renderState, _ := w.GetResource[*scene.RenderWorld]()
	renderState.Cameras[0].Transform.Matrix = mgl32.Mat4{}
	sun := NewDirectionalLight()
	sun.Shadows = NewShadowSettings()
	sun.Shadows.Cascades = 2
	lights := w.NewSpawner2[DirectionalLight, scene.GlobalTransform]()
	lights.Spawn(sun, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	if err := ExtractSystem("").Run(w); err == nil {
		t.Fatal("singular shadow camera was accepted")
	}
}

func TestRenderViewReleasesUnusedDeformationData(t *testing.T) {
	w, view := memoryPreparationFixture(t, 0)
	view.frame.Camera.viewport = scene.CameraViewport{Width: 800, Height: 600}
	view.deformations = []render.MeshDeformationData{{JointMatrices: make([][16]float32, 64), MorphWeights: []float32{1}}}
	if err := render3DView(graphics.CurrentContext(), WorldFor(w), view); err != nil {
		t.Fatal(err)
	}
	for _, deformation := range view.deformations[:cap(view.deformations)] {
		if deformation.JointMatrices != nil || deformation.MorphWeights != nil {
			t.Fatal("unused draw deformation retains joint or morph data")
		}
	}
}

func BenchmarkPreparedSnapshotMemory(b *testing.B) {
	w, view := memoryPreparationFixture(b, 2048)
	context := &ecs.SystemContext{World: w}
	if err := prepareRenderResources3D(context); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		view.opaque.items = view.opaque.items[:0]
		view.transparent.items = view.transparent.items[:0]
		if err := prepareRenderResources3D(context); err != nil {
			b.Fatal(err)
		}
	}
	bytes := uintptr(cap(view.extracted)) * unsafe.Sizeof(extractedMesh3D{})
	start := uintptr(unsafe.Pointer(unsafe.SliceData(view.extracted)))
	end := start + bytes
	for _, phase := range []renderPhase3D{view.opaque, view.transparent} {
		address := uintptr(unsafe.Pointer(unsafe.SliceData(phase.items)))
		if address < start || address >= end {
			bytes += uintptr(cap(phase.items)) * unsafe.Sizeof(extractedMesh3D{})
		}
	}
	b.ReportMetric(float64(bytes), "snapshot-reserved-B")
}
