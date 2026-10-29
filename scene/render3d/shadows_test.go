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

func TestPunctualShadowProjectionFacesAndDepth(t *testing.T) {
	settings := NewPunctualShadowSettings()
	point := NewPointLight()
	position := mgl32.Vec3{2, 3, 4}
	matrices := punctualShadowMatrices3D(point.renderLight(mgl32.Translate3D(position.X(), position.Y(), position.Z())), settings)
	axes := [...]mgl32.Vec3{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	for face, axis := range axes {
		for _, sample := range [...]struct{ distance, depth float32 }{{settings.Near, 1}, {point.Range, 0}} {
			clip := matrices[face].Mul4x1(position.Add(axis.Mul(sample.distance)).Vec4(1))
			if clip.W() <= 0 || math.Abs(float64(clip.X()/clip.W())) > 1e-5 || math.Abs(float64(clip.Y()/clip.W())) > 1e-5 || math.Abs(float64(clip.Z()/clip.W()-sample.depth)) > 1e-4 {
				t.Fatalf("face %d depth %g: clip=%v", face, sample.distance, clip)
			}
		}
	}
	for _, shape := range [...]SpotLightShape{SpotCone, SpotPyramid, SpotBox} {
		spot := NewSpotLight()
		spot.Shape, spot.AspectRatio = shape, 2
		if shape == SpotBox {
			spot.Unit = graphics.LightLux
		}
		light := spot.renderLight(mgl32.Ident4())
		m := punctualShadowMatrices3D(light, settings)[0]
		for _, sample := range [...]struct{ distance, depth float32 }{{settings.Near, 1}, {spot.Range, 0}} {
			clip := m.Mul4x1(mgl32.Vec4{0, 0, -sample.distance, 1})
			if math.Abs(float64(clip.Z()/clip.W()-sample.depth)) > 1e-4 {
				t.Fatalf("%s reversed depth=%v", shape, clip)
			}
		}
		if shape == SpotPyramid {
			x, y := spot.PyramidTangents()
			clip := m.Mul4x1(mgl32.Vec4{x * 2, y * 2, -2, 1})
			if math.Abs(float64(clip.X()/clip.W()-1)) > 1e-5 || math.Abs(float64(clip.Y()/clip.W()-1)) > 1e-5 {
				t.Fatal("pyramid shadow does not cover the light's rectangular cone")
			}
		}
	}
	if allocations := testing.AllocsPerRun(100, func() { _ = punctualShadowMatrices3D(point.renderLight(mgl32.Ident4()), settings) }); allocations != 0 {
		t.Fatal(allocations)
	}
}

func TestCascadeShadowsCoverFrustumSlices(t *testing.T) {
	settings := NewShadowSettings()
	settings.Cascades, settings.Distance, settings.Resolution = 4, 80, 128
	projections := [...]mgl32.Mat4{
		scene.NewPerspectiveProjection().Matrix(1.6),
		{.1, 0, 0, 0, 0, .2, 0, 0, 0, 0, .01, 0, 0, 0, 1, 1},
	}
	for _, projection := range projections {
		transform := mgl32.Translate3D(2, 3, 4).Mul4(mgl32.HomogRotate3DY(.7))
		camera := extractedView3D{viewProjection: projection.Mul4(transform.Inv()), position: transform.Col(3).Vec3()}
		matrices, ranges, forward, err := cascadeShadowMatrices3D(camera, mgl32.Vec3{.3, -.8, -.2}.Normalize(), settings)
		if err != nil {
			t.Fatal(err)
		}
		inverse := camera.viewProjection.Inv()
		for cascade := 0; cascade < 4; cascade++ {
			if ranges[cascade][1] <= ranges[cascade][0] || cascade > 0 && ranges[cascade][0] != ranges[cascade-1][1] {
				t.Fatal("cascade splits are not continuous and increasing")
			}
			for corner := 0; corner < 4; corner++ {
				x, y := float32((corner&1)*2-1), float32((corner>>1)*2-1)
				a4, b4 := inverse.Mul4x1(mgl32.Vec4{x, y, 1, 1}), inverse.Mul4x1(mgl32.Vec4{x, y, .5, 1})
				a, b := a4.Vec3().Mul(1/a4.W()), b4.Vec3().Mul(1/b4.W())
				delta, direction := b.Sub(a), mgl32.Vec3(forward[:3])
				for _, distance := range ranges[cascade] {
					point := a.Add(delta.Mul((distance - a.Sub(camera.position).Dot(direction)) / delta.Dot(direction)))
					clip := matrices[cascade].Mul4x1(point.Vec4(1))
					if math.Abs(float64(clip.X())) > clipTolerance3D || math.Abs(float64(clip.Y())) > clipTolerance3D || clip.Z() < 0 || clip.Z() > 1 {
						t.Fatalf("cascade %d excludes a frustum corner: %v", cascade, clip)
					}
				}
			}
		}
		if math.Abs(float64(ranges[3][1]-settings.Distance)) > 1e-4 {
			t.Fatal("cascade distance changed")
		}
		if allocations := testing.AllocsPerRun(100, func() { _, _, _, _ = cascadeShadowMatrices3D(camera, mgl32.Vec3{0, -1, 0}, settings) }); allocations != 0 {
			t.Fatal(allocations)
		}
	}
}

const clipTolerance3D = 1.00001

func TestMixedShadowAtlasRenderingAndReuse(t *testing.T) {
	driver := &meshTestDriver{viewport: render.Rect{Width: 320, Height: 240}}
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
	camera := ecs.NewBundle(4)
	camera.Put(scene.NewCamera())
	camera.Put(Camera{})
	camera.Put(scene.PerspectiveProjection3D())
	camera.Put(scene.GlobalTransform{Matrix: mgl32.Ident4()})
	entity := world.Spawn(camera)
	sun := NewDirectionalLight()
	sun.Shadows = NewShadowSettings()
	sun.Shadows.Cascades, sun.Shadows.Distance, sun.Shadows.Resolution = 4, 30, 64
	b := ecs.NewBundle(2)
	b.Put(sun)
	b.Put(scene.GlobalTransform{Matrix: mgl32.HomogRotate3DX(-.7)})
	world.Spawn(b)
	sun.Shadows.Cascades = 1
	b = ecs.NewBundle(2)
	b.Put(sun)
	b.Put(scene.GlobalTransform{Matrix: mgl32.Ident4()})
	world.Spawn(b)
	point := NewPointLight()
	point.Shadows = NewPunctualShadowSettings()
	point.Shadows.Resolution = 64
	b = ecs.NewBundle(2)
	b.Put(point)
	b.Put(scene.GlobalTransform{Matrix: mgl32.Translate3D(0, 0, -3)})
	world.Spawn(b)
	spot := NewSpotLight()
	spot.Shadows = point.Shadows
	b = ecs.NewBundle(2)
	b.Put(spot)
	b.Put(scene.GlobalTransform{Matrix: mgl32.Ident4()})
	world.Spawn(b)
	mesh := graphics.CubeMesh(.5)
	t.Cleanup(func() { _ = mesh.Dispose() })
	material := graphics.NewStandardMaterial(graphics.White)
	for _, axis := range [...]mgl32.Vec3{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}} {
		p := mgl32.Vec3{0, 0, -3}.Add(axis.Mul(2))
		b = ecs.NewBundle(3)
		b.Put(Mesh{Mesh: mesh})
		b.Put(Material[graphics.StandardMaterial]{Material: material})
		b.Put(scene.GlobalTransform{Matrix: mgl32.Translate3D(p.X(), p.Y(), p.Z())})
		world.Spawn(b)
	}
	for _, system := range []ecs.SystemConfig{scene.ExtractCamerasSystem(""), ExtractSystem(""), PrepareSystem(""), DrawSystem("")} {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	view := state.views[entity]
	if view.shadow.count != 12 || driver.targetBegins != 12 || driver.targetEnds != 12 {
		t.Fatalf("mixed shadow passes: maps=%d begin/end=%d/%d", view.shadow.count, driver.targetBegins, driver.targetEnds)
	}
	pointFirst := 0
	for index, expected := range [][2]float32{{0, 0}, {1, 4}, {5, 1}, {6, 1}, {7, 6}} {
		if got := view.lighting.LightShadows[index]; got[0] != expected[0] || got[1] != expected[1] {
			t.Fatalf("light %d mapping %v, expected %v", index, got, expected)
		}
		if view.lights[index].Kind == render.Light3DPoint {
			pointFirst = int(expected[0]) - 1
		}
	}
	for index, shadow := range view.shadow.views[:view.shadow.count] {
		if shadow.frame.Target != view.shadow.target || shadow.frame.Clear.ClearColor != (index == 0) || shadow.frame.Clear.ClearDepth != (index == 0) {
			t.Fatal("atlas attachments are not shared or later tiles clear earlier shadows")
		}
		if index >= pointFirst && index < pointFirst+6 && len(shadow.extracted) != 1 {
			t.Fatalf("point face %d includes %d casters", index-pointFirst, len(shadow.extracted))
		}
	}
	if err := canvas.FinishFrame(); err != nil {
		t.Fatal(err)
	}
	target := view.shadow.target
	frame := func() {
		ctx := &ecs.SystemContext{World: world}
		if err := extractRender3D(ctx); err != nil {
			panic(err)
		}
		if err := prepareRenderResources3D(ctx); err != nil {
			panic(err)
		}
		for _, shadow := range view.shadow.views[:view.shadow.count] {
			shadow.instances, shadow.draws = shadow.instances[:0], shadow.draws[:0]
			shadow.appendPhase(shadow.extracted, state.white, false)
		}
	}
	for range 5 {
		frame()
	}
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("mixed shadow extraction/preparation allocated %g times", allocations)
	}
	if view.shadow.target != target {
		t.Fatal("stable frames recreated the atlas")
	}
}

