package layout

import (
	"testing"

	"github.com/diiyw/onix/internal/css"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
)

func TestButtonAutoHeightContainsWrappedText(t *testing.T) {
	face := loadLayoutTestFace(t)
	for _, display := range []string{"block", "inline-block"} {
		for _, sizing := range []string{"content-box", "border-box"} {
			for _, message := range []struct {
				name, text string
				lines      int
			}{
				{"single", "short", 1},
				{"newline", "first\n    second\n    third", 3},
				{"wrap", "diagnostic diagnostic diagnostic", 3},
			} {
				t.Run(display+"/"+sizing+"/"+message.name, func(t *testing.T) {
					document, err := dom.Parse(`<html><body><div><button id="problem">` + message.text + `</button></div><div id="next">next</div></body></html>`)
					if err != nil {
						t.Fatal(err)
					}
					sheet, err := css.Parse(`button{display:` + display + `;box-sizing:` + sizing + `;width:120px;padding:5px 8px;border:1px solid;font:16px/20px monospace;white-space:pre-wrap;text-align:left}`)
					if err != nil {
						t.Fatal(err)
					}
					styles, err := style.Compute(document, sheet)
					if err != nil {
						t.Fatal(err)
					}
					root, err := Build(document, styles, face, 400, 300)
					if err != nil {
						t.Fatal(err)
					}
					button, next := findHashTestBoxByID(root, "problem"), findHashTestBoxByID(root, "next")
					if button == nil || next == nil || len(button.TextFragments) == 0 {
						t.Fatal("missing diagnostic layout")
					}
					if want := float64(message.lines * 20); !closeFloat(button.Content.Height, want) {
						t.Errorf("content height=%v, want %v", button.Content.Height, want)
					}
					for _, fragment := range button.TextFragments {
						if fragment.Rect.Y < button.Content.Y-1e-9 || fragment.Rect.Y+fragment.Rect.Height > button.Content.Y+button.Content.Height+1e-9 {
							t.Errorf("text %q at %+v outside content %+v", fragment.Text, fragment.Rect, button.Content)
						}
					}
					if next.BorderBox().Y < button.BorderBox().Y+button.BorderBox().Height-1e-9 {
						t.Errorf("next row overlaps diagnostic: button=%+v next=%+v", button.BorderBox(), next.BorderBox())
					}
				})
			}
		}
	}
}
