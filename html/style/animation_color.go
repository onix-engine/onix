package style

import (
	"math"
	"math/bits"
	"strings"

	"github.com/diiyw/onix/html/dom"
)

const (
	AnimationForeground = iota
	AnimationBackground
	AnimationBorderTop
	AnimationBorderRight
	AnimationBorderBottom
	AnimationBorderLeft
	AnimatedColorCount
)

const AnimatedPropertyCount = 2 + AnimatedColorCount

const AnimatedColors = AnimatedColor | AnimatedBackgroundColor | AnimatedBorderTopColor | AnimatedBorderRightColor | AnimatedBorderBottomColor | AnimatedBorderLeftColor
const AnimatedBorders = AnimatedColors &^ (AnimatedColor | AnimatedBackgroundColor)

var AnimatedPropertyList = [...]AnimatedProperties{AnimatedTransform, AnimatedOpacity, AnimatedColor, AnimatedBackgroundColor, AnimatedBorderTopColor, AnimatedBorderRightColor, AnimatedBorderBottomColor, AnimatedBorderLeftColor}

var animatedColorNames = [...]string{"color", "background-color", "border-top-color", "border-right-color", "border-bottom-color", "border-left-color"}

func AnimatedProperty(name string) AnimatedProperties {
	switch name {
	case "transform":
		return AnimatedTransform
	case "opacity":
		return AnimatedOpacity
	case "color":
		return AnimatedColor
	case "background", "background-color":
		return AnimatedBackgroundColor
	case "border", "border-color":
		return AnimatedBorders
	case "border-top", "border-top-color":
		return AnimatedBorderTopColor
	case "border-right", "border-right-color":
		return AnimatedBorderRightColor
	case "border-bottom", "border-bottom-color":
		return AnimatedBorderBottomColor
	case "border-left", "border-left-color":
		return AnimatedBorderLeftColor
	}
	return 0
}

// AnimationColor stores premultiplied, gamma-encoded sRGB. Current retains a
// currentcolor/inheritance contribution until the element's used color is known.
type AnimationColor struct {
	R, G, B, A, Current float64
}

func ColorForAnimation(value Value) AnimationColor {
	if value.Keyword == "currentcolor" {
		return AnimationColor{Current: 1}
	}
	if isColorMix(value.Keyword) {
		mixed, _ := parseMixedColor(value.Keyword, 0)
		return mixed
	}
	a := float64(value.Color.A) / 255
	return AnimationColor{R: float64(value.Color.R) / 255 * a, G: float64(value.Color.G) / 255 * a, B: float64(value.Color.B) / 255 * a, A: a}
}

func (c AnimationColor) Resolve(reference AnimationColor) AnimationColor {
	return AnimationColor{R: c.R + c.Current*reference.R, G: c.G + c.Current*reference.G, B: c.B + c.Current*reference.B, A: c.A + c.Current*reference.A}
}

func (c AnimationColor) RGBA() (r, g, b, a float64) {
	if c.A <= 0 {
		return 0, 0, 0, 0
	}
	return min(max(c.R/c.A, 0), 1), min(max(c.G/c.A, 0), 1), min(max(c.B/c.A, 0), 1), min(c.A, 1)
}

func (c AnimationColor) Color() Color {
	r, g, b, a := c.RGBA()
	return Color{R: uint8(math.Round(r * 255)), G: uint8(math.Round(g * 255)), B: uint8(math.Round(b * 255)), A: uint8(math.Round(a * 255))}
}

func currentColorValue() Value {
	return Value{Kind: ValueColor, Keyword: "currentcolor", Color: Color{A: 255}}
}

func parseColorValue(raw string) (Value, error) {
	raw = strings.TrimSpace(raw)
	if strings.EqualFold(raw, "currentcolor") {
		return currentColorValue(), nil
	}
	if isColorMix(raw) {
		mixed, err := parseMixedColor(raw, 0)
		value := ColorValue(mixed.Color())
		if mixed.Current != 0 {
			// Retain the expression until currentcolor is known, without
			// allocating an expression tree or enlarging every style Value.
			value.Keyword = raw
		}
		return value, err
	}
	color, err := parseColor(raw)
	return ColorValue(color), err
}

func paintColorFields(computed *ComputedStyle) [AnimatedColorCount]*Value {
	return [AnimatedColorCount]*Value{&computed.Color, &computed.BackgroundColor, &computed.BorderColor.Top, &computed.BorderColor.Right, &computed.BorderColor.Bottom, &computed.BorderColor.Left}
}

