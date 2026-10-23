package editor

import (
	"math"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	preview3DOrientationNone = 0
	// Actions 1–6 are -X, +X, -Y, +Y, -Z, +Z.
	preview3DOrientationOrbit      = 7
	preview3DOrientationProjection = 8
	preview3DOrientationRadius     = float32(33)
	preview3DOrientationNodeRadius = float32(12)
)

type preview3DOrientationState struct {
	hover, action int
	dragging      bool
	moved         bool
	press         mgl32.Vec2
	yaw, pitch    float32
}

type preview3DOrientationNode struct {
	point  mgl32.Vec2
	depth  float32
	action int
}

type preview3DOrientationLayout struct {
	bounds, projection rect
	center             mgl32.Vec2
	nodes              [6]preview3DOrientationNode
}

func (a *ModuleEditor) preview3DOrientationRect() rect {
	v := a.canvasBounds()
	const width, height, margin = float32(112), float32(140), float32(12)
	y := v.Y + margin
	if v.W < 288 {
		y += 34 // Keep the 2D/3D and View controls accessible in narrow viewports.
	}
	if v.W < width+margin*2 || y+height > v.Y+v.H-32 {
		return rect{}
	}
	return rect{X: v.X + v.W - width - margin, Y: y, W: width, H: height}
}

func preview3DOrientationGeometry(bounds rect, camera mgl32.Mat4) preview3DOrientationLayout {
	layout := preview3DOrientationLayout{
		bounds:     bounds,
		projection: rect{X: bounds.X + 8, Y: bounds.Y + 110, W: bounds.W - 16, H: 22},
		center:     mgl32.Vec2{bounds.X + bounds.W*0.5, bounds.Y + 53},
	}
	right, up, forward := camera.Col(0).Vec3(), camera.Col(1).Vec3(), camera.Col(2).Vec3()
	for i := range layout.nodes {
		axis, sign := i/2, float32((i%2)*2-1)
		node := preview3DOrientationNode{
			point:  layout.center.Add(mgl32.Vec2{right[axis], -up[axis]}.Mul(sign * preview3DOrientationRadius)),
			depth:  forward[axis] * sign,
			action: i + 1,
		}
		// Six stack-resident nodes, ordered back to front for drawing and picking.
		j := i
		for j > 0 && layout.nodes[j-1].depth > node.depth {
			layout.nodes[j] = layout.nodes[j-1]
			j--
		}
		layout.nodes[j] = node
	}
	return layout
}

func (layout *preview3DOrientationLayout) hit(x, y float32) int {
	if layout.bounds.W <= 0 || x < layout.bounds.X || y < layout.bounds.Y || x >= layout.bounds.X+layout.bounds.W || y >= layout.bounds.Y+layout.bounds.H {
		return preview3DOrientationNone
	}
	p := layout.projection
	if x >= p.X && y >= p.Y && x < p.X+p.W && y < p.Y+p.H {
		return preview3DOrientationProjection
	}
	point := mgl32.Vec2{x, y}
	for i := len(layout.nodes) - 1; i >= 0; i-- {
		if layout.nodes[i].point.Sub(point).LenSqr() <= (preview3DOrientationNodeRadius+2)*(preview3DOrientationNodeRadius+2) {
			return layout.nodes[i].action
		}
	}
	return preview3DOrientationOrbit
}

func (a *ModuleEditor) handlePreview3DOrientation(in *input.State) bool {
	v := &a.viewport3D.view
	s := &v.orientation
	if a.canvasDrag.ID != "" {
		s.hover = preview3DOrientationNone
		return false
	}
	if a.viewport3D.world == nil {
		*s = preview3DOrientationState{}
		return false
	}
	camera := preview3DCameraTransformAround(v.target, v.yaw, v.pitch, v.distance).LocalMatrix()
	layout := preview3DOrientationGeometry(a.preview3DOrientationRect(), camera)
	p := in.Pointer()
	hover := layout.hit(p.X, p.Y)
	if s.hover != hover || s.dragging || p.Pressed {
		a.changedCanvas()
	}
	s.hover = hover
	if !s.dragging {
		if !p.Pressed {
			return s.hover != preview3DOrientationNone
		}
		action := layout.hit(p.PressX, p.PressY)
		if action == preview3DOrientationNone {
			return false
		}
		s.action, s.dragging, s.moved = action, true, false
		s.press, s.yaw, s.pitch = mgl32.Vec2{p.PressX, p.PressY}, v.yaw, v.pitch
	}
	if in.KeyPressed(input.KeyEscape) {
		v.yaw, v.pitch = s.yaw, s.pitch
		s.dragging = false
		return true
	}
	delta := mgl32.Vec2{p.X, p.Y}.Sub(s.press)
	if delta.LenSqr() > 16 {
		s.moved = true
	}
	if s.moved && s.action != preview3DOrientationProjection {
		v.yaw, v.pitch = s.yaw-delta.X()*0.01, s.pitch-delta.Y()*0.01
	}
	if p.Released {
		if !s.moved && s.hover == s.action {
			if s.action == preview3DOrientationProjection {
				v.orthographic = !v.orthographic
				a.syncViewport3DProjection()
			} else if s.action < preview3DOrientationOrbit {
				a.snapPreview3DOrientation((s.action-1)/2, ((s.action-1)%2)*2-1)
			}
		}
		s.dragging = false
	} else if !p.Down {
		s.dragging = false
	}
	return true
}

