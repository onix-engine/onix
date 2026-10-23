package layout

import (
	"testing"

	"github.com/diiyw/onix/internal/css"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
)

func TestBuildAssignsStableBoxIDsAcrossRenders(t *testing.T) {
	first := buildHashTestLayout(t, 300, 200)
	second := buildHashTestLayout(t, 300, 200)

	firstCard := findHashTestBoxByID(first, "card")
	secondCard := findHashTestBoxByID(second, "card")
	if firstCard == nil || secondCard == nil {
		t.Fatalf("card boxes missing: %#v %#v", firstCard, secondCard)
	}
	if firstCard.StableID == "" {
		t.Fatal("stable ID should be assigned")
	}
	if firstCard.StableID != secondCard.StableID {
		t.Fatalf("stable ID = %q, want %q", secondCard.StableID, firstCard.StableID)
	}
	if first.GeometryHash() != second.GeometryHash() {
		t.Fatal("same layout should produce the same geometry hash")
	}
}

func TestGeometryHashChangesWhenViewportChangesLayout(t *testing.T) {
	small := buildHashTestLayout(t, 300, 200)
	large := buildHashTestLayout(t, 500, 200)

	if small.GeometryHash() == large.GeometryHash() {
		t.Fatal("geometry hash should change when viewport-dependent layout changes")
	}
	smallCard := findHashTestBoxByID(small, "card")
	largeCard := findHashTestBoxByID(large, "card")
	if smallCard == nil || largeCard == nil {
		t.Fatalf("card boxes missing: %#v %#v", smallCard, largeCard)
	}
	if smallCard.StableID != largeCard.StableID {
		t.Fatalf("stable ID changed across viewport resize: %q vs %q", smallCard.StableID, largeCard.StableID)
	}
}

func buildHashTestLayout(t *testing.T, width, height float64) *Box {
	t.Helper()
	doc, err := dom.Parse(`<html><body><div id="card"><div id="child"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`
		body { margin: 0; }
		#card { width: 50%; height: 40px; padding: 4px; }
		#child { width: 20px; height: 20px; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	root, err := Build(doc, styles, nil, width, height)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}
	return root
}

func findHashTestBoxByID(box *Box, id string) *Box {
	if box == nil {
		return nil
	}
	if box.Node != nil {
		if value, ok := box.Node.Attr("id"); ok && value == id {
			return box
		}
	}
	for _, child := range box.Children {
		if found := findHashTestBoxByID(child, id); found != nil {
			return found
		}
	}
	return nil
}
