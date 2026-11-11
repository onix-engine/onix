package style

import (
	"testing"

	"github.com/onix-engine/onix/html/css"
	"github.com/onix-engine/onix/html/dom"
)

func TestCursorInheritanceAndOverrides(t *testing.T) {
	doc, err := dom.Parse(`<div style="cursor:col-resize"><span id="inherited">text</span><textarea id="explicit" style="cursor:text"></textarea><button id="initial" style="cursor:initial">initial</button><button id="inherit" style="cursor:INHERIT">inherit</button><button id="unset" style="cursor:unset">unset</button><button id="invalid" style="cursor:bogus">invalid</button></div>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, sheet)
	if err != nil {
		t.Fatal(err)
	}
	for node, computed := range styles {
		id, ok := node.Attr("id")
		if !ok {
			continue
		}
		want := "col-resize"
		switch id {
		case "explicit":
			want = "text"
		case "initial":
			want = "auto"
		}
		if computed.Cursor.String() != want {
			t.Errorf("%s cursor = %+v, want %s", id, computed.Cursor, want)
		}
		for _, child := range node.Children {
			if styles[child].Cursor.String() != want {
				t.Errorf("%s child did not inherit cursor:%s", id, want)
			}
		}
	}
}