func (a *ModuleEditor) snapPreview3DOrientation(axis, sign int) {
	v := &a.viewport3D.view
	camera := preview3DCameraTransformAround(v.target, v.yaw, v.pitch, v.distance).LocalMatrix()
	// Clicking the facing pole again exposes the opposite side, including the
	// two nodes that overlap when looking straight along an axis.
	if v.orthographic && camera.Col(2)[axis]*float32(sign) > 0.999 {
		sign = -sign
	}
	switch axis {
	case 0:
		v.yaw, v.pitch = float32(sign)*math.Pi/2, 0
	case 1:
		v.yaw, v.pitch = 0, float32(sign)*math.Pi/2
	case 2:
		v.yaw, v.pitch = 0, 0
		if sign < 0 {
			v.yaw = math.Pi
		}
	}
	v.orthographic = true
	a.syncViewport3DProjection()
}

func (a *ModuleEditor) drawPreview3DOrientation(c *graphics.Canvas2D, camera mgl32.Mat4) {
	bounds := a.preview3DOrientationRect()
	if c == nil || bounds.W <= 0 {
		return
	}
	layout := preview3DOrientationGeometry(bounds, camera)
	s := &a.viewport3D.view.orientation
	border := editorTheme.panelBorder
	if s.hover != preview3DOrientationNone || s.dragging {
		border = editorTheme.accent
	}
	drawPreview3DOrientationShape(c, bounds, 14, rgba(24, 27, 34, 242), border, 1)
	center := layout.center
	drawPreview3DOrientationShape(c, rect{X: center.X() - 47, Y: center.Y() - 47, W: 94, H: 94}, 47,
		editorTheme.panelSoft, editorTheme.panelSoft, 1)
	drawPreview3DOrientationShape(c, rect{X: center.X() - 3, Y: center.Y() - 3, W: 6, H: 6}, 3,
		editorTheme.textMuted, graphics.ColorRGBA{}, 0)
	for _, node := range layout.nodes {
		index := node.action - 1
		color := [...]graphics.ColorRGBA{editorTheme.axisX, editorTheme.axisY, editorTheme.axisZ}[index/2]
		stem := color
		stem.A = 0.85
		fill, text := color, editorTheme.text
		if index%2 == 0 {
			stem.A = 0.35
			fill, text = editorTheme.panelSoft, color
		}
		if node.depth < 0 {
			stem.A *= 0.5
			fill.R, fill.G, fill.B = fill.R*0.7, fill.G*0.7, fill.B*0.7
		}
		point := node.point
		drawPreview3DScreenLine(c, center, point, 2, stem)
		hover := s.hover == node.action
		active := a.viewport3D.view.orthographic && node.depth > 0.999
		if hover || active {
			ring := color
			if hover {
				ring = editorTheme.text
			}
			drawPreview3DOrientationShape(c, rect{X: point.X() - 15, Y: point.Y() - 15, W: 30, H: 30}, 15,
				graphics.ColorRGBA{}, ring, 1.5)
		}
		if hover {
			fill.R, fill.G, fill.B = min(fill.R+0.15, 1), min(fill.G+0.15, 1), min(fill.B+0.15, 1)
			text = editorTheme.text
		}
		drawPreview3DOrientationShape(c, rect{X: point.X() - 12, Y: point.Y() - 12, W: 24, H: 24}, 12, fill, color, 1)
		a.drawPreview3DOrientationText(c,
			[...]string{"preview3d/orientation/-x", "preview3d/orientation/x", "preview3d/orientation/-y", "preview3d/orientation/y", "preview3d/orientation/-z", "preview3d/orientation/z"}[index],
			[...]string{"−X", "X", "−Y", "Y", "−Z", "Z"}[index], point, text)
	}
	fill := editorTheme.panelSoft
	if s.hover == preview3DOrientationProjection {
		fill = editorTheme.hover
	}
	drawPreview3DOrientationShape(c, layout.projection, 6, fill, border, 1)
	a.drawPreview3DOrientationText(c, "preview3d/orientation/projection",
		[...]string{"Persp · O", "Ortho · O"}[boolIndex(a.viewport3D.view.orthographic)],
		mgl32.Vec2{layout.projection.X + layout.projection.W*0.5, layout.projection.Y + layout.projection.H*0.5}, editorTheme.textMuted)
}

func drawPreview3DOrientationShape(c *graphics.Canvas2D, bounds rect, radius float32, fill, stroke graphics.ColorRGBA, strokeWidth float32) {
	c.DrawShapeMatrices(mgl32.Translate3D(bounds.X, bounds.Y, 0).Mul4(mgl32.Scale3D(bounds.W, bounds.H, 1)), mgl32.Ident3(), bounds.W, bounds.H,
		render.ShapeRoundedRect, fill, stroke, radius, strokeWidth, 1)
}

func (a *ModuleEditor) drawPreview3DOrientationText(c *graphics.Canvas2D, key, value string, center mgl32.Vec2, color graphics.ColorRGBA) {
	text := a.viewport3D.view.labels[key]
	if text == nil {
		label := scene.NewText2D("")
		label.Font, label.FontSize = a.font, 11
		text = &label
		a.viewport3D.view.labels[key] = text
	}
	text.Value, text.Color = value, color
	scene.DrawScreenText2D(c, text, mgl32.Translate3D(center.X(), center.Y(), 0), scene.AnchorCenter)
}
