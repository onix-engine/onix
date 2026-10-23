package style

import (
	"math"
	"testing"

	"github.com/diiyw/onix/internal/css"
	"github.com/diiyw/onix/internal/dom"
)

func TestComputeCascadeAndInheritance(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="app" class="card hero" style="color: #123456; font-size: 20px;
line-height: 1.5"><span>hello</span></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		div { color: #111111; width: 100px; margin: 8px 16px; }
		.hero { color: #222222; }
		#app { background-color: #abcdef; position: absolute; top: 10px; left: 12px; font-family: sans-serif; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	div := doc.Children[0].Children[1].Children[0]
	span := div.Children[0]

	divStyle := styles[div]
	if divStyle.Color.Color != (Color{R: 0x12, G: 0x34, B: 0x56, A: 255}) {
		t.Fatalf("div color = %#v", divStyle.Color.Color)
	}

	if divStyle.BackgroundColor.Color != (Color{R: 0xab, G: 0xcd, B: 0xef, A: 255}) {
		t.Fatalf("div background = %#v", divStyle.BackgroundColor.Color)
	}

	if divStyle.BorderRadius != uniformCorners(LengthPx(0)) {
		t.Fatalf("div border-radius default = %#v", divStyle.BorderRadius)
	}

	if divStyle.Width != LengthPx(100) {
		t.Fatalf("div width = %#v", divStyle.Width)
	}

	if divStyle.Margin.Top != LengthPx(8) || divStyle.Margin.Left != LengthPx(16) {
		t.Fatalf("div margin = %#v", divStyle.Margin)
	}

	if divStyle.Position != Keyword("absolute") || divStyle.Top != LengthPx(10) || divStyle.Left != LengthPx(12) {
		t.Fatalf("div positioning = %#v", divStyle)
	}

	if divStyle.LineHeight != LengthPx(30) {
		t.Fatalf("div line-height = %#v", divStyle.LineHeight)
	}

	spanStyle := styles[span]
	if spanStyle.Color != divStyle.Color {
		t.Fatalf("span color = %#v, want %#v", spanStyle.Color, divStyle.Color)
	}

	if spanStyle.FontSize != divStyle.FontSize || spanStyle.LineHeight != divStyle.LineHeight {
		t.Fatalf("span font inheritance = %#v", spanStyle)
	}

	if spanStyle.FontFamily != Keyword("sans-serif") {
		t.Fatalf("span font-family = %#v", spanStyle.FontFamily)
	}

	if spanStyle.TextAlign != Keyword("left") {
		t.Fatalf("span text-align = %#v", spanStyle.TextAlign)
	}

	if spanStyle.Display != Keyword("inline") {
		t.Fatalf("span display = %#v", spanStyle.Display)
	}

	if len(divStyle.Filters) != 0 {
		t.Fatalf("div filter default = %#v", divStyle.Filters)
	}
}

func TestComputeCSSWideKeywords(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="parent" style="color: #123456; font-size: 20px"><span id="kid">hi</span></div><button id="btn">ok</button></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`
		#parent { width: 120px; margin-left: 7px; padding: 4px; background: inherit; all: revert; }
		#kid { width: inherit; font-family: inherit; margin: 2px; margin-left: inherit; text-align: initial; }
		#btn { background-color: initial; text-align: inherit; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	find := func(id string) *dom.Node {
		stack := []*dom.Node{doc}
		for len(stack) > 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if node.Type == dom.ElementNode {
				if value, ok := node.Attr("id"); ok && value == id {
					return node
				}
			}
			stack = append(stack, node.Children...)
		}
		t.Fatalf("node %q not found", id)
		return nil
	}

	div := find("parent")
	kid := find("kid")
	button := find("btn")

	divStyle := styles[div]
	if divStyle.Width != LengthPx(120) {
		t.Fatalf("div width = %#v", divStyle.Width)
	}
	if divStyle.Color.Color != (Color{R: 0x12, G: 0x34, B: 0x56, A: 255}) || divStyle.FontSize != LengthPx(20) {
		t.Fatalf("div own style = %#v", divStyle)
	}

	kidStyle := styles[kid]
	if kidStyle.Color != divStyle.Color || kidStyle.FontSize != LengthPx(20) || kidStyle.FontFamily.Kind != ValueKeyword || kidStyle.FontFamily.Keyword == "" {
		t.Fatalf("kid inherit = %#v", kidStyle)
	}
	if kidStyle.Margin.Top != LengthPx(2) {
		t.Fatalf("kid margin = %#v", kidStyle.Margin)
	}
	// inherit applies the parent's computed value even for non-inherited
	// properties; the parent has the initial zero margin.
	if kidStyle.Margin.Left != LengthPx(7) {
		t.Fatalf("kid margin-left after inherit = %#v", kidStyle.Margin.Left)
	}
	if kidStyle.Width != LengthPx(120) {
		t.Fatalf("kid width after inherit = %#v", kidStyle.Width)
	}
	if kidStyle.TextAlign != Keyword("left") {
		t.Fatalf("kid text-align after initial = %#v", kidStyle.TextAlign)
	}

	buttonStyle := styles[button]
	// initial on a form control restores the UA default.
	if buttonStyle.BackgroundColor.Color != (Color{R: 239, G: 239, B: 239, A: 255}) {
		t.Fatalf("button background after initial = %#v", buttonStyle.BackgroundColor)
	}
	if buttonStyle.TextAlign != Keyword("left") {
		t.Fatalf("button text-align inherit on inherited property = %#v", buttonStyle.TextAlign)
	}
}

func TestComputeLinearGradientBackgroundImage(t *testing.T) {
	doc, err := dom.Parse(`<div id="gradient"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#gradient { background: linear-gradient(to right, #ef4444, #3b82f6); }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	gradient := doc.Children[0]
	computed := styles[gradient]
	if computed.BackgroundImage != `linear-gradient(to right, #ef4444, #3b82f6)` {
		t.Fatalf("background image = %q", computed.BackgroundImage)
	}
	if len(computed.BackgroundLayers) != 1 || computed.BackgroundLayers[0].Image != computed.BackgroundImage {
		t.Fatalf("background layers = %#v", computed.BackgroundLayers)
	}
}

func TestComputeBackgroundImageGradientList(t *testing.T) {
	doc, err := dom.Parse(`<div id="gradient"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#gradient { background-image: linear-gradient(to bottom, red 0%, blue 100%), linear-gradient(to right, white, black); }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	computed := styles[doc.Children[0]]
	if len(computed.BackgroundLayers) != 2 || computed.BackgroundLayers[0].Image == "" || computed.BackgroundLayers[1].Image == "" {
		t.Fatalf("background image layers = %#v", computed.BackgroundLayers)
	}
}

func TestComputeLinearGradientBackgroundShorthandComponents(t *testing.T) {
	doc, err := dom.Parse(`<div id="gradient"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#gradient {
		background: rgba(0, 0, 0, .25) linear-gradient(to right, red, blue) center / cover no-repeat;
	}`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	computed := styles[doc.Children[0]]
	if computed.BackgroundImage != `linear-gradient(to right, red, blue)` {
		t.Fatalf("background image = %q", computed.BackgroundImage)
	}
	if got := computed.BackgroundColor.Color; got != (Color{A: 64}) {
		t.Fatalf("background color = %#v, want alpha 64", got)
	}
	if computed.BackgroundPosX.Kind != ValueNumber || computed.BackgroundPosX.Number != .5 || computed.BackgroundPosY.Keyword != "center" {
		t.Fatalf("background position = %#v %#v", computed.BackgroundPosX, computed.BackgroundPosY)
	}
	if computed.BackgroundSizeX.Keyword != "cover" || computed.BackgroundSizeY.Keyword != "cover" {
		t.Fatalf("background size = %#v %#v", computed.BackgroundSizeX, computed.BackgroundSizeY)
	}
	if computed.BackgroundRepeatX.Keyword != "no-repeat" || computed.BackgroundRepeatY.Keyword != "no-repeat" {
		t.Fatalf("background repeat = %#v %#v", computed.BackgroundRepeatX, computed.BackgroundRepeatY)
	}
}

func TestComputeLinearGradientFunctionValidation(t *testing.T) {
	doc, err := dom.Parse(`<div id="gradient"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#gradient { background-image: LINEAR-GRADIENT(red, blue); }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	if got := styles[doc.Children[0]].BackgroundImage; got != `LINEAR-GRADIENT(red, blue)` {
		t.Fatalf("case-preserved background image = %q", got)
	}

	malformed, err := css.Parse(`#gradient { background: linear-gradient(red, blue)); }`)
	if err != nil {
		t.Fatal(err)
	}
	malformedStyles, err := Compute(doc, malformed)
	if err != nil {
		t.Fatalf("malformed declaration should be recovered: %v", err)
	}
	if got := malformedStyles[doc.Children[0]].BackgroundImage; got != "" {
		t.Fatalf("malformed gradient should be ignored, got %q", got)
	}
}

