package scene

import (
	"errors"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

type testCameraSettings struct {
	Depth float32
	Mode  render.RenderViewMode
}

func testCameraOptions(settings *testCameraSettings) CameraRenderOptions {
	return CameraRenderOptions{DepthClear: settings.Depth, ViewMode: settings.Mode}
}

func TestRegisteredCameraPipelinePreservesPrioritySnapshotAndErrors(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(200, 100))
	t.Cleanup(restore)
	world := ecs.NewWorld()
	var drawn ecs.Entity
	drawError := errors.New("camera draw failed")
	RegisterCameraRenderer[testCameraSettings](world, CameraPipeline3D, testCameraOptions,
		func(_ *graphics.Canvas2D, _ *ecs.World, camera ExtractedCamera) error {
			drawn = camera.Entity
			return drawError
		})
	base := NewCamera()
	base.Order = -1
	cameras := ecs.NewSpawner5[Camera, testCameraSettings, Camera2D, Projection, GlobalTransform](world)
	configured := cameras.Spawn(base, testCameraSettings{Depth: .75, Mode: render.RenderViewPicking}, Camera2D{}, PerspectiveProjection3D(), GlobalTransform{Matrix: mgl32.Ident4()})
	fallback := SpawnCamera2D(world)
	version := ecs.ComponentVersion[testCameraSettings](world)
	context := ecs.SystemContext{World: world}
	if err := ExtractCameras(&context); err != nil {
		t.Fatal(err)
	}
	state := renderWorld(world)
	if len(state.Cameras) != 2 || state.Cameras[0].Entity != configured || state.Cameras[0].Pipeline != CameraPipeline3D ||
		state.Cameras[0].DepthClear != .75 || state.Cameras[0].ViewMode != render.RenderViewPicking || state.Cameras[1].Entity != fallback {
		t.Fatalf("registered camera snapshot = %+v", state.Cameras)
	}
	if err := RenderCamerasSystem("").Run(world); !errors.Is(err, drawError) || drawn != configured {
		t.Fatalf("draw routing: entity=%v error=%v", drawn, err)
	}
	if ecs.ComponentVersion[testCameraSettings](world) != version {
		t.Fatal("extraction marked camera settings changed")
	}
	mutation := world.MutationVersion()
	if allocations := testing.AllocsPerRun(100, func() {
		RegisterCameraRenderer[testCameraSettings](world, CameraPipeline3D, testCameraOptions, nil)
	}); allocations != 0 || world.MutationVersion() != mutation {
		t.Fatalf("repeated registration: allocations=%v", allocations)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := ExtractCameras(&context); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("camera extraction allocated %v times", allocations)
	}
	ecs.Remove[testCameraSettings](world, configured)
	if err := ExtractCameras(&context); err != nil {
		t.Fatal(err)
	}
	if state.Cameras[0].Pipeline != CameraPipeline2D || state.Cameras[0].DepthClear != 0 {
		t.Fatal("removing settings did not restore the 2D camera")
	}
}

func TestCameraRegistrationInvalidatesIdleInputWithoutTopologyChange(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	ecs.InsertResource(world, state)
	camera := spawnInputCameraForTest(t, world, 200, 100)
	if err := ecs.Insert(world, camera, testCameraSettings{}); err != nil {
		t.Fatal(err)
	}
	targets := ecs.NewSpawner4[GlobalTransform, Visibility, InputTarget2D, InputState2D](world)
	target := targets.Spawn(GlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true}, InputTarget2D{Width: 20, Height: 20}, InputState2D{})
	system := DispatchInputEvents2DSystem("")
	run := func() {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	state.MoveMouse(100, 50)
	run()
	if !ecs.GetRef[InputState2D](world, target).Hovered {
		t.Fatal("initial 2D camera missed the world target")
	}
	state.EndFrame()
	run()
	structure := world.StructureVersion()
	RegisterCameraRenderer[testCameraSettings](world, CameraPipeline3D, testCameraOptions, nil)
	if world.StructureVersion() != structure {
		t.Fatal("registration changed entity topology")
	}
	run()
	if ecs.GetRef[InputState2D](world, target).Hovered {
		t.Fatal("idle input retained the previous camera pipeline")
	}
	ecs.Remove[testCameraSettings](world, camera)
	run()
	if !ecs.GetRef[InputState2D](world, target).Hovered {
		t.Fatal("removing registered settings did not restore 2D picking")
	}
	if allocations := testing.AllocsPerRun(100, run); allocations != 0 {
		t.Fatalf("idle input allocated %v times", allocations)
	}
}
