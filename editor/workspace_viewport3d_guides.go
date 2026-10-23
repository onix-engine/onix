package editor

// Grid and depth-tested infinite axes restored from the legacy viewport
// before b5619c5e, using fixed arrays for frame-local geometry.
import (
	"fmt"
	"image"
	"math"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func (a *ModuleEditor) setupPreview3DGuides() {
	a.viewport3D.view.gridTexture = graphics.CreateTextureByColor(graphics.White)
	// Linear sampling supplies a one-pixel coverage ramp on both sides of a
	// two-pixel axis. The rows interpolate distance fade; keep RGB white because
	// the unlit mesh shader premultiplies alpha.
	a.viewport3D.view.axisTexture = graphics.CreateTextureByImageNoMipmap(&image.RGBA{
		Pix: []byte{
			255, 255, 255, 0, 255, 255, 255, 0, 255, 255, 255, 0, 255, 255, 255, 0,
			255, 255, 255, 0, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 0,
		},
		Stride: 16, Rect: image.Rect(0, 0, 4, 2),
	})
}

func preview3DAxisColor(color graphics.ColorRGBA) graphics.ColorRGBA {
	color.A = 0.65
	return color
}

// drawPreview3DDepthGrid submits a fixed four-vertex plane. Its shader draws
// stable world-space lines and fades out before the plane's moving edges.

func (a *ModuleEditor) drawPreview3DDepthGrid(c *graphics.Canvas2D, projection *scene.Projection, cameraTransform mgl32.Mat4) error {
	if c == nil || projection == nil || a.viewport3D.view.renderTarget == nil || a.settings.Viewport3D.HideGrid {
		return nil
	}
	v := &a.viewport3D.view
	if v.gridTexture == nil {
		return nil
	}
	handle := v.gridTexture.RenderHandle()
	if err := v.gridTexture.UploadError(); err != nil {
		return err
	}
	vp := a.canvasBounds()
	if vp.W <= 0 || vp.H <= 0 {
		return nil
	}
	normal := preview3DGridNormal(projection, cameraTransform, a.settings.Viewport3D.GridPlane)
	center, radius := a.preview3DGuideBounds(normal, cameraTransform)
	viewDistance := max(v.distance/max(.05, a.zoom), abs(cameraTransform.Col(3)[normal]))
	firstAxis, secondAxis := (normal+1)%3, (normal+2)%3
	for i, offset := range [...][2]float32{{-1, -1}, {-1, 1}, {1, 1}, {1, -1}} {
		point := center
		point[firstAxis] += offset[0] * radius
		point[secondAxis] += offset[1] * radius
		v.gridVertices[i] = render.Vertex3D{Position: [3]float32(point)}
	}
	v.gridIndices = [6]uint32{0, 1, 2, 0, 2, 3}
	step, blend := preview3DGridLOD(preview3DWorldPerPixel(projection, viewDistance, vp))
	forward := cameraTransform.Col(2).Vec3()
	ortho := float32(0)
	if projection.Kind == scene.ProjectionOrthographic {
		ortho = 1
	}
	v.gridInstances[0] = render.Instance3D{
		Model: [16]float32(mgl32.Ident4()), BaseColor: [4]float32{0.48, 0.51, 0.56, 1},
		Material: [4]float32{-1, 0, 0, 8192}, UVTransform: [4]float32{1, 1, 0, 0},
		Custom: [4][4]float32{{step, blend, float32(preview3DRenderScale(a.settings.Viewport3D.Supersample)), float32(normal)}, {center[firstAxis], center[secondAxis], radius, 0}, {forward.X(), forward.Y(), forward.Z(), ortho}},
	}
	v.gridDraws[0] = render.MeshDraw3D{Texture: handle, IndexCount: 6, InstanceCount: 1, AlphaMode: uint32(graphics.AlphaBlend)}
	clip := projection.Matrix(vp.W, vp.H).Mul4(cameraTransform.Inv())
	return c.DrawMeshesTo(v.renderTarget, preview3DTargetViewport(v.renderTarget), render.RenderTargetClear{}, render.MeshBatch3D{
		ViewProjection: [16]float32(clip), CameraPosition: [4]float32{cameraTransform[12], cameraTransform[13], cameraTransform[14], 1},
		Vertices: v.gridVertices[:], Indices: v.gridIndices[:], Instances: v.gridInstances[:], Draws: v.gridDraws[:],
	})
}

func (a *ModuleEditor) preview3DGuideBounds(normal int, camera mgl32.Mat4) (mgl32.Vec3, float32) {
	center := a.viewport3D.view.target
	center[normal] = 0
	return center, max(max(a.viewport3D.view.distance/max(.05, a.zoom), abs(camera.Col(3)[normal]))*12, 1)
}

func preview3DGridNormal(projection *scene.Projection, camera mgl32.Mat4, plane uint8) int {
	if plane%4 != 0 {
		return [...]int{1, 1, 2, 0}[plane%4]
	}
	if projection.Kind != scene.ProjectionOrthographic {
		return 1
	}
	forward := camera.Col(2).Vec3()
	axis := 0
	for i := 1; i < 3; i++ {
		if abs(forward[i]) > abs(forward[axis]) {
			axis = i
		}
	}
	return axis
}

func preview3DGridLOD(worldPerPixel float32) (float32, float32) {
	level := math.Log10(float64(max(worldPerPixel*24, 0.000001)))
	base := math.Floor(level)
	return float32(math.Pow(10, base)), float32(level - base)
}

func (a *ModuleEditor) drawPreview3DAxesDepth(c *graphics.Canvas2D, projection *scene.Projection, cameraTransform mgl32.Mat4) error {
	if c == nil || projection == nil {
		return nil
	}
	texture := a.viewport3D.view.axisTexture
	if texture == nil {
		return nil
	}
	textureHandle := texture.RenderHandle()
	if err := texture.UploadError(); err != nil {
		return fmt.Errorf("editor: prepare 3D axis texture: %w", err)
	}
	if !textureHandle.Valid() {
		return fmt.Errorf("editor: 3D axis texture has an invalid renderer handle")
	}
	viewport := a.canvasBounds()
	if viewport.W <= 0 || viewport.H <= 0 {
		return nil
	}
	clipFromWorld := projection.Matrix(viewport.W, viewport.H).Mul4(cameraTransform.Inv())
	normal := preview3DGridNormal(projection, cameraTransform, a.settings.Viewport3D.GridPlane)
	center, radius := a.preview3DGuideBounds(normal, cameraTransform)
	vertices := a.viewport3D.view.axisVertices[:0]
	indices := a.viewport3D.view.axisIndices[:0]
	instances := a.viewport3D.view.axisInstances[:]
	draws := a.viewport3D.view.axisDraws[:]
	clear(instances)
	clear(draws)
	drawCount := 0
	for _, axis := range [...]struct {
		vector mgl32.Vec3
		color  graphics.ColorRGBA
	}{
		{vector: mgl32.Vec3{1, 0, 0}, color: editorTheme.axisX},
		{vector: mgl32.Vec3{0, 1, 0}, color: editorTheme.axisY},
		{vector: mgl32.Vec3{0, 0, 1}, color: editorTheme.axisZ},
	} {
		firstIndex := len(indices)
		for _, sign := range [...]float32{-1, 1} {
			start, end, ok := projectPreview3DAxisRay(clipFromWorld, axis.vector.Mul(sign))
			if ok {
				appendPreview3DFadedAxisRibbon(&vertices, &indices, viewport, start, end, 2,
					clipFromWorld, axis.vector, center, radius, normal, cameraTransform, projection.Kind == scene.ProjectionOrthographic)
			}
		}
		if len(indices) == firstIndex {
			continue
		}
		color := preview3DAxisColor(axis.color)
		instances[drawCount] = render.Instance3D{
			Model:       [16]float32(mgl32.Ident4()),
			BaseColor:   [4]float32{color.R, color.G, color.B, color.A},
			Material:    [4]float32{-1, 0, 0, 0},
			UVTransform: [4]float32{1, 1, 0, 0},
		}
		draws[drawCount] = render.MeshDraw3D{
			Texture:       textureHandle,
			FirstIndex:    uint32(firstIndex),
			IndexCount:    uint32(len(indices) - firstIndex),
			FirstInstance: uint32(drawCount),
			InstanceCount: 1,
			AlphaMode:     uint32(graphics.AlphaBlend),
		}
		drawCount++
	}
	if drawCount == 0 {
		return nil
	}
	batch := render.MeshBatch3D{
		ViewProjection: [16]float32(mgl32.Ident4()),
		CameraPosition: [4]float32{cameraTransform.Col(3).X(), cameraTransform.Col(3).Y(), cameraTransform.Col(3).Z(), 1},
		Vertices:       vertices,
		Indices:        indices,
		Instances:      instances[:drawCount],
		Draws:          draws[:drawCount],
	}
	return c.DrawMeshesTo(a.viewport3D.view.renderTarget,
		preview3DTargetViewport(a.viewport3D.view.renderTarget),
		render.RenderTargetClear{}, batch)
}

// drawPreview3DAxes draws the labels for the depth-tested world axes. Text is
// still a 2D overlay; the axis geometry itself is submitted by
// drawPreview3DAxesDepth before the scene texture is composited.

func (a *ModuleEditor) drawPreview3DAxes(c *graphics.Canvas2D, projection *scene.Projection, cameraTransform mgl32.Mat4) {
	if c == nil || projection == nil {
		return
	}
	viewport := a.canvasBounds()
	clipFromWorld := projection.Matrix(viewport.W, max(viewport.H, 1)).Mul4(cameraTransform.Inv())
	for _, axis := range [...]struct {
		vector mgl32.Vec3
		key    string
		label  string
		color  graphics.ColorRGBA
	}{
		{vector: mgl32.Vec3{1, 0, 0}, key: "preview3d/axis/x", label: "X", color: editorTheme.axisX},
		{vector: mgl32.Vec3{0, 1, 0}, key: "preview3d/axis/y", label: "Y", color: editorTheme.axisY},
		{vector: mgl32.Vec3{0, 0, 1}, key: "preview3d/axis/z", label: "Z", color: editorTheme.axisZ},
	} {
		start, end, ok := projectPreview3DAxisRay(clipFromWorld, axis.vector)
		if ok && (mgl32.Vec2{end.X() - start.X(), end.Y() - start.Y()}).LenSqr() > 0.000001 {
			point := mgl32.Vec2{viewport.X + (end.X()+1)*viewport.W*0.5, viewport.Y + (1-end.Y())*viewport.H*0.5}
			a.drawPreview3DAxisLabel(c, axis.key, axis.label, point, viewport, axis.color)
		}
	}
}

func projectPreview3DAxisRay(clipFromWorld mgl32.Mat4, direction mgl32.Vec3) (mgl32.Vec3, mgl32.Vec3, bool) {
	origin, infinity := clipFromWorld.Col(3), clipFromWorld.Mul4x1(direction.Vec4(0))
	// Clipping a distant origin against an endpoint at infinity suffers severe
	// cancellation in float32, even when the final NDC coordinates are small.
	start := [4]float64{float64(origin.X()), float64(origin.Y()), float64(origin.Z()), float64(origin.W())}
	end := [4]float64{float64(infinity.X()), float64(infinity.Y()), float64(infinity.Z()), float64(infinity.W())}
	for _, plane := range [...][5]float64{
		{1, 0, 0, 1, 0}, {-1, 0, 0, 1, 0},
		{0, 1, 0, 1, 0}, {0, -1, 0, 1, 0},
		{0, 0, 1, 0, 0}, {0, 0, -1, 1, 0},
		{0, 0, 0, 1, -0.000001},
	} {
		first := plane[0]*start[0] + plane[1]*start[1] + plane[2]*start[2] + plane[3]*start[3] + plane[4]
		second := plane[0]*end[0] + plane[1]*end[1] + plane[2]*end[2] + plane[3]*end[3] + plane[4]
		if first < 0 && second < 0 {
			return mgl32.Vec3{}, mgl32.Vec3{}, false
		}
		if first < 0 {
			t := first / (first - second)
			for i := range start {
				start[i] += (end[i] - start[i]) * t
			}
		} else if second < 0 {
			t := second / (second - first)
			for i := range end {
				end[i] += (start[i] - end[i]) * t
			}
		}
	}
	return mgl32.Vec3{float32(start[0] / start[3]), float32(start[1] / start[3]), float32(start[2] / start[3])},
		mgl32.Vec3{float32(end[0] / end[3]), float32(end[1] / end[3]), float32(end[2] / end[3])}, true
}

func preview3DAxisRibbonOffset(viewport rect, start, end mgl32.Vec3, width float32) (mgl32.Vec3, bool) {
	delta := mgl32.Vec2{(end.X() - start.X()) * viewport.W * 0.5, (start.Y() - end.Y()) * viewport.H * 0.5}
	length := delta.Len()
	if length <= 0.5 {
		return mgl32.Vec3{}, false
	}
	// The outer ribbon includes coverage fringes. Across three texel intervals,
	// the middle interval is opaque and each outer interval fades to zero;
	// their integrated coverage retains the requested line width.
	outerWidth := width * 1.5
	return mgl32.Vec3{-delta.Y() * outerWidth / (length * viewport.W), -delta.X() * outerWidth / (length * viewport.H), 0}, true
}

func appendPreview3DAxisRibbonVertices(vertices *[]render.Vertex3D, indices *[]uint32, start, end, offset mgl32.Vec3, firstFade, lastFade float32) {
	base := uint32(len(*vertices))
	for i, point := range [...]mgl32.Vec3{start.Sub(offset), start.Add(offset), end.Add(offset), end.Sub(offset)} {
		u := [...]float32{0.125, 0.875, 0.875, 0.125}[i]
		fade := firstFade
		if i >= 2 {
			fade = lastFade
		}
		// Texel-center V coordinates interpolate coverage * distance fade.
		*vertices = append(*vertices, render.Vertex3D{Position: [3]float32(point), Normal: [3]float32{0, 1, 0}, UV: [2]float32{u, 0.25 + 0.5*fade}})
	}
	*indices = append(*indices, base, base+1, base+2, base, base+2, base+3)
}

func appendPreview3DFadedAxisRibbon(vertices *[]render.Vertex3D, indices *[]uint32, viewport rect, start, end mgl32.Vec3, width float32,
	clip mgl32.Mat4, axis, center mgl32.Vec3, radius float32, normal int, camera mgl32.Mat4, orthographic bool) {
	offset, ok := preview3DAxisRibbonOffset(viewport, start, end, width)
	if !ok {
		return
	}
	delta := end.Sub(start)
	length := (mgl32.Vec2{delta.X() * viewport.W * 0.5, delta.Y() * viewport.H * 0.5}).Len()
	segments := max(1, min(preview3DAxisFadeSegments, int(length)))
	origin, direction := clip.Col(3), clip.Mul4x1(axis.Vec4(0))
	first := start
	firstFade := preview3DAxisFade(first, origin, direction, axis, center, radius, normal, camera, orthographic)
	for segment := 1; segment <= segments; segment++ {
		last := start.Add(delta.Mul(float32(segment) / float32(segments)))
		lastFade := preview3DAxisFade(last, origin, direction, axis, center, radius, normal, camera, orthographic)
		if firstFade > 0 || lastFade > 0 {
			// Share one offset across all segments to keep the edges collinear.
			appendPreview3DAxisRibbonVertices(vertices, indices, first, last, offset, firstFade, lastFade)
		}
		first, firstFade = last, lastFade
	}
}

func preview3DAxisFade(point mgl32.Vec3, origin, direction mgl32.Vec4, axis, center mgl32.Vec3, radius float32, normal int, camera mgl32.Mat4, orthographic bool) float32 {
	// Recover the world-axis coordinate directly, including when the origin is
	// outside the frustum. At a vanishing point the denominator is zero: fade out.
	component := 0
	denominator := float64(direction[0]) - float64(point[0])*float64(direction[3])
	for i := 1; i < 3; i++ {
		candidate := float64(direction[i]) - float64(point[i])*float64(direction[3])
		if math.Abs(candidate) > math.Abs(denominator) {
			component, denominator = i, candidate
		}
	}
	if denominator == 0 {
		return 0
	}
	coordinate := (float64(point[component])*float64(origin[3]) - float64(origin[component])) / denominator
	worldPoint := axis.Mul(float32(coordinate))
	distance := worldPoint.Sub(center).Len()
	fade := 1 - preview3DSmoothstep(radius*0.4, radius, distance)
	// Grazing-angle fade belongs to lines lying in the grid plane. Applying
	// it to the normal axis punches a transparent gap at camera height.
	if fade == 0 || axis[normal] != 0 {
		return fade
	}
	view := camera.Col(2).Vec3()
	if !orthographic {
		view = camera.Col(3).Vec3().Sub(worldPoint)
		view = view.Mul(1 / max(view.Len(), 0.000001))
	}
	return fade * preview3DSmoothstep(0.015, 0.15, abs(view[normal]))
}

func preview3DSmoothstep(low, high, value float32) float32 {
	t := clamp((value-low)/(high-low), 0, 1)
	return t * t * (3 - 2*t)
}

func (a *ModuleEditor) drawPreview3DAxisLabel(c *graphics.Canvas2D, key, label string, endpoint mgl32.Vec2, viewport rect, color graphics.ColorRGBA) {
	x, y := preview3DAxisLabelPosition(endpoint, viewport)
	a.drawViewport3DText(c, key, label, mgl32.Vec2{x, y}, 12, scene.AnchorTopLeft, color)
}

func preview3DAxisLabelPosition(endpoint mgl32.Vec2, viewport rect) (float32, float32) {
	const (
		labelWidth  = float32(14)
		labelHeight = float32(18)
	)
	minX := viewport.X + 4
	maxX := max(minX, viewport.X+viewport.W-labelWidth)
	minY := viewport.Y + 4
	maxY := max(minY, viewport.Y+viewport.H-labelHeight)
	return clamp(endpoint.X()+6, minX, maxX), clamp(endpoint.Y()-14, minY, maxY)
}

func drawPreview3DScreenLine(c *graphics.Canvas2D, start, end mgl32.Vec2, width float32, color graphics.ColorRGBA) {
	delta := end.Sub(start)
	lineLength := delta.Len()
	if lineLength <= 0.5 {
		if lineLength <= 0.001 {
			c.DrawShapeMatrices(
				mgl32.Translate3D(start.X()-width*0.5, start.Y()-width*0.5, 0).Mul4(mgl32.Scale3D(width, width, 1)),
				mgl32.Ident3(), width, width, render.ShapeRoundedRect,
				color, graphics.ColorRGBA{}, width*0.5, 0, 1,
			)
		}
		return
	}
	model := mgl32.Translate3D(start.X(), start.Y(), 0).
		Mul4(mgl32.HomogRotate3DZ(float32(math.Atan2(float64(delta.Y()), float64(delta.X()))))).
		Mul4(mgl32.Translate3D(0, -width*0.5, 0)).
		Mul4(mgl32.Scale3D(lineLength, width, 1))
	c.DrawShapeMatrices(model, mgl32.Ident3(), lineLength, width, render.ShapeRoundedRect,
		color, graphics.ColorRGBA{}, width*0.5, 0, 1)
}

const preview3DAxisFadeSegments = 64

func preview3DTargetViewport(target *render.RenderTarget) render.Rect {
	width, height := target.Size()
	return render.Rect{Width: int32(width), Height: int32(height)}
}

func preview3DWorldPerPixel(projection *scene.Projection, depth float32, viewport rect) float32 {
	if projection.Kind == scene.ProjectionOrthographic {
		p := projection.Orthographic
		p.Update(viewport.W, max(viewport.H, 1))
		return (p.Area.MaxY - p.Area.MinY) / max(viewport.H, 1)
	}
	return 2 * max(depth, .001) * float32(math.Tan(float64(projection.Perspective.Fov)*.5)) / max(viewport.H, 1)
}

func (a *ModuleEditor) drawViewport3DText(c *graphics.Canvas2D, key, value string, position mgl32.Vec2, size float32, anchor scene.Anchor, color graphics.ColorRGBA) {
	labels := a.viewport3D.view.labels
	label := labels[key]
	if label == nil {
		text := scene.NewText2D(value)
		text.Font, text.FontSize = a.font, float64(size)
		label = &text
		labels[key] = label
	}
	label.Value, label.Color = value, color
	scene.DrawScreenText2D(c, label, mgl32.Translate3D(position.X(), position.Y(), 0), anchor)
}