func resolvePaintColors(computed, parent *ComputedStyle, hasParent bool, node *dom.Node, specified map[string]cascadedValue) {
	_, declaredColor := specified["color"]
	computed.ColorInherited = hasParent && inheritsFromParent(specified, "color") && (!isDisabledControl(node) || declaredColor)
	if computed.Color.Keyword == "currentcolor" {
		computed.Color = ColorValue(Color{A: 255})
		computed.ColorInherited = hasParent
		if hasParent {
			computed.Color = parent.Color
		}
	}
	if isColorMix(computed.Color.Keyword) {
		reference := AnimationColor{A: 1}
		if hasParent {
			reference = ColorForAnimation(parent.Color)
		}
		computed.Color = ColorValue(ColorForAnimation(computed.Color).Resolve(reference).Color())
	}
	fields := paintColorFields(computed)
	for _, value := range fields[1:] {
		if value.Keyword == "currentcolor" {
			value.Color = computed.Color.Color
		} else if isColorMix(value.Keyword) {
			value.Color = ColorForAnimation(*value).Resolve(ColorForAnimation(computed.Color)).Color()
		}
	}
	if computed.PlaceholderColor.Keyword == "currentcolor" {
		computed.PlaceholderColor.Color = computed.Color.Color
	} else if isColorMix(computed.PlaceholderColor.Keyword) {
		computed.PlaceholderColor.Color = ColorForAnimation(computed.PlaceholderColor).Resolve(ColorForAnimation(computed.Color)).Color()
	}
}

func CopyAnimatedProperties(dst *AnimationValue, src AnimationValue, properties AnimatedProperties) {
	if properties&AnimatedTransform != 0 {
		dst.Rotation, dst.TranslateX, dst.TranslateY = src.Rotation, src.TranslateX, src.TranslateY
		dst.ScaleX, dst.ScaleY, dst.SkewX = src.ScaleX, src.ScaleY, src.SkewX
		dst.TranslateCrossX, dst.TranslateCrossY = src.TranslateCrossX, src.TranslateCrossY
	}
	if properties&AnimatedOpacity != 0 {
		dst.Opacity = src.Opacity
	}
	properties &= AnimatedColors
	for properties != 0 {
		i := bits.TrailingZeros8(uint8(properties)) - 2
		dst.Colors[i] = src.Colors[i]
		properties &^= AnimatedColor << i
	}
}

func SameAnimatedProperty(a, b AnimationValue, property AnimatedProperties) bool {
	switch property {
	case AnimatedTransform:
		return a.Rotation == b.Rotation && a.TranslateX == b.TranslateX && a.TranslateY == b.TranslateY && a.ScaleX == b.ScaleX && a.ScaleY == b.ScaleY && a.SkewX == b.SkewX && a.TranslateCrossX == b.TranslateCrossX && a.TranslateCrossY == b.TranslateCrossY
	case AnimatedOpacity:
		return a.Opacity == b.Opacity
	default:
		return a.Colors[bits.TrailingZeros8(uint8(property))-2] == b.Colors[bits.TrailingZeros8(uint8(property))-2]
	}
}

func InterpolateAnimatedProperty(dst *AnimationValue, a, b AnimationValue, property AnimatedProperties, t float64) {
	switch property {
	case AnimatedTransform:
		dst.Rotation = interpolate(a.Rotation, b.Rotation, t)
		dst.ScaleX = interpolate(a.ScaleX, b.ScaleX, t)
		dst.ScaleY = interpolate(a.ScaleY, b.ScaleY, t)
		dst.SkewX = interpolate(a.SkewX, b.SkewX, t)
		dst.TranslateCrossX = interpolate(a.TranslateCrossX, b.TranslateCrossX, t)
		dst.TranslateCrossY = interpolate(a.TranslateCrossY, b.TranslateCrossY, t)
		dst.TranslateX = interpolateLength(a.TranslateX, b.TranslateX, t)
		dst.TranslateY = interpolateLength(a.TranslateY, b.TranslateY, t)
	case AnimatedOpacity:
		dst.Opacity = min(max(interpolate(a.Opacity, b.Opacity, t), 0), 1)
	default:
		i := bits.TrailingZeros8(uint8(property)) - 2
		x, y := a.Colors[i], b.Colors[i]
		dst.Colors[i] = AnimationColor{R: interpolate(x.R, y.R, t), G: interpolate(x.G, y.G, t), B: interpolate(x.B, y.B, t), A: interpolate(x.A, y.A, t), Current: interpolate(x.Current, y.Current, t)}
	}
}

func (computed *ComputedStyle) TransitionFor(index int) TransformTransition {
	if computed.motion == nil {
		return TransformTransition{}
	}
	return computed.motion.transitions[index]
}
