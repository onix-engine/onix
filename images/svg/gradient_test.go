package svg

import (
	"fmt"
	"image/color"
	"math"
	"testing"
)

func pixelAt(t *testing.T, img interface {
	At(int, int) color.Color
}, x, y int) color.NRGBA {
	t.Helper()
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

func TestLinearGradientInterpolatesAlongItsAxis(t *testing.T) {
	source := `<svg width="20" height="4">
		<defs><linearGradient id="g"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient></defs>
		<rect width="20" height="4" fill="url(#g)"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// The default axis runs from x1=0% to x2=100%. Samples land on pixel centers,
	// so the first column is one half pixel along the axis.
	assertNear(t, pixelAt(t, img, 0, 2), color.NRGBA{R: 249, B: 6, A: 255})
	assertNear(t, pixelAt(t, img, 19, 2), color.NRGBA{R: 6, B: 249, A: 255})
	assertNear(t, pixelAt(t, img, 10, 2), color.NRGBA{R: 121, B: 134, A: 255})
}

func TestLinearGradientUserSpaceCoordinatesAndTransform(t *testing.T) {
	source := `<svg width="10" height="10">
		<defs><linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="0" y2="10">
			<stop offset="0" stop-color="#000000"/><stop offset="1" stop-color="#ffffff"/>
		</linearGradient></defs>
		<rect width="10" height="10" fill="url(#g)"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// userSpaceOnUse keeps the axis in viewport units, so the gradient runs down
	// the shape rather than across it.
	assertNear(t, pixelAt(t, img, 5, 0), color.NRGBA{R: 13, G: 13, B: 13, A: 255})
	assertNear(t, pixelAt(t, img, 5, 9), color.NRGBA{R: 242, G: 242, B: 242, A: 255})
}

func TestLinearGradientTransformRotatesTheAxis(t *testing.T) {
	source := `<svg width="10" height="10">
		<defs><linearGradient id="g" gradientTransform="rotate(90 0.5 0.5)">
			<stop offset="0" stop-color="#000000"/><stop offset="1" stop-color="#ffffff"/>
		</linearGradient></defs>
		<rect width="10" height="10" fill="url(#g)"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// A quarter turn maps the objectBoundingBox axis onto the vertical axis.
	assertNear(t, pixelAt(t, img, 5, 0), color.NRGBA{R: 13, G: 13, B: 13, A: 255})
	assertNear(t, pixelAt(t, img, 5, 9), color.NRGBA{R: 242, G: 242, B: 242, A: 255})
}

func TestLinearGradientTransformIsAppliedInGradientSpace(t *testing.T) {
	// The shape is twice as wide as it is tall, so a gradientTransform in
	// objectBoundingBox units shifts the axis by half the shape width; applying it
	// in device units instead would barely move it.
	source := `<svg width="20" height="10">
		<defs><linearGradient id="g" gradientTransform="translate(0.5 0)">
			<stop offset="0" stop-color="#000000"/><stop offset="0.5" stop-color="#ffffff"/>
			<stop offset="0.5" stop-color="#000000"/><stop offset="1" stop-color="#000000"/>
		</linearGradient></defs>
		<rect width="20" height="10" fill="url(#g)"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// The white band sits at the middle of the shifted axis, which is the right
	// edge of the shape.
	band := pixelAt(t, img, 19, 5)
	if band.R < 240 {
		t.Fatalf("the shifted axis should peak at the right edge, got %v", band)
	}
}

func TestGradientInheritsAttributesFromItsReference(t *testing.T) {
	source := `<svg width="20" height="4">
		<defs>
			<linearGradient id="base" x1="0" x2="1">
				<stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/>
			</linearGradient>
			<linearGradient id="derived" href="#base"/>
		</defs>
		<rect width="20" height="4" fill="url(#derived)"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertNear(t, pixelAt(t, img, 0, 2), color.NRGBA{R: 249, B: 6, A: 255})
	assertNear(t, pixelAt(t, img, 19, 2), color.NRGBA{R: 6, B: 249, A: 255})
}

func TestGradientStopsKeepDocumentOrderAndClampOffsets(t *testing.T) {
	source := `<svg width="9" height="9">
		<defs><linearGradient id="g">
			<stop offset="0.5" stop-color="red"/><stop offset="0" stop-color="green"/>
			<stop offset="0.25" stop-color="blue"/>
		</linearGradient></defs>
		<rect width="9" height="9" fill="url(#g)"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// All offsets become 0.5; the final stop wins exactly at the transition.
	assertPixel(t, img, 0, 4, 255, 0, 0, 255)
	assertPixel(t, img, 3, 4, 255, 0, 0, 255)
	assertPixel(t, img, 4, 4, 0, 0, 255, 255)
	assertPixel(t, img, 8, 4, 0, 0, 255, 255)
}

func TestGradientCoincidentStopsUseLastColor(t *testing.T) {
	for _, offset := range []string{"0", "0.5", "1"} {
		t.Run(offset, func(t *testing.T) {
			source := fmt.Sprintf(`<svg width="9" height="3">
				<linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0.5" x2="8.5">
					<stop offset="0" stop-color="black"/>
					<stop offset="%s" stop-color="red"/><stop offset="%s" stop-color="blue"/>
					<stop offset="1" stop-color="blue"/>
				</linearGradient><rect width="9" height="3" fill="url(#g)"/>
			</svg>`, offset, offset)
			img, err := RenderString(source, Options{})
			if err != nil {
				t.Fatal(err)
			}
			x := map[string]int{"0": 0, "0.5": 4, "1": 8}[offset]
			assertPixel(t, img, x, 1, 0, 0, 255, 255)
		})
	}
}

func TestObjectBoundingBoxGradientFollowsShapeTransform(t *testing.T) {
	for _, mode := range []string{"fill", "stroke"} {
		for _, test := range []struct {
			name, transform  string
			opts             Options
			fillX, fillY     int
			strokeX, strokeY int
			want             color.NRGBA
		}{
			{"identity", "", Options{}, 6, 4, 6, 2, color.NRGBA{R: 183, B: 72, A: 255}},
			{"translate", "translate(10 10)", Options{}, 16, 14, 16, 12, color.NRGBA{R: 183, B: 72, A: 255}},
			{"rotate", "rotate(90 20 20)", Options{}, 35, 6, 37, 6, color.NRGBA{R: 183, B: 72, A: 255}},
			{"scale", "scale(2 3)", Options{}, 12, 12, 12, 6, color.NRGBA{R: 187, B: 68, A: 255}},
			{"viewport", "", Options{Width: 80, Height: 80}, 12, 8, 12, 4, color.NRGBA{R: 187, B: 68, A: 255}},
		} {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				fill, stroke := "url(#g)", "none"
				x, y := test.fillX, test.fillY
				if mode == "stroke" {
					fill, stroke = "none", "url(#g)"
					x, y = test.strokeX, test.strokeY
				}
				source := fmt.Sprintf(`<svg width="40" height="40">
					<linearGradient id="g"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient>
					<rect x="2" y="2" width="16" height="8" transform="%s" fill="%s" stroke="%s" stroke-width="2"/>
				</svg>`, test.transform, fill, stroke)
				img, err := RenderString(source, test.opts)
				if err != nil {
					t.Fatal(err)
				}
				assertNear(t, pixelAt(t, img, x, y), test.want)
			})
		}
	}
}