func TestImportantCascadeAndInvalidDeclarationRecovery(t *testing.T) {
	doc, err := dom.Parse(`<div id="box" style="color: blue; width: bad; height: 8px"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#box { color: red !important; width: 20px; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	computed := styles[doc.Children[0]]
	if computed.Color.Color != (Color{R: 255, G: 0, B: 0, A: 255}) {
		t.Fatalf("important color = %#v", computed.Color)
	}
	if computed.Width != LengthPx(20) || computed.Height != LengthPx(8) {
		t.Fatalf("invalid inline width was not ignored: width=%#v height=%#v", computed.Width, computed.Height)
	}
}

func TestComputeInsetShorthand(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="one"></div><div id="two"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`#one { inset: 8px 16px; } #two { inset: auto 1px 2px 3px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	body := doc.Children[0].Children[1]
	one, two := body.Children[0], body.Children[1]
	if got := styles[one]; got.Top != LengthPx(8) || got.Right != LengthPx(16) || got.Bottom != LengthPx(8) || got.Left != LengthPx(16) {
		t.Fatalf("two-value inset = %#v", got)
	}
	if got := styles[two]; got.Top != Auto() || got.Right != LengthPx(1) || got.Bottom != LengthPx(2) || got.Left != LengthPx(3) {
		t.Fatalf("four-value inset = %#v", got)
	}
}

func TestComputeZIndex(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="overlay">overlay</div><div id="default">default</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#overlay { position: absolute; z-index: 100000; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	overlay := doc.Children[0].Children[1].Children[0]
	defaultNode := doc.Children[0].Children[1].Children[1]
	if got := styles[overlay].ZIndex; got != Number(100000) {
		t.Fatalf("overlay z-index = %#v, want 100000", got)
	}
	if got := styles[defaultNode].ZIndex; got != Keyword("auto") {
		t.Fatalf("default z-index = %#v, want auto", got)
	}
}

func TestComputeBoxSizing(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="content"></div><div id="border"></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#content { box-sizing: content-box; } #border { box-sizing: border-box; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	body := doc.Children[0].Children[1]
	if got := styles[body.Children[0]].BoxSizing; got != Keyword("content-box") {
		t.Fatalf("content-box = %#v", got)
	}
	if got := styles[body.Children[1]].BoxSizing; got != Keyword("border-box") {
		t.Fatalf("border-box = %#v", got)
	}
}

func TestComputeFlexProperties(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="row"><div id="item"></div></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#row {
			display: flex;
			flex-flow: column wrap-reverse;
			justify-content: space-evenly;
			align-items: center;
			align-content: space-between;
			gap: 6px 10px;
		}
		#item {
			flex: 2 3 40px;
			align-self: flex-end;
			order: 4;
		}
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	row := doc.Children[0].Children[1].Children[0]
	item := row.Children[0]

	rowStyle := styles[row]
	if rowStyle.Display != Keyword("flex") || rowStyle.FlexDirection != Keyword("column") || rowStyle.FlexWrap != Keyword("wrap-reverse") {
		t.Fatalf("row flex flow = %#v", rowStyle)
	}
	if rowStyle.JustifyContent != Keyword("space-evenly") || rowStyle.AlignItems != Keyword("center") || rowStyle.AlignContent != Keyword("space-between") {
		t.Fatalf("row alignment = %#v", rowStyle)
	}
	if rowStyle.RowGap != LengthPx(6) || rowStyle.ColumnGap != LengthPx(10) {
		t.Fatalf("row gap = %#v %#v", rowStyle.RowGap, rowStyle.ColumnGap)
	}

	itemStyle := styles[item]
	if itemStyle.FlexGrow != Number(2) || itemStyle.FlexShrink != Number(3) || itemStyle.FlexBasis != LengthPx(40) {
		t.Fatalf("item flex = %#v", itemStyle)
	}
	if itemStyle.AlignSelf != Keyword("flex-end") || itemStyle.Order != Number(4) {
		t.Fatalf("item alignment/order = %#v", itemStyle)
	}
}

func TestComputeGridProperties(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="grid"><div id="item"></div></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#grid { display:grid; grid-template-columns:repeat(2, 1fr); grid-template-rows:40px 1fr; justify-items:center; } #item { grid-column:1 / 3; grid-row:2; justify-self:end; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	grid := doc.Children[0].Children[1].Children[0]
	item := grid.Children[0]
	gridStyle, itemStyle := styles[grid], styles[item]
	if gridStyle.Display != Keyword("grid") || gridStyle.GridTemplateColumns != "repeat(2, 1fr)" || gridStyle.GridTemplateRows != "40px 1fr" {
		t.Fatalf("grid template = %#v", gridStyle)
	}
	if gridStyle.JustifyItems != Keyword("center") || itemStyle.JustifySelf != Keyword("end") || itemStyle.GridColumnStart != Number(1) || itemStyle.GridColumnEnd != Number(3) || itemStyle.GridRowStart != Number(2) {
		t.Fatalf("grid placement/alignment = %#v %#v", gridStyle, itemStyle)
	}
}

func TestComputeOverflowProperties(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="one"></div><div id="two"></div><div id="three"></div><div id="four"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#one { overflow: hidden auto; }
		#two { overflow: scroll; overflow-x: auto; }
		#three { overflow-x: visible; overflow-y: auto; }
		#four { overflow-x: clip; overflow-y: scroll; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	body := doc.Children[0].Children[1]
	one := styles[body.Children[0]]
	if one.OverflowX != Keyword("hidden") || one.OverflowY != Keyword("auto") {
		t.Fatalf("#one overflow = %#v %#v", one.OverflowX, one.OverflowY)
	}
	two := styles[body.Children[1]]
	if two.OverflowX != Keyword("auto") || two.OverflowY != Keyword("scroll") {
		t.Fatalf("#two overflow = %#v %#v", two.OverflowX, two.OverflowY)
	}
	three := styles[body.Children[2]]
	if three.OverflowX != Keyword("auto") || three.OverflowY != Keyword("auto") {
		t.Fatalf("#three overflow = %#v %#v", three.OverflowX, three.OverflowY)
	}
	four := styles[body.Children[3]]
	if four.OverflowX != Keyword("hidden") || four.OverflowY != Keyword("scroll") {
		t.Fatalf("#four overflow = %#v %#v", four.OverflowX, four.OverflowY)
	}
}

func TestComputeScrollbarProperties(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="scroll"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`#scroll {
		scrollbar-width: thin;
		scrollbar-color: #3a706f #0c1117;
		scrollbar-thumb-hover-color: #4eb7a9;
	}`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	value := styles[doc.Children[0].Children[1].Children[0]]
	if value.ScrollbarWidth != Keyword("thin") {
		t.Fatalf("scrollbar width = %#v", value.ScrollbarWidth)
	}
	if value.ScrollbarThumbColor != ColorValue(Color{R: 58, G: 112, B: 111, A: 255}) || value.ScrollbarTrackColor != ColorValue(Color{R: 12, G: 17, B: 23, A: 255}) {
		t.Fatalf("scrollbar colors = thumb:%#v track:%#v", value.ScrollbarThumbColor, value.ScrollbarTrackColor)
	}
	if value.ScrollbarThumbHoverColor != ColorValue(Color{R: 78, G: 183, B: 169, A: 255}) {
		t.Fatalf("scrollbar hover color = %#v", value.ScrollbarThumbHoverColor)
	}
}

