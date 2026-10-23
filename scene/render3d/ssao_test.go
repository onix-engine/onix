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

type ssaoTestDriver struct {
	*meshTestDriver
	destroyed int
}

func (d *ssaoTestDriver) DestroyRenderTarget(render.RenderTargetHandle) error {
	d.destroyed++
	return nil
}

func TestSSAOViewRenderingReuseAndLifecycle(t *testing.T) {
	driver := &ssaoTestDriver{meshTestDriver: &meshTestDriver{viewport: render.Rect{Width: 320, Height: 240}}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	canvas := graphics.Canvas2DContext(320, 240)
	restore := graphics.SetCurrentContextForTesting(canvas)
	t.Cleanup(restore)
	world := ecs.NewWorld()
	Register(world)
	state := renderWorld3D(world)
	t.Cleanup(func() { _ = state.Dispose() })
	b := ecs.NewBundle(4)
	ecs.Put(&b, scene.NewCamera())
	ecs.Put(&b, Camera{SSAO: NewSSAOSettings()})
	ecs.Put(&b, scene.PerspectiveProjection3D())
	ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	entity := world.Spawn(b)
	mesh := graphics.CubeMesh(.5)
	t.Cleanup(func() { _ = mesh.Dispose() })
	for index := 0; index < 4; index++ {
		material := graphics.NewStandardMaterial(graphics.White)
		material.Surface.Unlit = index == 1
		if index == 2 {
			material.AlphaMode = graphics.AlphaBlend
		}
		b = ecs.NewBundle(4)
		ecs.Put(&b, Mesh{Mesh: mesh})
		ecs.Put(&b, Material[graphics.StandardMaterial]{Material: material})
		ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Translate3D(float32(index)*.1, 0, -3)})
		if index == 3 {
			ecs.Put(&b, NoAmbientOcclusion{})
		}
		world.Spawn(b)
	}
	for _, system := range []ecs.SystemConfig{scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""), DrawSystem("")} {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	view := state.views[entity]
	if driver.targetBegins != 1 || driver.targetEnds != 1 || len(driver.batches) != 2 {
		t.Fatalf("SSAO must add exactly one depth prepass: begin/end=%d/%d batches=%d", driver.targetBegins, driver.targetEnds, len(driver.batches))
	}
	depth, lit := driver.batches[0], driver.batches[1]
	if depth.Lighting.Params[1] != 2 || len(depth.Instances) != 1 || len(lit.Instances) != 4 || !lit.Lighting.OcclusionTexture.Valid() {
		t.Fatalf("depth pass exclusions or lighting texture failed: depth=%d lit=%d", len(depth.Instances), len(lit.Instances))
	}
	if viewport := driver.meshViewports[0]; viewport.Width != 160 || viewport.Height != 120 || viewport.X != 0 || viewport.Y != 0 {
		t.Fatal(viewport)
	}
	if err := canvas.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	target := view.occlusion.frame.Target
	frame := func() {
		ctx := &ecs.SystemContext{World: world}
		if err := extractRender3D(ctx); err != nil {
			panic(err)
		}
		if err := prepareRenderResources3D(ctx); err != nil {
			panic(err)
		}
		pass := view.occlusion
		pass.instances, pass.draws = pass.instances[:0], pass.draws[:0]
		pass.appendPhase(view.opaque.items, state.white, false)
	}
	for range 5 {
		frame()
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("SSAO extraction/preparation allocated %g times", allocations)
	}
	if target != view.occlusion.frame.Target {
		t.Fatal("SSAO recreated its target")
	}
	view.frame.Viewport.Width = 640
	if err := setupViewLighting3D(world, state, view, entity); err != nil {
		t.Fatal(err)
	}
	if descriptor := target.Descriptor(); target != view.occlusion.frame.Target || descriptor.Width != 320 || descriptor.Height != 120 {
		t.Fatal("viewport resize did not update the cached SSAO target", descriptor)
	}
	// A temporary picking pass preserves the cache and clears its bindings.
	view.frame.Mode = render.RenderViewPicking
	if err := setupViewLighting3D(world, state, view, entity); err != nil {
		t.Fatal(err)
	}
	if view.occlusionActive || view.occlusion.frame.Target != target || view.lighting.OcclusionTexture.Valid() || driver.destroyed != 0 {
		t.Fatal("picking destroyed or consumed the SSAO cache")
	}
	settings := ecs.GetMut[Camera](world, entity)
	settings.SSAO.Enabled = false
	view.frame.Mode = render.RenderViewLit
	if err := setupViewLighting3D(world, state, view, entity); err != nil {
		t.Fatal(err)
	}
	if view.occlusion != nil || driver.destroyed != 1 {
		t.Fatal("disabling SSAO did not release the depth target")
	}
	settings.SSAO = NewSSAOSettings()
	frame()
	if _, err := view.occlusion.frame.Target.ColorTexture(); err != nil {
		t.Fatal(err)
	}
	world.Despawn(entity)
	if err := scene.ExtractCamerasSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := extractRender3D(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	if state.views[entity] != nil || driver.destroyed != 2 {
		t.Fatal("removing the camera retained its SSAO target")
	}
}

func TestSSAOSettingsRejectInvalidParameters(t *testing.T) {
	for _, change := range []func(*SSAOSettings){
		func(s *SSAOSettings) { s.Radius = 0 },
		func(s *SSAOSettings) { s.Bias = s.Radius },
		func(s *SSAOSettings) { s.Strength = -1 },
		func(s *SSAOSettings) { s.ResolutionScale = .1 },
		func(s *SSAOSettings) { s.Samples = 12 },
		func(s *SSAOSettings) { s.Radius = float32(math.NaN()) },
		func(s *SSAOSettings) { s.Bias = float32(math.Inf(1)) },
	} {
		s := NewSSAOSettings()
		change(&s)
		if s.Validate() == nil {
			t.Fatal("invalid SSAO settings accepted", s)
		}
	}
	if err := (SSAOSettings{}).Validate(); err != nil {
		t.Fatal(err)
	}
}