func TestGradientSpreadMethodRepeat(t *testing.T) {
	source := `<svg width="20" height="4">
		<defs><linearGradient id="g" spreadMethod="repeat" x1="0" x2="0.25">
			<stop offset="0" stop-color="#000000"/><stop offset="1" stop-color="#ffffff"/>
		</linearGradient></defs>
		<rect width="20" height="4" fill="url(#g)"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// The pattern restarts every quarter of the bounding box, so the fourth
	// period looks like the first.
	for x := 0; x < 4; x++ {
		first := pixelAt(t, img, x, 2)
		repeated := pixelAt(t, img, x+15, 2)
		if math.Abs(float64(first.R)-float64(repeated.R)) > 2 {
			t.Fatalf("repeat period differs: x=%d %v vs x=%d %v", x, first, x+15, repeated)
		}
	}
}

func TestRadialGradientUsesCenterAndRadius(t *testing.T) {
	source := `<svg width="20" height="20">
		<defs><radialGradient id="g" gradientUnits="userSpaceOnUse" cx="10" cy="10" r="10">
			<stop offset="0" stop-color="#ffffff"/><stop offset="1" stop-color="#000000"/>
		</radialGradient></defs>
		<rect width="20" height="20" fill="url(#g)"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	center := pixelAt(t, img, 10, 10)
	edge := pixelAt(t, img, 1, 10)
	if center.R < 230 {
		t.Fatalf("center should be close to the first stop, got %v", center)
	}
	if edge.R > 40 {
		t.Fatalf("the radius edge should reach the last stop, got %v", edge)
	}
}

