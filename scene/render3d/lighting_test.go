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

func TestHDREnvironmentEncodingAndZeroAllocations(t *testing.T) {
	world := ecs.NewWorld()
	Register(world)
	state := NewWorld()
	t.Cleanup(func() { _ = state.Dispose() })
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, Environment{Atlas: &graphics.Texture{Width: 128, Height: 512}, Encoding: render.EnvironmentRGBE, Intensity: .5, Exposure: 2, Background: true})
	entity := world.Spawn(bundle)
	view := state.viewFor(entity)
	view.frame.Mode = render.RenderViewLit
	prepare := func() {
		if err := setupViewLighting3D(world, state, view, entity); err != nil {
			panic(err)
		}
	}
	prepare()
	if view.lighting.EnvironmentEncoding != render.EnvironmentRGBE || view.lighting.Environment[0] != .5 || view.lighting.Params[0] != 2 {
		t.Fatalf("HDR encoding must not alter user intensity/exposure: %+v", view.lighting)
	}
	if allocations := testing.AllocsPerRun(100, prepare); allocations != 0 {
		t.Fatalf("HDR lighting setup allocated %g times", allocations)
	}
	if err := ecs.Insert(world, entity, Environment{Exposure: 1}); err != nil {
		t.Fatal(err)
	}
	prepare()
	if view.lighting.EnvironmentEncoding != render.EnvironmentRGBM || view.lighting.Environment != ([4]float32{}) {
		t.Fatal("removed HDR atlas retained encoding")
	}
}

func TestEnvironmentPanoramaSkyBindingAndZeroAllocations(t *testing.T) {
	driver := &meshTestDriver{}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(64, 32))
	t.Cleanup(restore)
	state := NewWorld()
	t.Cleanup(func() { _ = state.Dispose() })
	state.white = graphics.CreateTextureByColor(graphics.White)
	panorama := graphics.CreateTextureByColor(graphics.Black)
	t.Cleanup(func() { _ = panorama.Dispose() })
	world := ecs.NewWorld()
	Register(world)
	bundle := ecs.NewBundle(1)
	environment := Environment{Panorama: panorama, Rotation: .75, Background: true}
	ecs.Put(&bundle, environment)
	entity := world.Spawn(bundle)
	view := state.viewFor(entity)
	view.frame.Mode = render.RenderViewLit
	frame := func() {
		view.instances = view.instances[:0]
		view.draws = view.draws[:0]
		if err := setupViewLighting3D(world, state, view, entity); err != nil {
			panic(err)
		}
		if err := appendSky3D(state, view); err != nil {
			panic(err)
		}
	}
	frame()
	if view.draws[0].Texture != panorama.RenderHandle() || uint32(view.instances[0].Material[3])&8388608 == 0 {
		t.Fatal("sky did not bind the original panorama independently of the lighting atlas")
	}
	if view.lighting.Environment[1] != environment.Rotation || view.lighting.Environment[0] != 0 {
		t.Fatal("standalone panorama lost its rotation or enabled IBL without an atlas")
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("panorama sky setup allocated %g times", allocations)
	}
	sky := graphics.SkyRadiance{Zenith: mgl32.Vec3{0, 0, 1}}
	environment.Sky = &sky
	if err := ecs.Insert(world, entity, environment); err != nil {
		t.Fatal(err)
	}
	frame()
	flags := uint32(view.instances[0].Material[3])
	if view.draws[0].Texture != state.white.RenderHandle() || flags&8388608 != 0 || flags&16384 == 0 {
		t.Fatal("analytic sky did not override the panorama")
	}
}

func TestDirectionalShadowProjectionAndStability(t *testing.T) {
	s := NewShadowSettings()
	direction := mgl32.Vec3{0, 0, -1}
	m := directionalShadowMatrix(mgl32.Vec3{}, direction, s)
	for _, point := range []struct{ z, depth float32 }{{s.Depth * .5, 1}, {0, .5}, {-s.Depth * .5, 0}} {
		p := m.Mul4x1(mgl32.Vec4{0, 0, point.z, 1})
		if math.Abs(float64(p.Z()-point.depth)) > .00001 {
			t.Fatalf("depth %v != %v", p.Z(), point.depth)
		}
	}
	next := directionalShadowMatrix(mgl32.Vec3{s.Radius / float32(s.Resolution) * .1, 0, 0}, direction, s)
	if m != next {
		t.Fatal("sub-texel movement changed the shadow projection")
	}
	if allocations := testing.AllocsPerRun(100, func() { _ = directionalShadowMatrix(mgl32.Vec3{}, direction, s) }); allocations != 0 {
		t.Fatalf("shadow matrix allocated %g times", allocations)
	}
}

