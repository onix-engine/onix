package layout

import (
	"math"
	"testing"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/internal/css"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
)

func TestCompactControlsCenterTextAcrossFonts(t *testing.T) {
	resources := []font.SystemFont{{Family: "NotoSansSC-Regular", Path: "../../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf"}}
	installed, err := font.SystemFonts()
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range installed {
		if source.Monospace && source.Weight == 400 && !source.Italic {
			resources = append(resources, source)
			break
		}
	}
	for _, resource := range resources {
		t.Run(resource.Family, func(t *testing.T) {
			face, err := resource.Load()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = face.Close() })
			for _, display := range []string{"inline-block", "block"} {
				t.Run(display, func(t *testing.T) {
					document, err := dom.Parse(`<html><body><button id="button">Run</button><input id="input" value="Run"><select id="select"><option>Run</option></select></body></html>`)
					if err != nil {
						t.Fatal(err)
					}
					sheet, err := css.Parse(`button,input,select{box-sizing:border-box;display:` + display + `;width:100px;height:20px;padding:0 4px;border:1px solid;font:13px ui}`)
					if err != nil {
						t.Fatal(err)
					}
					styles, err := style.Compute(document, sheet)
					if err != nil {
						t.Fatal(err)
					}
					root, err := Build(document, styles, face, 400, 100)
					if err != nil {
						t.Fatal(err)
					}
					found := 0
					check := func(id string, content Rect, fragments []TextFragment) {
						t.Helper()
						found++
						if len(fragments) == 0 {
							t.Fatalf("%s has no text", id)
						}
						for _, fragment := range fragments {
							want := content.Y + content.Height/2
							if got := fragment.Rect.Y + fragment.Rect.Height/2; math.Abs(got-want) > 1e-9 {
								t.Errorf("%s text center = %v, want %v (line height %v, content height %v)", id, got, want, fragment.Rect.Height, content.Height)
							}
						}
					}
					var visit func(*Box)
					visit = func(box *Box) {
						if id, ok := box.Node.Attr("id"); ok {
							check(id, box.Content, box.TextFragments)
						}
						for _, control := range box.ControlFragments {
							id, _ := control.Node.Attr("id")
							check(id, control.Content, control.TextFragments)
						}
						for _, child := range box.Children {
							visit(child)
						}
					}
					visit(root)
					if found != 3 {
						t.Fatalf("found %d controls, want 3", found)
					}
				})
			}
		})
	}
}

func TestTextBoxTrimmingPreservesBaselineSpacing(t *testing.T) {
	face, err := font.Load("../../testdata/fonts/Noto_Sans_SC/static/NotoSansSC-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = face.Close() })
	for _, trim := range []string{"none", "trim-start", "trim-end", "trim-both"} {
		t.Run(trim, func(t *testing.T) {
			document, err := dom.Parse("<html><body><div id='text'>HH\nHH</div></body></html>")
			if err != nil {
				t.Fatal(err)
			}
			sheet, err := css.Parse(`div{font:16px/32px ui;white-space:pre;text-box-edge:cap alphabetic;text-box-trim:` + trim + `}`)
			if err != nil {
				t.Fatal(err)
			}
			styles, err := style.Compute(document, sheet)
			if err != nil {
				t.Fatal(err)
			}
			root, err := Build(document, styles, face, 200, 200)
			if err != nil {
				t.Fatal(err)
			}
			box := findHashTestBoxByID(root, "text")
			if box == nil || len(box.TextFragments) != 2 {
				t.Fatal("expected two text lines")
			}
			first, last := box.TextFragments[0].Glyphs[0].Y, box.TextFragments[1].Glyphs[0].Y
			if math.Abs(last-first-32) > 1e-9 {
				t.Fatalf("baseline distance=%v, want 32", last-first)
			}
			if trim == "trim-start" || trim == "trim-both" {
				capHeight := float64(face.CapHeight()) * 16 / float64(face.Head.UnitsPerEm)
				if math.Abs(first-capHeight-box.Content.Y) > 1e-9 {
					t.Fatalf("first cap edge=%v, content top=%v", first-capHeight, box.Content.Y)
				}
			}
			if trim == "trim-end" || trim == "trim-both" {
				if math.Abs(last-box.Content.Y-box.Content.Height) > 1e-9 {
					t.Fatalf("last baseline=%v, content bottom=%v", last, box.Content.Y+box.Content.Height)
				}
			}
			if allocs := testing.AllocsPerRun(100, func() {
				_, _ = textBoxTrimming(box.Style, face)
			}); allocs != 0 {
				t.Fatalf("resolving text edges allocated %g times", allocs)
			}
		})
	}
}
