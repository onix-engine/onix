package render3d

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestSceneCamerasIsolateMeshesAndLights(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 180))
	t.Cleanup(restore)
	w := ecs.NewWorld()
	Register(w)
	spawnCamera := func(id uint64) ecs.Entity {
		b := ecs.NewBundle(5)
		ecs.Put(&b, scene.NewCamera())
		ecs.Put(&b, Camera{})
		ecs.Put(&b, scene.PerspectiveProjection3D())
		ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Ident4()})
		ecs.Put(&b, scene.SceneLayer{ID: id, Order: id})
		return w.Spawn(b)
	}
	a, b := spawnCamera(1), spawnCamera(2)
	mesh := graphics.CubeMesh(1)
	defer mesh.Dispose()
	for _, camera := range []ecs.Entity{a, b} {
		entity := spawnMesh3DForTest(w, mesh, graphics.NewStandardMaterial(graphics.White), mgl32.Vec3{0, 0, -4})
		if err := ecs.Insert(w, entity, scene.Parent{Entity: camera}); err != nil {
			t.Fatal(err)
		}
	}
	if err := scene.ExtractCamerasSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	if err := ExtractSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	state, _ := ecs.GetResource[*World](w)
	if len(state.views[a].extracted) != 1 || len(state.views[b].extracted) != 1 {
		t.Fatal("camera extracted another scene's mesh")
	}
	view := &renderView3D{scene: 2}
	view.frame.Camera.screenProjection = scene.NewPerspectiveProjection().Matrix(1)
	lights := []extractedLight3D{
		{scene: 1, Light3D: render.Light3D{Kind: render.Light3DDirectional, Color: [4]float32{100, 100, 100, 1}}},
		{scene: 2, Light3D: render.Light3D{Kind: render.Light3DDirectional, Color: [4]float32{1, 1, 1, 1}}},
	}
	selectViewLights3D(view, lights, scene.RenderLayers{}, 8)
	if len(view.lights) != 1 || view.lights[0].Color[0] != 1 {
		t.Fatal("background scene light affected foreground scene")
	}
}

func TestSceneAmbientReplacesSharedAmbientInEitherOrder(t *testing.T) {
	shared := extractedLight3D{Light3D: render.Light3D{Kind: render.Light3DAmbient, Color: [4]float32{10, 10, 10, 1}}}
	local := extractedLight3D{scene: 2, Light3D: render.Light3D{Kind: render.Light3DAmbient, Color: [4]float32{1, 1, 1, 1}}}
	other := extractedLight3D{scene: 1, Light3D: render.Light3D{Kind: render.Light3DAmbient, Color: [4]float32{100, 100, 100, 1}}}
	for _, lights := range [][]extractedLight3D{{shared, other, local}, {local, other, shared}} {
		for _, limit := range []int{1, 8} {
			view := &renderView3D{scene: 2}
			selectViewLights3D(view, lights, scene.RenderLayers{}, limit)
			if len(view.lights) != 1 || view.lights[0].Color != local.Color {
				t.Fatal("scene ambient was mixed with shared or other-scene ambient", view.lights)
			}
		}
	}
}
