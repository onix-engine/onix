package render3d

import (
	"image"
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestMaterialLayersPreparePackAndReuseInstances(t *testing.T) {
	driver := &meshTestDriver{viewport: render.Rect{Width: 320, Height: 180}}
	render.Uninstall()
	if err := render.Install(driver); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(render.Uninstall)
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 180))
	t.Cleanup(restore)
	world := ecs.NewWorld()
	graphics.BindFrame(world, graphics.CurrentContext())
	Register(world)
	camera := ecs.NewBundle(4)
	camera.Put(scene.NewCamera())
	camera.Put(Camera{})
	camera.Put(scene.PerspectiveProjection3D())
	camera.Put(scene.GlobalTransform{Matrix: mgl32.Ident4()})
	entity := world.Spawn(camera)
	material := graphics.NewStandardMaterial(graphics.White)
	material.IOR, material.EmissiveStrength = 2.42, 64
	material.Emissive = graphics.ColorRGBA{R: .25, G: .5, B: 1, A: 1}
	s := &material.Surface
	s.ClearcoatFactor, s.ClearcoatRoughness, s.ClearcoatNormalScale = .8, .2, .7
	s.SheenColor, s.SheenRoughness = graphics.ColorRGBA{R: .2, G: .3, B: .4}, .6
	s.AnisotropyStrength, s.AnisotropyRotation = .9, math.Pi/2
	texture := graphics.TextureFromOwnedNRGBA(image.NewNRGBA(image.Rect(0, 0, 1, 1)), graphics.TextureOptions{})
	t.Cleanup(func() { _ = texture.Dispose() })
	s.ClearcoatTexture, s.ClearcoatRoughnessTexture, s.ClearcoatNormalTexture = texture, texture, texture
	s.SheenColorTexture, s.SheenRoughnessTexture, s.AnisotropyTexture = texture, texture, texture
	spawnMesh3DForTest(world, graphics.CubeMesh(1), material, mgl32.Vec3{0, 0, -4})
	if err := scene.ExtractCamerasSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	ctx := &ecs.SystemContext{World: world}
	var view *renderView3D
	frame := func() {
		if err := extractRender3D(ctx); err != nil {
			panic(err)
		}
		if err := prepareRenderResources3D(ctx); err != nil {
			panic(err)
		}
		state, _ := world.GetResource[*World]()
		view = state.views[entity]
		view.instances, view.draws = view.instances[:0], view.draws[:0]
		view.appendPhase(view.opaque.items, state.white, false)
	}
	for range 5 {
		frame()
	}
	instance := view.instances[0]
	if instance.Material[1] != 2.42 || instance.EmissiveMetallic != [4]float32{16, 32, 64, 0} {
		t.Fatalf("IOR/HDR emission lost: %+v", instance)
	}
	if instance.Custom[0][3] != .8 || instance.Custom[1][3] != .2 || instance.Custom[2] != [4]float32{.2, .3, .4, .6} || instance.Custom[3][0] != .9 || math.Abs(float64(instance.Custom[3][1])) > 1e-6 || instance.Custom[3][2] != 1 || instance.Custom[3][3] != .7 {
		t.Fatal(instance.Custom)
	}
	if uint32(instance.Material[3]) != 65536+131072+262144+524288+1048576+2097152 {
		t.Fatal(instance.Material)
	}
	for _, handle := range view.draws[0].SurfaceTextures[4:] {
		if handle != texture.RenderHandle() {
			t.Fatal("advanced texture binding was lost", handle)
		}
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("layered material allocated %g times", allocations)
	}
	// Removing maps must clear both the GPU handles and the material flags.
	s.ClearcoatTexture, s.ClearcoatRoughnessTexture, s.ClearcoatNormalTexture = nil, nil, nil
	s.SheenColorTexture, s.SheenRoughnessTexture, s.AnisotropyTexture = nil, nil, nil
	frame()
	if view.instances[0].Material[3] != 0 || view.draws[0].SurfaceTextures != [render.MeshSurfaceTextureSlots]render.TextureHandle{} {
		t.Fatal("removed texture retained flags or bindings")
	}
}
