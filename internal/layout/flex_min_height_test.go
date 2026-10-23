package layout

import (
	"testing"

	"github.com/diiyw/onix/internal/css"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
)

func TestFlexRowMinHeightCentersSingleLine(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="summary"><div id="toggle"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	stylesheet, err := css.Parse(`
		body { margin: 0; }
		#summary { display: flex; width: 100px; min-height: 38px; align-items: center; }
		#toggle { width: 20px; height: 32px; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	root, err := Build(doc, styles, nil, 100, 100)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}
	summary := findHashTestBoxByID(root, "summary")
	toggle := findHashTestBoxByID(root, "toggle")
	if summary == nil || toggle == nil {
		t.Fatalf("missing flex boxes: summary=%v toggle=%v", summary, toggle)
	}
	if got := summary.BorderBox(); got != (Rect{X: 0, Y: 0, Width: 100, Height: 38}) {
		t.Fatalf("summary rect = %+v, want 100x38 at origin", got)
	}
	if got := toggle.BorderBox(); got != (Rect{X: 0, Y: 3, Width: 20, Height: 32}) {
		t.Fatalf("toggle rect = %+v, want 20x32 centered in 38px row", got)
	}
}
