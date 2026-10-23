package editor

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/go-gl/mathgl/mgl32"
)

const preview3DHelperPickRadius = float32(6)

// One stack-resident buffer covers a camera and every light component on a
// single entity. Drawing, picking and framing consume the same world segments.
type preview3DHelperGeometry struct {
	lines     [512][2]mgl32.Vec3
	count     int
	bodyCount int // Frame physical geometry, independently of screen-sized icons.
}

var preview3DHelperCircle = func() (points [32]mgl32.Vec2) {
	for i := range points {
		s, c := math.Sincos(float64(i) * 2 * math.Pi / float64(len(points)))
		points[i] = mgl32.Vec2{float32(c), float32(s)}
	}
	return
}()

func viewport3DHasHelpers(c project.ModComponents) bool {
	return c.Camera != nil || c.AmbientLight != nil || c.DirectionalLight != nil || c.PointLight != nil || c.SpotLight != nil || c.AreaLight != nil
}

func (g *preview3DHelperGeometry) line(start, end mgl32.Vec3) {
	g.lines[g.count] = [2]mgl32.Vec3{start, end}
	g.count++
}

func (g *preview3DHelperGeometry) circle(center, right, up mgl32.Vec3) {
	previous := center.Add(right)
	for i := 1; i <= len(preview3DHelperCircle); i++ {
		point := preview3DHelperCircle[i%len(preview3DHelperCircle)]
		next := center.Add(right.Mul(point.X())).Add(up.Mul(point.Y()))
		g.line(previous, next)
		previous = next
	}
}

func (g *preview3DHelperGeometry) frame(center, right, up mgl32.Vec3) (corners [4]mgl32.Vec3) {
	corners = [4]mgl32.Vec3{center.Sub(right).Sub(up), center.Add(right).Sub(up), center.Add(right).Add(up), center.Sub(right).Add(up)}
	for i, corner := range corners {
		g.line(corner, corners[(i+1)%len(corners)])
	}
	return
}

func (g *preview3DHelperGeometry) direction(origin, forward, right mgl32.Vec3, length float32) {
	end := origin.Add(forward.Mul(length))
	g.line(origin, end)
	back := end.Sub(forward.Mul(length * .2))
	g.line(end, back.Add(right.Mul(length*.1)))
	g.line(end, back.Sub(right.Mul(length*.1)))
}

func (a *ModuleEditor) viewport3DCameraAspect(spec *project.Camera3DSpec) float32 {
	width, height := float32(a.Document.Spec.Viewport.Width), float32(a.Document.Spec.Viewport.Height)
	if width <= 0 {
		width = float32(a.Document.Spec.Window.Width)
	}
	if height <= 0 {
		height = float32(a.Document.Spec.Window.Height)
	}
	if spec.Viewport.Width > 0 {
		width = spec.Viewport.Width
	}
	if spec.Viewport.Height > 0 {
		height = spec.Viewport.Height
	}
	return max(width, 1) / max(height, 1)
}

func (g *preview3DHelperGeometry) camera(matrix mgl32.Mat4, spec *project.Camera3DSpec, aspect float32) {
	height := float32(math.Tan(float64(spec.Fov) * .5))
	width := height * aspect
	var corners [4]mgl32.Vec3
	for i, sign := range [...][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		corners[i] = matrix.Mul4x1(mgl32.Vec4{sign[0] * width, sign[1] * height, -1, 1}).Vec3()
		g.line(matrix.Col(3).Vec3(), corners[i])
	}
	for i, corner := range corners {
		g.line(corner, corners[(i+1)%len(corners)])
	}
	// Blender's triangular top marker makes the camera's roll visible.
	left := matrix.Mul4x1(mgl32.Vec4{-width * .35, height, -1, 1}).Vec3()
	right := matrix.Mul4x1(mgl32.Vec4{width * .35, height, -1, 1}).Vec3()
	top := matrix.Mul4x1(mgl32.Vec4{0, height + min(width, height)*.6, -1, 1}).Vec3()
	g.line(left, top)
	g.line(top, right)
	g.line(right, left)
}

// Match the renderer's orthonormal light basis: emitter dimensions are in
// meters and stay independent of transform scale, including scaled parents.
func preview3DHelperLightBasis(matrix mgl32.Mat4) (right, up, forward mgl32.Vec3) {
	forward = matrix.Col(2).Vec3().Mul(-1)
	if forward.LenSqr() < 1e-12 {
		forward = mgl32.Vec3{0, 0, -1}
	}
	forward = forward.Normalize()
	right = matrix.Col(0).Vec3()
	right = right.Sub(forward.Mul(right.Dot(forward)))
	if right.LenSqr() < 1e-12 {
		right = forward.Cross(mgl32.Vec3{0, 1, 0})
		if right.LenSqr() < 1e-12 {
			right = mgl32.Vec3{1, 0, 0}
		}
	}
	right = right.Normalize()
	up = right.Cross(forward).Normalize()
	return
}