func TestShadowAtlasBudgetAndValidation(t *testing.T) {
	view := &renderView3D{lights: make([]render.Light3D, 3)}
	var settings [render.MaxMeshLights]extractedShadowLight3D
	for index := range view.lights {
		view.lights[index] = render.Light3D{Kind: render.Light3DPoint, Params: [4]float32{10}}
		settings[index].punctual = NewPunctualShadowSettings()
	}
	if err := setupShadowAtlas3D(view, settings, false, 0); err != nil || view.shadow == nil || view.shadow.count != 12 || view.lighting.LightShadows[2][0] != 0 {
		t.Fatal("overflow shadows must leave the remaining light unshadowed", err)
	}
	t.Cleanup(func() { _ = disposeViewTargets3D(view) })
	for _, near := range [...]float32{0, -1, 10, float32(math.NaN())} {
		s := NewPunctualShadowSettings()
		s.Near = near
		if s.Validate(10) == nil {
			t.Fatalf("invalid near plane %g accepted", near)
		}
	}
}

func TestShadowAtlasPackingHonorsSizesAndBounds(t *testing.T) {
	var sizes [render.MaxShadowMaps]int
	for index := range sizes {
		sizes[index] = 512
		if index < 4 {
			sizes[index] = 2048
		}
	}
	tiles, width, height := packShadowAtlas3D(sizes, 11)
	for index, tile := range tiles[:11] {
		if tile.size != sizes[index] || tile.x < 0 || tile.y < 0 || tile.x+tile.size > width || tile.y+tile.size > height {
			t.Fatalf("tile %d was resized or exceeds atlas bounds: %+v in %dx%d", index, tile, width, height)
		}
		for _, other := range tiles[:index] {
			if tile.x < other.x+other.size && other.x < tile.x+tile.size && tile.y < other.y+other.size && other.y < tile.y+tile.size {
				t.Fatal("atlas tiles overlap")
			}
		}
	}
	if width*height > 4096*4608 {
		t.Fatalf("mixed resolution atlas wastes attachment space: %dx%d", width, height)
	}
	for index := range sizes {
		sizes[index] = 8192
	}
	tiles, width, height = packShadowAtlas3D(sizes, render.MaxShadowMaps)
	if width > 8192 || height > 8192 {
		t.Fatal("oversized atlas was not limited")
	}
	for _, tile := range tiles {
		if tile.size != 2048 {
			t.Fatal("large requests were not reduced consistently")
		}
	}
	if allocations := testing.AllocsPerRun(100, func() { _, _, _ = packShadowAtlas3D(sizes, render.MaxShadowMaps) }); allocations != 0 {
		t.Fatal(allocations)
	}
}
