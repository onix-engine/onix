package render3d

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestFXAACompleteCameraPhasesReuseAndLifecycle(t *testing.T) {
	driver := &ssrTestDriver{record: true, ssaoTestDriver: &ssaoTestDriver{meshTestDriver: &meshTestDriver{viewport: render.Rect{Width: 320, Height: 240}}}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.Canvas2DContext(320, 240)
	restore := graphics.SetCurrentContextForTesting(canvas)
	t.Cleanup(restore)
	world := ecs.NewWorld()
	graphics.BindFrame(world, graphics.CurrentContext())
	Register(world)
	state := renderWorld3D(world)
	t.Cleanup(func() { _ = state.Dispose() })
	target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: 320, Height: 240, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Dispose() })
	b := ecs.NewBundle(4)
	camera := scene.NewCamera()
	camera.Viewport = scene.CameraViewport{X: 24, Y: 16, Width: 160, Height: 120}
	ecs.Put(&b, camera)
	ecs.Put(&b, Camera{FXAA: true, SSAO: NewSSAOSettings(), SSR: NewSSRSettings(), Fog: NewFogSettings(), ToneMapping: graphics.ToneMappingACES, Target: target})
	ecs.Put(&b, scene.PerspectiveProjection3D())
	ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	entity := world.Spawn(b)
	mesh := graphics.CubeMesh(.5)
	t.Cleanup(func() { _ = mesh.Dispose() })
	shader, err := graphics.NewMeshShader(render.ShaderDescriptor{WebGL2FragmentSource: "void main(){}"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shader.Dispose() })
	for index := range 3 {
		material := graphics.NewStandardMaterial(graphics.White)
		if index == 1 {
			material.AlphaMode = graphics.AlphaBlend
		}
		if index == 2 {
			material.Shader = shader
		}
		b = ecs.NewBundle(3)
		ecs.Put(&b, Mesh{Mesh: mesh})
		ecs.Put(&b, Material[graphics.StandardMaterial]{Material: material})
		ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Translate3D(0, 0, -3)})
		world.Spawn(b)
	}
	ctx := &ecs.SystemContext{World: world}
	frame := func() {
		for _, run := range [...]func(*ecs.SystemContext) error{scene.ExtractCameras, extractRender3D, prepareRenderResources3D, sortRenderPhases3D, render3D} {
			if err := run(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
	}
	reset := func() { driver.batches, driver.clears, driver.meshViewports = nil, nil, nil }
	frame()
	view := state.views[entity]
	cache := view.fxaa
	if len(driver.batches) != 7 || driver.batches[6].Lighting.Params[1] != render.MeshPassFXAA {
		t.Fatal("FXAA did not follow SSAO/SSR and the normal phase", len(driver.batches))
	}
	normal, final := driver.batches[5], driver.batches[6]
	if normal.Lighting.FogParams[3] != 1 || normal.Lighting.ToneMapping != 1 || driver.batches[4].Lighting.FogColor != normal.Lighting.FogColor || driver.batches[4].Lighting.ToneMapping != 1 || final.Lighting.FogParams[3] != 0 || final.Lighting.ToneMapping != 0 {
		t.Fatal("camera fog/curve did not reach SSR resolve or leaked into FXAA")
	}
	if len(normal.Instances) != 2 || normal.Draws[0].Shader != shader.RenderHandle() || normal.Draws[1].AlphaMode != uint32(graphics.AlphaBlend) {
		t.Fatal("source omitted custom or transparent materials")
	}
	if normal.ViewProjection != [16]float32(view.frame.Camera.screenProjection) || driver.meshViewports[5] != (render.Rect{Width: 160, Height: 120}) || driver.meshViewports[6] != view.frame.Viewport {
		t.Fatal("source/final viewport placement was applied incorrectly")
	}
	if final.ViewProjection != [16]float32(mgl32.Ident4()) || len(final.Instances) != 1 || final.Draws[0].AlphaMode != uint32(graphics.AlphaBlend) || !final.Draws[0].Texture.Valid() || final.Draws[0].Texture == normal.Draws[0].Texture {
		t.Fatal("FXAA composite did not sample the camera image")
	}
	if driver.clears[5].ClearColor || driver.clears[6].ClearColor || !driver.clears[6].ClearDepth {
		t.Fatal("final composite erased color or did not release scene depth")
	}
	// Transparent overlay cameras always clear their private source to zero.
	cameraSettings := ecs.GetMut[scene.Camera](world, entity)
	cameraSettings.Clear = false
	reset()
	frame()
	if driver.clears[4].Color != (render.Color{}) || !driver.clears[4].ClearColor {
		t.Fatal("overlay camera retained old color in its private source")
	}
	driver.record = false
	for range 5 {
		frame()
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("complete FXAA/SSAO/SSR frame allocated %g times", allocations)
	}
	settings := ecs.GetMut[Camera](world, entity)
	settings.Target = nil
	for range 5 {
		frame()
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("FXAA swapchain frame allocated %g times", allocations)
	}
	if view.fxaa != cache {
		t.Fatal("stable frames recreated the camera target")
	}
	driver.record = true
	reset()
	frame()
	last := driver.batches[len(driver.batches)-1]
	if last.ViewProjection != [16]float32(view.frame.Camera.viewport.PlacementMatrix(320, 240)) {
		t.Fatal("swapchain composite lost its viewport placement")
	}
	if view.lighting.OcclusionInverseProjection != [16]float32(view.frame.Camera.screenProjection.Inv()) {
		t.Fatal("FXAA source SSAO retained the swapchain viewport transform")
	}
	settings.Target = target
	cameraSettings.Viewport.Width = 200
	reset()
	frame()
	if cache.target.Descriptor().Width != 200 || view.frame.Target != target || view.frame.Viewport.X != 24 {
		t.Fatal("resize or source frame restoration failed")
	}
	// A second camera has its own source and effects, even with a shared output.
	b = ecs.NewBundle(4)
	camera = scene.NewCamera()
	camera.Order, camera.Clear = 1, false
	camera.Viewport = scene.CameraViewport{X: 220, Y: 16, Width: 100, Height: 120}
	ecs.Put(&b, camera)
	ecs.Put(&b, Camera{FXAA: true, Target: target})
	ecs.Put(&b, scene.PerspectiveProjection3D())
	ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	second := world.Spawn(b)
	reset()
	frame()
	other := state.views[second]
	if other.fxaa == cache || other.fxaa.target == cache.target || len(driver.batches) != 9 || driver.batches[8].Lighting.Params[1] != render.MeshPassFXAA || driver.meshViewports[8].X != 220 {
		t.Fatal("camera sources or composites were not isolated")
	}
	world.Despawn(second)
	frame()
	settings = ecs.GetMut[Camera](world, entity)
	before := driver.destroyed
	view.frame.Mode = render.RenderViewPicking
	if err := setupViewLighting3D(world, state, view, entity); err != nil {
		t.Fatal(err)
	}
	if view.fxaaActive || view.fxaa != cache || driver.destroyed != before {
		t.Fatal("picking consumed or disposed the FXAA source")
	}
	view.frame.Mode = render.RenderViewLit
	settings.FXAA = false
	frame()
	if view.fxaa != nil || driver.destroyed != before+1 {
		t.Fatal("disabling FXAA did not release its target")
	}
	settings.FXAA = true
	frame()
	before = driver.destroyed
	world.Despawn(entity)
	frame()
	if state.views[entity] != nil || driver.destroyed != before+4 {
		t.Fatal("removed camera retained its four screen targets", driver.destroyed-before)
	}
}
