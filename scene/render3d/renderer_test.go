package render3d

import (
	"math"
	"slices"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestCompareOpaque3DSortsNormalTexturesForBatching(t *testing.T) {
	first := graphics.CreateTextureByColor(graphics.White)
	second := graphics.CreateTextureByColor(graphics.Black)
	items := []extractedMesh3D{
		{entity: 1, meshKey: 1, material: graphics.Material3DData{NormalTexture: second}},
		{entity: 2, meshKey: 1, material: graphics.Material3DData{NormalTexture: first}},
	}

	slices.SortFunc(items, compareOpaque3D)

	if items[0].material.NormalTexture != first || items[1].material.NormalTexture != second {
		t.Fatalf("opaque items were not ordered by normal texture: %+v", items)
	}
}

func TestCamera3DViewportMapsProjectionIntoCanvasRegion(t *testing.T) {
	ctx := graphics.Canvas2DContext(400, 300)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)
	world := ecs.NewWorld()
	Register(world)
	camera := scene.NewCamera()
	camera.Viewport = scene.CameraViewport{X: 100, Y: 50, Width: 200, Height: 100}
	bundle := ecs.NewBundle(4)
	ecs.Put(&bundle, camera)
	ecs.Put(&bundle, Camera{})
	ecs.Put(&bundle, scene.PerspectiveProjection3D())
	ecs.Put(&bundle, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	cameraEntity := world.Spawn(bundle)

	if err := scene.ExtractCamerasSystem("").Run(world); err != nil {
		t.Fatalf("extract cameras: %v", err)
	}
	if err := ExtractSystem("").Run(world); err != nil {
		t.Fatalf("extract viewport camera: %v", err)
	}
	state, ok := ecs.GetResource[*World](world)
	view := state.views[cameraEntity]
	if !ok || view == nil || view.frame.Camera.viewport != camera.Viewport {
		t.Fatalf("unexpected extracted viewport: %+v", view)
	}
	clip := view.frame.Camera.viewProjection.Mul4x1(mgl32.Vec4{0, 0, -1, 1})
	if math.Abs(float64(clip.X()/clip.W())) > 1e-5 || math.Abs(float64(clip.Y()/clip.W()-1.0/3.0)) > 1e-5 {
		t.Fatalf("viewport center mapped to clip %v", clip)
	}
}

func TestCamera3DRenderTargetUsesTargetSurfaceAndRestoresViewport(t *testing.T) {
	driver := &meshTestDriver{viewport: render.Rect{Width: 800, Height: 600}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	context := graphics.Canvas2DContext(800, 600)
	restore := graphics.SetCurrentContextForTesting(context)
	t.Cleanup(restore)
	target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: 200, Height: 100, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Dispose() })

	world := ecs.NewWorld()
	Register(world)
	camera := scene.NewCamera()
	cameraBundle := ecs.NewBundle(4)
	ecs.Put(&cameraBundle, camera)
	ecs.Put(&cameraBundle, Camera{Target: target})
	ecs.Put(&cameraBundle, scene.PerspectiveProjection3D())
	ecs.Put(&cameraBundle, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	cameraEntity := world.Spawn(cameraBundle)
	spawnMesh3DForTest(world, graphics.CubeMesh(1), graphics.NewStandardMaterial(graphics.White), mgl32.Vec3{0, 0, -4})

	for _, system := range []ecs.SystemConfig{
		scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""), SortSystem(""), DrawSystem(""),
	} {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	state, _ := ecs.GetResource[*World](world)
	view := state.views[cameraEntity]
	if view == nil || view.frame.Target != target || view.frame.Camera.viewport.Width != 200 || view.frame.Camera.viewport.Height != 100 {
		t.Fatalf("unexpected offscreen render view: %+v", view)
	}
	clip := view.frame.Camera.viewProjection.Mul4x1(mgl32.Vec4{0, 0, -1, 1})
	if math.Abs(float64(clip.X()/clip.W())) > 1e-5 || math.Abs(float64(clip.Y()/clip.W())) > 1e-5 {
		t.Fatalf("target center mapped to clip %v", clip)
	}
	if driver.targetBegins != 1 || driver.targetEnds != 1 || len(driver.meshViewports) != 1 || driver.meshViewports[0] != (render.Rect{Width: 200, Height: 100}) {
		t.Fatalf("target submissions = begin %d end %d viewports %+v", driver.targetBegins, driver.targetEnds, driver.meshViewports)
	}
	if driver.viewport != (render.Rect{Width: 800, Height: 600}) {
		t.Fatalf("restored viewport = %+v", driver.viewport)
	}
}

func TestCamera3DRenderTargetClearsEmptyView(t *testing.T) {
	driver := &meshTestDriver{viewport: render.Rect{Width: 800, Height: 600}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	context := graphics.Canvas2DContext(800, 600)
	restore := graphics.SetCurrentContextForTesting(context)
	t.Cleanup(restore)
	target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: 200, Height: 100, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Dispose() })

	world := ecs.NewWorld()
	Register(world)
	cameraBundle := ecs.NewBundle(4)
	ecs.Put(&cameraBundle, scene.NewCamera())
	ecs.Put(&cameraBundle, Camera{Target: target})
	ecs.Put(&cameraBundle, scene.PerspectiveProjection3D())
	ecs.Put(&cameraBundle, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	world.Spawn(cameraBundle)

	for _, system := range []ecs.SystemConfig{
		scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""),
		SortSystem(""), DrawSystem(""),
	} {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	if driver.targetBegins != 1 || driver.targetEnds != 1 || len(driver.batches) != 0 {
		t.Fatalf("empty target pass = begin %d end %d mesh batches %d", driver.targetBegins, driver.targetEnds, len(driver.batches))
	}
	if driver.viewport != (render.Rect{Width: 800, Height: 600}) {
		t.Fatalf("restored viewport = %+v", driver.viewport)
	}
}

func TestRender3DPhasesCullSortAndReuseGeometry(t *testing.T) {
	driver := &meshTestDriver{viewport: render.Rect{Width: 320, Height: 180}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatalf("install driver: %v", err)
	}
	t.Cleanup(render.Uninstall)
	ctx := graphics.Canvas2DContext(320, 180)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)

	world := ecs.NewWorld()
	Register(world)
	cameraBundle := ecs.NewBundle(4)
	ecs.Put(&cameraBundle, scene.NewCamera())
	ecs.Put(&cameraBundle, Camera{})
	ecs.Put(&cameraBundle, scene.PerspectiveProjection3D())
	ecs.Put(&cameraBundle, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	cameraEntity := world.Spawn(cameraBundle)
	mesh := graphics.CubeMesh(1)
	opaque := graphics.NewStandardMaterial(graphics.ColorRGBA{R: 1, A: 1})
	masked := graphics.NewStandardMaterial(graphics.White)
	masked.AlphaMode = graphics.AlphaMask
	transparent := graphics.NewStandardMaterial(graphics.ColorRGBA{G: 1, A: 0.5})
	transparent.AlphaMode = graphics.AlphaBlend
	spawnMesh3DForTest(world, mesh, opaque, mgl32.Vec3{0, 0, -4})
	spawnMesh3DForTest(world, mesh, opaque, mgl32.Vec3{1, 0, -4})
	secondMesh := graphics.CubeMesh(2)
	spawnMesh3DForTest(world, secondMesh, opaque, mgl32.Vec3{-1, 0, -5})
	spawnMesh3DForTest(world, mesh, masked, mgl32.Vec3{-1, 0, -4})
	far := spawnMesh3DForTest(world, mesh, transparent, mgl32.Vec3{0, 0, -12})
	near := spawnMesh3DForTest(world, mesh, transparent, mgl32.Vec3{0, 0, -6})
	spawnMesh3DForTest(world, graphics.CubeMesh(3), opaque, mgl32.Vec3{1000, 0, -4})

	for _, system := range []ecs.SystemConfig{
		scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""),
		SortSystem(""), DrawSystem(""),
	} {
		if err := system.Run(world); err != nil {
			t.Fatalf("run %s: %v", system.Name, err)
		}
	}
	if err := ctx.FinishFrame(); err != nil {
		t.Fatalf("finish frame: %v", err)
	}
	if len(driver.batches) != 1 {
		t.Fatalf("mesh submissions = %d, want 1", len(driver.batches))
	}
	batch := driver.batches[0]
	if len(batch.Draws) != 3 || len(batch.Instances) != 6 {
		t.Fatalf("batch sizes = %d draws, %d instances; want 3 draws, 6 instances after depth-writing and transparent batching", len(batch.Draws), len(batch.Instances))
	}
	if batch.Draws[0].InstanceCount != 3 {
		t.Fatalf("depth-writing instance count = %d, want 3", batch.Draws[0].InstanceCount)
	}
	if len(batch.Vertices) != 0 || len(batch.Indices) != 0 || batch.Draws[0].Mesh != mesh.RenderHandle() {
		t.Fatalf("static mesh was not submitted by handle: %+v", batch)
	}
	if batch.Draws[1].Mesh != secondMesh.RenderHandle() || batch.Draws[0].Mesh == batch.Draws[1].Mesh {
		t.Fatalf("different static meshes were merged: %+v", batch.Draws[:2])
	}
	state, _ := ecs.GetResource[*World](world)
	view := state.views[cameraEntity]
	if view.transparent.items[0].entity != far || view.transparent.items[1].entity != near {
		t.Fatalf("transparent phase is not back-to-front: %+v", view.transparent.items)
	}
	if batch.Draws[0].AlphaMode != uint32(graphics.AlphaOpaque) || batch.Draws[1].AlphaMode != uint32(graphics.AlphaOpaque) ||
		batch.Draws[2].AlphaMode != uint32(graphics.AlphaBlend) || batch.Draws[2].InstanceCount != 2 {
		t.Fatalf("unexpected alpha phase order: %+v", batch.Draws)
	}
	maskedInstance := false
	for _, instance := range batch.Instances[:3] {
		maskedInstance = maskedInstance || instance.Material[3] == float32(graphics.AlphaMask)
	}
	if !maskedInstance {
		t.Fatal("alpha-mask mode was not retained in the merged depth-writing instances")
	}

	driver.batches = driver.batches[:0]
	for _, system := range []ecs.SystemConfig{
		scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""),
		SortSystem(""), DrawSystem(""),
	} {
		if err := system.Run(world); err != nil {
			t.Fatalf("run second frame %s: %v", system.Name, err)
		}
	}
	if err := ctx.FinishFrame(); err != nil {
		t.Fatalf("finish second frame: %v", err)
	}
	if driver.meshUploads != 2 {
		t.Fatalf("mesh uploads after two frames = %d, want 2", driver.meshUploads)
	}
}

