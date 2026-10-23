package style

import (
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

func TestParseCalcLength(t *testing.T) {
	ctx := parseContext{Viewport: Viewport{Width: 1200, Height: 800}, FontSize: 16, RootFontSize: 16}
	for _, test := range []struct {
		input string
		want  Value
	}{
		{input: "calc(100vw - 32px)", want: LengthPx(1168)},
		{input: "calc(50% - 10px)", want: LengthCalc(-10, .5)},
		{input: "calc((20px + 10px) * 2)", want: LengthPx(60)},
		{input: "calc(10px / 2)", want: LengthPx(5)},
		{input: "calc(100% + 0px)", want: LengthPercent(1)},
	} {
		got, err := parseLength(test.input, ctx)
		if err != nil {
			t.Errorf("parseLength(%q): %v", test.input, err)
			continue
		}
		if got != test.want {
			t.Errorf("parseLength(%q) = %#v, want %#v", test.input, got, test.want)
		}
	}
}

func TestResolveCalcLength(t *testing.T) {
	value := LengthCalc(-10, .5)
	if got := ResolveLength(value, 240); got != 110 {
		t.Fatalf("ResolveLength(calc(50%% - 10px), 240) = %v, want 110", got)
	}
}

func TestComputeCalcLengthWithViewport(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="box"></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#box { width: calc(100vw - 32px); margin-left: calc(50% - 10px); }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := ComputeWithViewport(doc, stylesheet, Viewport{Width: 1200, Height: 800})
	if err != nil {
		t.Fatal(err)
	}
	box := doc.Children[0].Children[1].Children[0]
	computed := styles[box]
	if computed.Width != LengthPx(1168) {
		t.Fatalf("computed calc width = %#v, want 1168px", computed.Width)
	}
	if got := ResolveLength(computed.Margin.Left, 240); got != 110 {
		t.Fatalf("computed calc margin = %v, want 110px", got)
	}
}

func TestParseCalcLengthRejectsInvalidExpressions(t *testing.T) {
	ctx := parseContext{Viewport: Viewport{Width: 1200, Height: 800}}
	for _, input := range []string{"calc(10px + 2)", "calc(10px / 0)", "calc(10px +)", "calc(10qu)"} {
		if _, err := parseLength(input, ctx); err == nil {
			t.Errorf("parseLength(%q) accepted an invalid expression", input)
		}
	}
}
