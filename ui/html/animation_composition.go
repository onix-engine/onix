package html

import (
	"math"

	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
	"github.com/go-gl/mathgl/mgl32"
)

// Compose in page coordinates, then shift the result into each visual's local
// coordinates. Float64 avoids cancellation at distant scroll/layout origins.
// No inverse is needed, so singular CSS transforms remain valid.
type htmlAnimationAffine struct {
	a, b, c, d, x, y float64
}

func (outer htmlAnimationAffine) mul(inner htmlAnimationAffine) htmlAnimationAffine {
	return htmlAnimationAffine{
		a: outer.a*inner.a + outer.c*inner.b,
		b: outer.b*inner.a + outer.d*inner.b,
		c: outer.a*inner.c + outer.c*inner.d,
		d: outer.b*inner.c + outer.d*inner.d,
		x: outer.a*inner.x + outer.c*inner.y + outer.x,
		y: outer.b*inner.x + outer.d*inner.y + outer.y,
	}
}

func (matrix htmlAnimationAffine) shifted(shift mgl32.Vec2) mgl32.Mat4 {
	x, y := float64(shift.X()), float64(shift.Y())
	return mgl32.Mat4{
		float32(matrix.a), float32(matrix.b), 0, 0,
		float32(matrix.c), float32(matrix.d), 0, 0,
		0, 0, 1, 0,
		float32(matrix.x + x - matrix.a*x - matrix.c*y),
		float32(matrix.y + y - matrix.b*x - matrix.d*y), 0, 1,
	}
}

type htmlAnimationVisualValue struct {
	Foreground  style.AnimationColor
	Opacity     float64
	Transform   htmlAnimationTransformValue
	Transformed bool
}

type htmlAnimationTransformRoute struct {
	Node, Stop *dom.Node
}

type htmlAnimationTransformValue struct {
	Matrix htmlAnimationAffine
	Count  int
}

// Retain storage, but never retain results across operations: style, boxes and
// generated ECS parents can all change without advancing the animation clock.
type htmlAnimationComposition struct {
	Values     htmlAncestorCache[*dom.Node, htmlAnimationVisualValue]
	Transforms htmlAncestorCache[htmlAnimationTransformRoute, htmlAnimationTransformValue]
	Batch      bool
}

func (cache *htmlAnimationComposition) clear() {
	cache.Values.clear()
	cache.Transforms.clear()
}

func (state *htmlRenderState) animationVisualValue(node *dom.Node) htmlAnimationVisualValue {
	return state.AnimationComposition.Values.resolve(node, htmlNodeParent, func(current *dom.Node, inherited htmlAnimationVisualValue) htmlAnimationVisualValue {
		if current.Parent == nil {
			inherited.Opacity = 1
			inherited.Foreground = style.AnimationColor{A: 1}
		}
		result := htmlAnimationVisualValue{Opacity: inherited.Opacity, Foreground: inherited.Foreground, Transform: inherited.Transform}
		computed := state.Styles[current]
		if computed == nil || current.Type != dom.ElementNode {
			return result
		}
		var value style.AnimationValue
		if animation := state.Animations[current]; animation != nil {
			value = animation.Current
		} else {
			value = style.BaseAnimationValue(computed)
		}
		result.Opacity *= value.Opacity
		result.Foreground = value.Colors[style.AnimationForeground].Resolve(inherited.Foreground)
		local, transformed := state.animationLocalTransform(current, computed, value)
		result.Transformed = transformed
		if transformed {
			result.Transform.append(local)
		}
		return result
	})
}

func (state *htmlRenderState) animationLocalTransform(node *dom.Node, computed *style.ComputedStyle, value style.AnimationValue) (htmlAnimationAffine, bool) {
	box := state.boxForNode(node)
	if box == nil {
		return htmlAnimationAffine{}, false
	}
	rect := box.BorderBox()
	tx, ty := style.ResolveAnimatedTranslation(value, rect.Width, rect.Height)
	if value.Rotation == 0 && value.SkewX == 0 && value.ScaleX == 1 && value.ScaleY == 1 && tx == 0 && ty == 0 {
		return htmlAnimationAffine{}, false
	}
	sin, cos := math.Sincos(value.Rotation * math.Pi / 180)
	skew := math.Tan(value.SkewX * math.Pi / 180)
	x := rect.X + style.ResolveLength(computed.TransformOriginX, rect.Width)
	y := rect.Y + style.ResolveLength(computed.TransformOriginY, rect.Height)
	local := htmlAnimationAffine{a: cos * value.ScaleX, b: sin * value.ScaleX,
		c: (cos*skew - sin) * value.ScaleY, d: (sin*skew + cos) * value.ScaleY}
	local.x, local.y = x+tx-local.a*x-local.c*y, y+ty-local.b*x-local.d*y
	return local, true
}

func (value *htmlAnimationTransformValue) append(local htmlAnimationAffine) {
	if value.Count == 0 {
		value.Matrix = local
	} else {
		value.Matrix = value.Matrix.mul(local)
	}
	value.Count++
}

func (state *htmlRenderState) animationTransformValue(node, stop *dom.Node, full htmlAnimationTransformValue) htmlAnimationTransformValue {
	if node == nil || node == stop {
		return htmlAnimationTransformValue{}
	}
	// Most visuals have no generated parent, or have one without transformed
	// ancestors. Reuse the full composition without a second cache lookup.
	if stop == nil || state.animationVisualValue(stop).Transform.Count == 0 {
		return full
	}
	return state.AnimationComposition.Transforms.resolve(htmlAnimationTransformRoute{node, stop}, func(route htmlAnimationTransformRoute) htmlAnimationTransformRoute {
		parent := route.Node.Parent
		if parent == nil || parent == route.Stop {
			return htmlAnimationTransformRoute{}
		}
		return htmlAnimationTransformRoute{parent, route.Stop}
	}, func(route htmlAnimationTransformRoute, inherited htmlAnimationTransformValue) htmlAnimationTransformValue {
		if !state.animationVisualValue(route.Node).Transformed {
			return inherited
		}
		computed := state.Styles[route.Node]
		var value style.AnimationValue
		if animation := state.Animations[route.Node]; animation != nil {
			value = animation.Current
		} else {
			value = style.BaseAnimationValue(computed)
		}
		local, _ := state.animationLocalTransform(route.Node, computed, value)
		inherited.append(local)
		return inherited
	})
}
