package render3d

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

// Register enables 3D camera extraction and submission in this world.
// Repeated registration reuses the renderer, material readers, and camera binding.
func Register(world *ecs.World) {
	WorldFor(world)
	scene.RenderWorldFor(world).RegisterCameraRenderer[Camera](scene.CameraPipeline3D, cameraOptions, drawCamera)
}

func cameraOptions(camera *Camera) scene.CameraRenderOptions {
	return scene.CameraRenderOptions{Target: camera.Target, ViewMode: camera.ViewMode, DepthClear: camera.DepthClear}
}

func drawCamera(canvas *graphics.Canvas2D, world *ecs.World, camera scene.ExtractedCamera) error {
	state := WorldFor(world)
	return render3DView(canvas, state, state.views[camera.Entity])
}