func TestScrollbarWidthLengthIsApplied(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="scroll"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`#scroll { scrollbar-width: 12px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	value := styles[doc.Children[0].Children[1].Children[0]]
	if value.ScrollbarWidth != LengthPx(12) {
		t.Fatalf("scrollbar width = %#v, want 12px", value.ScrollbarWidth)
	}
}

func TestInvalidInlineScrollbarWidthKeepsLowerPriorityValue(t *testing.T) {
	stylesheet, err := css.Parse(`.scroll { scrollbar-width: thin; }`)
	if err != nil {
		t.Fatal(err)
	}
	root := dom.NewDocument()
	node := dom.NewElement("div", map[string]string{"class": "scroll", "style": "scrollbar-width: widest"})
	root.AppendChild(node)
	styles, err := Compute(root, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	if value := styles[node].ScrollbarWidth; value != Keyword("thin") {
		t.Fatalf("inline invalid scrollbar width = %#v, want thin", value)
	}
}

func TestComputeFormControlDefaults(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="radio" type="radio" checked><input id="disabled" disabled><button id="button" disabled>Send</button><select id="select" disabled><option>One</option></select></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	styles, err := Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	body := doc.Children[0].Children[1]
	radioStyle := styles[body.Children[0]]
	if radioStyle.Width != LengthPx(13) || radioStyle.Height != LengthPx(13) {
		t.Fatalf("radio size = %#v x %#v", radioStyle.Width, radioStyle.Height)
	}
	if radioStyle.BorderRadius != uniformCorners(LengthPx(999)) {
		t.Fatalf("radio radius = %#v", radioStyle.BorderRadius)
	}

	disabledInputStyle := styles[body.Children[1]]
	if disabledInputStyle.Width != LengthPx(defaultTextInputWidthPx) {
		t.Fatalf("default text input width = %#v", disabledInputStyle.Width)
	}
	if disabledInputStyle.BackgroundColor.Color != (Color{R: 240, G: 240, B: 240, A: 255}) {
		t.Fatalf("disabled input background = %#v", disabledInputStyle.BackgroundColor.Color)
	}
	if disabledInputStyle.Color.Color != (Color{R: 106, G: 106, B: 106, A: 255}) {
		t.Fatalf("disabled input color = %#v", disabledInputStyle.Color.Color)
	}

	disabledButtonStyle := styles[body.Children[2]]
	if disabledButtonStyle.BorderColor.Top.Color != (Color{R: 180, G: 180, B: 180, A: 255}) {
		t.Fatalf("disabled button border = %#v", disabledButtonStyle.BorderColor.Top.Color)
	}
	if disabledButtonStyle.TextAlign != Keyword("center") {
		t.Fatalf("button text-align = %#v", disabledButtonStyle.TextAlign)
	}

	disabledSelectStyle := styles[body.Children[3]]
	if disabledSelectStyle.BackgroundColor.Color != (Color{R: 240, G: 240, B: 240, A: 255}) {
		t.Fatalf("disabled select background = %#v", disabledSelectStyle.BackgroundColor.Color)
	}
}

func TestComputeFormControlAppearance(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="default" type="checkbox"><input id="custom" type="checkbox"><select id="select"><option>One</option></select></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`#custom { appearance: none; } #select { appearance: none; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	body := doc.Children[0].Children[1]
	if got := styles[body.Children[0]].Appearance; got != Keyword("auto") {
		t.Fatalf("default checkbox appearance = %#v, want auto", got)
	}
	if got := styles[body.Children[1]].Appearance; got != Keyword("none") {
		t.Fatalf("custom checkbox appearance = %#v, want none", got)
	}
	if got := styles[body.Children[2]].Appearance; got != Keyword("none") {
		t.Fatalf("custom select appearance = %#v, want none", got)
	}
}

func TestAppearanceChangeRequiresRenderTreeRebuild(t *testing.T) {
	previous := &ComputedStyle{styleCore: styleCore{Appearance: Keyword("none")}}
	current := &ComputedStyle{styleCore: styleCore{Appearance: Keyword("auto")}}
	if impact := computedStyleChangeForNode(nil, previous, current); impact != ChangeRebuild {
		t.Fatalf("appearance change impact = %v, want rebuild", impact)
	}
}

func TestUniformBorderColorChangeIsPaintOnly(t *testing.T) {
	previous := &ComputedStyle{styleCore: styleCore{BorderColor: Edges{
		Top:    ColorValue(Color{R: 0x3b, G: 0x40, B: 0x4b, A: 0xff}),
		Right:  ColorValue(Color{R: 0x3b, G: 0x40, B: 0x4b, A: 0xff}),
		Bottom: ColorValue(Color{R: 0x3b, G: 0x40, B: 0x4b, A: 0xff}),
		Left:   ColorValue(Color{R: 0x3b, G: 0x40, B: 0x4b, A: 0xff}),
	}}}
	current := &ComputedStyle{styleCore: styleCore{BorderColor: previous.BorderColor}}
	current.BorderColor.Top = ColorValue(Color{R: 0x84, G: 0xcc, B: 0xff, A: 0xff})
	current.BorderColor.Right = current.BorderColor.Top
	current.BorderColor.Bottom = current.BorderColor.Top
	current.BorderColor.Left = current.BorderColor.Top
	if impact := computedStyleChangeForNode(nil, previous, current); impact != ChangePaint {
		t.Fatalf("uniform border color impact = %v, want paint", impact)
	}
}

func TestControlBackgroundTransitionIsPaintOnly(t *testing.T) {
	document, err := dom.Parse(`<button id="close"></button>`)
	if err != nil {
		t.Fatal(err)
	}
	previous := &ComputedStyle{styleCore: styleCore{BackgroundColor: ColorValue(Color{})}}
	current := &ComputedStyle{styleCore: styleCore{BackgroundColor: ColorValue(Color{R: 0x34, G: 0x3a, B: 0x46, A: 0xff})}}
	if impact := computedStyleChangeForNode(document.Children[0], previous, current); impact != ChangePaint {
		t.Fatalf("control background transition impact = %v, want %v", impact, ChangePaint)
	}
	ordinaryElement := &dom.Node{Type: dom.ElementNode, Data: "div"}
	if impact := computedStyleChangeForNode(ordinaryElement, previous, current); impact != ChangeVisual {
		t.Fatalf("ordinary element background transition impact = %v, want %v", impact, ChangeVisual)
	}
}

