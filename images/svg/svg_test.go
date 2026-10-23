package svg

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestDecodeConfigUsesViewBoxWhenSizeMissing(t *testing.T) {
	cfg, err := DecodeConfig(strings.NewReader(`<svg viewBox="0 0 32 16"></svg>`))
	if err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.Width != 32 || cfg.Height != 16 {
		t.Fatalf("unexpected config size: %dx%d", cfg.Width, cfg.Height)
	}
}

func TestRenderRectFill(t *testing.T) {
	img, err := RenderString(`<svg width="10" height="10"><rect width="10" height="10" fill="#f00"/></svg>`, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 5, 5, 255, 0, 0, 255)
}

func TestStyleAttributeOverridesPresentationAttribute(t *testing.T) {
	img, err := RenderString(`<svg width="10" height="10"><rect width="10" height="10" fill="red" style="fill: blue"/></svg>`, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 5, 5, 0, 0, 255, 255)
}

func TestRenderPathWithTransform(t *testing.T) {
	src := `<svg width="20" height="20" viewBox="0 0 10 10">
		<g transform="translate(2 2)">
			<path d="M0 0 L6 0 L6 6 Z" fill="lime"/>
		</g>
	</svg>`
	img, err := RenderString(src, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 10, 8, 0, 255, 0, 255)
	assertPixel(t, img, 2, 2, 0, 0, 0, 0)
}

func TestDefinitionsAndUseSupportForwardReferencesAndInheritedStyle(t *testing.T) {
	source := `<svg width="12" height="8">
		<use href="#tile" x="6" fill="red"/>
		<defs><rect id="tile" width="4" height="4"/></defs>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 7, 1, 255, 0, 0, 255)
	assertPixel(t, img, 1, 1, 0, 0, 0, 0)
}

func TestClipPathSupportsUseReferences(t *testing.T) {
	source := `<svg width="10" height="10">
		<defs>
			<path id="left-half" d="M0 0 H5 V10 H0 Z"/>
			<clipPath id="left"><use href="#left-half"/></clipPath>
		</defs>
		<rect width="10" height="10" fill="red" clip-path="url(#left)"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 2, 5, 255, 0, 0, 255)
	assertPixel(t, img, 8, 5, 0, 0, 0, 0)
}

func TestInlineStyleClipPathOverridesPresentationAttribute(t *testing.T) {
	source := `<svg width="10" height="10">
		<defs>
			<clipPath id="left"><rect width="5" height="10"/></clipPath>
			<clipPath id="right"><rect x="5" width="5" height="10"/></clipPath>
		</defs>
		<rect width="10" height="10" fill="blue" clip-path="url(#left)" style="clip-path: url('#right')"/>
	</svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 2, 5, 0, 0, 0, 0)
	assertPixel(t, img, 8, 5, 0, 0, 255, 255)
}

func TestCircularUseReferenceDoesNotRenderOrRecurseForever(t *testing.T) {
	img, err := RenderString(`<svg width="4" height="4"><defs><use id="loop" href="#loop"/></defs><use href="#loop"/></svg>`, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 2, 2, 0, 0, 0, 0)
}

func TestRenderFillSupportsNonZeroHoles(t *testing.T) {
	src := `<svg width="20" height="20"><path d="M1 1 H19 V19 H1 Z M5 5 V15 H15 V5 Z" fill="red"/></svg>`
	img, err := RenderString(src, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 2, 2, 255, 0, 0, 255)
	assertPixel(t, img, 10, 10, 0, 0, 0, 0)
}

func TestRenderStroke(t *testing.T) {
	src := `<svg width="10" height="10"><line x1="1" y1="5" x2="9" y2="5" stroke="green" stroke-width="2"/></svg>`
	img, err := RenderString(src, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 5, 5, 0, 128, 0, 255)
	assertPixel(t, img, 5, 1, 0, 0, 0, 0)
}

func TestParsePathRejectsInputThatDoesNotAdvance(t *testing.T) {
	for _, source := range []string{"M0 0 X", "M0 0 L"} {
		if _, err := parsePath(source); err == nil {
			t.Fatalf("parsePath(%q) succeeded", source)
		}
	}
}

func TestRenderPolylineUsesDefaultFill(t *testing.T) {
	img, err := RenderString(`<svg width="10" height="10"><polyline points="1 1 9 1 5 9"/></svg>`, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 5, 3, 0, 0, 0, 255)
}

func TestRenderReturnsStraightAlphaNRGBA(t *testing.T) {
	img, err := RenderString(`<svg width="2" height="2"><rect width="2" height="2" fill="#382f37" fill-opacity="0.16"/></svg>`, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if img.ColorModel() != color.NRGBAModel {
		t.Fatalf("color model = %T, want NRGBA", img.ColorModel())
	}
	assertPixel(t, img, 1, 1, 56, 47, 55, 41)

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	got := color.NRGBAModel.Convert(decoded.At(1, 1)).(color.NRGBA)
	if got != (color.NRGBA{R: 56, G: 47, B: 55, A: 41}) {
		t.Fatalf("round-trip pixel = %#v", got)
	}
}

func TestGroupOpacityCompositesOverlappingChildrenOnce(t *testing.T) {
	source := `<svg width="10" height="4"><g opacity="0.5" fill="red"><rect width="7" height="4"/><rect x="3" width="7" height="4"/></g></svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 1, 1, 255, 0, 0, 128)
	assertPixel(t, img, 5, 1, 255, 0, 0, 128)
}

