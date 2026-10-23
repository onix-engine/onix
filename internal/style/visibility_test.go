package style

import (
	"testing"

	"github.com/diiyw/onix/internal/css"
	"github.com/diiyw/onix/internal/dom"
)

func TestVisibilityCascade(t *testing.T) {
	doc, err := dom.Parse(`<div style="visibility:hidden"><span id="inherited">a</span><span id="visible" class="visible">b</span><span id="inherit" style="visibility:INHERIT">c</span><span id="unset" style="visibility:unset">d</span><span id="initial" style="visibility:initial">e</span><span id="revert" style="visibility:revert">f</span><span id="invalid" class="invalid">g</span><span id="collapse" style="visibility:collapse">h</span></div>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(`.visible{visibility:VISIBLE}.invalid{visibility:visible;visibility:invalid}`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, sheet)
	if err != nil {
		t.Fatal(err)
	}
	wants := map[string]string{"inherited": "hidden", "visible": "visible", "inherit": "hidden", "unset": "hidden", "initial": "visible", "revert": "hidden", "invalid": "visible", "collapse": "collapse"}
	for node, computed := range styles {
		id, ok := node.Attr("id")
		if !ok {
			continue
		}
		want := Keyword(wants[id])
		if computed.Visibility != want {
			t.Errorf("%s visibility=%v, want %v", id, computed.Visibility, want)
		}
		for _, child := range node.Children {
			if styles[child].Visibility != want {
				t.Errorf("%s text visibility=%v, want %v", id, styles[child].Visibility, want)
			}
		}
	}
}

func TestVisibilityChangeDoesNotRequireLayout(t *testing.T) {
	previous, current := defaultStyle(nil), defaultStyle(nil)
	current.Visibility = Keyword("hidden")
	if got := computedStyleChangeForNode(nil, &previous, &current); got != ChangeVisual {
		t.Fatalf("visibility impact=%v, want visual", got)
	}
}