func TestExtractRender3DBuildsPerCameraLayerFilteredViews(t *testing.T) {
	ctx := graphics.Canvas2DContext(320, 180)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)
	world := ecs.NewWorld()
	Register(world)
	spawnCamera := func(layers scene.RenderLayers) ecs.Entity {
		bundle := ecs.NewBundle(5)
		ecs.Put(&bundle, scene.NewCamera())
		ecs.Put(&bundle, Camera{})
		ecs.Put(&bundle, scene.PerspectiveProjection3D())
		ecs.Put(&bundle, scene.GlobalTransform{Matrix: mgl32.Ident4()})
		ecs.Put(&bundle, layers)
		return world.Spawn(bundle)
	}
	defaultCamera := spawnCamera(scene.RenderLayer(0))
	highLayerCamera := spawnCamera(scene.RenderLayer(130))
	mesh := graphics.CubeMesh(1)
	entity := spawnMesh3DForTest(world, mesh, graphics.NewStandardMaterial(graphics.White), mgl32.Vec3{0, 0, -4})
	if err := ecs.Insert(world, entity, scene.RenderLayer(130)); err != nil {
		t.Fatal(err)
	}

	if err := scene.ExtractCamerasSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := ExtractSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state, _ := ecs.GetResource[*World](world)
	if len(state.views[defaultCamera].extracted) != 0 || len(state.views[highLayerCamera].extracted) != 1 {
		t.Fatalf("per-camera extracted views: default=%d high=%d", len(state.views[defaultCamera].extracted), len(state.views[highLayerCamera].extracted))
	}
}

