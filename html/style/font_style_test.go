package style

import (
	"github.com/onix-engine/onix/html/css"
	"github.com/onix-engine/onix/html/dom"
	"testing"
)

func TestFontStyleCascade(t *testing.T) {
	doc, err := dom.Parse(`<div style="font-style:italic"><span id="inherited">a</span><span id="reset" style="font-style:initial">b</span><span id="short" style="font:16px sans-serif">c</span><span id="both" style="font:italic bold 16px sans-serif">d</span><em id="em">e</em><i id="normal" style="font-style:normal">f</i><span id="oblique" style="font-style:oblique">g</span></div>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, _ := css.Parse("")
	styles, err := Compute(doc, sheet)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"inherited": "italic", "reset": "normal", "short": "normal", "both": "italic", "em": "italic", "normal": "normal", "oblique": "oblique"}
	for node, value := range styles {
		id, _ := node.Attr("id")
		if expected, ok := want[id]; ok {
			if value.FontStyle != fontStyleKeyword(expected) {
				t.Errorf("%s style = %v", id, value.FontStyle)
			}
			if id == "both" && value.FontWeight != Number(700) {
				t.Errorf("shorthand weight = %v", value.FontWeight)
			}
			delete(want, id)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing nodes: %v", want)
	}
}