func (a *ModuleEditor) viewport3DHelperGeometry(part *workspace3DPart, projection *scene.Projection, camera mgl32.Mat4, viewport rect, g *preview3DHelperGeometry) {
	g.count, g.bodyCount = 0, 0
	if !part.helper || part.entity == 0 {
		return
	}
	world := a.viewport3D.world
	matrix := ecs.GetRef[scene.GlobalTransform](world, part.entity).Matrix
	if part.cameraSpec != nil {
		g.camera(matrix, part.cameraSpec, a.viewport3DCameraAspect(part.cameraSpec))
	}
	origin := matrix.Col(3).Vec3()
	right, up, forward := preview3DHelperLightBasis(matrix)
	depth := origin.Sub(camera.Col(3).Vec3()).Dot(camera.Col(2).Vec3().Mul(-1))
	units := preview3DWorldPerPixel(projection, depth, viewport)
	iconRight, iconUp := camera.Col(0).Vec3().Mul(units), camera.Col(1).Vec3().Mul(units)
	ambient := ecs.Has[render3d.AmbientLight](world, part.entity)
	sun := ecs.Has[render3d.DirectionalLight](world, part.entity)
	hasLight := ambient || sun
	if sun {
		g.direction(origin, forward, right, 1)
	}
	if light := ecs.GetRef[render3d.PointLight](world, part.entity); light != nil {
		hasLight = true
		if light.Radius > 0 {
			x, y, z := right.Mul(light.Radius), up.Mul(light.Radius), forward.Mul(light.Radius)
			g.circle(origin, x, y)
			g.circle(origin, x, z)
			g.circle(origin, y, z)
		}
	}
	if light := ecs.GetRef[render3d.SpotLight](world, part.entity); light != nil {
		hasLight = true
		end := origin.Add(forward.Mul(light.Range))
		switch light.Shape {
		case render3d.SpotBox:
			near := g.frame(origin, right.Mul(light.Width*.5), up.Mul(light.Height*.5))
			far := g.frame(end, right.Mul(light.Width*.5), up.Mul(light.Height*.5))
			for i := range near {
				g.line(near[i], far[i])
			}
		case render3d.SpotPyramid:
			x, y := light.PyramidTangents()
			for _, corner := range g.frame(end, right.Mul(light.Range*x), up.Mul(light.Range*y)) {
				g.line(origin, corner)
			}
		default:
			radius := light.Range * float32(math.Tan(float64(light.OuterConeAngle)*.5))
			g.circle(end, right.Mul(radius), up.Mul(radius))
			for _, vector := range [...]mgl32.Vec3{right, up, right.Mul(-1), up.Mul(-1)} {
				g.line(origin, end.Add(vector.Mul(radius)))
			}
			if light.InnerConeAngle > 0 && light.InnerConeAngle < light.OuterConeAngle {
				inner := light.Range * float32(math.Tan(float64(light.InnerConeAngle)*.5))
				g.circle(end, right.Mul(inner), up.Mul(inner))
			}
		}
		g.direction(origin, forward, right, min(light.Range, .6))
	}
	if light := ecs.GetRef[render3d.AreaLight](world, part.entity); light != nil {
		hasLight = true
		switch light.Shape {
		case render3d.AreaDisc:
			g.circle(origin, right.Mul(light.Radius), up.Mul(light.Radius))
		case render3d.AreaTube:
			start, end := origin.Sub(right.Mul(light.Width*.5)), origin.Add(right.Mul(light.Width*.5))
			g.line(start, end)
			g.line(start.Sub(up.Mul(.05)), start.Add(up.Mul(.05)))
			g.line(end.Sub(up.Mul(.05)), end.Add(up.Mul(.05)))
		default:
			g.frame(origin, right.Mul(light.Width*.5), up.Mul(light.Height*.5))
		}
		if light.Shape != render3d.AreaTube {
			g.direction(origin, forward, right, .6)
		}
	}
	g.bodyCount = g.count
	if hasLight {
		// Keep the source selectable even when its physical radius is zero or
		// the emission plane is seen edge-on.
		g.line(origin, origin)
		g.circle(origin, iconRight.Mul(8), iconUp.Mul(8))
	}
	if ambient {
		g.circle(origin, iconRight.Mul(4), iconUp.Mul(4))
	}
	if sun {
		for i := 0; i < len(preview3DHelperCircle); i += 4 {
			p := preview3DHelperCircle[i]
			vector := iconRight.Mul(p.X()).Add(iconUp.Mul(p.Y()))
			g.line(origin.Add(vector.Mul(11)), origin.Add(vector.Mul(17)))
		}
	}
}