func TestExtractRender3DCollectsNodeLightComponents(t *testing.T) {
	ctx := graphics.Canvas2DContext(320, 180)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)
	world := ecs.NewWorld()
	Register(world)
	camera := ecs.NewBundle(4)
	ecs.Put(&camera, scene.NewCamera())
	ecs.Put(&camera, Camera{})
	ecs.Put(&camera, scene.PerspectiveProjection3D())
	ecs.Put(&camera, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	world.Spawn(camera)
	for _, value := range []any{AmbientLight{Color: graphics.White, Brightness: 0.2}, DirectionalLight{Color: graphics.White, Illuminance: 10000}, PointLight{Color: graphics.White, Intensity: 500, Unit: graphics.LightLumen, Range: 8}, SpotLight{Color: graphics.White, Intensity: 700, Unit: graphics.LightLumen, Shape: SpotCone, Range: 9, InnerConeAngle: 0.2, OuterConeAngle: 0.5}} {
		bundle := ecs.NewBundle(2)
		ecs.Put(&bundle, scene.GlobalTransform{Matrix: mgl32.Translate3D(1, 2, 3)})
		switch light := value.(type) {
		case AmbientLight:
			ecs.Put(&bundle, light)
		case DirectionalLight:
			ecs.Put(&bundle, light)
		case PointLight:
			ecs.Put(&bundle, light)
		case SpotLight:
			ecs.Put(&bundle, light)
		}
		world.Spawn(bundle)
	}
	if err := scene.ExtractCamerasSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := ExtractSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state, _ := ecs.GetResource[*World](world)
	if len(state.lights) != 4 || state.lights[1].Kind != render.Light3DDirectional || state.lights[2].Kind != render.Light3DPoint || state.lights[3].Kind != render.Light3DSpot {
		t.Fatalf("extracted lights = %+v", state.lights)
	}
	if state.lights[3].Direction[2] >= 0 || state.lights[3].PositionDirection[3] != 1 {
		t.Fatalf("spot light transform data = %+v", state.lights[3])
	}
}

