package style

import (
	"testing"

	"github.com/diiyw/onix/html/dom"
)

func TestInlineDeclarationsKeepSemicolonsInsideFunctions(t *testing.T) {
	doc, err := dom.Parse(`<div style="background-image: url(data:image/svg+xml;base64,abc;def); /* ignored; declaration */ color: #010203"></div>`)
	if err != nil {
		t.Fatalf("parse DOM: %v", err)
	}
	styles, err := Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	computed := styles[doc.Children[0]]
	if computed.BackgroundImage != "data:image/svg+xml;base64,abc;def" {
		t.Fatalf("background image = %q", computed.BackgroundImage)
	}
	if computed.Color.Color != (Color{R: 1, G: 2, B: 3, A: 255}) {
		t.Fatalf("color = %#v", computed.Color.Color)
	}
}
