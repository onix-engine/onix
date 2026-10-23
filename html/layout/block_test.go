package layout

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func closeFloat(got, want float64) bool {
	return math.Abs(got-want) < 0.000001
}

func TestBuildBlockLayout(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="outer"><div id="inner"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#outer { width: 200px; margin: 10px; padding: 5px; border: 2px solid #000; }
		#inner { height: 40px; margin: 3px 4px; padding: 6px; border: 1px solid #000; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 300, 200)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	html := root.Children[0]
	body := html.Children[0]
	outer := body.Children[0]
	inner := outer.Children[0]

	if outer.Content.X != 17 || outer.Content.Y != 17 {
		t.Fatalf("outer content origin = (%v, %v)", outer.Content.X, outer.Content.Y)
	}

	if outer.Content.Width != 200 {
		t.Fatalf("outer content width = %v", outer.Content.Width)
	}

	if inner.Content.X != 28 || inner.Content.Y != 27 {
		t.Fatalf("inner content origin = (%v, %v)", inner.Content.X, inner.Content.Y)
	}

	if inner.Content.Width != 178 || inner.Content.Height != 40 {
		t.Fatalf("inner content rect = %#v", inner.Content)
	}

	if outer.Content.Height != 60 {
		t.Fatalf("outer content height = %v", outer.Content.Height)
	}
}