func TestExtractRender3DRemovesViewsForDespawnedCameras(t *testing.T) {
	ctx := graphics.Canvas2DContext(320, 180)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)
	world := ecs.NewWorld()
	Register(world)
	bundle := ecs.NewBundle(4)
	ecs.Put(&bundle, scene.NewCamera())
	ecs.Put(&bundle, Camera{})
	ecs.Put(&bundle, scene.PerspectiveProjection3D())
	ecs.Put(&bundle, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	camera := world.Spawn(bundle)

	if err := scene.ExtractCamerasSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := ExtractSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state, _ := ecs.GetResource[*World](world)
	if state.views[camera] == nil {
		t.Fatal("camera view was not created")
	}
	world.Despawn(camera)
	if err := scene.ExtractCamerasSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := ExtractSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.views[camera]; ok {
		t.Fatal("despawned camera retained its render view")
	}
}

type customMaterial3DTest struct {
	shader   *graphics.MeshShader
	strength float32
}

func (m customMaterial3DTest) Material3D() graphics.Material3DData {
	return graphics.Material3DData{
		Shader:              m.shader,
		BaseColor:           graphics.White,
		PerceptualRoughness: 0.5,
		IOR:                 1.5,
		EmissiveStrength:    1,
		AlphaCutoff:         0.5,
		Custom:              [4][4]float32{{m.strength}},
	}
}

func TestRegisteredCustomMaterialSelectsShaderAndUploadsParameters(t *testing.T) {
	driver := &meshTestDriver{viewport: render.Rect{Width: 320, Height: 180}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatalf("install driver: %v", err)
	}
	t.Cleanup(render.Uninstall)
	ctx := graphics.Canvas2DContext(320, 180)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)

	shader, err := graphics.NewMeshShader(render.ShaderDescriptor{
		WebGL2VertexSource:   "vertex",
		WebGL2FragmentSource: "fragment",
	})
	if err != nil {
		t.Fatalf("create mesh shader: %v", err)
	}
	world := ecs.NewWorld()
	Register(world)
	RegisterMaterial[customMaterial3DTest](world)
	camera := ecs.NewBundle(4)
	ecs.Put(&camera, scene.NewCamera())
	ecs.Put(&camera, Camera{})
	ecs.Put(&camera, scene.PerspectiveProjection3D())
	ecs.Put(&camera, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	world.Spawn(camera)
	material := customMaterial3DTest{shader: shader, strength: 0.75}
	mesh := ecs.NewBundle(3)
	ecs.Put(&mesh, Mesh{Mesh: graphics.CubeMesh(1)})
	ecs.Put(&mesh, Material[customMaterial3DTest]{Material: &material})
	ecs.Put(&mesh, scene.GlobalTransform{Matrix: mgl32.Translate3D(0, 0, -4)})
	world.Spawn(mesh)

	for _, system := range []ecs.SystemConfig{
		scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""),
		SortSystem(""), DrawSystem(""),
	} {
		if err := system.Run(world); err != nil {
			t.Fatalf("run %s: %v", system.Name, err)
		}
	}
	if err := ctx.FinishFrame(); err != nil {
		t.Fatalf("finish frame: %v", err)
	}
	if len(driver.batches) != 1 || len(driver.batches[0].Draws) != 1 {
		t.Fatalf("unexpected custom material batches: %+v", driver.batches)
	}
	batch := driver.batches[0]
	if !batch.Draws[0].Shader.Valid() || batch.Draws[0].Shader != shader.RenderHandle() {
		t.Fatalf("custom shader handle was not submitted: %s", batch.Draws[0].Shader)
	}
	if got := batch.Instances[0].Custom[0][0]; got != material.strength {
		t.Fatalf("custom shader parameter = %f, want %f", got, material.strength)
	}
}

