package html

import (
	"encoding/json"
	"image"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
)

func TestHTMLBrowserBatch1(t *testing.T) { testHTMLBrowserFixture(t, "batch1", "batch1/") }
func TestHTMLBrowserBatch2(t *testing.T) { testHTMLBrowserFixture(t, "batch2", "batch2/") }
func TestHTMLBrowserBatch3(t *testing.T) { testHTMLBrowserFixture(t, "batch3", "batch3/") }
func TestHTMLBrowserBatch6(t *testing.T) { testHTMLBrowserFixture(t, "batch6", "batch6/") }
func TestHTMLBrowserBatch7(t *testing.T) { testHTMLBrowserFixture(t, "batch7", "batch7/") }
func TestHTMLBrowserText(t *testing.T)   { testHTMLBrowserFixture(t, "text", "text/") }
func TestHTMLBrowserTextBox(t *testing.T) {
	testHTMLBrowserFixture(t, "text-box", "text-box/")
}

func TestHTMLBrowserLineHeight(t *testing.T) {
	source, err := os.ReadFile("../../tests/htmlcompat/resources/line-height.html")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../tests/htmlcompat/resources/line-height/chromium-text-vertical.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]struct {
		X      float64
		Y      float64
		Width  float64
		Height float64
	}
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}

	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(string(source))
	html.Width, html.Height, html.Font = 1200, 650, notosanssc.NotoSansSCRegular
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, _ := world.Get[htmlRenderState](owner)
	actual := make(map[string]layout.Rect)
	normalLineHeight := make(map[string]bool)
	var visit func(*layout.Box)
	visit = func(box *layout.Box) {
		if box.Node != nil {
			if id, ok := box.Node.Attr("id"); ok {
				if len(box.TextFragments) > 0 {
					actual[id] = box.TextFragments[0].Rect
					normalLineHeight[id] = box.TextFragments[0].Style.LineHeightNormal
				}
				for _, control := range box.ControlFragments {
					for _, fragment := range control.TextFragments {
						actual[id] = fragment.Rect
						normalLineHeight[id] = fragment.Style.LineHeightNormal
						break
					}
				}
			}
		}
		for _, child := range box.Children {
			visit(child)
		}
	}
	visit(state.Boxes)
	for id, want := range expected {
		got, ok := actual[id]
		if !ok {
			t.Fatalf("%s missing Onix text fragment", id)
		}
		if !normalLineHeight[id] {
			t.Errorf("%s did not retain line-height: normal", id)
		}
		gotCenter := got.Y + got.Height*0.5
		wantCenter := want.Y + want.Height*0.5
		if math.Abs(gotCenter-wantCenter) > 0.5 {
			t.Errorf("%s Onix text rect=%+v; Chromium range rect=%+v", id, got, want)
		}
	}
}

