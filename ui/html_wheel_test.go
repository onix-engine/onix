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
	"github.com/go-gl/mathgl/mgl32"
)

// A code editor overlays a transparent <textarea> sized to its whole content and
// leaves scrolling to an ancestor overflow container. Chromium chains a wheel the
// hovered box cannot absorb to the nearest scrollable ancestor.
func TestHTMLWheelOverNonScrollableTextareaChainsToScrollContainer(t *testing.T) {
	world, editor := newHTMLWheelTextareaWorld(t, `position:absolute;top:0;left:0;right:0;bottom:0;height:100%`)
	area, areas := ecs.Entity(0), 0
	for entity, _ := range ecs.Query[scene.UIScroll](world).All {
		area, areas = entity, areas+1
	}
	if areas != 1 {
		t.Fatalf("scroll areas = %d, want 1", areas)
	}

	state, _ := ecs.GetResource[*input.State](world)
	state.MoveMouse(150, 100)
	state.ScrollMouse(0, -1)
	if err := runUIUpdate(world); err != nil {
		t.Fatal(err)
	}
	if wheel := state.Pointer().WheelY; wheel == 0 {
		t.Fatal("textarea consumed a wheel it cannot scroll")
	}
	if err := scene.UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	scroll, ok := ecs.Get[scene.UIScroll](world, area)
	if !ok {
		t.Fatal("missing scroll viewport")
	}
	if scroll.Position.Y() == 0 {
		t.Fatalf("wheel over the textarea never reached the container (content %v, viewport %v)", scroll.ContentSize, scroll.ViewportSize)
	}
	textarea, ok := ecs.Get[Textarea](world, editor)
	if !ok {
		t.Fatal("missing textarea")
	}
	if textarea.ScrollY != 0 {
		t.Fatalf("textarea scrolled to %v, want 0 for a box as tall as its content", textarea.ScrollY)
	}
}

// A textarea that can scroll in the wheel direction keeps the wheel, so chaining
// must not leak it to the ancestor container.
func TestHTMLWheelOverScrollableTextareaStaysInTextarea(t *testing.T) {
	world, editor := newHTMLWheelTextareaWorld(t, `position:absolute;top:0;left:0;right:0;bottom:0;height:120px;overflow:auto`)

	state, _ := ecs.GetResource[*input.State](world)
	state.MoveMouse(150, 60)
	state.ScrollMouse(0, -1)
	if err := runUIUpdate(world); err != nil {
		t.Fatal(err)
	}
	if pointer := state.Pointer(); pointer.WheelX != 0 || pointer.WheelY != 0 {
		t.Fatalf("scrollable textarea did not consume the wheel: %+v", pointer)
	}

	textarea, ok := ecs.Get[Textarea](world, editor)
	if !ok {
		t.Fatal("missing textarea")
	}
	if textarea.ScrollY <= 0 {
		t.Fatalf("textarea scroll y = %v, want the wheel to scroll it", textarea.ScrollY)
	}
}

// newHTMLWheelTextareaWorld lays out a 300x200 overflow container holding 60
// lines of code with an absolutely positioned textarea over them, and reports
// the textarea.
func newHTMLWheelTextareaWorld(t *testing.T, editorStyle string) (*ecs.World, ecs.Entity) {
	t.Helper()
	world := newUIWorld(t)
	code := strings.Builder{}
	for index := 1; index <= 60; index++ {
		fmt.Fprintf(&code, "const value%d = %d;\n", index, index)
	}
	html := htmlui.New(`<div style="position:relative;width:300px;height:200px">` +
		`<div id="scroller" style="position:absolute;inset:0;overflow:auto">` +
		`<div style="position:relative;width:300px;height:1300px">` +
		`<textarea id="editor" style="` + editorStyle + `;margin:0;padding:0;border:0;white-space:pre">` + code.String() + `</textarea>` +
		`</div></div></div>`)
	html.Width, html.Height, html.Font = 300, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	owner := world.Spawn(bundle)
	for range 3 {
		if err := runUIFrame(world); err != nil {
			t.Fatal(err)
		}
	}
	return world, htmlControlByID(t, world, owner, "editor")
}