func TestShadowCastersOutsideCameraFrustumAndNoShadowExclusion(t *testing.T) {
	driver := &meshTestDriver{viewport: render.Rect{Width: 800, Height: 600}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	context := graphics.Canvas2DContext(800, 600)
	restore := graphics.SetCurrentContextForTesting(context)
	t.Cleanup(restore)
	world := ecs.NewWorld()
	Register(world)
	camera := ecs.NewBundle(4)
	ecs.Put(&camera, scene.NewCamera())
	ecs.Put(&camera, Camera{})
	ecs.Put(&camera, scene.PerspectiveProjection3D())
	ecs.Put(&camera, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	cameraEntity := world.Spawn(camera)
	sun := ecs.NewBundle(2)
	light := NewDirectionalLight()
	light.Shadows = NewShadowSettings()
	ecs.Put(&sun, light)
	ecs.Put(&sun, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	sunEntity := world.Spawn(sun)
	mesh := graphics.CubeMesh(1)
	t.Cleanup(func() { _ = mesh.Dispose() })
	material := graphics.NewStandardMaterial(graphics.White)
	for i, z := range []float32{-4, 4, -3} {
		b := ecs.NewBundle(4)
		ecs.Put(&b, Mesh{Mesh: mesh})
		ecs.Put(&b, Material[graphics.StandardMaterial]{Material: material})
		ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Translate3D(0, 0, z)})
		if i == 2 {
			ecs.Put(&b, NoShadow{})
		}
		world.Spawn(b)
	}
	for _, system := range []ecs.SystemConfig{scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""), SortSystem(""), DrawSystem("")} {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	view := renderWorld3D(world).views[cameraEntity]
	shadowTarget := view.shadow.target
	t.Cleanup(func() { _ = shadowTarget.Dispose() })
	if len(view.extracted) != 2 || len(view.shadow.views[0].extracted) != 2 {
		t.Fatalf("visible=%d casters=%d; off-camera cube must cast, first-person cube must not", len(view.extracted), len(view.shadow.views[0].extracted))
	}
	if driver.targetBegins != 1 || driver.targetEnds != 1 || len(driver.batches) != 2 {
		t.Fatalf("shadow/main pass count: targets=%d batches=%d", driver.targetBegins, len(driver.batches))
	}
	if !driver.batches[1].Lighting.ShadowTexture.Valid() || driver.batches[0].Lighting.Params[1] != 1 {
		t.Fatal("shadow target was not passed to the main draw")
	}
	if err := context.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	frame := func() {
		ctx := &ecs.SystemContext{World: world}
		if err := extractRender3D(ctx); err != nil {
			panic(err)
		}
		if err := prepareRenderResources3D(ctx); err != nil {
			panic(err)
		}
		for _, v := range [2]*renderView3D{view, view.shadow.views[0]} {
			v.instances = v.instances[:0]
			v.draws = v.draws[:0]
			v.appendPhase(v.extracted, renderWorld3D(world).white, false)
		}
	}
	for range 5 {
		frame()
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("shadow extraction/preparation allocated %g times per frame", allocations)
	}
	// Turning off shadows releases the view's target on the next extraction.
	light.Shadows.Enabled = false
	if err := ecs.Insert(world, sunEntity, light); err != nil {
		t.Fatal(err)
	}
	if err := extractRender3D(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	if view.shadow != nil {
		t.Fatal("disabled shadow view retained its target")
	}
}

func TestViewLightingKeepsShadowsAcrossPickingAndIsolatesOverrides(t *testing.T) {
	world := ecs.NewWorld()
	Register(world)
	camera := world.Spawn(ecs.NewBundle(0))
	state := NewWorld()
	t.Cleanup(func() { _ = state.Dispose() })
	state.ambient = [4]float32{0.1, 0.2, 0.3, 1}
	state.lights = []extractedLight3D{{Light3D: render.Light3D{Kind: render.Light3DDirectional, Direction: [4]float32{0, -1, 0}, Color: [4]float32{1, 1, 1}}, shadows: extractedShadowLight3D{directional: NewShadowSettings()}}}
	view := state.viewFor(camera)
	view.frame.Mode = render.RenderViewLit
	if err := setupViewLighting3D(world, state, view, camera); err != nil {
		t.Fatal(err)
	}
	shadow := view.shadow
	if shadow == nil || !view.shadowActive {
		t.Fatal("shadow cache was not created")
	}
	frame := func() {
		view.frame.Mode = render.RenderViewPicking
		if err := setupViewLighting3D(world, state, view, camera); err != nil {
			panic(err)
		}
		if view.shadowActive || view.shadow != shadow {
			panic("picking invalidated shadow cache")
		}
		view.frame.Mode = render.RenderViewLit
		if err := setupViewLighting3D(world, state, view, camera); err != nil {
			panic(err)
		}
		if !view.shadowActive || view.shadow != shadow {
			panic("lit pass did not reuse shadow cache")
		}
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("lit/picking switch allocated %g times", allocations)
	}
	config := ViewLighting{Override: true, Ambient: AmbientLight{Color: graphics.White, Brightness: 0.5}, Sun: NewDirectionalLight(), Direction: mgl32.Vec3{1, -1, 0}}
	if err := ecs.Insert(world, camera, config); err != nil {
		t.Fatal(err)
	}
	if err := setupViewLighting3D(world, state, view, camera); err != nil {
		t.Fatal(err)
	}
	if view.ambient[0] != 0.5 || state.ambient[0] != 0.1 || len(view.lights) != 2 {
		t.Fatal("camera override changed global scene lights")
	}
	if view.shadow != nil {
		t.Fatal("disabled light retained its shadow cache")
	}
}

func TestViewGroundAndGradientDoNotEnterPicking(t *testing.T) {
	world := ecs.NewWorld()
	Register(world)
	bundle := ecs.NewBundle(2)
	sky := graphics.SkyRadiance{Zenith: mgl32.Vec3{0.1, 0.2, 0.3}}
	ecs.Put(&bundle, Environment{Sky: &sky, Background: true})
	ecs.Put(&bundle, GroundPlane{Size: 10, Color: graphics.White})
	camera := world.Spawn(bundle)
	state := NewWorld()
	view := state.viewFor(camera)
	view.frame.Mode = render.RenderViewLit
	if err := setupViewLighting3D(world, state, view, camera); err != nil {
		t.Fatal(err)
	}
	if view.environment.Sky != &sky || view.ground.Size != 10 {
		t.Fatal("lit view lost its background or ground")
	}
	view.frame.Mode = render.RenderViewPicking
	if err := setupViewLighting3D(world, state, view, camera); err != nil {
		t.Fatal(err)
	}
	if view.environment.Sky != nil || view.ground.Size != 0 {
		t.Fatal("picking includes editor ground or sky")
	}
}
