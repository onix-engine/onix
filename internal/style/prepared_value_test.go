package style

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/internal/css"
	"github.com/diiyw/onix/internal/dom"
)

func TestPreparedDeclarationsRetainFontAndViewportContext(t *testing.T) {
	const declarations = `width:2em;height:3rem;min-width:10vw;max-height:20vh;left:calc(1em + 2px);letter-spacing:.1em`
	for _, inline := range []bool{false, true} {
		t.Run(fmt.Sprintf("inline=%v", inline), func(t *testing.T) {
			source := `<html style="font-size:18px"><body style="font-size:20px"><div></div></body></html>`
			doc, err := dom.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			body := doc.Children[0].Children[1]
			target := body.Children[0]
			cssSource := "div{" + declarations + "}"
			if inline {
				target.Attributes = map[string]string{"style": declarations}
				cssSource = ""
			}
			sheet, err := css.Parse(cssSource)
			if err != nil {
				t.Fatal(err)
			}
			for index, fontSize := range []float64{20, 30} {
				body.Attributes["style"] = fmt.Sprintf("font-size:%gpx", fontSize)
				viewport := Viewport{Width: 400 * float64(index+1), Height: 300 * float64(index+1)}
				styles, err := ComputeWithViewport(doc, sheet, viewport)
				if err != nil {
					t.Fatal(err)
				}
				computed := styles[target]
				if computed.Width != LengthPx(2*fontSize) || computed.Height != LengthPx(54) || computed.MinWidth != LengthPx(viewport.Width*.1) || computed.MaxHeight != LengthPx(viewport.Height*.2) || ResolveLength(computed.Left, 0) != fontSize+2 || computed.LetterSpacing != LengthPx(fontSize*.1) {
					t.Fatalf("declaration froze its context: width=%v height=%v min=%v max=%v left=%v spacing=%v", computed.Width, computed.Height, computed.MinWidth, computed.MaxHeight, computed.Left, computed.LetterSpacing)
				}
			}
		})
	}
}

func TestInvalidDeclarationsPreserveEarlierValidValues(t *testing.T) {
	doc, err := dom.Parse(`<div style="display:block;display:banana;line-height:2;line-height:invalid;font:16px/invalid sans-serif;color:blue"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(`div{display:flex;display:banana;line-height:3;line-height:invalid;font:20px/invalid sans-serif}`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, sheet)
	if err != nil {
		t.Fatal(err)
	}
	computed := styles[doc.Children[0]]
	if computed.Display != Keyword("block") || computed.LineHeight != LengthPx(32) || computed.Color != ColorValue(Color{B: 255, A: 255}) {
		t.Fatalf("invalid declaration overrode valid values: display=%v line-height=%v color=%v", computed.Display, computed.LineHeight, computed.Color)
	}
}

func TestFilterFunctionDefaultsAndInvalidArguments(t *testing.T) {
	ctx := parseContext{FontSize: 16, ParentFontSize: 16, RootFontSize: 16}
	for _, source := range []string{"blur()", "brightness()", "opacity()", "contrast()", "grayscale()", "saturate()", "sepia()", "invert()", "hue-rotate()"} {
		if _, err := parseFilterFunction(source, ctx); err != nil {
			t.Fatalf("valid default %s: %v", source, err)
		}
	}
	for _, source := range []string{"blur(-1px)", "blur(10%)", "brightness(-1)", "opacity(NaN)", "contrast(+Inf)", "drop-shadow(1px 2px -3px)", "drop-shadow(1% 2px)"} {
		if _, err := parseFilterFunction(source, ctx); err == nil {
			t.Fatalf("accepted invalid filter: %s", source)
		}
	}
}