func TestComputeTransformRotationAndTransition(t *testing.T) {
	document, err := dom.Parse(`<html><body><div id="arrow"></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#arrow { transform: rotate(90deg); transition: transform 160ms ease; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(document, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	arrow := document.Children[0].Children[1].Children[0]
	computed := styles[arrow]
	if computed.TransformRotation != 90 {
		t.Fatalf("transform rotation = %v, want 90 degrees", computed.TransformRotation)
	}
	if computed.TransformTransition.Duration != 0.16 || computed.TransformTransition.Easing != TransformEase {
		t.Fatalf("transform transition = %#v, want 160ms ease", computed.TransformTransition)
	}
	previous := *computed
	current := *computed
	current.TransformRotation = 0
	if impact := computedStyleChangeForNode(arrow, &previous, &current); impact != ChangePaint {
		t.Fatalf("transform change impact = %v, want %v", impact, ChangePaint)
	}
}

func TestComputeHiddenAttribute(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="hidden" hidden></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	node := doc.Children[0].Children[1].Children[0]
	if got := styles[node].Display; got != Keyword("none") {
		t.Fatalf("hidden display = %#v", got)
	}
}

func TestComputeHiddenAttributeOnInput(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="hidden" hidden></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	node := doc.Children[0].Children[1].Children[0]
	if got := styles[node].Display; got != Keyword("none") {
		t.Fatalf("hidden input display = %#v", got)
	}
}

func TestAuthorCSSOverridesTextInputDefaultWidth(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="field"></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#field { width: 88px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	field := doc.Children[0].Children[1].Children[0]
	if got := styles[field].Width; got != LengthPx(88) {
		t.Fatalf("text input width = %#v, want author CSS width", got)
	}
}

func TestComputeTextAlignPropertyAndInheritance(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="parent"><span id="child">copy</span><button id="button">Go</button></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#parent { text-align: right; }
		#button { text-align: left; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	parent := doc.Children[0].Children[1].Children[0]
	child := parent.Children[0]
	button := parent.Children[1]

	if styles[parent].TextAlign != Keyword("right") {
		t.Fatalf("parent text-align = %#v", styles[parent].TextAlign)
	}
	if styles[child].TextAlign != Keyword("right") {
		t.Fatalf("child text-align = %#v", styles[child].TextAlign)
	}
	if styles[button].TextAlign != Keyword("left") {
		t.Fatalf("button text-align = %#v", styles[button].TextAlign)
	}
}

func TestComputeTextDecorationLineReachesTextNodes(t *testing.T) {
	doc, err := dom.Parse(`<html><body><span id="deleted">deleted</span><span id="plain">plain</span></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`#deleted { text-decoration-line: line-through; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	deleted := doc.Children[0].Children[1].Children[0]
	plain := doc.Children[0].Children[1].Children[1]
	if styles[deleted].TextDecorationLine != Keyword("line-through") || styles[deleted.Children[0]].TextDecorationLine != Keyword("line-through") {
		t.Fatalf("deleted decoration did not reach its text node: element=%#v text=%#v", styles[deleted].TextDecorationLine, styles[deleted.Children[0]].TextDecorationLine)
	}
	if styles[plain].TextDecorationLine != Keyword("none") {
		t.Fatalf("plain decoration = %#v, want none", styles[plain].TextDecorationLine)
	}
}

func TestComputeSelectorBucketsPreserveCascadeOrder(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card" class="item">hover</div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		:hover { color: #112233; }
		.item { color: #445566; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	styles, err := ComputeWithState(doc, stylesheet, &css.State{
		Hovered: card,
	})
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	if got := styles[card].Color.Color; got != (Color{R: 0x44, G: 0x55, B: 0x66, A: 255}) {
		t.Fatalf("card color = %#v", got)
	}
}

func TestComputeSelectorBucketsHandleDescendantClassSelectors(t *testing.T) {
	doc, err := dom.Parse(`<html><body><section class="panel"><div id="card" class="item">copy</div></section></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		.panel .item { color: #123456; }
		.panel .missing { color: #abcdef; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0].Children[0]
	if got := styles[card].Color.Color; got != (Color{R: 0x12, G: 0x34, B: 0x56, A: 255}) {
		t.Fatalf("card color = %#v", got)
	}
}

func TestComputeTextStyleProperties(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="parent"><input id="field" placeholder="Name"><textarea id="notes"></textarea></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#parent {
			font-weight: bold;
			text-indent: 12px;
			tab-size: 2;
			white-space: pre-line;
			placeholder-color: #445566;
		}
		#field {
			white-space: nowrap;
			font-weight: 300;
		}
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	parent := doc.Children[0].Children[1].Children[0]
	field := parent.Children[0]
	notes := parent.Children[1]
	if styles[parent].FontWeight != Number(700) || styles[parent].TextIndent != LengthPx(12) || styles[parent].TabSize != Number(2) || styles[parent].WhiteSpace != Keyword("pre-line") {
		t.Fatalf("parent text styles = %#v", styles[parent])
	}
	if styles[field].FontWeight != Number(300) || styles[field].TabSize != Number(2) || styles[field].WhiteSpace != Keyword("nowrap") {
		t.Fatalf("field text styles = %#v", styles[field])
	}
	if styles[field].PlaceholderColor.Color != (Color{R: 68, G: 85, B: 102, A: 255}) {
		t.Fatalf("field placeholder color = %#v", styles[field].PlaceholderColor.Color)
	}
	if styles[notes].WhiteSpace != Keyword("pre-wrap") {
		t.Fatalf("textarea white-space default = %#v", styles[notes].WhiteSpace)
	}
}

func TestComputeTabSizeLength(t *testing.T) {
	doc, err := dom.Parse(`<span id="code"></span>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`#code { tab-size: 12px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	if got := styles[doc.Children[0]].TabSize; got != LengthPx(12) {
		t.Fatalf("tab-size length = %#v, want 12px", got)
	}
}

func TestComputeCSSWideKeywordsOnInheritedProperties(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="parent"><span id="take"></span><span id="reset"></span></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`
		#parent {
			white-space: pre;
			tab-size: 3;
			color: #112233;
			line-height: 20px;
			font-size: 15px;
		}
		#take {
			white-space: inherit;
			tab-size: inherit;
			color: inherit;
			line-height: inherit;
			font: inherit;
		}
		#reset { white-space: initial; tab-size: initial; line-height: initial; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	parent := doc.Children[0].Children[1].Children[0]
	take := parent.Children[0]
	reset := parent.Children[1]
	if styles[take].WhiteSpace != Keyword("pre") || styles[take].TabSize != Number(3) || styles[take].Color.Color != (Color{R: 0x11, G: 0x22, B: 0x33, A: 255}) || styles[take].LineHeight != LengthPx(20) || styles[take].FontSize != LengthPx(15) {
		t.Fatalf("inherit keyword did not take parent values: %#v", styles[take])
	}
	if styles[reset].WhiteSpace != Keyword("normal") {
		t.Fatalf("initial white-space = %#v, want normal", styles[reset].WhiteSpace)
	}
	if styles[reset].TabSize != Number(8) {
		t.Fatalf("initial tab-size = %#v, want 8", styles[reset].TabSize)
	}
	if styles[reset].LineHeight != LengthPx(19.2) || !styles[reset].LineHeightNormal {
		t.Fatalf("initial line-height = %#v normal=%v, want 19.2px default", styles[reset].LineHeight, styles[reset].LineHeightNormal)
	}
}

func TestComputeCSSWideBackgroundLonghandsPreserveSiblingFields(t *testing.T) {
	doc, err := dom.Parse(`<div id="parent"><div id="image"></div><div id="position"></div><div id="reset"></div></div>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}
	stylesheet, err := css.Parse(`
		#parent { background-image: url(parent.png); background-position: 10px 20px; background-repeat: no-repeat; background-size: 40px 50px; }
		#image { background-image: inherit; background-position: 1px 2px; background-repeat: repeat-x; background-size: 3px 4px; }
		#position { background-image: url(child.png); background-position: inherit; background-repeat: repeat-x; background-size: 3px 4px; }
		#reset { background-image: url(child.png); background-position: initial; background-repeat: repeat-x; background-size: 3px 4px; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}
	parent := doc.Children[0]
	image, position, reset := parent.Children[0], parent.Children[1], parent.Children[2]
	if styles[image].BackgroundImage != "parent.png" || styles[image].BackgroundPosX != LengthPx(1) || styles[image].BackgroundPosY != LengthPx(2) || styles[image].BackgroundRepeatX != Keyword("repeat") || styles[image].BackgroundRepeatY != Keyword("no-repeat") || styles[image].BackgroundSizeX != LengthPx(3) || styles[image].BackgroundSizeY != LengthPx(4) {
		t.Fatalf("background-image inherit changed sibling fields: %#v", styles[image])
	}
	if styles[position].BackgroundImage != "child.png" || styles[position].BackgroundPosX != LengthPx(10) || styles[position].BackgroundPosY != LengthPx(20) || styles[position].BackgroundRepeatX != Keyword("repeat") || styles[position].BackgroundRepeatY != Keyword("no-repeat") || styles[position].BackgroundSizeX != LengthPx(3) || styles[position].BackgroundSizeY != LengthPx(4) {
		t.Fatalf("background-position inherit changed sibling fields: %#v", styles[position])
	}
	if styles[reset].BackgroundImage != "child.png" || styles[reset].BackgroundPosX != Number(0) || styles[reset].BackgroundPosY != Number(0) || styles[reset].BackgroundRepeatX != Keyword("repeat") || styles[reset].BackgroundRepeatY != Keyword("no-repeat") || styles[reset].BackgroundSizeX != LengthPx(3) || styles[reset].BackgroundSizeY != LengthPx(4) {
		t.Fatalf("background-position initial changed sibling fields: %#v", styles[reset])
	}
}

func TestComputeColorDoesNotOverridePlaceholderColor(t *testing.T) {
	doc, err := dom.Parse(`<html><body style="color:#1f2937"><input id="field" placeholder="Name"></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	styles, err := Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	field := doc.Children[0].Children[1].Children[0]
	if got := styles[field].Color.Color; got != (Color{R: 31, G: 41, B: 55, A: 255}) {
		t.Fatalf("field color = %#v", got)
	}
	if got := styles[field].PlaceholderColor.Color; got != (Color{R: 117, G: 117, B: 117, A: 255}) {
		t.Fatalf("field placeholder color = %#v", got)
	}
}

func TestComputeBorderLonghands(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="field"></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#field {
			border-width: 2px 3px 4px 5px;
			border-style: solid;
			border-color: #010203 #111213 #212223 #313233;
		}
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	field := doc.Children[0].Children[1].Children[0]
	computed := styles[field]
	if computed.BorderWidth.Top != LengthPx(2) || computed.BorderWidth.Right != LengthPx(3) || computed.BorderWidth.Bottom != LengthPx(4) || computed.BorderWidth.Left != LengthPx(5) {
		t.Fatalf("border widths = %#v", computed.BorderWidth)
	}
	if computed.BorderStyle != uniformEdges(Keyword("solid")) {
		t.Fatalf("border style = %#v", computed.BorderStyle)
	}
	if computed.BorderColor.Top.Color != (Color{R: 1, G: 2, B: 3, A: 255}) ||
		computed.BorderColor.Right.Color != (Color{R: 17, G: 18, B: 19, A: 255}) ||
		computed.BorderColor.Bottom.Color != (Color{R: 33, G: 34, B: 35, A: 255}) ||
		computed.BorderColor.Left.Color != (Color{R: 49, G: 50, B: 51, A: 255}) {
		t.Fatalf("border colors = %#v", computed.BorderColor)
	}
}

