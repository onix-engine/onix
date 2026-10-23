package style

import (
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

func TestCompiledMatcherPreservesClassAndIDCase(t *testing.T) {
	stylesheet, err := css.Parse(`#Card.Hero { color: red; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	doc, err := dom.Parse(`<div id="Card" class="Hero"></div>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	compiled := compileStylesheet(stylesheet)
	candidates := compiled.candidates(doc.Children[0])
	if len(candidates) != 1 || !css.Match(candidates[0].selector, doc.Children[0]) {
		t.Fatalf("case-sensitive selector was not indexed and matched: %#v", candidates)
	}
}