func TestHTMLBrowserWorkbench(t *testing.T) {
	source, err := os.ReadFile("../../tests/htmlcompat/resources/workbench.html")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../tests/htmlcompat/resources/workbench/chromium-layout.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]layout.Rect
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	displayData, err := os.ReadFile("../../tests/htmlcompat/resources/workbench/chromium-control-display.json")
	if err != nil {
		t.Fatal(err)
	}
	var chromiumDisplays struct {
		Standalone map[string]string `json:"standalone"`
		Workbench  map[string]string `json:"workbench"`
	}
	if err := json.Unmarshal(displayData, &chromiumDisplays); err != nil {
		t.Fatal(err)
	}

	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(string(source))
	html.Width, html.Height, html.Font = 1200, 650, notosanssc.NotoSansSCRegular
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, _ := world.Get[htmlRenderState](owner)
	actual := make(map[string]layout.Rect)
	displays := make(map[string]string)
	var visit func(*layout.Box)
	visit = func(box *layout.Box) {
		if box.Node != nil {
			if id, ok := box.Node.Attr("id"); ok {
				actual[id] = box.BorderBox()
				displays[id] = box.Style.Display.Keyword
			}
			if class, _ := box.Node.Attr("class"); class == "dialog-card" {
				actual["dialog-card"] = box.BorderBox()
			} else if class == "actions" {
				actual["dialog-actions"] = box.BorderBox()
			}
		}
		for _, child := range box.Children {
			visit(child)
		}
	}
	visit(state.Boxes)

	assertRect := func(id string, tolerance float64) {
		t.Helper()
		got, ok := actual[id]
		if !ok {
			t.Fatalf("%s missing Onix box", id)
		}
		want, ok := expected[id]
		if !ok {
			t.Fatalf("%s missing Chromium probe", id)
		}
		if math.Abs(got.X-want.X) > tolerance || math.Abs(got.Y-want.Y) > tolerance || math.Abs(got.Width-want.Width) > tolerance || math.Abs(got.Height-want.Height) > tolerance {
			t.Errorf("%s: Onix %+v; Chromium %+v; tolerance %.1f", id, got, want, tolerance)
		}
	}
	for _, id := range []string{
		"dialog", "new-file", "tabs", "breadcrumb", "find-bar", "hide-find",
		"editor-host", "empty-editor", "terminal-panel", "terminal-host",
		"collapse-tree", "refresh-tree",
	} {
		assertRect(id, 0.1)
	}
	for _, id := range []string{"find-query", "find-next", "dialog-cancel", "dialog-confirm", "new-path", "dialog-actions"} {
		assertRect(id, 1)
	}
	assertRect("dialog-card", 1.5)

	for _, tag := range []string{"button", "input", "select", "textarea"} {
		if got := chromiumDisplays.Standalone[tag]; got != "inline-block" {
			t.Errorf("Chromium standalone %s display baseline = %q, want UA inline-block", tag, got)
		}
	}
	for id, want := range map[string]string{
		"find-query":     "block",
		"new-path":       "inline-block",
		"dialog-cancel":  "block",
		"dialog-confirm": "block",
	} {
		if got := chromiumDisplays.Workbench[id]; got != want {
			t.Errorf("Chromium %s display baseline = %q, want %q", id, got, want)
		}
		if got := displays[id]; got != "inline-block" {
			t.Errorf("Onix %s cascaded display = %q, want UA inline-block before layout-context blockification", id, got)
		}
	}

	for name, boxes := range map[string]map[string]layout.Rect{"Onix": actual, "Chromium": expected} {
		title, message := boxes["dialog-title"], boxes["dialog-message"]
		input, actions := boxes["new-path"], boxes["dialog-actions"]
		if !(title.Y < message.Y && message.Y < input.Y && input.Y < actions.Y) {
			t.Errorf("%s dialog flow order: title=%g message=%g input=%g actions=%g", name, title.Y, message.Y, input.Y, actions.Y)
		}
		dialog, card := boxes["dialog"], boxes["dialog-card"]
		if math.Abs(card.X+card.Width/2-(dialog.X+dialog.Width/2)) > 0.1 || math.Abs(card.Y+card.Height/2-(dialog.Y+dialog.Height/2)) > 0.1 {
			t.Errorf("%s dialog card is not centered: dialog=%+v card=%+v", name, dialog, card)
		}
	}
}

func TestHTMLBrowserWorkbenchImages(t *testing.T) {
	source, err := os.ReadFile("../../tests/htmlcompat/resources/workbench.html")
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(string(source))
	html.Width, html.Height, html.Font = 1200, 650, notosanssc.NotoSansSCRegular
	bundle.Put(html)
	owner := world.Spawn(bundle)
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 24, 24)))
	t.Cleanup(func() { _ = texture.Dispose() })
	world.InsertResource[ImageProvider](testHTMLImageProvider{texture: texture})
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	want := strings.Count(string(source), "<img ")
	got := 0
	for _, access := range world.Query2[htmlChild, scene.Sprite2D]().All {
		child := access.A.Get()
		sprite := access.B.Get()

		if child.Owner == owner && sprite.Texture == texture {
			got++
		}
	}
	if got != want {
		t.Fatalf("workbench image visuals = %d, want %d", got, want)
	}
}

