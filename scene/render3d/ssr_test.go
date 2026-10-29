package render3d

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

type ssrTestDriver struct {
	*ssaoTestDriver
	clears []render.RenderTargetClear
	record bool
}

func (d *ssrTestDriver) BeginRenderTarget(h render.RenderTargetHandle, c render.RenderTargetClear) error {
	if d.record {
		d.clears = append(d.clears, c)
	}
	return d.meshTestDriver.BeginRenderTarget(h, c)
}

func (d *ssrTestDriver) DrawMeshes(batch render.MeshBatch3D) error {
	if d.record {
		return d.meshTestDriver.DrawMeshes(batch)
	}
	return nil
}

func TestSSRRenderingPhasesReuseAndLifecycle(t *testing.T) {
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
	state := WorldFor(world)
	t.Cleanup(func() { _ = state.Dispose() })
	target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: 320, Height: 240, Depth: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Dispose() })
	b := ecs.NewBundle(4)
	camera := scene.NewCamera()
	camera.Viewport = scene.CameraViewport{X: 24, Y: 16, Width: 160, Height: 120}
	b.Put(camera)
	b.Put(Camera{SSR: NewSSRSettings(), Target: target})
	b.Put(scene.PerspectiveProjection3D())
	b.Put(scene.GlobalTransform{Matrix: mgl32.Ident4()})
	entity := world.Spawn(b)
	mesh := graphics.CubeMesh(.5)
	t.Cleanup(func() { _ = mesh.Dispose() })
	shader, err := graphics.NewMeshShader(render.ShaderDescriptor{WebGL2FragmentSource: "void main(){}"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shader.Dispose() })
	for index := 0; index < 5; index++ {
		material := graphics.NewStandardMaterial(graphics.White)
		material.Surface.Unlit = index == 1
		if index == 2 {
			material.AlphaMode = graphics.AlphaBlend
		}
		if index == 4 {
			material.Shader = shader
		}
		b = ecs.NewBundle(5)
		b.Put(Mesh{Mesh: mesh})
		b.Put(Material[graphics.StandardMaterial]{Material: material})
		b.Put(scene.GlobalTransform{Matrix: mgl32.Translate3D(float32(index)*.05, 0, -3)})
		if index == 3 {
			b.Put(NoAmbientOcclusion{})
			b.Put(NoScreenReflection{})
		}
		world.Spawn(b)
	}
	ctx := &ecs.SystemContext{World: world}
	frame := func() {
		for _, run := range []func(*ecs.SystemContext) error{scene.ExtractCameras, extractRender3D, prepareRenderResources3D, sortRenderPhases3D, render3D} {
			if err := run(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if err := canvas.FinishFrame(); err != nil {
			t.Fatal(err)
		}
	}
	frame()
	view := state.views[entity]
	s := view.reflection
	if len(driver.batches) != 4 || driver.targetBegins != 4 || driver.targetEnds != 4 {
		t.Fatalf("SSR phases = %d; targets %d/%d", len(driver.batches), driver.targetBegins, driver.targetEnds)
	}
	for i, expected := range []struct{ pass, instances int }{{render.MeshPassReflectionDepth, 4}, {render.MeshPassReflectionCapture, 4}, {render.MeshPassReflectionResolve, 3}, {render.MeshPassLit, 2}} {
		batch := driver.batches[i]
		if batch.Lighting.Params[1] != float32(expected.pass) || len(batch.Instances) != expected.instances {
			t.Fatalf("phase %d: pass=%g instances=%d", i, batch.Lighting.Params[1], len(batch.Instances))
		}
	}
	if !driver.clears[2].ClearColor || driver.clears[3].ClearColor || driver.clears[3].ClearDepth {
		t.Fatal("custom/transparent phase erased SSR output or its depth")
	}
	if driver.meshViewports[0] != (render.Rect{Width: 160, Height: 120}) || driver.meshViewports[2] != view.frame.Viewport || driver.viewport != (render.Rect{Width: 320, Height: 240}) {
		t.Fatal("SSR viewport offset or restoration failed", driver.meshViewports)
	}
	if driver.batches[0].ViewProjection != [16]float32(view.frame.Camera.screenProjection) || driver.batches[2].Lighting.Shadows[0].Matrix != driver.batches[0].ViewProjection {
		t.Fatal("SSR capture and sampling did not use the local camera projection")
	}
	clip := view.frame.Camera.viewProjection.Mul4x1(mgl32.Vec4{0, 0, -1, 1})
	if math.Abs(float64(clip.X()/clip.W())) > .00001 || math.Abs(float64(clip.Y()/clip.W())) > .00001 {
		t.Fatal("offscreen projection applied the viewport offset twice")
	}
	resolve, normal := driver.batches[2].Lighting, driver.batches[3].Lighting
	if !resolve.ShadowTexture.Valid() || !resolve.OcclusionTexture.Valid() || resolve.OcclusionTexture == normal.OcclusionTexture || normal.Params[1] != render.MeshPassLit {
		t.Fatal("SSR bindings leaked into the normal transparent phase")
	}
	invalid, excluded := false, false
	for _, instance := range driver.batches[1].Instances {
		invalid = invalid || instance.Material[1] < 0
		excluded = excluded || uint32(instance.Material[3])&render.MeshFlagNoReflection != 0
	}
	if !invalid || !excluded || driver.batches[3].Draws[0].Shader != shader.RenderHandle() {
		t.Fatal("custom shader isolation or receiver exclusion failed")
	}
	pack := func() {
		if err := extractRender3D(ctx); err != nil {
			panic(err)
		}
		if err := prepareRenderResources3D(ctx); err != nil {
			panic(err)
		}
		for _, pass := range [...]*renderView3D{&s.depth, &s.radiance, &s.resolve} {
			pass.instances, pass.draws = pass.instances[:0], pass.draws[:0]
			pass.appendPhase(view.opaque.items, state.white, false)
		}
	}
	for range 5 {
		pack()
	}
	if allocations := testing.AllocsPerRun(100, pack); allocations != 0 {
		t.Fatalf("SSR extraction/preparation/packing allocated %g times", allocations)
	}
	if view.reflection != s {
		t.Fatal("SSR cache was recreated")
	}
	settings := world.GetMut[Camera](entity)
	settings.SSAO = NewSSAOSettings()
	driver.batches, driver.clears, driver.meshViewports = nil, nil, nil
	frame()
	if s.radiance.frame.Target.Descriptor().Width != 320 || len(driver.batches) != 6 || driver.batches[3].Lighting.Params[1] != render.MeshPassReflectionOcclusion || driver.clears[3].ClearColor || !driver.clears[3].ClearDepth || driver.meshViewports[3].X != 160 || driver.batches[4].Lighting.Shadows[0].Rect[2] != 1 {
		t.Fatal("SSR + SSAO did not preserve the RGBE/AO tiles")
	}
	driver.record = false
	for range 5 {
		frame()
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("complete SSR + SSAO frame allocated %g times", allocations)
	}
	settings.Target = nil
	for range 5 {
		frame()
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("SSR + SSAO swapchain frame allocated %g times", allocations)
	}
	if view.frame.Camera.viewProjection == view.reflection.projection {
		t.Fatal("swapchain viewport transform was lost")
	}
	settings.Target = target
	frame()
	driver.record = true
	view.frame.Viewport.Width = 200
	if err := setupViewLighting3D(world, state, view, entity); err != nil {
		t.Fatal(err)
	}
	if s.depth.frame.Target.Descriptor().Width != 200 || s.radiance.frame.Target.Descriptor().Width != 400 {
		t.Fatal("SSR targets did not resize")
	}
	view.frame.Mode = render.RenderViewPicking
	if err := setupViewLighting3D(world, state, view, entity); err != nil {
		t.Fatal(err)
	}
	if view.reflectionActive || view.reflection != s || driver.destroyed != 0 {
		t.Fatal("picking destroyed or consumed the SSR cache")
	}
	view.frame.Mode = render.RenderViewLit
	settings.SSR.Enabled = false
	if err := setupViewLighting3D(world, state, view, entity); err != nil {
		t.Fatal(err)
	}
	if view.reflection != nil || driver.destroyed != 2 {
		t.Fatal("disabling SSR did not release both targets")
	}
	settings.SSR = NewSSRSettings()
	frame()
	world.Despawn(entity)
	if err := scene.ExtractCameras(ctx); err != nil {
		t.Fatal(err)
	}
	if err := extractRender3D(ctx); err != nil {
		t.Fatal(err)
	}
	if state.views[entity] != nil || driver.destroyed != 5 {
		t.Fatal("removing the camera retained screen targets", driver.destroyed)
	}
}

func TestSSRSettingsAndCameraProjectionValidation(t *testing.T) {
	for _, change := range []func(*SSRSettings){
		func(s *SSRSettings) { s.MaxDistance = 0 },
		func(s *SSRSettings) { s.Thickness = s.MaxDistance },
		func(s *SSRSettings) { s.Strength = -1 },
		func(s *SSRSettings) { s.MaxRoughness = 0 },
		func(s *SSRSettings) { s.EdgeFade = 0 },
		func(s *SSRSettings) { s.Steps = 48 },
		func(s *SSRSettings) { s.MaxDistance = float32(math.NaN()) },
		func(s *SSRSettings) { s.Thickness = float32(math.Inf(1)) },
	} {
		s := NewSSRSettings()
		change(&s)
		if s.Validate() == nil {
			t.Fatal("invalid SSR settings accepted", s)
		}
	}
	if err := (SSRSettings{}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := setupViewSSR3D(&renderView3D{}, NewSSRSettings()); err == nil {
		t.Fatal("SSR accepted a singular projection")
	}
	// Use the engine's reversed-Z infinite projection for the forward test.
	projection := mgl32.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, -1, 0, 0, .1, 0}.Mul4(mgl32.HomogRotate3DY(.5))
	want := mgl32.HomogRotate3DY(-.5).Mul4x1(mgl32.Vec4{0, 0, -1, 0}).Vec3()
	if cameraForwardFromInverse3D(projection.Inv()).Sub(want).Len() > .00001 {
		t.Fatal("SSR camera forward ignored rotation")
	}
}