// Nested overflow containers keep both axes live: an axis the hovered inner
// container cannot move chains to the outer container instead of being lost.
func TestHTMLOverflowWheelChainsNestedAxisToAncestor(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	state := input.NewState()
	ecs.InsertResource(world, state)
	html := htmlui.New(`<div id="outer" style="width:300px;height:200px;overflow-y:auto;overflow-x:hidden">` +
		`<div style="height:600px">` +
		`<div id="inner" style="width:200px;height:100px;overflow-x:auto;overflow-y:hidden">` +
		`<div style="width:600px;height:100px">A</div>` +
		`</div></div></div>`)
	html.Width, html.Height = 300, 200
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	scrollAreas := func() (outer, inner scene.UIScroll) {
		t.Helper()
		for _, access := range ecs.Query[scene.UIScroll](world).All {
			scroll := access.Get()

			if scroll.Vertical && !scroll.Horizontal {
				outer = scroll
			}
			if scroll.Horizontal && !scroll.Vertical {
				inner = scroll
			}
		}
		return outer, inner
	}
	outer, inner := scrollAreas()
	if !outer.Vertical || !inner.Horizontal {
		t.Fatalf("nested scroll areas missing: outer=%+v inner=%+v", outer, inner)
	}

	state.MoveMouse(50, 50)
	state.ScrollMouse(0, -1)
	if err := scene.UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	outer, inner = scrollAreas()
	if outer.Position.Y() != -48 {
		t.Fatalf("vertical wheel offset = %v, want the outer container to scroll", outer.Position.Y())
	}
	if inner.Position.X() != 0 {
		t.Fatalf("vertical wheel moved the inner container to %v", inner.Position.X())
	}
	if wheel := state.Pointer().WheelY; wheel != 0 {
		t.Fatalf("nested wheel left wheel=%v, want consumed", wheel)
	}

	state.EndFrame()
	state.ScrollMouse(-1, 0)
	if err := scene.UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	outer, inner = scrollAreas()
	if inner.Position.X() != -48 {
		t.Fatalf("horizontal wheel offset = %v, want the inner container to scroll", inner.Position.X())
	}
	if outer.Position.Y() != -48 {
		t.Fatalf("horizontal wheel moved the outer container to %v", outer.Position.Y())
	}
}

func TestHTMLOverflowWheelScrollsHoveredContainer(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	state := input.NewState()
	ecs.InsertResource(world, state)
	html := htmlui.New(`<div style="display:flex;width:300px;height:200px">` +
		`<div id="list" style="width:100px;height:200px;overflow:auto"><div style="height:600px">A</div></div>` +
		`<div id="side" style="width:200px;height:200px;overflow:auto"><div style="height:600px">B</div></div>` +
		`</div>`)
	html.Width, html.Height, html.Font = 300, 200, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, html)
	world.Spawn(bundle)
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state.MoveMouse(50, 100)
	state.ScrollMouse(0, -1)
	if err := scene.UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	var areas []mgl32.Vec3
	seen := map[ecs.Entity]bool{}
	for _, access := range ecs.Query[htmlui.ScrollContent](world).All {
		content := access.Get()

		if content.Area != 0 && !seen[content.Area] {
			seen[content.Area] = true
			if scroll, ok := ecs.Get[scene.UIScroll](world, content.Area); ok {
				transform, _ := ecs.Get[scene.UITransform](world, content.Area)
				areas = append(areas, mgl32.Vec3{transform.Translation.X.Value, scroll.Position.X(), scroll.Position.Y()})
			}
		}
	}
	if len(areas) != 2 {
		t.Fatalf("scroll areas = %d, want 2", len(areas))
	}
	scrolled := 0
	for _, area := range areas {
		if area[2] < 0 {
			scrolled++
			if area[2] != -48 {
				t.Fatalf("HTML wheel offset = %v, want -48", area[2])
			}
			if area[0] > 50 {
				t.Fatalf("wheel scrolled the non-hovered container at x=%v (offset %v)", area[0], area[1:])
			}
		}
	}
	if scrolled != 1 {
		t.Fatalf("wheel scrolled %d containers, want exactly 1 (areas %v)", scrolled, areas)
	}
}