func testHTMLBrowserFixture(t *testing.T, fixture, baseline string) {
	source, err := os.ReadFile("../../tests/htmlcompat/resources/" + fixture + ".html")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../tests/htmlcompat/resources/" + baseline + "chromium-layout.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]layout.Rect
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	html := New(string(source))
	html.Width, html.Height, html.Font = 1200, 650, notosanssc.NotoSansSCRegular
	bundle.Put(html)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, _ := world.Get[htmlRenderState](owner)
	for e, access := range world.Query2[htmlChild, scene.Sprite2D]().All {
		c := access.A.Get()
		s := access.B.Get()

		if c.Node != nil {
			if id, _ := c.Node.Attr("id"); id == "opacity" {
				alpha := s.Alpha
				if world.Has[htmlOpacitySurface](e) {
					raster := state.Textures.rasters[c.Node]
					alpha *= float32(raster.image.RGBAAt(raster.image.Rect.Dx()/2, raster.image.Rect.Dy()/2).A) / 255
				}
				if math.Abs(float64(alpha)-0.5) > 1.0/255 {
					t.Errorf("opacity alpha=%v want 0.5", s.Alpha)
				}
			}
		}
	}
	textData, err := os.ReadFile("../../tests/htmlcompat/resources/" + baseline + "chromium-text.json")
	if err != nil {
		t.Fatal(err)
	}
	var textStarts map[string]float64
	if err := json.Unmarshal(textData, &textStarts); err != nil {
		t.Fatal(err)
	}
	actual := map[string]layout.Rect{}
	var visit func(*layout.Box)
	visit = func(box *layout.Box) {
		if id, ok := box.Node.Attr("id"); ok {
			actual[id] = box.BorderBox()
			if want, ok := textStarts[id]; ok {
				if len(box.TextFragments) == 0 {
					t.Errorf("%s missing text", id)
				} else if got := box.TextFragments[0].Rect.X; math.Abs(got-want) > 0.1 {
					t.Errorf("%s text start=%v Chromium=%v", id, got, want)
				}
			}
		}
		for _, child := range box.Children {
			visit(child)
		}
	}
	visit(state.Boxes)
	for id, want := range expected {
		t.Run(id, func(t *testing.T) {
			got, ok := actual[id]
			if !ok {
				t.Fatal("missing box")
			}
			if math.Abs(got.X-want.X) > 0.1 || math.Abs(got.Y-want.Y) > 0.1 || math.Abs(got.Width-want.Width) > 0.1 || math.Abs(got.Height-want.Height) > 0.1 {
				t.Errorf("Onix %+v; Chromium %+v", got, want)
			}
		})
	}
}

func TestHTMLAdjacentMutationRefreshesSibling(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.active + div {width:90px} div {width:30px;height:20px}</style><div id="first"></div><div id="second"></div>`)
	incrementalHTMLNode(t, world, owner, "first").SetAttribute("class", "active")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	node := incrementalHTMLNode(t, world, owner, "second")
	if got := state.BoxesByNode[node.node].Content.Width; got != 90 {
		t.Fatalf("sibling width = %v", got)
	}
}

func TestHTMLCanvasBackgroundMutation(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<html><body id="body" style="background:red"><div style="position:absolute;width:20px;height:20px"></div></body></html>`)
	incrementalHTMLNode(t, world, owner, "body").SetAttribute("style", "background:blue")
	incrementalSync(t, world)
	found := false
	for _, access := range world.Query2[htmlCanvasSurface, scene.Sprite2D]().All {
		s := access.B.Get()

		found = true
		if s.Shape.Fill.B != 1 || s.Shape.Fill.R != 0 {
			t.Errorf("canvas=%+v", s.Shape.Fill)
		}
	}
	if !found {
		t.Fatal("missing canvas")
	}
}

func TestHTMLAutoPositionedPaintOrder(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="earlier" style="position:relative"><div style="position:relative"><div id="deep" style="position:relative;width:30px;height:30px;background:red"></div></div></div><div id="later" style="position:relative;width:30px;height:30px;background:blue"></div><div id="ordinary" style="width:30px;height:30px;background:green"></div>`)
	state, _ := world.Get[htmlRenderState](owner)
	stack := func(id string) int {
		node := incrementalHTMLNode(t, world, owner, id)
		return htmlBoxStackIndex(state.BoxesByNode[node.node], 0)
	}
	if stack("later") <= stack("deep") || stack("deep") <= stack("ordinary") {
		t.Fatal("positioned auto paint order differs from document order")
	}
}

func TestHTMLLeafOpacityMutation(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target" style="width:40px;height:40px;background:red;opacity:0.5"></div>`)
	incrementalHTMLNode(t, world, owner, "target").SetAttribute("style", "width:40px;height:40px;background:red;opacity:25%")
	incrementalSync(t, world)
	found := false
	state, _ := world.Get[htmlRenderState](owner)
	for entity, access := range world.Query2[htmlChild, scene.Sprite2D]().All {
		c := access.A.Get()
		s := access.B.Get()

		if c.Owner == owner && c.Node != nil {
			if id, _ := c.Node.Attr("id"); id == "target" {
				found = true
				alpha := s.Alpha
				if world.Has[htmlOpacitySurface](entity) {
					alpha *= float32(state.Textures.rasters[c.Node].image.RGBAAt(10, 10).A) / 255
				}
				if math.Abs(float64(alpha)-0.25) > 1.0/255 {
					t.Errorf("alpha=%v", alpha)
				}
			}
		}
	}
	if !found {
		t.Fatal("missing opacity surface")
	}
}

