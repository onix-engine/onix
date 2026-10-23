package render3d

import (
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

// FXAA keeps a camera's complete SDR color image separate from subsequent UI.
// The final composite blends premultiplied RGBA and does not write scene depth.
type antialiasing3D struct {
	target    *render.RenderTarget
	instances [1]render.Instance3D
	draws     [1]render.MeshDraw3D
}

func setupViewFXAA3D(view *renderView3D, enabled bool) error {
	view.fxaaActive = false
	if view.frame.Mode != render.RenderViewLit {
		return nil
	}
	if !enabled {
		if view.fxaa != nil {
			if err := view.fxaa.target.Dispose(); err != nil {
				return err
			}
			view.fxaa = nil
		}
		return nil
	}
	width, height := max(1, int(view.frame.Viewport.Width)), max(1, int(view.frame.Viewport.Height))
	if view.fxaa == nil {
		target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: width, Height: height, Depth: true})
		if err != nil {
			return err
		}
		view.fxaa = &antialiasing3D{target: target}
		view.fxaa.instances[0] = render.Instance3D{
			Model: [16]float32(mgl32.Ident4()), BaseColor: [4]float32{1, 1, 1, 1},
			Material: [4]float32{0, 1.5, 0, 512}, UVTransform: [4]float32{1, 1, 0, 0},
		}
	}
	view.fxaa.target.Resize(width, height)
	view.fxaaActive = true
	return nil
}

func renderCamera3D(canvas *graphics.Canvas2D, state *World, view *renderView3D) error {
	if !view.fxaaActive {
		return render3DView(canvas, state, view)
	}
	if state.fxaaMesh == nil {
		mesh, err := graphics.NewMeshOwned([]render.Vertex3D{
			{Position: [3]float32{-1, -1, 1}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{0, 0}},
			{Position: [3]float32{1, -1, 1}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{1, 0}},
			{Position: [3]float32{1, 1, 1}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{1, 1}},
			{Position: [3]float32{-1, 1, 1}, Normal: [3]float32{0, 0, 1}, UV: [2]float32{0, 1}},
		}, []uint32{0, 1, 2, 0, 2, 3})
		if err != nil {
			return err
		}
		state.fxaaMesh = mesh
	}
	handle := state.fxaaMesh.RenderHandle()
	if err := state.fxaaMesh.UploadError(); err != nil {
		return err
	}
	s := view.fxaa
	frame := view.frame
	view.frame.Target = s.target
	width, height := s.target.Size()
	view.frame.Viewport = render.Rect{Width: int32(width), Height: int32(height)}
	view.frame.Camera.viewport = scene.CameraViewport{Width: float32(width), Height: float32(height)}
	view.frame.Camera.viewProjection = frame.Camera.screenProjection
	view.frame.Clear.ClearColor = true
	if !frame.Clear.ClearColor {
		view.frame.Clear.Color = render.Color{}
	}
	err := render3DView(canvas, state, view)
	view.frame = frame
	if err != nil {
		return err
	}
	color, err := s.target.ColorTexture()
	if err != nil {
		return err
	}
	s.draws[0] = render.MeshDraw3D{Mesh: handle, Texture: color, IndexCount: 6, InstanceCount: 1, AlphaMode: uint32(graphics.AlphaBlend)}
	projection := mgl32.Ident4()
	if frame.Target == nil {
		surfaceWidth, surfaceHeight := canvas.Size()
		projection = frame.Camera.viewport.PlacementMatrix(surfaceWidth, surfaceHeight)
		viewport := frame.Camera.viewport
		canvas.PushClipRect(viewport.X, viewport.Y, viewport.Width, viewport.Height)
		defer canvas.PopClipRect()
	}
	batch := render.MeshBatch3D{
		ViewProjection: [16]float32(projection), Instances: s.instances[:], Draws: s.draws[:],
		Lighting: render.MeshLighting{Params: [4]float32{0, render.MeshPassFXAA}},
	}
	if frame.Target != nil {
		// Scene depth belongs to the source target; keep the destination ready
		// for independent cameras and editor overlays after compositing.
		return canvas.DrawMeshesTo(frame.Target, frame.Viewport, render.RenderTargetClear{ClearDepth: true}, batch)
	}
	return canvas.DrawMeshes(batch)
}
