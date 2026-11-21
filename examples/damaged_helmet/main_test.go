package main

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/input"
	"github.com/onix-engine/onix/project"
	"github.com/onix-engine/onix/render"
	"github.com/onix-engine/onix/scene"
	"github.com/onix-engine/onix/scene/render3d"
)

// Run through builder test to exercise packaged resources as well as source
// assets. This loads the real model and HDR without opening a window.
func TestDamagedHelmetComparison(t *testing.T) {
	plan, source, err := project.OpenModuleProject()
	if err != nil {
		t.Fatal(err)
	}
	registry, err := OnixRegistry()
	if err != nil {
		t.Fatal(err)
	}
	app := onix.NewApp()
	t.Cleanup(app.Destroy)
	if _, err := app.LoadModules[OnixContentProfile](plan, registry, OnixModuleOptions(source)); err != nil {
		t.Fatal(err)
	}
	var camera, helmet ecs.Entity
	for entity, name := range app.World.Query[scene.Name]().All {
		switch name.Get().Value {
		case "camera":
			camera = entity
		case "helmet":
			helmet = entity
		}
	}
	if camera == 0 || helmet == 0 {
		t.Fatal("comparison scene is incomplete")
	}
	geometry, ok := app.World.Get[render3d.Mesh](helmet)
	if !ok || geometry.Mesh == nil {
		t.Fatal("imported mesh missing")
	}
	if len(geometry.Mesh.Vertices) != 14556 || len(geometry.Mesh.Indices) != 46356 {
		t.Fatalf("unexpected DamagedHelmet geometry: %d vertices, %d indices", len(geometry.Mesh.Vertices), len(geometry.Mesh.Indices))
	}
	materials, ok := app.World.Get[render3d.Material[graphics.StandardMaterial]](helmet)
	if !ok || len(materials.Materials) == 0 {
		t.Fatal("imported material missing")
	}
	material := materials.Materials[0]
	for slot, texture := range [...]*graphics.Texture{material.BaseColorTexture, material.MetallicRoughnessTexture, material.NormalTexture, material.Surface.OcclusionTexture, material.Surface.EmissiveTexture} {
		if texture == nil || !texture.Valid() || texture.Width != 2048 || texture.Height != 2048 {
			t.Fatalf("PBR texture %d missing or changed", slot)
		}
	}
	if material.Metallic != 1 || material.PerceptualRoughness != 1 || material.Emissive.R != 1 || material.Surface.NormalScale != 1 || material.Surface.OcclusionStrength != 1 {
		t.Fatalf("glTF material factors were overridden: %+v", material)
	}
	view, _ := app.World.Get[render3d.Camera](camera)
	environment, _ := app.World.Get[render3d.Environment](camera)
	if view.ToneMapping != graphics.ToneMappingACES || math.Abs(float64(view.Exposure.Multiplier()-1)) > 1e-6 {
		t.Fatalf("comparison must use ACES and unit exposure: %+v", view)
	}
	if environment.Panorama == nil || environment.Panorama.Width != 2048 || environment.Panorama.Height != 1024 || environment.Atlas == nil || environment.Intensity != 1 || !environment.Background || environment.Encoding != render.EnvironmentRGBE {
		t.Fatalf("HDR environment missing or clipped: %+v", environment)
	}
	for range app.World.Query[render3d.DirectionalLight]().All {
		t.Fatal("additional lighting changes the reference scene")
	}
	ambientCount := 0
	for _, ambient := range app.World.Query[render3d.AmbientLight]().All {
		ambientCount++
		if ambient.Get().Brightness != 0 {
			t.Fatal("additional lighting changes the reference scene")
		}
	}
	if ambientCount != 1 {
		t.Fatal("engine default ambient must be explicitly disabled")
	}
	projection, _ := app.World.Get[scene.Projection](camera)
	initial, _ := app.World.Get[scene.Transform](camera)
	// Bounds after the source glTF's +90 degree X node rotation.
	center := mgl32.Vec3{-.00248158, .00001052, -.18715495}
	distance := float32(1.3 / math.Atan(math.Pi/8))
	expected := center.Add(mgl32.Vec3{-1.8, .6, 2.9}.Normalize().Mul(distance))
	if initial.Translation.Sub(expected).Len() > 1e-5 || math.Abs(float64(projection.Perspective.Near-distance/100)) > 1e-6 {
		t.Fatalf("camera does not match three.js fit: position %v, near %g", initial.Translation, projection.Perspective.Near)
	}
	forward := initial.Rotation.Rotate(mgl32.Vec3{0, 0, -1})
	if forward.Sub(center.Sub(initial.Translation).Normalize()).Len() > 1e-5 {
		t.Fatal("camera does not face helmet center")
	}
	runtime := app.Modules()
	send := func(kind scene.InputEventKind, key input.KeyCode) {
		t.Helper()
		if err := runtime.HandleInput(scene.InputEvent{Kind: kind, Key: key}); err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the platform pointer state used by the running example; mouse
	// motion alone and right-button drags must leave the camera stationary.
	pointerInput := input.NewState()
	app.World.InsertResource(pointerInput)
	update := func() scene.Transform {
		t.Helper()
		if err := runtime.UpdateFrame(0); err != nil {
			t.Fatal(err)
		}
		transform, _ := app.World.Get[scene.Transform](camera)
		return transform
	}
	pointerInput.MoveMouse(100, 100)
	if update() != initial {
		t.Fatal("hover movement rotated the camera")
	}
	pointerInput.EndFrame()
	pointerInput.PressMouse(input.MouseRight, 100, 100)
	pointerInput.MoveMouse(200, 160)
	if update() != initial {
		t.Fatal("right-button drag rotated the camera")
	}
	pointerInput.ReleaseMouse(input.MouseRight, 200, 160)
	pointerInput.EndFrame()
	pointerInput.PressMouse(input.MouseLeft, 200, 160)
	if update() != initial {
		t.Fatal("pressing left mouse jumped the camera")
	}
	pointerInput.MoveMouse(240, 180)
	dragged := update()
	if dragged == initial || math.Abs(float64(dragged.Translation.Sub(center).Len()-distance)) > 1e-5 {
		t.Fatal("left-button drag did not orbit around the helmet at a fixed distance")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		pointerInput.EndFrame()
		p := pointerInput.Pointer()
		pointerInput.MoveMouse(float64(p.X+1), float64(p.Y+1))
		if err := runtime.UpdateFrame(0); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("active mouse orbit allocated %g", allocations)
	}
	stopped, _ := app.World.Get[scene.Transform](camera)
	point := pointerInput.Pointer()
	pointerInput.EndFrame()
	pointerInput.ReleaseMouse(input.MouseLeft, float64(point.X), float64(point.Y))
	pointerInput.MoveMouse(400, 300)
	if update() != stopped {
		t.Fatal("mouse orbit continued after release")
	}
	pointerInput.EndFrame()
	send(scene.InputEventKeyDown, input.KeyR)
	if update() != initial {
		t.Fatal("reset did not restore the camera after a mouse orbit")
	}
	send(scene.InputEventKeyDown, input.KeyRight)
	if err := runtime.UpdateFrame(.1); err != nil {
		t.Fatal(err)
	}
	after, _ := app.World.Get[scene.Transform](camera)
	if after == initial {
		t.Fatal("keyboard input did not reach orbit controller")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := runtime.UpdateFrame(1.0 / 60); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("active orbit update allocated %g", allocations)
	}
	send(scene.InputEventKeyUp, input.KeyRight)
	send(scene.InputEventKeyDown, input.KeyE)
	for range 100 {
		if err := runtime.UpdateFrame(1); err != nil {
			t.Fatal(err)
		}
	}
	zoomed, _ := app.World.Get[scene.Transform](camera)
	if math.Abs(float64(zoomed.Translation.Sub(center).Len()-distance*10)) > 1e-4 {
		t.Fatal("zoom did not clamp to reference maximum")
	}
	send(scene.InputEventKeyUp, input.KeyE)
	send(scene.InputEventKeyDown, input.KeyR)
	if err := runtime.UpdateFrame(0); err != nil {
		t.Fatal(err)
	}
	reset, _ := app.World.Get[scene.Transform](camera)
	if reset != initial {
		t.Fatal("reset did not restore comparison camera")
	}
	if runtime.NeedsTick() {
		t.Fatal("stationary viewer keeps the application awake")
	}
	app.SetRenderMode(onix.RenderOnDemand)
	app.SetIdleEnabled(true)
	app.SetTargetFPS(0)
	pointerInput.EndFrame()
	for range 3 {
		if _, err := app.Step(); err != nil {
			t.Fatal(err)
		}
	}
	for range 10 {
		if rendered, err := app.Step(); err != nil || rendered {
			t.Fatalf("stationary viewer rendered: rendered=%v err=%v", rendered, err)
		}
	}
	pointerInput.PressMouse(input.MouseLeft, 400, 300)
	pointerInput.MoveMouse(440, 320)
	app.RequestRender() // Native pointer delivery wakes the window loop.
	if rendered, err := app.Step(); err != nil || !rendered {
		t.Fatalf("mouse orbit did not wake the viewer: rendered=%v err=%v", rendered, err)
	}
	if current, _ := app.World.Get[scene.Transform](camera); current == initial {
		t.Fatal("first awake frame did not update the camera")
	}
}