func TestComputeSideBoxLonghands(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="field"></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#field {
			margin-top: 1px;
			margin-right: 2px;
			margin-bottom: 3px;
			margin-left: 4px;
			padding-top: 5px;
			padding-right: 6px;
			padding-bottom: 7px;
			padding-left: 8px;
			border-top-width: 9px;
			border-right-width: 10px;
			border-bottom-width: 11px;
			border-left-width: 12px;
			border-top-color: #010203;
			border-right-color: #111213;
			border-bottom-color: #212223;
			border-left-color: #313233;
		}
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	field := doc.Children[0].Children[1].Children[0]
	computed := styles[field]
	if computed.Margin.Top != LengthPx(1) || computed.Margin.Right != LengthPx(2) || computed.Margin.Bottom != LengthPx(3) || computed.Margin.Left != LengthPx(4) {
		t.Fatalf("margin = %#v", computed.Margin)
	}
	if computed.Padding.Top != LengthPx(5) || computed.Padding.Right != LengthPx(6) || computed.Padding.Bottom != LengthPx(7) || computed.Padding.Left != LengthPx(8) {
		t.Fatalf("padding = %#v", computed.Padding)
	}
	if computed.BorderWidth.Top != LengthPx(9) || computed.BorderWidth.Right != LengthPx(10) || computed.BorderWidth.Bottom != LengthPx(11) || computed.BorderWidth.Left != LengthPx(12) {
		t.Fatalf("border widths = %#v", computed.BorderWidth)
	}
	if computed.BorderColor.Top.Color != (Color{R: 1, G: 2, B: 3, A: 255}) ||
		computed.BorderColor.Right.Color != (Color{R: 17, G: 18, B: 19, A: 255}) ||
		computed.BorderColor.Bottom.Color != (Color{R: 33, G: 34, B: 35, A: 255}) ||
		computed.BorderColor.Left.Color != (Color{R: 49, G: 50, B: 51, A: 255}) {
		t.Fatalf("border colors = %#v", computed.BorderColor)
	}
}

func TestComputeBorderSideStylesAreIndependent(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="field"></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#field {
			border-width: 2px 3px 4px 5px;
			border-style: solid none;
			border-left-style: solid;
		}
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	field := doc.Children[0].Children[1].Children[0]
	computed := styles[field]
	if computed.BorderWidth.Top != LengthPx(2) || computed.BorderWidth.Right != LengthPx(3) || computed.BorderWidth.Bottom != LengthPx(4) || computed.BorderWidth.Left != LengthPx(5) {
		t.Fatalf("border widths = %#v", computed.BorderWidth)
	}
	if computed.BorderStyle.Top != Keyword("solid") || computed.BorderStyle.Right != Keyword("none") || computed.BorderStyle.Bottom != Keyword("solid") || computed.BorderStyle.Left != Keyword("solid") {
		t.Fatalf("border styles = %#v", computed.BorderStyle)
	}
}

func TestComputeBorderSideShorthand(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="box"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#box { border-top: 2px solid #010203; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	computed := styles[doc.Children[0].Children[1].Children[0]]
	if computed.BorderWidth.Top != LengthPx(2) || computed.BorderStyle.Top != Keyword("solid") || computed.BorderColor.Top.Color != (Color{R: 1, G: 2, B: 3, A: 255}) {
		t.Fatalf("top border shorthand = width:%#v style:%#v color:%#v", computed.BorderWidth.Top, computed.BorderStyle.Top, computed.BorderColor.Top)
	}
	if computed.BorderWidth.Right != LengthPx(0) || computed.BorderStyle.Right != Keyword("none") {
		t.Fatalf("top border shorthand changed right edge: width:%#v style:%#v", computed.BorderWidth.Right, computed.BorderStyle.Right)
	}
}

