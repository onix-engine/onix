package style

import (
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

func TestTextBoxEdgesCascadeAndInherit(t *testing.T) {
	document, err := dom.Parse(`<html><body><div id="parent"><span id="child"></span><span id="inherit"></span><span id="reset"></span><span id="invalid"></span></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(`#parent{text-box-trim:trim-both;text-box-edge:cap alphabetic}#inherit{text-box-trim:inherit}#reset{text-box-trim:unset;text-box-edge:initial}#invalid{text-box-trim:invalid;text-box-edge:cap}`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(document, sheet)
	if err != nil {
		t.Fatal(err)
	}
	for node, computed := range styles {
		id, _ := node.Attr("id")
		wantTrim, wantEdge := "none", "cap alphabetic"
		switch id {
		case "parent", "inherit":
			wantTrim = "trim-both"
		case "reset":
			wantEdge = "auto"
		case "child", "invalid":
		default:
			continue
		}
		if computed.TextBoxTrim.Keyword != wantTrim || computed.TextBoxEdge.Keyword != wantEdge {
			t.Errorf("%s: trim=%v edge=%v, want %s / %s", id, computed.TextBoxTrim, computed.TextBoxEdge, wantTrim, wantEdge)
		}
	}
}
