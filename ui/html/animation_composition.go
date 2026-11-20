package html

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/html/style"
	uinode "github.com/onix-engine/onix/scene/node"
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
	Node, Stop *uinode.Node
}

type htmlAnimationTransformValue struct {
	Matrix htmlAnimationAffine
	Count  int
}

// Retain storage, but never retain results across operations: style, boxes and
// generated ECS parents can all change without advancing the animation clock.
type htmlAnimationComposition struct {
	Values     htmlAncestorCache[*uinode.Node, htmlAnimationVisualValue]
	Transforms htmlAncestorCache[htmlAnimationTransformRoute, htmlAnimationTransformValue]
	Batch      bool
}

func (cache *htmlAnimationComposition) clear() {
	cache.Values.clear()
	cache.Transforms.clear()
}

func (state *htmlRenderState) animationVisualValue(node *uinode.Node) htmlAnimationVisualValue {
	return state.AnimationComposition.Values.resolve(node, htmlNodeParent, func(current *uinode.Node, inherited htmlAnimationVisualValue) htmlAnimationVisualValue {
		if current.Parent == nil {
			inherited.Opacity = 1
			inherited.Foreground = style.AnimationColor{A: 1}
		}
		result := htmlAnimationVisualValue{Opacity: inherited.Opacity, Foreground: inherited.Foreground, Transform: inherited.Transform}
		computed := state.Styles[current]
		if computed == nil || current.Type != uinode.ElementNode {
			return result
		}
		animation := state.Animations[current]
		if animation != nil {
			result.Opacity *= animation.Current.Opacity
			result.Foreground = animation.Current.Colors[style.AnimationForeground].Resolve(inherited.Foreground)
		} else {
			result.Opacity *= computed.Opacity.Number
			if !computed.ColorInherited {
				result.Foreground = style.ColorForAnimation(computed.Color).Resolve(inherited.Foreground)
			}
		}
		local, transformed := state.animationLocalTransform(current, computed, animation)
		result.Transformed = transformed
		if transformed {
			result.Transform.append(local)
		}
		return result
	})
}

func (state *htmlRenderState) animationLocalTransform(node *uinode.Node, computed *style.ComputedStyle, animation *htmlNodeAnimation) (htmlAnimationAffine, bool) {
	rotation, skew := computed.TransformRotation, computed.TransformSkewX
	sx, sy := computed.TransformScaleX, computed.TransformScaleY
	xOffset, yOffset := computed.TransformTranslateX, computed.TransformTranslateY
	xCross, yCross := computed.TransformTranslateCrossX, computed.TransformTranslateCrossY
	if animation != nil {
		value := &animation.Current
		rotation, skew = value.Rotation, value.SkewX
		sx, sy = value.ScaleX, value.ScaleY
		xOffset, yOffset = value.TranslateX, value.TranslateY
		xCross, yCross = value.TranslateCrossX, value.TranslateCrossY
	}
	if rotation == 0 && skew == 0 && sx == 1 && sy == 1 && xOffset == style.LengthPx(0) && yOffset == style.LengthPx(0) && xCross == 0 && yCross == 0 {
		return htmlAnimationAffine{}, false
	}
	box := state.boxForNode(node)
	if box == nil {
		return htmlAnimationAffine{}, false
	}
	rect := box.BorderBox()
	tx := style.ResolveLength(xOffset, rect.Width) + xCross*rect.Height
	ty := style.ResolveLength(yOffset, rect.Height) + yCross*rect.Width
	if rotation == 0 && skew == 0 && sx == 1 && sy == 1 && tx == 0 && ty == 0 {
		return htmlAnimationAffine{}, false
	}
	sin, cos := math.Sincos(rotation * math.Pi / 180)
	skew = math.Tan(skew * math.Pi / 180)
	x := rect.X + style.ResolveLength(computed.TransformOriginX, rect.Width)
	y := rect.Y + style.ResolveLength(computed.TransformOriginY, rect.Height)
	local := htmlAnimationAffine{a: cos * sx, b: sin * sx,
		c: (cos*skew - sin) * sy, d: (sin*skew + cos) * sy}
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

func (state *htmlRenderState) animationTransformValue(node, stop *uinode.Node, full htmlAnimationTransformValue) htmlAnimationTransformValue {
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
		local, _ := state.animationLocalTransform(route.Node, computed, state.Animations[route.Node])
		inherited.append(local)
		return inherited
	})
}
