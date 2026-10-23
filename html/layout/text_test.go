package layout

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func loadLayoutTestFace(t *testing.T) *font.Face {
	t.Helper()
	face, err := font.LoadDefault()
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	return face
}

func TestEmptyButtonsDoNotSynthesizeLabels(t *testing.T) {
	doc, err := dom.Parse(`<html><body><button type="button"></button><input type="button"><input type="submit"><input type="reset"></body></html>`)
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	body := doc.Children[0].Children[1]
	want := []string{"", "", "Submit", "Reset"}
	for index, node := range body.Children {
		text, ok := syntheticControlText(node)
		if !ok || text.Text != want[index] {
			t.Errorf("control %d text = %q, want %q", index, text.Text, want[index])
		}
	}
}

func TestTokenizeStyledTextPreservesCSSWhitespaceModes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		mode  string
		want  []textToken
	}{
		{
			name:  "normal collapses Unicode whitespace and splits CJK",
			input: "  Go\t语言 \n now  ",
			mode:  "normal",
			want: []textToken{
				{Text: "Go"}, {Text: " ", IsSpace: true}, {Text: "语"}, {Text: "言"},
				{Text: " ", IsSpace: true}, {Text: "now"},
			},
		},
		{
			name:  "pre preserves spaces tabs and line breaks",
			input: "a  b\t中\r\n",
			mode:  "pre",
			want: []textToken{
				{Text: "a"}, {Text: " ", IsSpace: true}, {Text: " ", IsSpace: true},
				{Text: "b"}, {IsTab: true}, {Text: "中"}, {IsBreak: true},
			},
		},
		{
			name:  "pre-line collapses spaces and preserves CRLF",
			input: " a  b\r\n  c ",
			mode:  "pre-line",
			want: []textToken{
				{Text: "a"}, {Text: " ", IsSpace: true}, {Text: "b"}, {IsBreak: true}, {Text: "c"},
			},
		},
		{
			name:  "pre-line treats lone CR as collapsible whitespace",
			input: "a\rb",
			mode:  "pre-line",
			want:  []textToken{{Text: "a"}, {Text: " ", IsSpace: true}, {Text: "b"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := tokenizeStyledTextInto(nil, test.input, test.mode, true, false)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("tokens = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestPreformattedTabUsesComputedTabSize(t *testing.T) {
	face := loadLayoutTestFace(t)
	cssText := `pre { font-family: monospace; font-size: 16px; white-space: pre; tab-size: 2; }`
	textX := func(source string) float64 {
		t.Helper()
		doc, err := dom.Parse(`<html><body><pre>` + source + `</pre></body></html>`)
		if err != nil {
			t.Fatalf("parse html: %v", err)
		}
		stylesheet, err := css.Parse(cssText)
		if err != nil {
			t.Fatalf("parse css: %v", err)
		}
		styles, err := style.Compute(doc, stylesheet)
		if err != nil {
			t.Fatalf("compute styles: %v", err)
		}
		root, err := Build(doc, styles, face, 500, 100)
		if err != nil {
			t.Fatalf("build layout: %v", err)
		}
		var found bool
		var x float64
		var visit func(*Box)
		visit = func(box *Box) {
			if box == nil || found {
				return
			}
			for _, fragment := range box.TextFragments {
				if fragment.Text == "value" {
					x, found = fragment.Rect.X, true
					return
				}
			}
			for _, child := range box.Children {
				visit(child)
			}
		}
		visit(root)
		if !found {
			t.Fatalf("source %q produced no value text fragment", source)
		}
		return x
	}
	if tabX, spacesX := textX("\tvalue"), textX("  value"); math.Abs(tabX-spacesX) > 0.01 {
		t.Fatalf("tab-size:2 text x = %v, two-space text x = %v", tabX, spacesX)
	}
}

func TestShapeTokenCachesShapedResult(t *testing.T) {
	ctx := newLayoutContext()
	face := loadLayoutTestFace(t)
	token := textToken{Text: "Hello"}

	firstGlyphs, firstWidth, err := shapeToken(face, token, 16, 0, ctx)
	if err != nil {
		t.Fatalf("first shape token: %v", err)
	}
	secondGlyphs, secondWidth, err := shapeToken(face, token, 16, 0, ctx)
	if err != nil {
		t.Fatalf("second shape token: %v", err)
	}
	if firstWidth != secondWidth {
		t.Fatalf("width mismatch = %v vs %v", firstWidth, secondWidth)
	}
	if len(firstGlyphs) == 0 || len(secondGlyphs) == 0 {
		t.Fatal("expected shaped glyphs")
	}
	if &firstGlyphs[0] != &secondGlyphs[0] {
		t.Fatal("shape cache did not reuse glyph slice")
	}

	if _, _, err := shapeToken(face, token, 18, 0, ctx); err != nil {
		t.Fatalf("shape token at different size: %v", err)
	}
}

func TestEllipsisReflowPreservesCachedGlyphsAndGeometry(t *testing.T) {
	face := loadLayoutTestFace(t)
	defer face.Close()
	const source = "abcdefghijklmnopqrstuvwxyz"
	for _, align := range []string{"left", "center", "right"} {
		t.Run(align, func(t *testing.T) {
			doc, err := dom.Parse(`<html><body><div>` + source + `</div></body></html>`)
			if err != nil {
				t.Fatal(err)
			}
			sheet, err := css.Parse(`div { font-size: 16px; white-space: nowrap; text-overflow: ellipsis; text-align: ` + align + `; }`)
			if err != nil {
				t.Fatal(err)
			}
			styles, err := style.Compute(doc, sheet)
			if err != nil {
				t.Fatal(err)
			}
			ctx := newLayoutContext()
			cached, _, err := shapeToken(face, textToken{Text: source}, 16, 0, ctx)
			if err != nil {
				t.Fatal(err)
			}
			before := append([]shapedGlyph(nil), cached...)
			reuse := &Box{}
			options := TextLayoutOptions{Node: doc.Children[0].Children[1].Children[0], StyleMap: styles, Face: face, Width: 80, Context: ctx, Reuse: reuse}
			reflow := func() {
				fragments, _, _, _, _, err := layoutText(options)
				if err != nil {
					t.Fatal(err)
				}
				reuse.TextFragments = fragments
			}
			reflow()
			if len(reuse.TextFragments) != 1 {
				t.Fatalf("fragments = %d", len(reuse.TextFragments))
			}
			fragment := reuse.TextFragments[0]
			if !strings.HasSuffix(fragment.Text, "…") || len(fragment.Glyphs) < 2 {
				t.Fatalf("missing truncated prefix: %q", fragment.Text)
			}
			last := fragment.Glyphs[len(fragment.Glyphs)-1]
			ellipsis, _, err := shapeToken(face, textToken{Text: "…"}, 16, 0, ctx)
			if err != nil {
				t.Fatal(err)
			}
			wantX := fragment.Rect.X + glyphRunWidth(cached[:len(fragment.Glyphs)-1]) + ellipsis[0].X
			if math.Abs(last.X-wantX) > .01 || fragment.Rect.X+fragment.Rect.Width > 80.01 || fragment.Rect.X < -.01 {
				t.Fatalf("ellipsis geometry overlaps or overflows: last=%+v wantX=%v rect=%+v", last, wantX, fragment.Rect)
			}
			if allocs := testing.AllocsPerRun(100, reflow); allocs != 0 {
				t.Fatalf("warmed ellipsis reflow allocated %g times", allocs)
			}
			if !reflect.DeepEqual(cached, before) {
				t.Fatal("ellipsis modified the shared shaped token")
			}
		})
	}
}

func TestEllipsisTruncatesAcrossWords(t *testing.T) {
	face := loadLayoutTestFace(t)
	defer face.Close()
	for _, source := range []string{"one two three four five six", "first <span>second third fourth</span> fifth sixth"} {
		doc, err := dom.Parse(`<html><body><div>` + source + `</div></body></html>`)
		if err != nil {
			t.Fatal(err)
		}
		sheet, err := css.Parse(`div { font-size: 16px; white-space: nowrap; text-overflow: ellipsis; overflow: hidden; }`)
		if err != nil {
			t.Fatal(err)
		}
		styles, err := style.Compute(doc, sheet)
		if err != nil {
			t.Fatal(err)
		}
		reuse := &Box{}
		options := TextLayoutOptions{Node: doc.Children[0].Children[1].Children[0], StyleMap: styles, Face: face, Width: 80, Context: newLayoutContext(), Reuse: reuse}
		reflow := func() {
			fragments, _, _, _, _, err := layoutText(options)
			if err != nil {
				t.Fatal(err)
			}
			reuse.TextFragments = fragments
		}
		reflow()
		fragments := reuse.TextFragments
		if len(fragments) == 0 || !strings.HasSuffix(fragments[len(fragments)-1].Text, "…") {
			t.Fatalf("missing visible ellipsis for %q: %+v", source, fragments)
		}
		for _, fragment := range fragments {
			if fragment.Rect.X < -.01 || fragment.Rect.X+fragment.Rect.Width > 80.01 {
				t.Fatalf("truncated word exceeds line: %+v", fragment.Rect)
			}
		}
		if allocations := testing.AllocsPerRun(100, reflow); allocations != 0 {
			t.Fatalf("warmed multiword ellipsis allocated %g times", allocations)
		}
	}
}

func TestShapeTokenCacheSeparatesFonts(t *testing.T) {
	regular := loadLayoutTestFace(t)
	bold, err := font.Parse(append([]byte(nil), regular.Data...))
	if err != nil {
		t.Fatalf("clone font: %v", err)
	}
	glyph := bold.GlyphIndex('H')
	bold.Metrics[glyph].AdvanceWidth += 1000

	ctx := newLayoutContext()
	token := textToken{Text: "Hello"}
	_, regularWidth, err := shapeToken(regular, token, 16, 0, ctx)
	if err != nil {
		t.Fatalf("shape regular token: %v", err)
	}
	_, boldWidth, err := shapeToken(bold, token, 16, 0, ctx)
	if err != nil {
		t.Fatalf("shape bold token: %v", err)
	}
	if boldWidth <= regularWidth {
		t.Fatalf("bold width = %v, regular width = %v; distinct font metrics were not used", boldWidth, regularWidth)
	}
}

func TestSelectedOptionTextFollowsLiveSelection(t *testing.T) {
	document, err := dom.Parse(`<select><option selected>内置</option><optgroup label="Extensions"><option>Material Design Icons</option><option></option><option disabled>Unavailable</option></optgroup></select>`)
	if err != nil {
		t.Fatal(err)
	}
	selectNode := document.Children[0]
	for _, test := range []struct {
		index int
		want  string
	}{{0, "内置"}, {1, "Material Design Icons"}, {2, ""}, {3, "Unavailable"}, {-1, ""}} {
		dom.SetSelectedIndex(selectNode, test.index)
		if got := selectedOptionText(selectNode); got != test.want {
			t.Fatalf("selected index %d: text=%q, want %q", test.index, got, test.want)
		}
	}
	dom.SetSelectedIndex(selectNode, 1)
	if allocations := testing.AllocsPerRun(100, func() { selectedOptionText(selectNode) }); allocations != 0 {
		t.Fatalf("selected option text allocated %g times", allocations)
	}
}

func TestShapeTokenDoesNotCacheLongSourceTokens(t *testing.T) {
	face := loadLayoutTestFace(t)
	token := textToken{Text: strings.Repeat("x", textShapeCacheTokenLimit+1)}
	first, _, err := shapeToken(face, token, 16, 0, nil)
	if err != nil {
		t.Fatalf("first long token shape: %v", err)
	}
	second, _, err := shapeToken(face, token, 16, 0, nil)
	if err != nil {
		t.Fatalf("second long token shape: %v", err)
	}
	if len(first) == 0 || len(second) == 0 || &first[0] == &second[0] {
		t.Fatal("long source token was retained by the shape cache")
	}
}

func TestLineHeightKeepsNegativeHalfLeading(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="text">+</div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`#text { font-size: 16px; line-height: 1px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	face := loadLayoutTestFace(t)
	root, err := Build(doc, styles, face, 120, 80)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}
	var textBox *Box
	var find func(*Box)
	find = func(box *Box) {
		if box == nil || textBox != nil {
			return
		}
		if value, ok := box.Node.Attr("id"); ok && value == "text" {
			textBox = box
			return
		}
		for _, child := range box.Children {
			find(child)
		}
	}
	find(root)
	if textBox == nil || len(textBox.TextFragments) != 1 || len(textBox.TextFragments[0].Glyphs) != 1 {
		t.Fatalf("text layout = %#v", textBox)
	}
	fragment := textBox.TextFragments[0]
	fontSize := 16.0
	fontAscent := float64(face.Hhea.Ascent) * (fontSize / float64(face.Head.UnitsPerEm))
	fontDescent := math.Abs(float64(face.Hhea.Descent)) * (fontSize / float64(face.Head.UnitsPerEm))
	leading := 1 - (fontAscent + fontDescent)
	wantAscent := fontAscent + leading/2
	if got := fragment.Glyphs[0].Y - fragment.Rect.Y; math.Abs(got-wantAscent) > 1e-9 {
		t.Fatalf("glyph baseline ascent = %v, want %v", got, wantAscent)
	}
}

func TestUsedLineHeightUsesMetricsForNamedUIFont(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="text">+</div></body></html>`)
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	stylesheet, err := css.Parse(`#text { font-family: ui; font-size: 13px; line-height: normal; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	face := loadLayoutTestFace(t)
	text := doc.Children[0].Children[1].Children[0]
	computed := styles[text]
	descent := float64(face.Hhea.Descent)
	if descent > 0 {
		descent = 0
	}
	want := (float64(face.Hhea.Ascent) - descent + float64(face.Hhea.LineGap)) * (13 / float64(face.Head.UnitsPerEm))
	if got := UsedLineHeight(computed, face); math.Abs(got-want) > 1e-9 {
		t.Fatalf("named ui line-height = %v, want %v", got, want)
	}
}