func spawnMesh3DForTest(world *ecs.World, mesh *graphics.Mesh, material *graphics.StandardMaterial, position mgl32.Vec3) ecs.Entity {
	bundle := ecs.NewBundle(3)
	ecs.Put(&bundle, Mesh{Mesh: mesh})
	ecs.Put(&bundle, Material[graphics.StandardMaterial]{Material: material})
	ecs.Put(&bundle, scene.GlobalTransform{Matrix: mgl32.Translate3D(position.X(), position.Y(), position.Z())})
	return world.Spawn(bundle)
}

func TestRender3DSubmitsDeformedMeshAsReusableDynamicGeometry(t *testing.T) {
	driver := &meshTestDriver{viewport: render.Rect{Width: 320, Height: 180}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	ctx := graphics.Canvas2DContext(320, 180)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)

	world := ecs.NewWorld()
	Register(world)
	camera := ecs.NewBundle(4)
	ecs.Put(&camera, scene.NewCamera())
	ecs.Put(&camera, Camera{})
	ecs.Put(&camera, scene.PerspectiveProjection3D())
	ecs.Put(&camera, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	world.Spawn(camera)
	mesh, err := graphics.NewMesh([]render.Vertex3D{
		{Position: [3]float32{-1, -1, 0}, Normal: [3]float32{0, 0, 1}},
		{Position: [3]float32{1, -1, 0}, Normal: [3]float32{0, 0, 1}},
		{Position: [3]float32{0, 1, 0}, Normal: [3]float32{0, 0, 1}},
	}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	mesh.MorphTargets = []graphics.MorphTarget{{PositionDelta: [][3]float32{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}}}}
	material := graphics.NewStandardMaterial(graphics.White)
	entity := ecs.NewBundle(4)
	ecs.Put(&entity, Mesh{Mesh: mesh})
	ecs.Put(&entity, Material[graphics.StandardMaterial]{Material: material})
	ecs.Put(&entity, Deformation{MorphWeights: []float32{0.5}})
	ecs.Put(&entity, scene.GlobalTransform{Matrix: mgl32.Translate3D(0, 0, -4)})
	world.Spawn(entity)

	for _, system := range []ecs.SystemConfig{scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""), SortSystem(""), DrawSystem("")} {
		if err := system.Run(world); err != nil {
			t.Fatalf("run %s: %v", system.Name, err)
		}
	}
	if err := ctx.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	if driver.meshUploads != 0 || len(driver.batches) != 1 || len(driver.batches[0].Vertices) != 3 || driver.batches[0].Vertices[0].Position[2] != 0.5 || driver.batches[0].Draws[0].Mesh.Valid() {
		t.Fatalf("deformed batch = %+v, mesh uploads=%d", driver.batches, driver.meshUploads)
	}
}