func TestElementOpacityCompositesFillAndStrokeOnce(t *testing.T) {
	source := `<svg width="8" height="8"><rect x="2" y="2" width="4" height="4" fill="red" stroke="red" stroke-width="2" opacity="0.5"/></svg>`
	img, err := RenderString(source, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	assertPixel(t, img, 2, 3, 255, 0, 0, 128)
}

func TestStrokeLineCapControlsEndpointCoverage(t *testing.T) {
	for _, test := range []struct {
		name      string
		lineCap   string
		wantAlpha bool
	}{
		{name: "butt"},
		{name: "round", lineCap: ` stroke-linecap="round"`, wantAlpha: true},
		{name: "square", lineCap: ` stroke-linecap="square"`, wantAlpha: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := `<svg width="12" height="8"><line x1="3" y1="4" x2="9" y2="4" stroke="black" stroke-width="4"` + test.lineCap + `/></svg>`
			img, err := RenderString(source, Options{})
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			alpha := img.NRGBAAt(1, 4).A
			if (alpha > 0) != test.wantAlpha {
				t.Fatalf("endpoint alpha = %d, want coverage %v", alpha, test.wantAlpha)
			}
		})
	}
}

func TestStrokeMiterJoinExtendsBeyondBevel(t *testing.T) {
	alpha := func(lineJoin string) uint8 {
		source := `<svg width="12" height="10"><polyline points="2 8 6 4 10 8" fill="none" stroke="black" stroke-width="4" stroke-linejoin="` + lineJoin + `"/></svg>`
		img, err := RenderString(source, Options{})
		if err != nil {
			t.Fatalf("render %s join: %v", lineJoin, err)
		}
		return img.NRGBAAt(6, 1).A
	}
	if miter, bevel := alpha("miter"), alpha("bevel"); miter == 0 || bevel != 0 {
		t.Fatalf("miter alpha = %d, bevel alpha = %d", miter, bevel)
	}
}

func TestRenderSubpixelShapeKeepsCoverage(t *testing.T) {
	img, err := RenderString(`<svg width="1" height="1"><rect y="0.43" width="1" height="0.14" fill="black"/></svg>`, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	alpha := img.Pix[3]
	if alpha == 0 || alpha == 255 {
		t.Fatalf("expected subpixel rect to render with partial alpha, got %d", alpha)
	}
}

func TestRoundedRectUsesFineArcTessellation(t *testing.T) {
	path := rectPath(0, 0, 100, 100, 16, 16)
	if len(path.subpaths) != 1 || len(path.subpaths[0].points) != 4*roundedRectArcSegments+5 {
		t.Fatalf("rounded rect has insufficient arc tessellation: %+v", path.subpaths)
	}
}

func TestParseArcPath(t *testing.T) {
	path, err := parsePath("M 1 5 A 4 4 0 0 1 9 5")
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}
	if len(path.subpaths) != 1 || len(path.subpaths[0].points) < 4 {
		t.Fatalf("expected flattened arc points, got %+v", path)
	}
}

func BenchmarkRenderLargeRoundedRect(b *testing.B) {
	const src = `<svg width="500" height="112"><rect x="1" y="1" width="498" height="110" rx="16" fill="#2a2a2a" stroke="#414141"/></svg>`
	b.ReportAllocs()
	for b.Loop() {
		if _, err := RenderString(src, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func assertPixel(t *testing.T, img *image.NRGBA, x, y int, r, g, b, a uint8) {
	t.Helper()
	i := img.PixOffset(x, y)
	got := img.Pix[i : i+4]
	if got[0] != r || got[1] != g || got[2] != b || got[3] != a {
		t.Fatalf("pixel %d,%d = rgba(%d,%d,%d,%d), want rgba(%d,%d,%d,%d)", x, y, got[0], got[1], got[2], got[3], r, g, b, a)
	}
}
