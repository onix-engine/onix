package editor

import (
	"math"
	"strings"

	"github.com/diiyw/onix/ecs"
	editorresources "github.com/diiyw/onix/editor/resources"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

const workspaceCanvasImage = "onix-scene-viewport"

const workspaceCanvasHTML = `<style>body,#scene-preview{border-radius:7px}</style><body style="margin:0;background:#262a32;color:#bdcce5;overflow:hidden;font-family:sans-serif">
<img id="scene-preview" src="onix-scene-viewport" alt="Live 2D scene" style="position:absolute;left:0;top:0;width:100%;height:100%">
</body>`

func (a *ModuleEditor) layoutViewport2D() {
	width, height := a.viewport2DSize()
	bounds := a.canvasBounds()
	a.canvasScale = max(.01, min((bounds.W-24)/float32(width), (bounds.H-40)/float32(height))*a.zoom)
	w, h := float32(width)*a.canvasScale, float32(height)*a.canvasScale
	a.viewport2D.frame = rect{X: (bounds.W-w)/2 + a.viewport2D.offset[0], Y: (bounds.H-h)/2 + a.viewport2D.offset[1], W: w, H: h}
}

func (a *ModuleEditor) updateViewport2DInput(in *input.State) {
	v := &a.viewport2D
	if a.Document == nil || a.viewport2D.world == nil || a.modalVisible() || a.workspaceMenuOpen() || a.canvasDrag.ID != "" {
		v.panning = false
		return
	}
	pointer, bounds := in.Pointer(), a.canvasBounds()
	inside := pointer.X >= bounds.X && pointer.X < bounds.X+bounds.W && pointer.Y >= bounds.Y && pointer.Y < bounds.Y+bounds.H
	if !in.MouseHeld(input.MouseMiddle) {
		v.panning = false
	} else if in.MousePressed(input.MouseMiddle) {
		v.panning = inside
	}
	if v.panning && (pointer.DeltaX != 0 || pointer.DeltaY != 0) {
		v.offset = v.offset.Add(mgl32.Vec2{pointer.DeltaX, pointer.DeltaY})
		a.layoutViewport2D()
		a.changedCanvas()
	}
	if pointer.WheelY == 0 || !inside {
		return
	}
	zoom := clamp(a.zoom*float32(math.Pow(1.12, float64(pointer.WheelY))), .05, 8)
	if zoom != a.zoom {
		// Keep the scene point under the cursor fixed as the frame changes size.
		cursor := mgl32.Vec2{pointer.X - bounds.X, pointer.Y - bounds.Y}
		frame := a.viewport2D.frame
		point := cursor.Sub(mgl32.Vec2{frame.X, frame.Y}).Mul(1 / a.canvasScale)
		a.zoom = zoom
		a.layoutViewport2D()
		frame = a.viewport2D.frame
		a.viewport2D.offset = a.viewport2D.offset.Add(cursor.Sub(mgl32.Vec2{frame.X, frame.Y}.Add(point.Mul(a.canvasScale))))
		a.layoutViewport2D()
		a.changed()
	}
	in.ConsumePointerWheel()
}

func (a *ModuleEditor) syncCanvas(w *ecs.World) error {
	bounds := a.canvasBounds()
	if a.canvas == 0 {
		a.canvas = spawnWorkspaceSurface(w, workspaceCanvasHTML, "", a.font, bounds, 10)
		ecs.InsertResource[htmlui.ImageProvider](w, a)
		root, err := htmlui.Root(w, a.canvas)
		if err != nil {
			return err
		}
		preview, _ := root.FindByID("scene-preview")
		preview.OnPointer(func(event *htmlui.PointerEvent) {
			if event.Kind != scene.InputEventPress {
				return
			}
			if a.tilemapPress(event.X, event.Y) {
				event.Consume()
				return
			}
			a.pressViewport2D(event.X, event.Y)
			event.Consume()
		})
	}
	transform := ecs.GetMut[scene.UITransform](w, a.canvas)
	transform.Translation = scene.Px2(bounds.X, bounds.Y)
	surface := ecs.GetMut[htmlui.Surface](w, a.canvas)
	surface.Width, surface.Height = bounds.W, bounds.H
	// Dialogs block input with their own InputBlocker2D, so the canvas stays visible behind the overlay.
	ecs.GetMut[scene.Visibility](w, a.canvas).Visible = a.Document != nil && a.dimension != previewDimension3D
	if a.Document == nil {
		return nil
	}
	if a.dimension == previewDimension3D {
		if a.viewport2D.world != nil && a.viewport2D.structure != a.Preview.structure {
			a.disposeCanvasResources()
		}
		if err := a.syncViewport3D(); err != nil && a.status != err.Error() {
			a.report(err)
		}
		return nil
	}
	if a.viewport3D.world != nil && a.viewport3D.structure != a.Preview.structure {
		a.viewport3D.dispose()
	}
	if err := a.syncViewport2D(); err != nil {
		if a.status != err.Error() {
			a.report(err)
		}
		return nil
	}
	if _, err := a.viewport2D.target.ensureTarget(graphics.DeviceFor(w), int(bounds.W), int(bounds.H), 1); err != nil {
		return err
	}
	root, err := htmlui.Root(w, a.canvas)
	if err != nil {
		return err
	}
	preview, _ := root.FindByID("scene-preview")
	preview.SetAttribute("src", a.viewport2D.imageName)
	style := "position:absolute;left:0;top:0;width:100%;height:100%"
	if a.viewport2D.target.colorTexture != nil && a.viewport2D.bottomLeft() {
		style += ";transform:scaleY(-1)"
	}
	preview.SetAttribute("style", style)
	return nil
}

func (a *ModuleEditor) ResolveHTMLImage(name string) (*graphics.Texture, bool) {
	if icon, ok := strings.CutPrefix(name, "onix-file-icons/"); ok {
		texture := editorresources.FileIconTexture(icon)
		return texture, texture != nil
	}
	if name == a.viewport2D.imageName {
		texture := a.viewport2D.target.colorTexture
		return texture, texture != nil
	}
	for _, prefix := range [...]string{"onix-titlebar/", "onix-leftbar/", "onix-rightbar/"} {
		if icon, ok := strings.CutPrefix(name, prefix); ok {
			return editorHTMLIcon(icon, graphics.ColorFromRGB(176, 183, 195)), true
		}
	}
	return nil, false
}

func (a *ModuleEditor) disposeCanvasResources() {
	a.releaseAnimationPreview()
	a.cancelPlacementDrag()
	a.viewport2D.dispose()
	a.canvasImageVersion++
	if a.world != nil {
		ecs.InsertResource[htmlui.ImageProvider](a.world, a)
	}
}

func viewportGridStep(zoom float32) float32 {
	step := float32(64)
	for step*zoom < 24 {
		step *= 2
	}
	for step*zoom > 96 && step > 8 {
		step *= .5
	}
	return step
}
