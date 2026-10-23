package layout

import (
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func TestFlexAutoHeightIncludesBlocksAndWrappedLines(t *testing.T) {
	doc, err := dom.Parse(`<html><body>
		<div id="overlay"><div id="dialog"><div class="block"></div><div class="block"></div></div></div>
		<div id="column"><div id="actions"><div class="action"></div><div class="action"></div><div class="action"></div></div><div id="preview"></div></div>
	</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`
		* { box-sizing:border-box } body { margin:0 }
		#overlay { display:flex; align-items:center; justify-content:center; width:400px; height:300px }
		#dialog { width:200px; padding:10px; border:1px solid black }
		.block { height:40px; margin-bottom:8px }
		#column { display:flex; flex-direction:column; gap:12px; width:240px }
		#actions { display:flex; flex-wrap:wrap; gap:8px }
		.action { flex:1 0 100px; height:32px }
		#preview { height:100px }
	`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, nil, 500, 600)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		id   string
		rect Rect
	}{
		{"dialog", Rect{X: 100, Y: 91, Width: 200, Height: 118}},
		{"actions", Rect{X: 0, Y: 300, Width: 240, Height: 72}},
		{"preview", Rect{X: 0, Y: 384, Width: 240, Height: 100}},
	} {
		box := findHashTestBoxByID(root, expected.id)
		if box == nil || box.BorderBox() != expected.rect {
			t.Fatalf("%s: got %+v, want %+v", expected.id, box, expected.rect)
		}
	}
}
