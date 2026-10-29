package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

// A CSS scroll container must mount only the lines its scrollport paints, the
// way Chromium keeps layout whole and skips offscreen paint objects.
func TestHTMLScrollportCullsTextVisuals(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	world.InsertResource(input.NewState())
	const lines = 400
	body := strings.Builder{}
	for index := 1; index <= lines; index++ {
		fmt.Fprintf(&body, `<div style="height:20px">L%03d</div>`, index)
	}
	html := htmlui.New(`<div id="scroller" style="width:200px;height:120px;overflow:auto">` + body.String() + `</div>`)
	html.Width, html.Height, html.Font = 300, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(html)
	world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	mounted, area := htmlTextMounts(world)
	if _, ok := world.Get[scene.UIScroll](area); !ok {
		t.Fatal("missing scroll viewport")
	}
	if mounted["L001"] == "" {
		t.Fatal("first line missing")
	}
	if len(mounted) == 0 || len(mounted) > 60 {
		t.Fatalf("mounted lines = %d, want a scrollport window of 400 lines", len(mounted))
	}
	if _, ok := mounted["L400"]; ok {
		t.Fatal("offscreen tail mounted")
	}

	scroll := world.GetMut[scene.UIScroll](area)
	if scroll == nil {
		t.Fatal("missing scroll viewport")
	}
	scroll.Position[1] = -(scroll.ContentSize.Y() - scroll.ViewportSize.Y())
	if err := scene.UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	mounted, _ = htmlTextMounts(world)
	if len(mounted) == 0 || len(mounted) > 60 {
		t.Fatalf("mounted lines after scroll = %d, want the window to move, not grow", len(mounted))
	}
	if mounted["L400"] == "" {
		t.Fatalf("scrolled tail missing from %v", mounted)
	}
	if _, ok := mounted["L001"]; ok {
		t.Fatal("scrolled-away head still mounted")
	}
}

// htmlTextMounts maps each mounted line value to its text, and reports the
// scroll viewport those visuals are routed through.
func htmlTextMounts(world *ecs.World) (map[string]string, ecs.Entity) {
	mounted := map[string]string{}
	area := ecs.Entity(0)
	for _, access := range world.Query2[scene.Text2D, htmlui.ScrollContent]().All {
		text := access.A.Get()
		content := access.B.Get()

		mounted[text.Value] = text.Value
		area = content.Area
	}
	return mounted, area
}

// A static ancestor clip must not clamp a scrolled descendant scrollport: the
// two bands live in different coordinate spaces once the inner box scrolls.
func TestHTMLScrollportCullSurvivesStaticAncestorClip(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	world.InsertResource(input.NewState())
	const lines = 400
	body := strings.Builder{}
	for index := 1; index <= lines; index++ {
		fmt.Fprintf(&body, `<div style="height:20px">L%03d</div>`, index)
	}
	html := htmlui.New(`<div style="height:200px;overflow:hidden"><div id="scroller" style="width:200px;height:120px;overflow:auto">` + body.String() + `</div></div>`)
	html.Width, html.Height, html.Font = 300, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(html)
	world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	mounted, area := htmlTextMounts(world)
	scroll := world.GetMut[scene.UIScroll](area)
	if scroll == nil {
		t.Fatal("missing scroll viewport")
	}
	scroll.Position[1] = -600
	if err := scene.UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	mounted, _ = htmlTextMounts(world)
	for index := 31; index <= 36; index++ {
		if mounted[fmt.Sprintf("L%03d", index)] == "" {
			t.Fatalf("visible line %d missing from %v", index, mounted)
		}
	}
	if _, ok := mounted["L001"]; ok {
		t.Fatal("scrolled-away head still mounted")
	}
}