func TestRadialGradientFocusDefaultsPerAxis(t *testing.T) {
	for _, test := range []struct {
		name, base, attrs string
		x, y              int
		gray              uint8
	}{
		{"defaults", "", "", 10, 10, 237},
		{"fx", "", `fx="5.5" cy="10.5"`, 5, 10, 255},
		{"fy", "", `fy="5.5" cx="10.5"`, 10, 5, 255},
		{"inherited-center", `cx="8.5" cy="12.5"`, "", 8, 12, 255},
		{"inherited-fx", `fx="5.5"`, `cy="12.5"`, 5, 12, 255},
		{"inherited-fy", `fy="5.5"`, `cx="12.5"`, 12, 5, 255},
		{"overridden-center", `cx="8.5" cy="12.5"`, `cx="10.5"`, 10, 12, 255},
		{"overridden-focus", `fx="5.5" fy="6.5"`, `fx="8.5"`, 8, 6, 255},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := fmt.Sprintf(`<svg width="20" height="20">
				<radialGradient id="base" %s><stop offset="0" stop-color="white"/><stop offset="1" stop-color="black"/></radialGradient>
				<radialGradient id="g" gradientUnits="userSpaceOnUse" href="#base" %s/>
				<rect width="20" height="20" fill="url(#g)"/>
			</svg>`, test.base, test.attrs)
			img, err := RenderString(source, Options{})
			if err != nil {
				t.Fatal(err)
			}
			assertNear(t, pixelAt(t, img, test.x, test.y), color.NRGBA{R: test.gray, G: test.gray, B: test.gray, A: 255})
		})
	}
}

func TestUnknownPaintReferencePaintsNothing(t *testing.T) {
	img, err := RenderString(`<svg width="4" height="4"><rect width="4" height="4" fill="url(#missing)"/></svg>`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, img, 2, 2, 0, 0, 0, 0)
}

func TestGradientStrokeUsesTheSamePaintServer(t *testing.T) {
	source := `<svg width="20" height="4">
		<defs><linearGradient id="g"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient></defs>
		<rect x="1" y="1" width="18" height="2" fill="none" stroke="url(#g)" stroke-width="2"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	left, right := pixelAt(t, img, 1, 2), pixelAt(t, img, 18, 2)
	if left.R <= left.B {
		t.Fatalf("left stroke should be closer to the first stop: %v", left)
	}
	if right.B <= right.R {
		t.Fatalf("right stroke should be closer to the last stop: %v", right)
	}
}

func TestGradientStopOpacityAndHexColors(t *testing.T) {
	source := `<svg width="10" height="10">
		<defs><linearGradient id="g">
			<stop offset="0" stop-color="#ff0000" stop-opacity="0.5"/><stop offset="1" stop-color="#ff0000" stop-opacity="0.5"/>
		</linearGradient></defs>
		<rect width="10" height="10" fill="url(#g)"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertNear(t, pixelAt(t, img, 5, 5), color.NRGBA{R: 255, A: 128})
}

func TestGradientStopDefaultsToOpaqueBlack(t *testing.T) {
	for _, test := range []struct {
		name, attrs string
		alpha       uint8
	}{
		{"default", "", 255},
		{"opacity", `stop-opacity="0.5"`, 128},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := fmt.Sprintf(`<svg width="4" height="4">
				<linearGradient id="g"><stop offset="0" %s/><stop offset="1" %s/></linearGradient>
				<rect width="4" height="4" fill="url(#g)"/>
			</svg>`, test.attrs, test.attrs)
			img, err := RenderString(source, Options{})
			if err != nil {
				t.Fatal(err)
			}
			assertPixel(t, img, 2, 2, 0, 0, 0, test.alpha)
		})
	}
}

// assertNear compares channels within a small tolerance, because the rasterizer
// anti-aliases and rounds.
func assertNear(t *testing.T, got, want color.NRGBA) {
	t.Helper()
	channels := []struct {
		name     string
		got, exp uint8
	}{
		{"R", got.R, want.R},
		{"G", got.G, want.G},
		{"B", got.B, want.B},
		{"A", got.A, want.A},
	}
	for _, channel := range channels {
		difference := int(channel.got) - int(channel.exp)
		if difference < 0 {
			difference = -difference
		}
		if difference > 4 {
			t.Fatalf("pixel = rgba(%d,%d,%d,%d), want rgba(%d,%d,%d,%d)", got.R, got.G, got.B, got.A, want.R, want.G, want.B, want.A)
		}
	}
}
