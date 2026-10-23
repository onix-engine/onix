package style

import (
	"math"
	"strings"
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

func TestColorMixSRGB(t *testing.T) {
	for _, test := range []struct {
		input string
		want  Color
	}{
		{"color-mix(in srgb, red, blue)", Color{R: 128, B: 128, A: 255}},
		{"color-mix(in srgb, red 25%, blue)", Color{R: 64, B: 191, A: 255}},
		{"color-mix(in srgb, red, blue 75%)", Color{R: 64, B: 191, A: 255}},
		{"COLOR-MIX(IN SRGB, 25% RED, 75% BLUE)", Color{R: 64, B: 191, A: 255}},
		{"color-mix(in\tsrgb, red 100%, blue 100%)", Color{R: 128, B: 128, A: 255}},
		{"color-mix(in srgb, red 20%, blue 20%)", Color{R: 128, B: 128, A: 102}},
		{"color-mix(in srgb, red 20%, transparent)", Color{R: 255, A: 51}},
		{"color-mix(in srgb, red 20%, transparent 20%)", Color{R: 255, A: 51}},
		{"color-mix(in srgb, red 0%, blue)", Color{B: 255, A: 255}},
		{"color-mix(in srgb, transparent, rgba(255, 0, 0, 0))", Color{}},
		{"color-mix(in srgb, #1234 100%, transparent 0%)", Color{R: 17, G: 34, B: 51, A: 68}},
		{"color-mix(in srgb, rgb(100% 0% 0% / .7) 20%, rgb(0% 100% 0% / .2) 60%)", Color{R: 137, G: 118, A: 66}},
		{"color-mix(in srgb, rgba(255, 0, 0, .7) 20%, rgba(0, 255, 0, .2) 60%)", Color{R: 137, G: 118, A: 66}},
		{"color-mix(in srgb, color-mix(in srgb, red, blue), white)", Color{R: 191, G: 128, B: 191, A: 255}},
		{"color-mix(in srgb, color-mix(in srgb, red .1%, transparent) 10%, transparent)", Color{R: 255}},
		{"color-mix(in srgb, red 2.5e1%, blue)", Color{R: 64, B: 191, A: 255}},
	} {
		t.Run(test.input, func(t *testing.T) {
			got, err := ParseColor(test.input)
			if err != nil || got != test.want {
				t.Fatalf("ParseColor = %+v, %v; want %+v", got, err, test.want)
			}
			if allocs := testing.AllocsPerRun(100, func() {
				if _, err := ParseColor(test.input); err != nil {
					panic(err)
				}
			}); allocs != 0 {
				t.Fatalf("color-mix allocated %g times", allocs)
			}
		})
	}
	// Nested mixes must retain even sub-byte alpha before the final conversion.
	mixed, err := parseMixedColor("color-mix(in srgb, color-mix(in srgb, red .1%, transparent) 10%, transparent)", 0)
	if err != nil || math.Abs(mixed.A-.0001) > 1e-12 || mixed.R != mixed.A {
		t.Fatalf("nested precision = %+v, %v", mixed, err)
	}
}

func TestColorMixInvalid(t *testing.T) {
	for _, input := range []string{
		"color-mix(in srgb, red)", "color-mix(in srgb, red, blue, white)",
		"color-mix(in srgb, red,)", "color-mix(in srgb,,blue)",
		"color-mix(in srgb, red, blue", "color-mix(in srgb, red, blue))",
		"color-mix(in srgb, red, blue) garbage", "color-mix(in srgb, red, blue) white)",
		"color-mix(in srgb, red -1%, blue)", "color-mix(in srgb, red 101%, blue)",
		"color-mix(in srgb, red 0%, blue 0%)", "color-mix(in srgb, red 25, blue)",
		"color-mix(in srgb, red 20% 30%, blue)", "color-mix(in srgb, 20% 30%, blue)",
		"color-mix(in srgb, red NaN%, blue)", "color-mix(in srgb, red 1e999%, blue)",
		"color-mix(in srgb, red 0x1p2%, blue)", "color-mix(in srgb, red 2_0%, blue)",
		"color-mix(in srgb, red 20.%, blue)", "color-mix(in srgb, red 20 %, blue)",
		"color-mix(in srgb, #ggg, blue)", "color-mix(in srgb, #xyz1, blue)",
		"color-mix(in srgb, rgb(1, 2, 3)garbage, blue)",
		"color-mix(in srgb, rgb(1, 2, 3)), blue)",
		"color-mix(in srgb, rgb(1 2), blue)",
		"color-mix(in srgb, rgba(1, 2, 3, NaN), blue)",
		"color-mix(in srgb, rgb(1% 2 3 / .5 / .2), blue)",
		"color-mix(in srgb, rgb(1%, 2, 3), blue)",
		"color-mix(in srgb, rgb(1, 2, 3), blue))",
		"color-mix(in oklab, red, blue)", "color-mix(in srgb-linear, red, blue)",
		"color-mix(in srgb shorter hue, red, blue)", "color-mix(red, blue)",
		strings.Repeat("color-mix(in srgb, red, ", 65) + "blue" + strings.Repeat(")", 65),
	} {
		if _, err := parseColorValue(input); err == nil {
			t.Errorf("accepted invalid color %q", input)
		}
	}
}

func TestColorMixCascadeAndCurrentColor(t *testing.T) {
	document, err := dom.Parse(`<div id="parent" style="color:red"><div id="mixed"><span id="child"></span></div></div>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(`#mixed {
		color: color-mix(in srgb, currentColor, blue);
		background: color-mix(in srgb, currentColor 20%, transparent);
		border: 1px solid color-mix(in srgb, currentColor 40%, transparent);
		box-shadow: 0 0 2px color-mix(in srgb, red 10%, transparent);
	}`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(document, sheet)
	if err != nil {
		t.Fatal(err)
	}
	var mixed, child *ComputedStyle
	for node, computed := range styles {
		id, _ := node.Attr("id")
		if id == "mixed" {
			mixed = computed
		} else if id == "child" {
			child = computed
		}
	}
	purple := Color{R: 128, B: 128, A: 255}
	if mixed == nil || child == nil || mixed.Color.Color != purple || child.Color.Color != purple {
		t.Fatalf("mixed/inherited color = %+v / %+v", mixed, child)
	}
	if mixed.BackgroundColor.Color != (Color{R: 128, B: 128, A: 51}) || mixed.BorderColor.Top.Color != (Color{R: 128, B: 128, A: 102}) {
		t.Fatalf("mixed paint = %+v / %+v", mixed.BackgroundColor, mixed.BorderColor)
	}
	if len(mixed.BoxShadow) != 1 || mixed.BoxShadow[0].Color != (Color{R: 255, A: 26}) {
		t.Fatalf("mixed shadow = %+v", mixed.BoxShadow)
	}
	// Animation sampling keeps currentcolor symbolic so a changing foreground
	// also changes dependent mixed backgrounds and borders.
	animated := ColorForAnimation(mixed.BackgroundColor).Resolve(ColorForAnimation(ColorValue(Color{G: 255, A: 255})))
	if animated.Color() != (Color{G: 255, A: 51}) {
		t.Fatalf("animated currentcolor mix = %+v", animated)
	}
}

func TestColorMixPropertyAllocations(t *testing.T) {
	for _, property := range []string{"color", "background", "background-color", "border-color", "border"} {
		t.Run(property, func(t *testing.T) {
			input := "color-mix(in srgb, #e56d6d 8%, transparent)"
			if property == "border" {
				input = "1px solid " + input
			}
			computed := defaultStyle(nil)
			if allocs := testing.AllocsPerRun(100, func() {
				if err := applyProperty(&computed, property, input, nil, parseContext{}); err != nil {
					panic(err)
				}
			}); allocs != 0 {
				t.Fatalf("mixed %s allocated %g times", property, allocs)
			}
		})
	}
	if allocations := testing.AllocsPerRun(100, func() {
		value, err := parseColorValue("color-mix(in srgb, currentColor 40%, color-mix(in srgb, red, transparent))")
		if err != nil {
			panic(err)
		}
		ColorForAnimation(value).Resolve(AnimationColor{B: 1, A: 1}).Color()
	}); allocations != 0 {
		t.Fatalf("currentcolor mix allocated %g times", allocations)
	}
}

func TestRGBColorComponents(t *testing.T) {
	for _, test := range []struct {
		input string
		want  Color
	}{
		{"rgb(255, 128, 0)", Color{R: 255, G: 128, A: 255}},
		{"rgba(255, 0, 0, 0)", Color{R: 255}},
		{"RGB(100%, 50%, 0%, 25%)", Color{R: 255, G: 128, A: 64}},
		{"rgba(100% 50% 0% / 25%)", Color{R: 255, G: 128, A: 64}},
		{"rgb(300 -10 1.5 / 2)", Color{R: 255, B: 2, A: 255}},
		{"rgb(.5e2 0 0)", Color{R: 50, A: 255}},
	} {
		got, err := ParseColor(test.input)
		if err != nil || got != test.want {
			t.Errorf("ParseColor(%q) = %+v, %v; want %+v", test.input, got, err, test.want)
		}
	}
}