func TestListItemMarkersUseOrderedValuesAndListStyle(t *testing.T) {
	doc, err := dom.Parse(`<html><body><ul><li id="bullet">One</li></ul><ol start="3"><li id="three">Three</li><li id="seven" value="7">Seven</li></ol></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`#bullet { list-style-type: square; } #seven { list-style-type: upper-roman; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	root, err := Build(doc, styles, loadLayoutTestFace(t), 320, 200)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}
	find := func(node *dom.Node) *Box {
		var visit func(*Box) *Box
		visit = func(box *Box) *Box {
			if box == nil {
				return nil
			}
			if box.Node == node {
				return box
			}
			for _, child := range box.Children {
				if found := visit(child); found != nil {
					return found
				}
			}
			return nil
		}
		return visit(root)
	}
	byID := func(id string) *dom.Node {
		var found *dom.Node
		var visit func(*dom.Node)
		visit = func(node *dom.Node) {
			if value, ok := node.Attr("id"); ok && value == id {
				found = node
				return
			}
			for _, child := range node.Children {
				if found == nil {
					visit(child)
				}
			}
		}
		visit(doc)
		return found
	}
	for id, want := range map[string]string{"bullet": "▪", "three": "3.", "seven": "VII."} {
		box := find(byID(id))
		if box == nil {
			t.Fatalf("missing layout box for %s", id)
		}
		found := false
		for _, fragment := range box.TextFragments {
			if fragment.Text == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s marker missing %q: %#v", id, want, box.TextFragments)
		}
	}
}

func TestRelativePositionOffsetsVisualBoxWithoutChangingFlow(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="first"></div><div id="second"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#first { width: 40px; height: 20px; position: relative; left: 15px; top: 10px; }
		#second { width: 30px; height: 10px; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 200, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	body := root.Children[0].Children[0]
	first := body.Children[0]
	second := body.Children[1]

	if first.Content.X != 15 || first.Content.Y != 10 {
		t.Fatalf("first content origin = (%v, %v)", first.Content.X, first.Content.Y)
	}

	if first.FlowMarginBox.X != 0 || first.FlowMarginBox.Y != 0 || first.FlowMarginBox.Height != 20 {
		t.Fatalf("first flow margin box = %#v", first.FlowMarginBox)
	}

	if second.Content.X != 0 || second.Content.Y != 20 {
		t.Fatalf("second content origin = (%v, %v)", second.Content.X, second.Content.Y)
	}
	if body.Content.Height != 30 {
		t.Fatalf("body content height = %v", body.Content.Height)
	}
}

func TestRelativePositionMovesInlineImageFragments(t *testing.T) {
	tempDir := t.TempDir()
	imagePath := filepath.Join(tempDir, "photo.png")
	file, err := os.Create(imagePath)
	if err != nil {
		t.Fatalf("create image: %v", err)
	}
	asset := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			asset.SetRGBA(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	if err := png.Encode(file, asset); err != nil {
		file.Close()
		t.Fatalf("encode png: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close image: %v", err)
	}

	doc, err := dom.Parse(`<html><body><p id="line"><img id="photo" src="` + imagePath + `"></p></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#line { position: relative; left: 12px; top: 8px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 120, 80)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	line := root.Children[0].Children[0].Children[0]
	if len(line.ImageFragments) != 1 {
		t.Fatalf("image fragments = %d, want 1", len(line.ImageFragments))
	}
	fragment := line.ImageFragments[0]
	if fragment.Rect.X != line.Content.X || fragment.Rect.Y != line.Content.Y {
		t.Fatalf("image fragment rect = %#v, want origin (%v, %v)", fragment.Rect, line.Content.X, line.Content.Y)
	}
	if line.FlowMarginBox.X != 0 || line.FlowMarginBox.Y != 0 {
		t.Fatalf("flow margin box moved with relative positioning: %#v", line.FlowMarginBox)
	}
	if body := root.Children[0].Children[0]; body.Content.Height != 2 {
		t.Fatalf("body content height = %v", body.Content.Height)
	}
	if fragment.Rect.Width != 2 || fragment.Rect.Height != 2 {
		t.Fatalf("image fragment size = %#v, want 2x2", fragment.Rect)
	}
}

func TestInlineImageUsesExplicitSizeWithoutResourceSource(t *testing.T) {
	doc, err := dom.Parse(`<html><body><p id="line"><img id="icon" src="missing.svg"></p></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`#icon { width: 16px; height: 16px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	root, err := Build(doc, styles, nil, 120, 80)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}
	line := root.Children[0].Children[0].Children[0]
	if len(line.ImageFragments) != 1 {
		t.Fatalf("image fragments = %d, want 1", len(line.ImageFragments))
	}
	fragment := line.ImageFragments[0]
	if fragment.Rect.Width != 16 || fragment.Rect.Height != 16 {
		t.Fatalf("image fragment size = %#v, want 16x16", fragment.Rect)
	}
}

func TestAbsolutePositionUsesNearestPositionedAncestor(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="outer"><div id="abs"></div><div id="flow"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#outer { width: 120px; margin: 10px; padding: 5px; position: relative; }
		#abs { width: 20px; height: 10px; position: absolute; left: 7px; top: 9px; }
		#flow { width: 40px; height: 15px; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 240, 160)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	outer := root.Children[0].Children[0].Children[0]
	abs := outer.Children[0]
	flow := outer.Children[1]

	if abs.Content.X != 17 || abs.Content.Y != 19 {
		t.Fatalf("absolute content origin = (%v, %v)", abs.Content.X, abs.Content.Y)
	}
	if flow.Content.X != 15 || flow.Content.Y != 15 {
		t.Fatalf("flow content origin = (%v, %v)", flow.Content.X, flow.Content.Y)
	}
	if outer.Content.Height != 15 {
		t.Fatalf("outer content height = %v", outer.Content.Height)
	}
}

func TestAbsoluteInlineElementIsBlockified(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div style="position:relative;width:200px;height:120px"><span id="label" style="position:absolute;left:24px;top:16px;width:80px;height:20px">Label</span></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, nil, 200, 120)
	if err != nil {
		t.Fatal(err)
	}
	container := root.Children[0].Children[0].Children[0]
	if len(container.Children) != 1 {
		t.Fatalf("absolute inline child missing from box tree: %#v", container.Children)
	}
	id, _ := container.Children[0].Node.Attr("id")
	if id != "label" {
		t.Fatalf("absolute inline child id = %q", id)
	}
	label := container.Children[0]
	if label.Content.X != 24 || label.Content.Y != 16 {
		t.Fatalf("absolute inline position = %+v", label.Content)
	}
}

func TestAbsoluteBottomPositionUsesAutoHeight(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="box"><div id="label">label</div></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`
		#box { width: 100px; height: 60px; position: relative; }
		#label { position: absolute; bottom: 9px; }
	`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, loadLayoutTestFace(t), 120, 80)
	if err != nil {
		t.Fatal(err)
	}
	label := root.Children[0].Children[0].Children[0].Children[0]
	wantY := 60 - 9 - label.Content.Height
	if math.Abs(label.Content.Y-wantY) > 0.000001 {
		t.Fatalf("absolute bottom content y = %v, want %v", label.Content.Y, wantY)
	}
}

func TestColumnFlexCentersIntrinsicItems(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="column"><span id="item">center</span></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#column { width: 100px; height: 60px; display: flex; flex-direction: column; align-items: center; justify-content: center; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, loadLayoutTestFace(t), 120, 80)
	if err != nil {
		t.Fatal(err)
	}
	item := root.Children[0].Children[0].Children[0].Children[0]
	if item.Content.Width <= 0 || item.Content.Width >= 100 {
		t.Fatalf("intrinsic item width = %v", item.Content.Width)
	}
	wantX := (100 - item.Content.Width) / 2
	if math.Abs(item.Content.X-wantX) > 0.000001 {
		t.Fatalf("centered item x = %v, want %v", item.Content.X, wantX)
	}
}

func TestColumnFlexAutoItemMeasuresFlowChildren(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="column"><div id="item"><div id="child"></div></div></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`
		#column { display: flex; flex-direction: column; width: 100px; height: 100px; }
		#child { height: 20px; }
	`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, nil, 120, 120)
	if err != nil {
		t.Fatal(err)
	}
	column := root.Children[0].Children[0].Children[0]
	item := column.Children[0]
	if item.Content.Height != 20 {
		t.Fatalf("auto flex item height = %v, want 20", item.Content.Height)
	}
	if item.Children[0].Content.Y != item.Content.Y {
		t.Fatalf("flow child y = %v, want %v", item.Children[0].Content.Y, item.Content.Y)
	}
}

func TestFlexLayoutPlacesChildrenInRow(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="row"><div id="left"></div><div id="right"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#row { display: flex; width: 200px; margin: 10px; padding: 5px; border: 2px solid #000; }
		#left { width: 40px; height: 30px; margin: 0 6px 0 0; }
		#right { width: 50px; height: 20px; margin: 0 0 0 4px; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 320, 200)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	row := root.Children[0].Children[0].Children[0]
	left := row.Children[0]
	right := row.Children[1]

	if row.Content.X != 17 || row.Content.Y != 17 {
		t.Fatalf("row content origin = (%v, %v)", row.Content.X, row.Content.Y)
	}
	if left.Content.X != 17 || left.Content.Y != 17 {
		t.Fatalf("left content origin = (%v, %v)", left.Content.X, left.Content.Y)
	}
	if right.Content.X != 67 || right.Content.Y != 17 {
		t.Fatalf("right content origin = (%v, %v)", right.Content.X, right.Content.Y)
	}
	if row.Content.Height != 30 {
		t.Fatalf("row content height = %v", row.Content.Height)
	}
	if row.FlowMarginBox.Width != 234 || row.FlowMarginBox.Height != 64 {
		t.Fatalf("row flow margin box = %#v", row.FlowMarginBox)
	}
	if right.Content.Width != 50 || right.Content.Height != 20 {
		t.Fatalf("right content rect = %#v", right.Content)
	}
}

func TestFlexBasisUsesBorderBoxForBorderBoxItems(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="row"><div id="item"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`
		#row { display: flex; width: 200px; height: 80px; }
		#item { flex: 0 0 60px; box-sizing: border-box; padding: 10px; border: 1px solid #000; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	root, err := Build(doc, styles, nil, 240, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}
	item := root.Children[0].Children[0].Children[0].Children[0]
	if got := item.BorderBox(); got.Width != 60 || got.Height != 80 {
		t.Fatalf("flex item border box = %#v, want 60x80", got)
	}
	if item.Content.Width != 38 {
		t.Fatalf("flex item content width = %v, want 38", item.Content.Width)
	}
}

func TestColumnFlexBasisUsesBorderBoxForBorderBoxItems(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="column"><div id="item"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`
		#column { display: flex; flex-direction: column; width: 100px; height: 200px; }
		#item { flex: 0 0 60px; box-sizing: border-box; padding: 10px; border: 1px solid #000; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	root, err := Build(doc, styles, nil, 240, 240)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}
	item := root.Children[0].Children[0].Children[0].Children[0]
	if got := item.BorderBox(); got.Width != 100 || got.Height != 60 {
		t.Fatalf("column flex item border box = %#v, want 100x60", got)
	}
	if item.Content.Width != 78 || item.Content.Height != 38 {
		t.Fatalf("column flex item content size = %v x %v, want 78 x 38", item.Content.Width, item.Content.Height)
	}
}

func TestFlexLayoutIgnoresCollapsedWhitespaceItems(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="row">
  <div id="left"></div>
  <div id="right"></div>
</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#row { display: flex; width: 200px; gap: 10px; } #left, #right { width: 40px; height: 20px; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, nil, 240, 120)
	if err != nil {
		t.Fatal(err)
	}
	row := root.Children[0].Children[0].Children[0]
	if len(row.Children) != 2 {
		t.Fatalf("flex children = %d, want 2", len(row.Children))
	}
	if row.Children[0].Content.X != 0 || row.Children[1].Content.X != 50 {
		t.Fatalf("flex child positions = %v, %v", row.Children[0].Content.X, row.Children[1].Content.X)
	}
}

func TestFlexLayoutOmitsDisplayNoneItems(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="row"><div id="hidden"></div><div id="visible"></div></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#row { display:flex; gap:10px; width:100px; } #hidden { display:none; width:40px; } #visible { width:20px; height:10px; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, nil, 120, 80)
	if err != nil {
		t.Fatal(err)
	}
	row := root.Children[0].Children[0].Children[0]
	if len(row.Children) != 1 {
		t.Fatalf("display:none flex children = %#v", row.Children)
	}
	id, _ := row.Children[0].Node.Attr("id")
	if id != "visible" {
		t.Fatalf("display:none remaining child = %q", id)
	}
}

func TestInlineBlockInputAndButtonProduceAtomicBoxes(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="line"><input id="query" type="text" value="Go"><input id="submit" type="submit" value="Search"><button id="lucky">Lucky</button></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#line { width: 320px; }
		#query, #submit, #lucky { display: inline-block; margin-right: 8px; }
		#query { width: 120px; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	face := loadLayoutTestFace(t)
	root, err := Build(doc, styles, face, 400, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	line := root.Children[0].Children[0].Children[0]
	if len(line.Children) != 3 {
		t.Fatalf("control boxes = %d, want 3", len(line.Children))
	}
	query := line.Children[0]
	submit := line.Children[1]
	lucky := line.Children[2]
	if query.BorderBox().X >= submit.BorderBox().X || submit.BorderBox().X >= lucky.BorderBox().X {
		t.Fatalf("control box order/positions = %#v %#v %#v", query.BorderBox(), submit.BorderBox(), lucky.BorderBox())
	}
	if len(query.TextFragments) == 0 || len(submit.TextFragments) == 0 || len(lucky.TextFragments) == 0 {
		t.Fatal("inline control text fragments missing")
	}
	if query.TextFragments[0].Text != "Go" {
		t.Fatalf("query text = %q", query.TextFragments[0].Text)
	}
	if submit.TextFragments[0].Text != "Search" {
		t.Fatalf("submit text = %q", submit.TextFragments[0].Text)
	}
	if lucky.TextFragments[0].Text != "Lucky" {
		t.Fatalf("button text = %q", lucky.TextFragments[0].Text)
	}
}

func TestInlineControlTextAlignUsesContentBox(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="line"><input id="right" value="Go"><button id="center">OK</button></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#line { width: 320px; }
		#right, #center { display: inline-block; width: 80px; }
		#right { text-align: right; margin-right: 8px; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	face := loadLayoutTestFace(t)
	root, err := Build(doc, styles, face, 400, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	line := root.Children[0].Children[0].Children[0]
	if len(line.Children) != 2 {
		t.Fatalf("control boxes = %d, want 2", len(line.Children))
	}
	right := line.Children[0]
	center := line.Children[1]
	if len(right.TextFragments) == 0 || len(center.TextFragments) == 0 {
		t.Fatal("control text fragments missing")
	}

	rightText := right.TextFragments[0]
	if rightText.Rect.X+rightText.Rect.Width > right.Content.X+right.Content.Width+0.01 {
		t.Fatalf("right text overflows content: text=%#v content=%#v", rightText.Rect, right.Content)
	}
	if rightText.Rect.X <= right.Content.X {
		t.Fatalf("right text was not aligned right: text=%#v content=%#v", rightText.Rect, right.Content)
	}

	centerText := center.TextFragments[0]
	centerTextMid := centerText.Rect.X + centerText.Rect.Width/2
	contentMid := center.Content.X + center.Content.Width/2
	if diff := math.Abs(centerTextMid - contentMid); diff > 0.01 {
		t.Fatalf("center text midpoint = %v, content midpoint = %v", centerTextMid, contentMid)
	}
}

func TestLongTextInputMiddleCaretKeepsFollowingTextVisible(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="field" value="abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz" data-caret="30"></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#field { width: 72px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	face := loadLayoutTestFace(t)
	root, err := Build(doc, styles, face, 220, 80)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	field := root.Children[0].Children[0].Children[0]
	if len(field.TextFragments) == 0 || len(field.TextFragments[0].Glyphs) <= 31 {
		t.Fatalf("field fragments = %#v", field.TextFragments)
	}
	caretX, ok := CaretXForTextFragments(field.TextFragments, 30)
	if !ok {
		t.Fatal("caret x missing")
	}
	visibleRight := field.Content.X + field.Content.Width
	if caretX >= visibleRight-1 {
		t.Fatalf("caret x = %v should leave room before visible right %v", caretX, visibleRight)
	}
	if nextGlyphX := field.TextFragments[0].Glyphs[30].X; nextGlyphX >= visibleRight {
		t.Fatalf("next glyph x = %v should remain visible before %v", nextGlyphX, visibleRight)
	}
}

func TestInlineControlSideLonghandsAffectBoxGeometry(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="line"><input id="field" value="x"><input id="next" value="y"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#line { width: 400px; }
		#field, #next { display: inline-block; width: 20px; height: 10px; }
		#field {
			margin-left: 4px;
			margin-right: 6px;
			padding-left: 8px;
			padding-right: 10px;
			border-left-width: 2px;
			border-right-width: 3px;
		}
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	face := loadLayoutTestFace(t)
	root, err := Build(doc, styles, face, 480, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	line := root.Children[0].Children[0].Children[0]
	if len(line.Children) != 2 {
		t.Fatalf("control boxes = %d, want 2", len(line.Children))
	}
	field := line.Children[0]
	next := line.Children[1]

	if field.BorderBox().X != 4 {
		t.Fatalf("field border box x = %v, want margin-left offset 4", field.BorderBox().X)
	}
	if field.Content.X != 14 {
		t.Fatalf("field content x = %v, want border+padding offset 14", field.Content.X)
	}
	if field.Content.Width != 20 {
		t.Fatalf("field content width = %v, want css width 20", field.Content.Width)
	}
	wantNextX := 4.0 + 2 + 8 + 20 + 10 + 3 + 6
	if next.BorderBox().X != wantNextX {
		t.Fatalf("next border box x = %v, want %v", next.BorderBox().X, wantNextX)
	}
}

func TestFlexGrowAndJustifyContent(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="row"><div id="a"></div><div id="b"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#row { display: flex; width: 200px; justify-content: space-between; }
		#a { flex: 1; height: 20px; }
		#b { flex: 2; height: 20px; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 240, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	row := root.Children[0].Children[0].Children[0]
	a := row.Children[0]
	b := row.Children[1]

	if !closeFloat(a.Content.Width, 200.0/3.0) {
		t.Fatalf("a width = %v", a.Content.Width)
	}
	if !closeFloat(b.Content.Width, 400.0/3.0) {
		t.Fatalf("b width = %v", b.Content.Width)
	}
	if !closeFloat(a.Content.X, 0) || !closeFloat(b.Content.X, a.Content.Width) {
		t.Fatalf("row positions = %v, %v", a.Content.X, b.Content.X)
	}
}

func TestFlexColumnAndAlignItems(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="col"><div id="a"></div><div id="b"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#col { display: flex; flex-direction: column; width: 200px; align-items: center; row-gap: 10px; }
		#a { width: 40px; height: 20px; }
		#b { width: 60px; height: 30px; align-self: flex-end; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 240, 200)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	col := root.Children[0].Children[0].Children[0]
	a := col.Children[0]
	b := col.Children[1]

	if a.Content.X != 80 || a.Content.Y != 0 {
		t.Fatalf("a origin = (%v, %v)", a.Content.X, a.Content.Y)
	}
	if b.Content.X != 140 || b.Content.Y != 30 {
		t.Fatalf("b origin = (%v, %v)", b.Content.X, b.Content.Y)
	}
	if col.Content.Height != 60 {
		t.Fatalf("column height = %v", col.Content.Height)
	}
}

func TestBuildIncludesFormControls(t *testing.T) {
	doc, err := dom.Parse(`<html><body><button id="btn">Save</button><input id="name" value="Alice"><input id="agree" type="checkbox" checked><select id="kind"><option>One</option><option selected>Two</option></select></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	styles, err := style.Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 320, 240)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	body := root.Children[0].Children[0]
	if len(body.Children) != 4 {
		t.Fatalf("body children = %d, want 4", len(body.Children))
	}

	button := body.Children[0]
	if got := button.Node.Data; got != "button" {
		t.Fatalf("button node = %q", got)
	}
	if button.Content.Width <= 0 || button.Content.Height <= 0 {
		t.Fatalf("button content = %#v", button.Content)
	}

	input := body.Children[1]
	if got := input.Node.Data; got != "input" {
		t.Fatalf("text input node = %q", got)
	}
	if input.Content.Width <= 0 || input.Content.Height <= 0 {
		t.Fatalf("text input content = %#v", input.Content)
	}

	checkbox := body.Children[2]
	if checkbox.Content.Width != 13 || checkbox.Content.Height != 13 {
		t.Fatalf("checkbox content = %#v, want 13x13", checkbox.Content)
	}

	selectNode := body.Children[3]
	if got := selectNode.Node.Data; got != "select" {
		t.Fatalf("select node = %q", got)
	}
	if selectNode.Content.Width <= 0 || selectNode.Content.Height <= 0 {
		t.Fatalf("select content = %#v", selectNode.Content)
	}
}

func TestSelectDisplayTextFollowsLiveSelection(t *testing.T) {
	doc, err := dom.Parse(`<html><body><select id="kind"><option>One</option><option selected>Two</option><option>Three</option></select></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	styles, err := style.Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	root, err := Build(doc, styles, loadLayoutTestFace(t), 320, 240)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}
	kind := findHashTestBoxByID(root, "kind")
	if kind == nil {
		t.Fatal("select box missing")
	}
	if len(kind.TextFragments) == 0 || kind.TextFragments[0].Text != "Two" {
		t.Fatalf("authored select text = %#v", kind.TextFragments)
	}

	// A popup selection dirties live selectedness without rewriting the
	// authored selected attribute; the collapsed label must follow it.
	dom.SetSelectedIndex(kind.Node, 2)
	root, err = Build(doc, styles, loadLayoutTestFace(t), 320, 240)
	if err != nil {
		t.Fatalf("rebuild layout: %v", err)
	}
	kind = findHashTestBoxByID(root, "kind")
	if kind == nil {
		t.Fatal("select box missing after rebuild")
	}
	if len(kind.TextFragments) == 0 || kind.TextFragments[0].Text != "Three" {
		t.Fatalf("live-selected select text = %#v, want Three", kind.TextFragments)
	}
}

func TestAutoFormControlDimensionsIncludePaddingOnce(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="field" style="font:12px ui; padding:5px 7px; border:1px solid #334252"></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	face := loadLayoutTestFace(t)
	root, err := Build(doc, styles, face, 320, 120)
	if err != nil {
		t.Fatal(err)
	}
	field := root.Children[0].Children[0].Children[0]
	lineHeight := UsedLineHeight(styles[field.Node], face)
	want := lineHeight + 5*2 + 1*2
	if got := field.BorderBox().Height; math.Abs(got-want) > 0.000001 {
		t.Fatalf("auto input border box height = %v, want %v", got, want)
	}
}

func TestFlexControlIntrinsicWidthIncludesInlineImageAndGap(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="toolbar"><button id="save"><img src="icons/save.svg" width="16" height="16"><span>Save</span></button></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#toolbar { display:flex; width:200px; } #save { display:inline-flex; align-items:center; gap:7px; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, loadLayoutTestFace(t), 240, 120)
	if err != nil {
		t.Fatal(err)
	}
	button := root.Children[0].Children[0].Children[0].Children[0]
	if button.BorderBox().Width < 16+7+20+14 {
		t.Fatalf("flex button border box width = %v, image/text/gap overflow", button.BorderBox().Width)
	}
	if len(button.Children) != 2 || button.Children[0].Node.Data != "img" || button.Children[1].Node.Data != "span" {
		t.Fatalf("flex button children = %#v", button.Children)
	}
	if button.Children[1].Content.X-button.Children[0].BorderBox().X != 23 {
		t.Fatalf("flex button image/text gap = %v, want 23", button.Children[1].Content.X-button.Children[0].BorderBox().X)
	}
}

func TestFlexButtonIntrinsicWidthIncludesNestedFlexContent(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="tabs"><div id="tab"><button id="select"><span id="dot"></span><span id="name">README.md</span></button><button id="close"></button></div></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`
		* { box-sizing: border-box; }
		button { border: 1px solid transparent; font-family: ui; font-size: 13px; }
		#tabs { display: flex; width: 400px; }
		#tab { display: flex; min-width: 120px; height: 44px; border-right: 1px solid #292e36; }
		#select { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; padding: 0 8px 0 13px; border: 0; }
		#dot { width: 6px; height: 6px; flex: 0 0 6px; }
		#name { min-width: 0; white-space: nowrap; }
		#close { width: 30px; min-width: 30px; height: 30px; display: flex; padding: 0; }
	`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	face := loadLayoutTestFace(t)
	root, err := Build(doc, styles, face, 400, 100)
	if err != nil {
		t.Fatal(err)
	}
	tab := findHashTestBoxByID(root, "tab")
	name := findHashTestBoxByID(root, "name")
	if tab == nil || name == nil {
		t.Fatalf("missing tab boxes: tab=%v name=%v", tab, name)
	}
	nameWidth := measureTextWidth(face, "README.md", 13, 0, nil)
	wantTabWidth := nameWidth + 6 + 8 + 13 + 8 + 30 + 1
	if got := tab.BorderBox().Width; math.Abs(got-wantTabWidth) > 0.000001 {
		t.Fatalf("tab width = %v, want %v", got, wantTabWidth)
	}
	if got := name.Content.Width; math.Abs(got-nameWidth) > 0.000001 {
		t.Fatalf("tab name width = %v, want %v", got, nameWidth)
	}
}

func TestBuildIncludesRadioInput(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="pick" type="radio" checked></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	styles, err := style.Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 80, 80)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	radio := root.Children[0].Children[0].Children[0]
	if got := radio.Node.Data; got != "input" {
		t.Fatalf("radio node = %q", got)
	}
	if radio.Content.Width != 13 || radio.Content.Height != 13 {
		t.Fatalf("radio content = %#v, want 13x13", radio.Content)
	}
}

func TestBuildIncludesTextarea(t *testing.T) {
	doc, err := dom.Parse(`<html><body><textarea id="notes" rows="3" cols="10">Hello world</textarea></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	styles, err := style.Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 200, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	textarea := root.Children[0].Children[0].Children[0]
	if got := textarea.Node.Data; got != "textarea" {
		t.Fatalf("textarea node = %q", got)
	}
	if textarea.Content.Width <= 0 || textarea.Content.Height <= 0 {
		t.Fatalf("textarea content = %#v", textarea.Content)
	}
}

func TestBuildButtonLikeAndHiddenInputs(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="submit" type="submit"><input id="hidden" type="hidden"><input id="button" type="button" value="Run"></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	styles, err := style.Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 240, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	body := root.Children[0].Children[0]
	if len(body.Children) != 2 {
		t.Fatalf("body children = %d, want 2", len(body.Children))
	}
	if value, _ := body.Children[0].Node.Attr("id"); value != "submit" {
		t.Fatalf("first visible control id = %q", value)
	}
	if value, _ := body.Children[1].Node.Attr("id"); value != "button" {
		t.Fatalf("second visible control id = %q", value)
	}
	if body.Children[0].Content.Width <= 0 || body.Children[1].Content.Width <= 0 {
		t.Fatalf("button-like input widths = %#v %#v", body.Children[0].Content, body.Children[1].Content)
	}
}

func TestBuildHiddenInputAttribute(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="wrapper"><input id="hidden" hidden value="secret"><span>visible</span></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}

	styles, err := style.Compute(doc, nil)
	if err != nil {
		t.Fatal(err)
	}

	root, err := Build(doc, styles, loadLayoutTestFace(t), 240, 120)
	if err != nil {
		t.Fatal(err)
	}

	wrapper := root.Children[0].Children[0].Children[0]
	if len(wrapper.ControlFragments) != 0 {
		t.Fatalf("hidden input control fragments = %#v, want none", wrapper.ControlFragments)
	}
	if len(wrapper.TextFragments) != 1 || wrapper.TextFragments[0].Text != "visible" {
		t.Fatalf("visible text fragments = %#v, want visible only", wrapper.TextFragments)
	}
}

func TestFlexContainerKeepsExplicitButtonSizeAndText(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="toolbar"><button id="save" style="display:flex;align-items:center;justify-content:center;width:46px;height:44px">保存</button></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`#toolbar { display:flex; flex-direction:column; width:120px; height:100px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	face, err := font.LoadDefault()
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	root, err := Build(doc, styles, face, 200, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}
	toolbar := root.Children[0].Children[0].Children[0]
	button := toolbar.Children[0]
	if button.BorderBox().Width != 60 || button.BorderBox().Height != 50 {
		t.Fatalf("button border box = %#v, want 60x50", button.BorderBox())
	}
	var text strings.Builder
	if len(button.Children) == 1 {
		for _, fragment := range button.Children[0].TextFragments {
			text.WriteString(fragment.Text)
		}
	}
	if text.String() != "保存" {
		t.Fatalf("button text layout = %#v", button.Children)
	}
}

func TestBlockLayoutAppliesMinimumDimensions(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="panel" style="width:20px;height:20px;min-width:80px;min-height:40px"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	styles, err := style.Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	root, err := Build(doc, styles, nil, 120, 80)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}
	panel := root.Children[0].Children[0].Children[0]
	if panel.Content.Width != 80 || panel.Content.Height != 40 {
		t.Fatalf("minimum dimensions = %#v, want 80x40", panel.Content)
	}
}

func TestFlexWrapAlignContentAndOrder(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="wrap"><div id="first"></div><div id="second"></div><div id="third"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#wrap { display: flex; width: 120px; height: 100px; flex-wrap: wrap; align-content: space-between; row-gap: 10px; }
		#first { width: 60px; height: 20px; order: 2; }
		#second { width: 60px; height: 30px; order: 1; }
		#third { width: 60px; height: 25px; order: 3; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 200, 160)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	wrap := root.Children[0].Children[0].Children[0]
	second := wrap.Children[0]
	first := wrap.Children[1]
	third := wrap.Children[2]

	if second.Content.Y != 0 || first.Content.Y != 0 {
		t.Fatalf("first line y = %v, %v", second.Content.Y, first.Content.Y)
	}
	if third.Content.Y != 75 {
		t.Fatalf("third line y = %v", third.Content.Y)
	}
}

func TestBuildResolvesViewportAndPercentLengths(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#card {
			position: absolute;
			left: 10vw;
			top: 25%;
			width: 50%;
			height: 20vh;
			padding: 10%;
		}
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.ComputeWithViewport(doc, stylesheet, style.Viewport{Width: 400, Height: 200})
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	root, err := Build(doc, styles, nil, 400, 200)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	card := root.Children[0].Children[0].Children[0]
	if card.Content.X != 80 || card.Content.Y != 90 {
		t.Fatalf("content origin = %#v", card.Content)
	}
	if card.Content.Width != 200 || card.Content.Height != 40 {
		t.Fatalf("content size = %#v", card.Content)
	}
	if card.Padding.Left != 40 || card.Padding.Top != 40 {
		t.Fatalf("padding = %#v", card.Padding)
	}
}

func TestBuildHonorsBorderBoxDimensions(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="parent"><div id="child"></div></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#parent { width: 200px; height: 100px; padding: 10px; border: 2px solid #000; box-sizing: border-box; } #child { width: 100%; height: 76px; box-sizing: border-box; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.ComputeWithViewport(doc, stylesheet, style.Viewport{Width: 400, Height: 200})
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, nil, 400, 200)
	if err != nil {
		t.Fatal(err)
	}
	parent := root.Children[0].Children[0].Children[0]
	child := parent.Children[0]
	if got := parent.BorderBox(); got.Width != 200 || got.Height != 100 {
		t.Fatalf("parent border box = %#v", got)
	}
	if got := child.BorderBox(); got.Width != 176 || got.Height != 76 {
		t.Fatalf("child border box = %#v", got)
	}
}

func TestAbsoluteRightIgnoresSiblingOverflow(t *testing.T) {
	// CSS Positioned Layout: insets resolve against the containing block, not
	// the visual overflow of siblings. This also covers an open titlebar menu.
	for _, display := range []string{"block", "flex"} {
		t.Run(display, func(t *testing.T) {
			doc, err := dom.Parse(`<div id="bar"><div id="overflow"></div><div id="controls"></div></div>`)
			if err != nil {
				t.Fatal(err)
			}
			sheet, err := css.Parse(`#bar {position:relative;display:` + display + `;width:800px;height:38px;padding:0;border:0} #overflow{position:absolute;left:16px;top:0;width:1000px;height:30px} #controls{position:absolute;right:0;top:0;width:138px;height:38px}`)
			if err != nil {
				t.Fatal(err)
			}
			styles, err := style.Compute(doc, sheet)
			if err != nil {
				t.Fatal(err)
			}
			root, err := Build(doc, styles, nil, 800, 600)
			if err != nil {
				t.Fatal(err)
			}
			bar, controls := findHashTestBoxByID(root, "bar"), findHashTestBoxByID(root, "controls")
			if bar == nil || controls == nil {
				t.Fatal("missing layout boxes")
			}
			if controls.BorderBox().X+controls.BorderBox().Width != bar.PaddingBox().X+bar.PaddingBox().Width {
				t.Fatalf("controls shifted by sibling overflow: %+v", controls.BorderBox())
			}
		})
	}
}