func TestHTMLTextOpacityAndLCDMutation(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="backdrop" style="background:white"><div id="target" style="opacity:0.5">Alpha</div></div>`)
	assertText := func(alpha float32, lcd bool) {
		t.Helper()
		found := false
		state, _ := world.Get[htmlRenderState](owner)
		if !lcd {
			var composite *htmlOpacityRaster
			var compositeNode Node
			for _, id := range []string{"backdrop", "target"} {
				node := incrementalHTMLNode(t, world, owner, id)
				if rendered := state.BoxRenderings[node.node]; rendered != nil && rendered.Composite && state.Textures.rasters[node.node] != nil {
					composite = state.Textures.rasters[node.node]
					compositeNode = node
					break
				}
			}
			if composite != nil {
				maxAlpha := uint8(0)
				ink := false
				for index := 0; index < len(composite.image.Pix); index += 4 {
					maxAlpha = max(maxAlpha, composite.image.Pix[index+3])
					ink = ink || composite.image.Pix[index+3] > 0 && composite.image.Pix[index] < 128
				}
				visual := htmlCullRole(world, owner, compositeNode, htmlRoleSurface)
				sprite, _ := world.Get[scene.Sprite2D](visual)
				if !ink || math.Abs(float64(maxAlpha)/255*float64(sprite.Alpha)-float64(alpha)) > 1.0/255 {
					t.Fatalf("composited text: ink=%v alpha=%d, want %g", ink, maxAlpha, alpha)
				}
				return
			}
		}
		for _, access := range world.Query2[htmlChild, scene.Text2D]().All {
			c := access.A.Get()
			text := access.B.Get()

			if c.Owner == owner && text.Value == "Alpha" {
				found = true
				if text.Alpha != alpha || text.LCD != lcd {
					t.Errorf("paint alpha=%g LCD=%v, want %g %v", text.Alpha, text.LCD, alpha, lcd)
				}
			}
		}
		if !found {
			t.Fatal("missing text")
		}
	}
	assertText(0.5, false)
	incrementalHTMLNode(t, world, owner, "target").SetAttribute("style", "opacity:1")
	incrementalSync(t, world)
	assertText(1, true)
	incrementalHTMLNode(t, world, owner, "backdrop").SetAttribute("style", "background:transparent")
	incrementalSync(t, world)
	assertText(1, false)
	incrementalHTMLNode(t, world, owner, "backdrop").SetAttribute("style", "background:white;opacity:0.5")
	incrementalSync(t, world)
	assertText(0.5, false)
	incrementalHTMLNode(t, world, owner, "target").SetAttribute("style", "opacity:0.25")
	incrementalSync(t, world)
	assertText(0.5, false)
}

func TestHTMLFlexConstraintMutationRedistributesSiblings(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="display:flex;width:160px;height:60px"><div id="limited" style="flex:1;max-width:40px"></div><div id="remaining" style="flex:1"></div></div>`)
	for _, limit := range []struct {
		css           string
		first, second float64
	}{
		{"flex:1;max-width:40px", 40, 120},
		{"flex:1;min-width:100px", 100, 60},
		{"flex:1", 80, 80},
	} {
		incrementalHTMLNode(t, world, owner, "limited").SetAttribute("style", limit.css)
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		first := state.BoxesByNode[incrementalHTMLNode(t, world, owner, "limited").node].Content
		second := state.BoxesByNode[incrementalHTMLNode(t, world, owner, "remaining").node].Content
		if first.Width != limit.first || second.Width != limit.second || second.X != first.X+first.Width {
			t.Fatalf("%s: first=%+v second=%+v", limit.css, first, second)
		}
	}
}
func TestHTMLBrowserBatch4(t *testing.T) { testHTMLBrowserFixture(t, "batch4", "batch4/") }
func TestHTMLBrowserBatch5(t *testing.T) { testHTMLBrowserFixture(t, "batch5", "batch5/") }
