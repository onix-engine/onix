package layout

import (
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func tableFixture(t *testing.T, source, cssSource string) (*dom.Node, style.Map, *Box, map[string]*Box) {
	t.Helper()
	doc, err := dom.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(cssSource)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, sheet)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, loadLayoutTestFace(t), 320, 200)
	if err != nil {
		t.Fatal(err)
	}
	boxes := map[string]*Box{}
	var visit func(*Box)
	visit = func(box *Box) {
		if id, ok := box.Node.Attr("id"); ok {
			boxes[id] = box
		}
		for _, child := range box.Children {
			visit(child)
		}
	}
	visit(root)
	return doc, styles, root, boxes
}

func TestTableFixedLayoutUsesFirstRowWidthsAndIgnoresLaterText(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<table id="table"><tr><td id="first" style="width:60px">A</td><td id="second">B</td></tr><tr><td id="later">averylongwordwhichmustnotsizethefirstcolumn</td><td id="last">C</td></tr></table>`, `table { width:200px; table-layout:fixed; border-spacing:0; }`)
	if boxes["first"].BorderBox().Width != 60 || boxes["second"].BorderBox().Width != 140 {
		t.Fatalf("fixed column widths = %g, %g", boxes["first"].BorderBox().Width, boxes["second"].BorderBox().Width)
	}
	if boxes["later"].Content.X != boxes["first"].Content.X || boxes["last"].Content.X != boxes["second"].Content.X {
		t.Fatal("columns are not aligned between rows")
	}
}

func TestTableSpansAndSpacing(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<table id="table"><tr><td id="spanning" rowspan="2">A</td><td id="top" colspan="2">B</td></tr><tr><td id="middle">C</td><td id="last">D</td></tr></table>`, `table { width:240px; table-layout:fixed; border-spacing:4px 6px; } td { height:20px; }`)
	if boxes["middle"].BorderBox().X <= boxes["spanning"].BorderBox().X || boxes["top"].BorderBox().X != boxes["middle"].BorderBox().X {
		t.Fatal("rowspan did not reserve its column")
	}
	if !closeFloat(boxes["top"].BorderBox().Width, boxes["middle"].BorderBox().Width+4+boxes["last"].BorderBox().Width) {
		t.Fatal("colspan did not include intervening spacing")
	}
	if !closeFloat(boxes["spanning"].BorderBox().Height, boxes["top"].BorderBox().Height+6+boxes["middle"].BorderBox().Height) {
		t.Fatal("rowspan did not span both row heights")
	}
}

func TestTableAutoLayoutRetainsMinimumContentWidth(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<table id="table"><tr><td id="word">unbreakableword</td><td id="other">B</td></tr></table>`, `table { width:20px; border-spacing:0; }`)
	cell := boxes["word"]
	if cell.Content.Width < cell.TextFragments[0].Rect.Width {
		t.Fatal("auto layout shrank a column below its minimum content width")
	}
	if boxes["table"].Content.Width <= 20 {
		t.Fatal("auto layout did not expand to minimum column widths")
	}
}

func TestTableRowSpansStayInsideTheirRowGroup(t *testing.T) {
	for _, span := range []string{"0", "5"} {
		_, _, _, boxes := tableFixture(t, `<table><tbody><tr><td id="span" rowspan="`+span+`">A</td><td id="first">B</td></tr><tr><td id="second">C</td></tr></tbody><tbody><tr><td id="next">D</td><td>E</td></tr></tbody></table>`, `table{width:200px;table-layout:fixed;border-spacing:0}td{height:20px}`)
		if boxes["span"].BorderBox().Height != 40 || boxes["second"].Content.X != boxes["first"].Content.X || boxes["next"].Content.X != boxes["span"].Content.X {
			t.Fatalf("rowspan=%s escaped its group: span=%+v second=%+v next=%+v", span, boxes["span"].BorderBox(), boxes["second"].Content, boxes["next"].Content)
		}
	}
}

func TestTableHeaderFooterAndCaptionOrder(t *testing.T) {
	_, _, _, boxes := tableFixture(t, `<table><caption id="caption">Caption</caption><tfoot><tr><td id="footer">Footer</td></tr></tfoot><tbody><tr><td id="body">Body</td></tr></tbody><thead><tr><td id="header">Header</td></tr></thead></table>`, `table { border-spacing:0; }`)
	if !(boxes["caption"].Content.Y < boxes["header"].Content.Y && boxes["header"].Content.Y < boxes["body"].Content.Y && boxes["body"].Content.Y < boxes["footer"].Content.Y) {
		t.Fatal("caption, header, body and footer are not laid out in table order")
	}
}

func TestTableIncrementalLayoutMatchesFreshAcrossCellChanges(t *testing.T) {
	doc, styles, _, boxes := tableFixture(t, `<table id="table"><tr><td id="span" rowspan="2">Span</td><td id="target">Short</td></tr><tr><td id="last">Last</td></tr></table><div>Tail</div>`, `table{width:240px;border-spacing:3px 5px}td{padding:2px;vertical-align:middle}`)
	face := loadLayoutTestFace(t)
	var cache Cache
	updateRetained(t, &cache, doc, styles, face, nil, true)
	target := boxes["target"].Node
	dirty := []*dom.Node{target}
	computed := styles.Exclusive(target)
	for _, height := range []float64{60, 20, 40, 0} {
		computed.Height = style.LengthPx(height)
		got := updateRetained(t, &cache, doc, styles, face, dirty, false)
		want, err := BuildWithFonts(doc, styles, face, face, 320, 200)
		if err != nil {
			t.Fatal(err)
		}
		assertRetainedMatchesFull(t, got, want)
	}
	for range 2 {
		computed.Height = style.LengthPx(60)
		updateRetained(t, &cache, doc, styles, face, dirty, false)
		computed.Height = style.LengthPx(20)
		updateRetained(t, &cache, doc, styles, face, dirty, false)
	}
	if allocations := testing.AllocsPerRun(50, func() {
		computed.Height = style.LengthPx(60)
		updateRetained(t, &cache, doc, styles, face, dirty, false)
		computed.Height = style.LengthPx(20)
		updateRetained(t, &cache, doc, styles, face, dirty, false)
	}); allocations != 0 {
		t.Fatalf("warm table reflow allocations = %g", allocations)
	}
}