func TestFlexIntrinsicWidthExcludesPositionedChildrenAndTheirGaps(t *testing.T) {
	face := loadLayoutTestFace(t)
	for _, direction := range []string{"row", "row-reverse", "column", "column-reverse"} {
		t.Run(direction, func(t *testing.T) {
			doc, err := dom.Parse(`<div id="outer"><div id="host"><div class="popup">Popup text</div><div id="first"></div><div class="popup"></div><div id="second"></div><div class="popup"></div></div><div id="empty"><div class="popup">Only positioned text</div></div></div>`)
			if err != nil {
				t.Fatal(err)
			}
			sheet, err := css.Parse(`#outer{display:flex;width:800px} #host,#empty{display:flex;position:relative;flex-direction:` + direction + `;gap:12px} #first{width:40px;height:20px} #second{width:20px;height:20px} .popup{position:absolute;width:300px;height:100px;padding:8px;border:1px solid black}`)
			if err != nil {
				t.Fatal(err)
			}
			styles, err := style.Compute(doc, sheet)
			if err != nil {
				t.Fatal(err)
			}
			root, err := Build(doc, styles, face, 800, 600)
			if err != nil {
				t.Fatal(err)
			}
			want := 40.0
			if strings.HasPrefix(direction, "row") {
				want = 72
			}
			host, empty := findHashTestBoxByID(root, "host"), findHashTestBoxByID(root, "empty")
			if host == nil || empty == nil {
				t.Fatal("missing flex containers")
			}
			if host.Content.Width != want {
				t.Errorf("host width=%v, want %v excluding positioned children and gaps", host.Content.Width, want)
			}
			if empty.Content.Width != 0 {
				t.Errorf("positioned-only container width=%v, want 0", empty.Content.Width)
			}
		})
	}
}

