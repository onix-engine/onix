package layout

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func TestLongTextLayoutRecordings(t *testing.T) {
	face, err := font.Load("../../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = face.Close() })
	for _, test := range []struct{ name, source, declarations, want string }{
		{"cjk", strings.Repeat("长文排版与中文 English 文字。", 40), "", "bbba68a5f9fdef9022cbe298e2333dc72528de6d3d4062eae723a8880a55f99c"},
		{"long-word", strings.Repeat("AVjéß", 140), "overflow-wrap:break-word;letter-spacing:.75px", "4e4a10b7778ada78b7b73ab979daec5b2295f4072e4518143e2518b506dc6b3c"},
		{"pre-wrap", strings.Repeat("  line one\tline two\r\n\n中文\rtext  ", 12), "white-space:pre-wrap;tab-size:3", "8fcbfce724e7e5478d06be66f9d643c7f6537e5de9e9d671bb4a626243c32f1f"},
		{"pre-line", "  One\t 中文 \r\n\n <b>Bold words</b>\n  trailing ", "white-space:pre-line", "3a699347dff83ea9dac182d432cc996b38ff09667c7a0cf6a9d281b2fa9a44d0"},
		{"ellipsis", strings.Repeat("AVj English 中文 ", 16), "white-space:nowrap;text-overflow:ellipsis;text-align:right", "bd026215bbfa2d73a2f6b34f5ecc7690d42a3d5f826b181adc3da080fd1e6c3a"},
		{"inline", `A<span>mixed 中文 text</span><img style="width:12px;height:9px"><button>Button 中文</button><span style="font-size:19px"> large text</span> tail`, "", "4ba275bff971f2fe0c77d6f0508912dc967d0932a6038158b0a9b2834da9448b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, err := dom.Parse(`<div style="width:137px;` + test.declarations + `">` + test.source + `</div>`)
			if err != nil {
				t.Fatal(err)
			}
			sheet, err := css.Parse("")
			if err != nil {
				t.Fatal(err)
			}
			styles, err := style.Compute(root, sheet)
			if err != nil {
				t.Fatal(err)
			}
			box, err := BuildWithFonts(root, styles, face, face, 320, 480)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.New()
			var text func([]TextFragment)
			text = func(fragments []TextFragment) {
				for _, fragment := range fragments {
					positions := make([]struct {
						Rune    rune
						X, Y    float64
						Advance float64
					}, len(fragment.Glyphs))
					for index, glyph := range fragment.Glyphs {
						positions[index].Rune, positions[index].X, positions[index].Y, positions[index].Advance = glyph.Rune, glyph.X, glyph.Y, glyph.Advance
					}
					fmt.Fprintf(hash, "%q %d %v %v\n", fragment.Text, fragment.Line, fragment.Rect, positions)
				}
			}
			var visit func(*Box)
			visit = func(box *Box) {
				fmt.Fprintf(hash, "%v %v\n", box.Content, box.TextHeight)
				text(box.TextFragments)
				for _, image := range box.ImageFragments {
					fmt.Fprintln(hash, image.Rect, image.Line)
				}
				for _, control := range box.ControlFragments {
					fmt.Fprintln(hash, control.Rect, control.Content)
					text(control.TextFragments)
				}
				for _, child := range box.Children {
					visit(child)
				}
			}
			visit(box)
			if got := fmt.Sprintf("%x", hash.Sum(nil)); got != test.want {
				t.Fatalf("layout recording = %s; want %s", got, test.want)
			}
		})
	}
}

func longTextCases() []struct{ name, text, declarations string } {
	var words strings.Builder
	for i := range 1024 {
		fmt.Fprintf(&words, "paragraph%04d renders long document text ", i)
	}
	return []struct{ name, text, declarations string }{
		{"cjk", strings.Repeat("长文本页面需要精确断行与稳定字形布局。", 512), ""},
		{"latin", words.String(), ""},
		{"long-word", strings.Repeat("Abcdefgh", 2048), "overflow-wrap:break-word"},
		{"pre-wrap", strings.Repeat("line one\tline two\r\n", 1024), "white-space:pre-wrap"},
	}
}