func TestRender3DSubmitsGPUDeformationBuffersWhenDriverSupportsThem(t *testing.T) {
	base := &meshTestDriver{viewport: render.Rect{Width: 320, Height: 180}}
	driver := &gpuMeshTestDriver{meshTestDriver: base}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	ctx := graphics.Canvas2DContext(320, 180)
	restore := graphics.SetCurrentContextForTesting(ctx)
	t.Cleanup(restore)

	world := ecs.NewWorld()
	Register(world)
	camera := ecs.NewBundle(4)
	ecs.Put(&camera, scene.NewCamera())
	ecs.Put(&camera, Camera{})
	ecs.Put(&camera, scene.PerspectiveProjection3D())
	ecs.Put(&camera, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	world.Spawn(camera)
	mesh, err := graphics.NewMesh([]render.Vertex3D{
		{Position: [3]float32{-1, -1, 0}, Normal: [3]float32{0, 0, 1}},
		{Position: [3]float32{1, -1, 0}, Normal: [3]float32{0, 0, 1}},
		{Position: [3]float32{0, 1, 0}, Normal: [3]float32{0, 0, 1}},
	}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	mesh.Skin = []graphics.SkinVertex{{Joints: [4]uint32{0}, Weights: [4]float32{1}, Active: true}, {Joints: [4]uint32{0}, Weights: [4]float32{1}, Active: true}, {Joints: [4]uint32{0}, Weights: [4]float32{1}, Active: true}}
	mesh.MorphTargets = []graphics.MorphTarget{{PositionDelta: [][3]float32{{-200, 0, 1}, {-200, 0, 1}, {-200, 0, 1}}}}
	material := graphics.NewStandardMaterial(graphics.White)
	entity := ecs.NewBundle(4)
	ecs.Put(&entity, Mesh{Mesh: mesh})
	ecs.Put(&entity, Material[graphics.StandardMaterial]{Material: material})
	ecs.Put(&entity, Deformation{MorphWeights: []float32{0.5}, JointMatrices: []mgl32.Mat4{mgl32.Ident4()}})
	ecs.Put(&entity, scene.GlobalTransform{Matrix: mgl32.Translate3D(100, 0, -4)})
	world.Spawn(entity)

	for _, system := range []ecs.SystemConfig{scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""), SortSystem(""), DrawSystem("")} {
		if err := system.Run(world); err != nil {
			t.Fatalf("run %s: %v", system.Name, err)
		}
	}
	if err := ctx.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	if len(base.batches) != 1 {
		t.Fatalf("GPU batches = %d", len(base.batches))
	}
	batch := base.batches[0]
	if len(batch.Vertices) != 0 || len(batch.Deformations) != 1 || batch.Draws[0].DeformationIndex != 1 || !batch.Deformations[0].Mesh.Valid() || len(driver.static.Skin) != 3 || len(driver.static.MorphTargets) != 3 {
		t.Fatalf("GPU deformation batch = %+v", batch)
	}
}

func TestUpdateMeshDeformationResolvesJointEntities(t *testing.T) {
	world := ecs.NewWorld()
	Register(world)
	joint := ecs.NewBundle(1)
	ecs.Put(&joint, scene.GlobalTransform{Matrix: mgl32.Translate3D(7, 0, 0)})
	jointEntity := world.Spawn(joint)
	deformation := ecs.NewBundle(2)
	ecs.Put(&deformation, Deformation{JointEntities: []ecs.Entity{jointEntity}, InverseBind: []mgl32.Mat4{mgl32.Ident4()}})
	ecs.Put(&deformation, scene.GlobalTransform{Matrix: mgl32.Translate3D(5, 0, 0)})
	deformationEntity := world.Spawn(deformation)
	if err := UpdateDeformationsSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	value, ok := ecs.Get[Deformation](world, deformationEntity)
	if !ok || len(value.JointMatrices) != 1 || value.JointMatrices[0].Col(3).Vec3() != (mgl32.Vec3{2, 0, 0}) {
		t.Fatalf("joint matrices = %+v", value)
	}
}

type meshTestDriver struct {
	viewport      render.Rect
	nextID        uint32
	nextMeshID    uint32
	nextShaderID  uint32
	meshUploads   int
	sprites       int
	shapes        int
	batches       []render.MeshBatch3D
	meshViewports []render.Rect
	targetBegins  int
	targetEnds    int
}

type gpuMeshTestDriver struct {
	*meshTestDriver
	staticUploads int
	static        render.MeshDeformationDescriptor
}

func (d *gpuMeshTestDriver) SetMeshDeformation(_ render.MeshHandle, descriptor render.MeshDeformationDescriptor) error {
	d.staticUploads++
	d.static = descriptor
	return nil
}

func (d *gpuMeshTestDriver) DrawMeshesDeformed(batch render.MeshBatch3D) error {
	d.batches = append(d.batches, cloneMeshBatch3D(batch))
	return nil
}

func (d *meshTestDriver) RendererInfo() render.RendererInfo { return render.RendererInfo{} }
func (d *meshTestDriver) BeginFrame(render.Color) error     { return nil }
func (d *meshTestDriver) SetViewport(value render.Rect)     { d.viewport = value }
func (d *meshTestDriver) Viewport() render.Rect             { return d.viewport }
func (d *meshTestDriver) SetBlend(render.BlendState) error  { return nil }
func (d *meshTestDriver) SetScissor(*render.Rect)           {}
func (d *meshTestDriver) CreateTexture(descriptor render.TextureDescriptor, pixels []byte) (render.TextureHandle, error) {
	if err := descriptor.Validate(pixels); err != nil {
		return render.TextureHandle{}, err
	}
	d.nextID++
	return render.NewHandle[render.TextureResource](d.nextID, 1), nil
}
func (d *meshTestDriver) UpdateTextures([]render.TextureUpdate) error { return nil }
func (d *meshTestDriver) DestroyTexture(render.TextureHandle) error   { return nil }
func (d *meshTestDriver) DrawSprites(batch render.SpriteBatch) error {
	d.sprites += len(batch.Instances)
	return nil
}
func (d *meshTestDriver) DrawShapes(batch render.ShapeBatch) error {
	d.shapes += len(batch.Instances)
	return nil
}
func (d *meshTestDriver) CreateMesh(vertices []render.Vertex3D, indices []uint32) (render.MeshHandle, error) {
	d.nextMeshID++
	d.meshUploads++
	return render.NewHandle[render.MeshResource](d.nextMeshID, 1), nil
}
func (d *meshTestDriver) DestroyMesh(render.MeshHandle) error { return nil }
func (d *meshTestDriver) DrawMeshes(batch render.MeshBatch3D) error {
	d.batches = append(d.batches, cloneMeshBatch3D(batch))
	d.meshViewports = append(d.meshViewports, d.viewport)
	return nil
}
func (d *meshTestDriver) CreateMeshShader(render.ShaderDescriptor) (render.MeshShaderHandle, error) {
	d.nextShaderID++
	return render.NewHandle[render.MeshShaderResource](d.nextShaderID, 1), nil
}
func (d *meshTestDriver) DestroyMeshShader(render.MeshShaderHandle) error { return nil }
func (d *meshTestDriver) Present() error                                  { return nil }
func (d *meshTestDriver) ResizeSurface(int, int) error                    { return nil }
func (d *meshTestDriver) SetVSync(bool)                                   {}
func (d *meshTestDriver) Destroy()                                        {}

func (d *meshTestDriver) CreateRenderTarget(render.RenderTargetDescriptor) (render.RenderTargetHandle, render.TextureHandle, error) {
	d.nextID++
	target := render.NewHandle[render.RenderTargetResource](d.nextID, 1)
	d.nextID++
	return target, render.NewHandle[render.TextureResource](d.nextID, 1), nil
}

func (d *meshTestDriver) ResizeRenderTarget(render.RenderTargetHandle, render.RenderTargetDescriptor) (render.TextureHandle, error) {
	d.nextID++
	return render.NewHandle[render.TextureResource](d.nextID, 1), nil
}

func (*meshTestDriver) DestroyRenderTarget(render.RenderTargetHandle) error { return nil }

func (d *meshTestDriver) BeginRenderTarget(render.RenderTargetHandle, render.RenderTargetClear) error {
	d.targetBegins++
	return nil
}

func (d *meshTestDriver) EndRenderTarget() error {
	d.targetEnds++
	return nil
}

func cloneMeshBatch3D(batch render.MeshBatch3D) render.MeshBatch3D {
	batch.Vertices = append([]render.Vertex3D(nil), batch.Vertices...)
	batch.Indices = append([]uint32(nil), batch.Indices...)
	batch.Instances = append([]render.Instance3D(nil), batch.Instances...)
	batch.Draws = append([]render.MeshDraw3D(nil), batch.Draws...)
	batch.Deformations = append([]render.MeshDeformationData(nil), batch.Deformations...)
	batch.Lights = append([]render.Light3D(nil), batch.Lights...)
	for index := range batch.Deformations {
		batch.Deformations[index].JointMatrices = append([][16]float32(nil), batch.Deformations[index].JointMatrices...)
		batch.Deformations[index].MorphWeights = append([]float32(nil), batch.Deformations[index].MorphWeights...)
	}
	return batch
}