func TestInlineBlockElementFormsAtomicInlineBox(t *testing.T) {
	doc, err := dom.Parse(`<html><body><p>before<span id="chip">chip</span>after</p></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#chip { display: inline-block; padding: 4px 6px; border: 2px solid #000; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	face := loadLayoutTestFace(t)
	root, err := BuildWithFonts(doc, styles, face, face, 400, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	paragraph := root.Children[0].Children[0].Children[0]
	var chip *Box
	for _, child := range paragraph.Children {
		if child.Node != nil && child.Node.Data == "span" {
			chip = child
			break
		}
	}
	if chip == nil {
		t.Fatal("inline-block span is missing from the box tree")
	}

	border := chip.BorderBox()
	if border.Width <= 0 || border.Height <= 0 {
		t.Fatalf("chip border box = %#v", border)
	}
	// The chip's border box must include its horizontal padding and border.
	if border.Width != chip.Content.Width+16 {
		t.Fatalf("chip border width = %v, content width = %v", border.Width, chip.Content.Width)
	}

	// Inline flow must position the chip between the surrounding text.
	textFragments := paragraph.TextFragments
	if len(textFragments) < 2 {
		t.Fatalf("paragraph fragments = %#v", textFragments)
	}
	if last := textFragments[len(textFragments)-1]; last.Rect.X < border.X+border.Width {
		t.Fatalf("trailing text starts at %v, chip ends at %v", last.Rect.X, border.X+border.Width)
	}
	foundLeading := false
	for _, fragment := range textFragments {
		if fragment.Rect.X+fragment.Rect.Width <= border.X+0.01 {
			foundLeading = true
		}
	}
	if !foundLeading {
		t.Fatalf("no text fragment before the chip (chip at %v, fragments %#v)", border.X, textFragments)
	}
}

func TestInlineFlowPreservesBlockOrderAndStopsMarginCollapse(t *testing.T) {
	doc, err := dom.Parse(`<div id="host"><div id="before"></div><span id="inline"></span><div id="after"></div></div>`)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := css.Parse(`
		#host { width: 200px; }
		#before { height: 10px; margin-bottom: 20px; }
		#inline { display: inline-block; width: 10px; height: 10px; }
		#after { height: 10px; margin-top: 15px; }
	`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := style.Compute(doc, sheet)
	if err != nil {
		t.Fatal(err)
	}
	root, err := Build(doc, styles, nil, 200, 100)
	if err != nil {
		t.Fatal(err)
	}

	beforeBox := findHashTestBoxByID(root, "before")
	inlineBox := findHashTestBoxByID(root, "inline")
	afterBox := findHashTestBoxByID(root, "after")
	if beforeBox == nil || inlineBox == nil || afterBox == nil {
		t.Fatal("mixed-flow boxes are missing")
	}
	before := beforeBox.BorderBox()
	inline := inlineBox.BorderBox()
	after := afterBox.BorderBox()
	if !(before.Y < inline.Y && inline.Y < after.Y) {
		t.Fatalf("flow order: before=%+v inline=%+v after=%+v", before, inline, after)
	}
	if got := after.Y - (inline.Y + inline.Height); got != 15 {
		t.Fatalf("margin after inline box = %v, want 15 without cross-line collapse", got)
	}
}

func TestLetterSpacingWidensTextFragments(t *testing.T) {
	doc, err := dom.Parse(`<html><body><p id="a">Hello</p><p id="b">Hello</p></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#a { letter-spacing: 4px; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	face := loadLayoutTestFace(t)
	root, err := BuildWithFonts(doc, styles, face, face, 400, 120)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	body := root.Children[0].Children[0]
	spacedWidth := 0.0
	plainWidth := 0.0
	for _, paragraph := range body.Children {
		total := 0.0
		for _, fragment := range paragraph.TextFragments {
			total += fragment.Rect.Width
		}
		if paragraph.Node.Data == "p" {
			id, _ := paragraph.Node.Attr("id")
			if id == "a" {
				spacedWidth = total
			} else {
				plainWidth = total
			}
		}
	}
	const expectedSpacing = 4 * 5 // 4px per rune across "Hello"
	diff := spacedWidth - plainWidth
	if diff < expectedSpacing-0.01 || diff > expectedSpacing+0.01 {
		t.Fatalf("letter-spacing width delta = %v, want %v (spaced %v, plain %v)", diff, expectedSpacing, spacedWidth, plainWidth)
	}
}

func TestWordBreakBreakAllWrapsInsideWord(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="a">abcdefghijkl</div><div id="b">abcdefghijkl</div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#a, #b { width: 60px; }
		#a { word-break: break-all; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	face := loadLayoutTestFace(t)
	root, err := BuildWithFonts(doc, styles, face, face, 400, 200)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	body := root.Children[0].Children[0]
	lineCount := map[string]int{}
	for _, child := range body.Children {
		id, _ := child.Node.Attr("id")
		lines := map[float64]bool{}
		for _, fragment := range child.TextFragments {
			lines[fragment.Rect.Y] = true
		}
		lineCount[id] = len(lines)
	}
	if lineCount["a"] < 2 {
		t.Fatalf("break-all line count = %d, want at least 2", lineCount["a"])
	}
	if lineCount["b"] != 1 {
		t.Fatalf("default line count = %d, want 1 (word must not wrap)", lineCount["b"])
	}
}

func TestOverflowWrapBreakWordSplitsOnlyOverflowingWord(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="a">hi abcdefghijklmnopqrstuvwxyz</div><div id="b">hi abcdefghijklmnopqrstuvwxyz</div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#a, #b { width: 80px; }
		#a { overflow-wrap: break-word; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	face := loadLayoutTestFace(t)
	root, err := BuildWithFonts(doc, styles, face, face, 400, 200)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	body := root.Children[0].Children[0]
	for _, child := range body.Children {
		id, _ := child.Node.Attr("id")
		// Non-space fragments after the leading "hi" belong to the long word;
		// count how many pieces it was split into.
		pieces := 0
		for _, fragment := range child.TextFragments {
			if fragment.Text != "hi" && strings.TrimSpace(fragment.Text) != "" {
				pieces++
			}
		}
		switch id {
		case "a":
			if pieces < 2 {
				t.Fatalf("break-word split the long word into %d piece(s), want at least 2", pieces)
			}
			if first := child.TextFragments[0]; first.Text != "hi" {
				t.Fatalf("first fragment text = %q, want the unsplit word %q", first.Text, "hi")
			}
		case "b":
			if pieces != 1 {
				t.Fatalf("default long word pieces = %d, want 1 (word must wrap whole)", pieces)
			}
		}
	}
}

func TestTextOverflowEllipsisTruncatesNowrapLine(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="a">abcdefghijklmnopqrstuvwxyz</div><div id="b">abcdefghijklmnopqrstuvwxyz</div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`
		#a, #b { width: 80px; white-space: nowrap; }
		#a { text-overflow: ellipsis; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := style.Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	face := loadLayoutTestFace(t)
	root, err := BuildWithFonts(doc, styles, face, face, 400, 200)
	if err != nil {
		t.Fatalf("build layout: %v", err)
	}

	body := root.Children[0].Children[0]
	for _, child := range body.Children {
		id, _ := child.Node.Attr("id")
		last := child.TextFragments[len(child.TextFragments)-1]
		endsWithEllipsis := strings.HasSuffix(last.Text, "…")
		switch id {
		case "a":
			if !endsWithEllipsis {
				t.Fatalf("ellipsis fragment text = %q, want it to end with %q", last.Text, "…")
			}
			for _, glyph := range last.Glyphs {
				if right := glyph.X + glyph.Advance - child.Content.X; right > child.Content.Width+1 {
					t.Fatalf("ellipsis glyph overflows line box: right=%v width=%v", right, child.Content.Width)
				}
			}
		case "b":
			if endsWithEllipsis {
				t.Fatalf("clip text = %q, must not be truncated", last.Text)
			}
		}
	}
}