func TestComputeTextareaDefaults(t *testing.T) {
	doc, err := dom.Parse(`<html><body><textarea id="notes" disabled placeholder="Type here"></textarea></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	styles, err := Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	textareaStyle := styles[doc.Children[0].Children[1].Children[0]]
	if textareaStyle.Display != Keyword("inline-block") {
		t.Fatalf("textarea display = %#v", textareaStyle.Display)
	}
	if textareaStyle.BackgroundColor.Color != (Color{R: 240, G: 240, B: 240, A: 255}) {
		t.Fatalf("textarea background = %#v", textareaStyle.BackgroundColor.Color)
	}
	if textareaStyle.BorderColor.Top.Color != (Color{R: 180, G: 180, B: 180, A: 255}) {
		t.Fatalf("textarea border = %#v", textareaStyle.BorderColor.Top.Color)
	}
}

func TestComputeFormControlDisplayDefaults(t *testing.T) {
	doc, err := dom.Parse(`<html><body><button></button><input><select></select><textarea></textarea></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	styles, err := Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	for _, control := range doc.Children[0].Children[1].Children {
		if got := styles[control].Display; got != Keyword("inline-block") {
			t.Errorf("%s display = %#v, want inline-block", control.Data, got)
		}
	}
}

func TestComputePseudoClassSelectors(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="empty"></div><input id="check" type="checkbox" checked disabled><select><option id="selected" selected>One</option></select></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		html:root { color: #010203; }
		div:empty { background-color: #111111; }
		input:disabled { color: #222222; }
		input:checked { background-color: #333333; }
		option:checked { color: #444444; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	html := doc.Children[0]
	empty := html.Children[1].Children[0]
	check := html.Children[1].Children[1]
	selected := html.Children[1].Children[2].Children[0]

	if styles[html].Color.Color != (Color{R: 1, G: 2, B: 3, A: 255}) {
		t.Fatalf("html root color = %#v", styles[html].Color.Color)
	}
	if styles[empty].BackgroundColor.Color != (Color{R: 17, G: 17, B: 17, A: 255}) {
		t.Fatalf("empty div background = %#v", styles[empty].BackgroundColor.Color)
	}
	if styles[check].Color.Color != (Color{R: 34, G: 34, B: 34, A: 255}) {
		t.Fatalf("disabled input color = %#v", styles[check].Color.Color)
	}
	if styles[check].BackgroundColor.Color != (Color{R: 51, G: 51, B: 51, A: 255}) {
		t.Fatalf("checked input background = %#v", styles[check].BackgroundColor.Color)
	}
	if styles[selected].Color.Color != (Color{R: 68, G: 68, B: 68, A: 255}) {
		t.Fatalf("selected option color = %#v", styles[selected].Color.Color)
	}
}

func TestComputeFunctionalPseudoClassSelectors(t *testing.T) {
	doc, err := dom.Parse(`<html><body><ul><li id="first"></li><li id="second" class="pick"></li><li id="third"></li></ul><div id="hero" class="hero"></div><p id="lead" class="lead"></p></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		li:nth-child(2) { color: #111111; }
		li:not(.pick) { background-color: #222222; }
		div:is(.hero, .card) { background-color: #333333; }
		p:where(.lead) { color: #444444; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	body := doc.Children[0].Children[1]
	first := body.Children[0].Children[0]
	second := body.Children[0].Children[1]
	third := body.Children[0].Children[2]
	hero := body.Children[1]
	lead := body.Children[2]

	if styles[second].Color.Color != (Color{R: 17, G: 17, B: 17, A: 255}) {
		t.Fatalf("second li color = %#v", styles[second].Color.Color)
	}
	if styles[first].BackgroundColor.Color != (Color{R: 34, G: 34, B: 34, A: 255}) {
		t.Fatalf("first li background = %#v", styles[first].BackgroundColor.Color)
	}
	if styles[second].BackgroundColor.Color.A != 0 {
		t.Fatalf("second li background = %#v", styles[second].BackgroundColor.Color)
	}
	if styles[third].BackgroundColor.Color != (Color{R: 34, G: 34, B: 34, A: 255}) {
		t.Fatalf("third li background = %#v", styles[third].BackgroundColor.Color)
	}
	if styles[hero].BackgroundColor.Color != (Color{R: 51, G: 51, B: 51, A: 255}) {
		t.Fatalf("hero background = %#v", styles[hero].BackgroundColor.Color)
	}
	if styles[lead].Color.Color != (Color{R: 68, G: 68, B: 68, A: 255}) {
		t.Fatalf("lead color = %#v", styles[lead].Color.Color)
	}
}

func TestComputeNthLastAndPlaceholderShownSelectors(t *testing.T) {
	doc, err := dom.Parse(`<html><body><ul><li id="first"></li><li id="second"></li><li id="third"></li></ul><div><span id="left"></span><span id="right"></span></div><input id="shown" placeholder="Email"><input id="filled" placeholder="Email" value="a@b.c"><textarea id="notes" placeholder="Notes"></textarea></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		li:nth-last-child(2) { color: #111111; }
		span:nth-last-of-type(1) { background-color: #222222; }
		input:placeholder-shown { background-color: #333333; }
		textarea:placeholder-shown { color: #444444; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	body := doc.Children[0].Children[1]
	second := body.Children[0].Children[1]
	right := body.Children[1].Children[1]
	shown := body.Children[2]
	filled := body.Children[3]
	notes := body.Children[4]

	if styles[second].Color.Color != (Color{R: 17, G: 17, B: 17, A: 255}) {
		t.Fatalf("second li color = %#v", styles[second].Color.Color)
	}
	if styles[right].BackgroundColor.Color != (Color{R: 34, G: 34, B: 34, A: 255}) {
		t.Fatalf("right span background = %#v", styles[right].BackgroundColor.Color)
	}
	if styles[shown].BackgroundColor.Color != (Color{R: 51, G: 51, B: 51, A: 255}) {
		t.Fatalf("shown input background = %#v", styles[shown].BackgroundColor.Color)
	}
	if styles[filled].BackgroundColor.Color == (Color{R: 51, G: 51, B: 51, A: 255}) {
		t.Fatalf("filled input should not match placeholder-shown: %#v", styles[filled].BackgroundColor.Color)
	}
	if styles[notes].Color.Color != (Color{R: 68, G: 68, B: 68, A: 255}) {
		t.Fatalf("notes textarea color = %#v", styles[notes].Color.Color)
	}
}

func TestComputeDoesNotInferInputStateFromAttributes(t *testing.T) {
	doc, err := dom.Parse(`<html><body><button id="focus" data-focus>ok</button><div id="wrap"><span><input id="inside" data-focus></span></div><a id="hovered" data-hover>link</a><button id="pressed" data-active>go</button></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		button:focus { background-color: #111111; }
		div:focus-within { color: #222222; }
		a:hover { color: #333333; }
		button:active { color: #444444; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	body := doc.Children[0].Children[1]
	focus := body.Children[0]
	wrap := body.Children[1]
	hovered := body.Children[2]
	pressed := body.Children[3]

	if styles[focus].BackgroundColor.Color == (Color{R: 17, G: 17, B: 17, A: 255}) {
		t.Fatalf("focus button matched data-focus: %#v", styles[focus].BackgroundColor.Color)
	}
	if styles[wrap].Color.Color == (Color{R: 34, G: 34, B: 34, A: 255}) {
		t.Fatalf("focus-within wrapper matched data-focus: %#v", styles[wrap].Color.Color)
	}
	if styles[hovered].Color.Color == (Color{R: 51, G: 51, B: 51, A: 255}) {
		t.Fatalf("hovered link matched data-hover: %#v", styles[hovered].Color.Color)
	}
	if styles[pressed].Color.Color == (Color{R: 68, G: 68, B: 68, A: 255}) {
		t.Fatalf("active button matched data-active: %#v", styles[pressed].Color.Color)
	}
}

func TestComputeInputPseudoClassSelectorsWithExplicitState(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="wrap"><span><input id="inside"></span></div><a id="hovered">link</a><button id="pressed">go</button></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		input:focus { background-color: #111111; }
		div:focus-within { color: #222222; }
		a:hover { color: #333333; }
		button:active { color: #444444; }
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	body := doc.Children[0].Children[1]
	wrap := body.Children[0]
	inside := wrap.Children[0].Children[0]
	hovered := body.Children[1]
	pressed := body.Children[2]
	input := &css.State{
		Focused: inside,
		Hovered: hovered,
		Active:  pressed,
	}

	styles, err := ComputeWithState(doc, stylesheet, input)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	if styles[inside].BackgroundColor.Color != (Color{R: 17, G: 17, B: 17, A: 255}) {
		t.Fatalf("focus input background = %#v", styles[inside].BackgroundColor.Color)
	}
	if styles[wrap].Color.Color != (Color{R: 34, G: 34, B: 34, A: 255}) {
		t.Fatalf("focus-within wrapper color = %#v", styles[wrap].Color.Color)
	}
	if styles[hovered].Color.Color != (Color{R: 51, G: 51, B: 51, A: 255}) {
		t.Fatalf("hovered link color = %#v", styles[hovered].Color.Color)
	}
	if styles[pressed].Color.Color != (Color{R: 68, G: 68, B: 68, A: 255}) {
		t.Fatalf("active button color = %#v", styles[pressed].Color.Color)
	}
}

func TestComputeButtonLikeAndHiddenInputDefaults(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="submit" type="submit"><input id="button" type="button" value="Run"><input id="hidden" type="hidden"></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	styles, err := Compute(doc, nil)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	body := doc.Children[0].Children[1]
	submitStyle := styles[body.Children[0]]
	if submitStyle.BackgroundColor.Color != (Color{R: 239, G: 239, B: 239, A: 255}) {
		t.Fatalf("submit background = %#v", submitStyle.BackgroundColor.Color)
	}
	if submitStyle.BorderRadius != uniformCorners(LengthPx(4)) {
		t.Fatalf("submit border radius = %#v", submitStyle.BorderRadius)
	}

	hiddenStyle := styles[body.Children[2]]
	if hiddenStyle.Display != Keyword("none") {
		t.Fatalf("hidden display = %#v", hiddenStyle.Display)
	}
}

func TestComputeBorderRadiusAndBoxShadow(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#card {
			border-radius: 4px 8px 12px 16px;
			box-shadow: 2px 3px 5px 1px rgba(10, 20, 30, 0.5);
		}
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	computed := styles[card]
	if computed.BorderRadius.TopLeft != uniformRadius(LengthPx(4)) || computed.BorderRadius.TopRight != uniformRadius(LengthPx(8)) || computed.BorderRadius.BottomRight != uniformRadius(LengthPx(12)) || computed.BorderRadius.BottomLeft != uniformRadius(LengthPx(16)) {
		t.Fatalf("border-radius = %#v", computed.BorderRadius)
	}

	if len(computed.BoxShadow) != 1 {
		t.Fatalf("box-shadow count = %d", len(computed.BoxShadow))
	}
	shadow := computed.BoxShadow[0]
	if shadow.OffsetX != LengthPx(2) || shadow.OffsetY != LengthPx(3) || shadow.BlurRadius != LengthPx(5) || shadow.SpreadRadius != LengthPx(1) {
		t.Fatalf("box-shadow lengths = %#v", shadow)
	}
	if shadow.Color != (Color{R: 10, G: 20, B: 30, A: 128}) {
		t.Fatalf("box-shadow color = %#v", shadow.Color)
	}
}

func TestComputeMultipleInsetBoxShadows(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#card { box-shadow: 2px 3px 4px #000, inset -1px -2px 0 3px rgba(255, 0, 0, 0.25); }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	shadows := styles[card].BoxShadow
	if len(shadows) != 2 {
		t.Fatalf("box-shadow count = %d", len(shadows))
	}
	if shadows[0].Inset {
		t.Fatalf("first shadow inset = %v", shadows[0].Inset)
	}
	if !shadows[1].Inset {
		t.Fatalf("second shadow inset = %v", shadows[1].Inset)
	}
	if shadows[1].OffsetX != LengthPx(-1) || shadows[1].OffsetY != LengthPx(-2) || shadows[1].SpreadRadius != LengthPx(3) {
		t.Fatalf("inset shadow = %#v", shadows[1])
	}
	if shadows[1].Color != (Color{R: 255, A: 64}) {
		t.Fatalf("inset shadow color = %#v", shadows[1].Color)
	}
}

func TestComputeEllipticalBorderRadiusLonghands(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		#card {
			border-radius: 10px 20px 30px 40px / 5px 6px 7px 8px;
			border-top-right-radius: 12px 9px;
			border-bottom-left-radius: 14px;
		}
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	radius := styles[card].BorderRadius
	if radius.TopLeft != (Radius{Horizontal: LengthPx(10), Vertical: LengthPx(5)}) {
		t.Fatalf("top-left radius = %#v", radius.TopLeft)
	}
	if radius.TopRight != (Radius{Horizontal: LengthPx(12), Vertical: LengthPx(9)}) {
		t.Fatalf("top-right radius = %#v", radius.TopRight)
	}
	if radius.BottomRight != (Radius{Horizontal: LengthPx(30), Vertical: LengthPx(7)}) {
		t.Fatalf("bottom-right radius = %#v", radius.BottomRight)
	}
	if radius.BottomLeft != uniformRadius(LengthPx(14)) {
		t.Fatalf("bottom-left radius = %#v", radius.BottomLeft)
	}
}

func TestComputeFilterBlur(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#card { filter: blur(6px); }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	filters := styles[card].Filters
	if len(filters) != 1 || filters[0] != (Filter{Kind: FilterBlur, Amount: 6}) {
		t.Fatalf("filters = %#v", filters)
	}
}

func TestComputeFilterChain(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#card { filter: blur(4px) brightness(150%) opacity(0.25); }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	filters := styles[card].Filters
	if len(filters) != 3 {
		t.Fatalf("filter count = %d", len(filters))
	}
	if filters[0] != (Filter{Kind: FilterBlur, Amount: 4}) {
		t.Fatalf("blur filter = %#v", filters[0])
	}
	if filters[1] != (Filter{Kind: FilterBrightness, Amount: 1.5}) {
		t.Fatalf("brightness filter = %#v", filters[1])
	}
	if filters[2] != (Filter{Kind: FilterOpacity, Amount: 0.25}) {
		t.Fatalf("opacity filter = %#v", filters[2])
	}
}

func TestComputeExtendedFilterChain(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#card { filter: contrast(120%) grayscale(25%) drop-shadow(2px 3px 4px rgba(10, 20, 30, 0.5)); }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	filters := styles[card].Filters
	if len(filters) != 3 {
		t.Fatalf("filter count = %d", len(filters))
	}
	if filters[0] != (Filter{Kind: FilterContrast, Amount: 1.2}) {
		t.Fatalf("contrast filter = %#v", filters[0])
	}
	if filters[1] != (Filter{Kind: FilterGrayscale, Amount: 0.25}) {
		t.Fatalf("grayscale filter = %#v", filters[1])
	}
	if filters[2].Kind != FilterDropShadow {
		t.Fatalf("drop-shadow kind = %#v", filters[2])
	}
	shadow := filters[2].Shadow
	if shadow.OffsetX != LengthPx(2) || shadow.OffsetY != LengthPx(3) || shadow.BlurRadius != LengthPx(4) {
		t.Fatalf("drop-shadow geometry = %#v", shadow)
	}
	if shadow.Color != (Color{R: 10, G: 20, B: 30, A: 128}) {
		t.Fatalf("drop-shadow color = %#v", shadow.Color)
	}
}

func TestComputeColorFilters(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#card { filter: saturate(0%) sepia(100%); }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	filters := styles[card].Filters
	if len(filters) != 2 {
		t.Fatalf("filter count = %d", len(filters))
	}
	if filters[0] != (Filter{Kind: FilterSaturate, Amount: 0}) {
		t.Fatalf("saturate filter = %#v", filters[0])
	}
	if filters[1] != (Filter{Kind: FilterSepia, Amount: 1}) {
		t.Fatalf("sepia filter = %#v", filters[1])
	}
}

func TestComputeRemainingColorFilters(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#card { filter: invert(100%) hue-rotate(0.5turn); }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	filters := styles[card].Filters
	if len(filters) != 2 {
		t.Fatalf("filter count = %d", len(filters))
	}
	if filters[0] != (Filter{Kind: FilterInvert, Amount: 1}) {
		t.Fatalf("invert filter = %#v", filters[0])
	}
	if filters[1] != (Filter{Kind: FilterHueRotate, Amount: 180}) {
		t.Fatalf("hue-rotate filter = %#v", filters[1])
	}
}

func TestComputeFilterURLReference(t *testing.T) {
	doc, err := dom.Parse(`<html><body><svg><filter id="fx"><feGaussianBlur stdDeviation="2"></feGaussianBlur><feColorMatrix type="saturate" values="0"></feColorMatrix></filter></svg><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#card { filter: url(#fx) brightness(150%); }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[1]
	filters := styles[card].Filters
	if len(filters) != 3 {
		t.Fatalf("filter count = %d", len(filters))
	}
	if filters[0] != (Filter{Kind: FilterBlur, Amount: 2}) {
		t.Fatalf("url blur filter = %#v", filters[0])
	}
	if filters[1] != (Filter{Kind: FilterSaturate, Amount: 0}) {
		t.Fatalf("url saturate filter = %#v", filters[1])
	}
	if filters[2] != (Filter{Kind: FilterBrightness, Amount: 1.5}) {
		t.Fatalf("tail filter = %#v", filters[2])
	}
}

func TestComputeBackgroundPositionVariants(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#card { background-position: center right, right 10px bottom 20px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	computed := styles[card]
	if len(computed.BackgroundLayers) != 2 {
		t.Fatalf("background layer count = %d", len(computed.BackgroundLayers))
	}
	first := computed.BackgroundLayers[0]
	if first.PosXOrigin != "right" || first.PosX != LengthPx(0) {
		t.Fatalf("first x position = %#v %#v", first.PosXOrigin, first.PosX)
	}
	if first.PosYOrigin != "center" || first.PosY != Keyword("center") {
		t.Fatalf("first y position = %#v %#v", first.PosYOrigin, first.PosY)
	}
	second := computed.BackgroundLayers[1]
	if second.PosXOrigin != "right" || second.PosX != LengthPx(10) {
		t.Fatalf("second x position = %#v %#v", second.PosXOrigin, second.PosX)
	}
	if second.PosYOrigin != "bottom" || second.PosY != LengthPx(20) {
		t.Fatalf("second y position = %#v %#v", second.PosYOrigin, second.PosY)
	}
}

func TestComputeObjectPositionVariants(t *testing.T) {
	doc, err := dom.Parse(`<html><body><img id="photo" src="a.png"></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`#photo { object-position: bottom 25% right 10px; }`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	photo := doc.Children[0].Children[1].Children[0]
	computed := styles[photo]
	if computed.ObjectPosXOrigin != "right" || computed.ObjectPosX != LengthPx(10) {
		t.Fatalf("object-position x = %#v %#v", computed.ObjectPosXOrigin, computed.ObjectPosX)
	}
	if computed.ObjectPosYOrigin != "bottom" || computed.ObjectPosY != Number(0.25) {
		t.Fatalf("object-position y = %#v %#v", computed.ObjectPosYOrigin, computed.ObjectPosY)
	}
}

func TestComputeResponsiveLengthUnits(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="card"></div></body></html>`)
	if err != nil {
		t.Fatalf("parse dom: %v", err)
	}

	stylesheet, err := css.Parse(`
		html { font-size: 20px; }
		#card {
			width: 50vw;
			height: 25vh;
			margin: 10%;
			padding: 2em;
			font-size: 150%;
			line-height: 1.5em;
			left: 3rem;
		}
	`)
	if err != nil {
		t.Fatalf("parse css: %v", err)
	}

	styles, err := ComputeWithViewport(doc, stylesheet, Viewport{Width: 400, Height: 200})
	if err != nil {
		t.Fatalf("compute styles: %v", err)
	}

	card := doc.Children[0].Children[1].Children[0]
	computed := styles[card]
	if computed.Width != LengthPx(200) || computed.Height != LengthPx(50) {
		t.Fatalf("viewport lengths = %#v %#v", computed.Width, computed.Height)
	}
	if computed.Margin.Top != LengthPercent(0.1) {
		t.Fatalf("margin percent = %#v", computed.Margin)
	}
	if computed.FontSize != LengthPx(30) || computed.Padding.Left != LengthPx(60) || computed.LineHeight != LengthPx(45) {
		t.Fatalf("font-relative lengths = %#v", computed)
	}
	if computed.Left != LengthPx(60) {
		t.Fatalf("rem length = %#v", computed.Left)
	}
}

func TestFragmentRootFontSizeControlsRem(t *testing.T) {
	doc, err := dom.Parse(`<div id="root" style="font-size:14px"><span id="label">Label</span></div>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#label { font-size: calc(13rem / 14); }`)
	if err != nil {
		t.Fatal(err)
	}
	root := doc.Children[0]
	label := root.Children[0]
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	if got := styles[label].FontSize; got != LengthPx(13) {
		t.Fatalf("initial font size = %#v, want 13px", got)
	}
	root.Attributes["style"] = "font-size:18px"
	styles, err = RecomputeSubtreesInPlaceWithViewportAndStateAndQueries(doc, stylesheet, Viewport{}, nil, nil, styles, []*dom.Node{root})
	if err != nil {
		t.Fatal(err)
	}
	if got := styles[label].FontSize; math.Abs(got.LengthPx-13*18.0/14) > 1e-9 {
		t.Fatalf("updated font size = %#v, want %gpx", got, 13*18.0/14)
	}
}

func TestComputeFontShorthandUpdatesSizeLineHeightAndFamily(t *testing.T) {
	doc, err := dom.Parse(`<html><body><input id="field"><textarea id="notes"></textarea></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#field { font: 12px ui; } #notes { font: 14px/1.6 ui; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	field := doc.Children[0].Children[1].Children[0]
	notes := doc.Children[0].Children[1].Children[1]
	fieldStyle := styles[field]
	notesStyle := styles[notes]
	if fieldStyle.FontSize != LengthPx(12) || math.Abs(fieldStyle.LineHeight.LengthPx-14.4) > 0.001 || fieldStyle.FontFamily != Keyword("ui") {
		t.Fatalf("font shorthand field = %#v", fieldStyle)
	}
	if notesStyle.FontSize != LengthPx(14) || math.Abs(notesStyle.LineHeight.LengthPx-22.4) > 0.001 || notesStyle.FontFamily != Keyword("ui") {
		t.Fatalf("font shorthand notes = %#v", notesStyle)
	}
}

func TestShorthandCascadeRespectsSourceOrder(t *testing.T) {
	doc, err := dom.Parse(`<div id="card"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	card := doc.Children[0]

	tests := []struct {
		name        string
		declaration string
		wantMargin  float64
		wantColor   Color
		wantFont    float64
	}{
		{name: "longhand before shorthand", declaration: "margin-top: 10px; margin: 0; font-size: 20px; font: 12px ui; background-color: red; background: blue", wantMargin: 0, wantColor: Color{R: 0, G: 0, B: 255, A: 255}, wantFont: 12},
		{name: "longhand after shorthand", declaration: "margin: 0; margin-top: 10px; font: 12px ui; font-size: 20px; background: blue; background-color: red", wantMargin: 10, wantColor: Color{R: 255, G: 0, B: 0, A: 255}, wantFont: 20},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stylesheet, err := css.Parse("#card { " + test.declaration + "; }")
			if err != nil {
				t.Fatal(err)
			}
			styles, err := Compute(doc, stylesheet)
			if err != nil {
				t.Fatal(err)
			}
			computed := styles[card]
			if computed.Margin.Top != LengthPx(test.wantMargin) {
				t.Fatalf("margin-top = %#v, want %vpx", computed.Margin.Top, test.wantMargin)
			}
			if computed.BackgroundColor != ColorValue(test.wantColor) {
				t.Fatalf("background-color = %#v, want %#v", computed.BackgroundColor, test.wantColor)
			}
			if computed.FontSize != LengthPx(test.wantFont) {
				t.Fatalf("font-size = %#v, want %vpx", computed.FontSize, test.wantFont)
			}
		})
	}
}

// Stylesheets are treated as immutable: the compiled matcher caches by
// pointer identity, so rule changes require re-parsing, which yields a new
// stylesheet and a fresh compiled matcher.
func TestStylesheetMatcherInvalidatesForReparsedStylesheet(t *testing.T) {
	doc, err := dom.Parse(`<div id="card"></div>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`#card { color: red; }`)
	if err != nil {
		t.Fatal(err)
	}
	card := doc.Children[0]
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	if styles[card].Color != ColorValue(Color{R: 255, A: 255}) {
		t.Fatalf("initial color = %#v", styles[card].Color)
	}

	stylesheet, err = css.Parse(`#card { color: blue; }`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err = Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	if styles[card].Color != ColorValue(Color{B: 255, A: 255}) {
		t.Fatalf("updated color = %#v, want blue", styles[card].Color)
	}
}

func TestComputeTextBreakPropertiesInherit(t *testing.T) {
	doc, err := dom.Parse(`<html><body><div id="parent"><span id="child">copy</span></div><span id="other">x</span></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(`
		#parent { letter-spacing: 2px; word-break: break-all; overflow-wrap: break-word; }
	`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	parent := doc.Children[0].Children[1].Children[0]
	child := parent.Children[0]
	other := doc.Children[0].Children[1].Children[1]

	if styles[parent].LetterSpacing != LengthPx(2) {
		t.Fatalf("parent letter-spacing = %#v", styles[parent].LetterSpacing)
	}
	if styles[child].LetterSpacing != LengthPx(2) {
		t.Fatalf("child letter-spacing should inherit, got %#v", styles[child].LetterSpacing)
	}
	if styles[child].WordBreak != Keyword("break-all") {
		t.Fatalf("child word-break should inherit, got %#v", styles[child].WordBreak)
	}
	if styles[child].OverflowWrap != Keyword("break-word") {
		t.Fatalf("child overflow-wrap should inherit, got %#v", styles[child].OverflowWrap)
	}
	if styles[other].LetterSpacing != LengthPx(0) {
		t.Fatalf("unrelated letter-spacing = %#v, want default 0", styles[other].LetterSpacing)
	}
	if styles[other].WordBreak != Keyword("normal") {
		t.Fatalf("unrelated word-break = %#v, want normal", styles[other].WordBreak)
	}
}

func TestPointerEventsInheritanceAndOverrides(t *testing.T) {
	doc, err := dom.Parse(`<div style="pointer-events:none"><span id="inherited">text</span><button id="auto" style="pointer-events:auto">auto</button><button id="initial" style="pointer-events:initial">initial</button><button id="inherit" style="pointer-events:INHERIT">inherit</button><button id="unset" style="pointer-events:unset">unset</button></div>`)
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(doc, stylesheet)
	if err != nil {
		t.Fatal(err)
	}
	for node, computed := range styles {
		id, ok := node.Attr("id")
		if !ok {
			continue
		}
		want := "none"
		if id == "auto" || id == "initial" {
			want = "auto"
		}
		if computed.PointerEvents != Keyword(want) {
			t.Errorf("%s pointer-events = %+v, want %s", id, computed.PointerEvents, want)
		}
		for _, child := range node.Children {
			if styles[child].PointerEvents != Keyword(want) {
				t.Errorf("%s text did not inherit pointer-events:%s", id, want)
			}
		}
	}
}
