package layout

import (
	"testing"

	"github.com/onix-engine/onix/html/dom"
	"github.com/onix-engine/onix/html/style"
)

func TestTableMeasurementStylesShareOnlyEqualInputs(t *testing.T) {
	doc, styles, _, boxes := tableFixture(t, `<table id="table"><tr><td style="width:60px">A</td><td style="width:140px">B</td></tr><tr><td id="first">C</td><td id="wide">D</td></tr><tr><td id="peer">E</td><td id="wide-peer">F</td></tr></table>`, `table{width:200px;table-layout:fixed;border-spacing:0}td{box-sizing:border-box;padding:2px}`)
	face := loadLayoutTestFace(t)
	var cache Cache
	updateRetained(t, &cache, doc, styles, face, nil, true)
	measurement := func(id string) *style.ComputedStyle {
		return cache.nodes[boxes[id].Node].reflow.measurement
	}
	first, wide := measurement("first"), measurement("wide")
	if first == nil || wide == nil || first != measurement("peer") || wide != measurement("wide-peer") {
		t.Fatal("equal cells did not share their measured styles")
	}
	if first == wide || first.Width != style.LengthPx(56) || wide.Width != style.LengthPx(136) {
		t.Fatal("different column widths shared a measured style")
	}
	if styles[boxes["first"].Node].Width == first.Width || first.BoxSizing != style.Keyword("content-box") {
		t.Fatal("measurement changed the authored style")
	}

	// A private style can change in place between passes without changing its key.
	node := boxes["first"].Node
	computed := styles.Exclusive(node)
	for _, height := range []float64{40, 60, 20} {
		computed.Height = style.LengthPx(height)
		computed.Color = style.ColorValue(style.Color{R: uint8(height), A: 255})
		got := updateRetained(t, &cache, doc, styles, face, []*dom.Node{node}, false)
		if measured := measurement("first"); measured == measurement("peer") || measured.Height != computed.Height || measured.Color != computed.Color {
			t.Fatal("changed cell kept a stale or shared measurement")
		}
		if measurement("peer") != first || first.Height == computed.Height {
			t.Fatal("changing one cell mutated its peer's measurement")
		}
		want, err := BuildWithFonts(doc, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, want)
	}
}

func TestTableMeasurementCacheReleasesChangedWidthsAndRemovedRows(t *testing.T) {
	doc, styles, _, boxes := tableFixture(t, `<table id="table"><tr><td id="header" style="width:60px">A</td><td style="width:140px">B</td></tr><tr><td id="body">C</td><td>D</td></tr><tr id="removed-row"><td id="removed" style="color:red">E</td><td>F</td></tr></table>`, `table{width:200px;table-layout:fixed;border-spacing:0}td{box-sizing:border-box;padding:2px}`)
	face := loadLayoutTestFace(t)
	var cache Cache
	updateRetained(t, &cache, doc, styles, face, nil, true)
	table := cache.nodes[boxes["table"].Node]
	count := len(table.table.measured)
	header := boxes["header"].Node
	computed := styles.Exclusive(header)
	for _, width := range []float64{80, 100, 70, 90, 60} {
		computed.Width = style.LengthPx(width)
		got := updateRetained(t, &cache, doc, styles, face, []*dom.Node{header}, false)
		if got := cache.nodes[boxes["body"].Node].BorderBox().Width; got != width {
			t.Fatalf("changed column width = %g, want %g", got, width)
		}
		if len(table.table.measured) != count {
			t.Fatal("measurement cache retained old column widths")
		}
		want, err := BuildWithFonts(doc, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, want)
	}

	removedStyle := styles[boxes["removed"].Node]
	row := boxes["removed-row"].Node
	parent := row.Parent
	parent.RemoveChild(row)
	got := updateRetained(t, &cache, doc, styles, face, []*dom.Node{parent}, true)
	for key := range table.table.measured {
		if key.source == removedStyle {
			t.Fatal("measurement cache retained a removed row's style")
		}
	}
	if len(table.table.measured) != count-1 {
		t.Fatal("removing the unique cell style did not shrink the measurement cache")
	}
	want, err := BuildWithFonts(doc, styles, face, face, 320, 200)
	if err != nil {
		t.Fatal(err)
	}
	assertRetainedMatchesFull(t, got, want)
}