func TestLongTextWarmReflowAllocations(t *testing.T) {
	for _, test := range longTextCases() {
		t.Run(test.name, func(t *testing.T) {
			root, styles, face := retainedFixture(t, `<div id="text" style="`+test.declarations+`">`+test.text+`</div>`)
			var cache Cache
			dirty := []*dom.Node{retainedNode(root, "text")}
			if allocs := testing.AllocsPerRun(20, func() {
				if _, err := cache.Update(root, styles, face, face, 640, 480, dirty, nil, false, false); err != nil {
					panic(err)
				}
			}); allocs != 0 {
				t.Fatalf("warm long-text reflow allocated %g times", allocs)
			}
		})
	}
}

func TestLongTextGlyphStorageAndReflow(t *testing.T) {
	text := strings.Repeat("different words 中文文字 ", 100)
	root, styles, face := retainedFixture(t, `<div id="text">`+text+`<button>`+strings.Repeat("abcdefgh", 128)+`</button>尾</div>`)
	var cache Cache
	node := retainedNode(root, "text")
	for _, width := range []float64{640, 137, 920, 640} {
		got, err := cache.Update(root, styles, face, face, width, 480, []*dom.Node{node}, nil, false, false)
		if err != nil {
			t.Fatal(err)
		}
		want, err := BuildWithFonts(root, styles, face, face, width, 480)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, want)
	}
	fragments := cache.nodes[node].TextFragments
	if len(fragments) < 2 || len(fragments[0].Glyphs) == 0 || len(fragments[1].Glyphs) == 0 {
		t.Fatal("missing adjacent text fragments")
	}
	neighbor := slices.Clone(fragments[1].Glyphs)
	_ = append(fragments[0].Glyphs, GlyphFragment{Rune: 'x'})
	if !slices.Equal(neighbor, fragments[1].Glyphs) {
		t.Fatal("appending a glyph overwrote an adjacent fragment")
	}
	node.Children[0].Data = "short"
	if _, err := cache.Update(root, styles, face, face, 640, 480, []*dom.Node{node}, nil, false, false); err != nil {
		t.Fatal(err)
	}
	want, err := BuildWithFonts(root, styles, face, face, 640, 480)
	if err != nil {
		t.Fatal(err)
	}
	assertRetainedMatchesFull(t, cache.Root, want)
}

func TestLongWordWrapWithIndentAndInvalidUTF8(t *testing.T) {
	root, styles, face := retainedFixture(t, `<div id="text" style="width:70px;text-indent:18px;overflow-wrap:break-word"></div>`)
	node := retainedNode(root, "text")
	node.AppendChild(dom.NewText(strings.Repeat("Abé\xff", 256)))
	var err error
	styles, err = style.Compute(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	box, err := BuildWithFonts(root, styles, face, face, 320, 480)
	if err != nil {
		t.Fatal(err)
	}
	var joined strings.Builder
	var visit func(*Box)
	visit = func(box *Box) {
		for _, fragment := range box.TextFragments {
			joined.WriteString(fragment.Text)
			if fragment.Rect.Width > 70.01 {
				t.Fatal("wrapped text exceeds its line")
			}
		}
		for _, child := range box.Children {
			visit(child)
		}
	}
	visit(box)
	if joined.String() != strings.Repeat("Abé\uFFFD", 256) {
		t.Fatal("wrapping lost text or failed to replace invalid UTF-8")
	}
}

func BenchmarkLayoutLongText(b *testing.B) {
	for _, test := range longTextCases() {
		b.Run(test.name, func(b *testing.B) {
			root, styles, face := buildBenchDocument(b, `<div id="text" style="`+test.declarations+`">`+test.text+`</div>`, "", 640, 480)
			for _, retained := range []bool{false, true} {
				name := "fresh"
				if retained {
					name = "retained"
				}
				b.Run(name, func(b *testing.B) {
					var cache Cache
					var dirty []*dom.Node
					if retained {
						dirty = []*dom.Node{retainedNode(root, "text")}
					}
					build := func() {
						var err error
						if retained {
							_, err = cache.Update(root, styles, face, face, 640, 480, dirty, nil, false, false)
							if cache.Work.Reflowed == 0 {
								b.Fatal("text layout was skipped")
							}
						} else {
							_, err = BuildWithFonts(root, styles, face, face, 640, 480)
						}
						if err != nil {
							b.Fatal(err)
						}
					}
					build()
					b.ReportAllocs()
					for b.Loop() {
						build()
					}
				})
			}
		})
	}
}