// Clip before dividing by W. The renderer uses reverse Z in [0, W], in both
// perspective and orthographic views. Picking uses these exact visible strokes.
func preview3DProjectHelperLine(clip mgl32.Mat4, viewport rect, line [2]mgl32.Vec3) (start, end mgl32.Vec3, ok bool) {
	first, second := clip.Mul4x1(line[0].Vec4(1)), clip.Mul4x1(line[1].Vec4(1))
	for _, plane := range [...][5]float32{
		{1, 0, 0, 1, 0}, {-1, 0, 0, 1, 0}, {0, 1, 0, 1, 0}, {0, -1, 0, 1, 0},
		{0, 0, 1, 0, 0}, {0, 0, -1, 1, 0}, {0, 0, 0, 1, -1e-6},
	} {
		d1 := first.Dot(mgl32.Vec4{plane[0], plane[1], plane[2], plane[3]}) + plane[4]
		d2 := second.Dot(mgl32.Vec4{plane[0], plane[1], plane[2], plane[3]}) + plane[4]
		if d1 < 0 && d2 < 0 {
			return start, end, false
		}
		if d1 < 0 {
			first = first.Add(second.Sub(first).Mul(d1 / (d1 - d2)))
		} else if d2 < 0 {
			second = second.Add(first.Sub(second).Mul(d2 / (d2 - d1)))
		}
	}
	start = mgl32.Vec3{viewport.X + (first.X()/first.W()+1)*viewport.W*.5, viewport.Y + (1-first.Y()/first.W())*viewport.H*.5, first.Z() / first.W()}
	end = mgl32.Vec3{viewport.X + (second.X()/second.W()+1)*viewport.W*.5, viewport.Y + (1-second.Y()/second.W())*viewport.H*.5, second.Z() / second.W()}
	return start, end, true
}

func (g *preview3DHelperGeometry) hit(clip mgl32.Mat4, viewport rect, point mgl32.Vec2) (float32, bool) {
	best := float32(-1)
	for _, line := range g.lines[:g.count] {
		start, end, ok := preview3DProjectHelperLine(clip, viewport, line)
		if !ok {
			continue
		}
		delta := end.Vec2().Sub(start.Vec2())
		t := float32(0)
		if length := delta.LenSqr(); length > 1e-6 {
			t = clamp(point.Sub(start.Vec2()).Dot(delta)/length, 0, 1)
		}
		if point.Sub(start.Vec2().Add(delta.Mul(t))).LenSqr() <= preview3DHelperPickRadius*preview3DHelperPickRadius {
			best = max(best, start.Z()+(end.Z()-start.Z())*t)
		}
	}
	return best, best >= 0
}

func preview3DHelperColor(selected bool) graphics.ColorRGBA {
	if selected {
		return rgba(255, 175, 41, 255)
	}
	return rgba(148, 154, 163, 230)
}

func (a *ModuleEditor) drawViewport3DHelpers(c *graphics.Canvas2D, projection *scene.Projection, camera mgl32.Mat4) {
	v, viewport := &a.viewport3D, a.canvasBounds()
	clip := projection.Matrix(viewport.W, viewport.H).Mul4(camera.Inv())
	showSelection := a.selectedResource == "" && a.selectedSlot == "" && !a.slotCreating
	var geometry preview3DHelperGeometry
	for index := range v.rows {
		row := &v.rows[index]
		parent := a.Preview.Items[index].Parent
		row.selected = showSelection && (a.isSelected(row.id) || a.Local != "" && a.Selected == a.editingModule.ID || a.selectedElement == "" && parent >= 0 && v.rows[parent].selected)
		mod := a.previewDocument().Mods[row.mod]
		for i := range row.parts {
			part := &row.parts[i]
			part.selected = row.selected && (a.selectedElement == "" || i > 0 && mod.Plan.Elements[i-1].Spec.ID == a.selectedElement)
			if i > 0 && mod.Plan.Elements[i-1].Parent >= 0 {
				part.selected = part.selected || row.parts[mod.Plan.Elements[i-1].Parent+1].selected
			}
			if !part.helper || part.entity == 0 || !ecs.GetRef[scene.Visibility](v.world, part.entity).Visible {
				continue
			}
			a.viewport3DHelperGeometry(part, projection, camera, viewport, &geometry)
			color := preview3DHelperColor(part.selected)
			for _, line := range geometry.lines[:geometry.count] {
				if start, end, ok := preview3DProjectHelperLine(clip, viewport, line); ok {
					drawPreview3DScreenLine(c, start.Vec2(), end.Vec2(), 1.25, color)
				}
			}
		}
	}
}
