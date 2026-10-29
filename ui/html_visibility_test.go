package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLVisibilityFocusAndTabNavigation(t *testing.T) {
	world := ecs.NewWorld()
	RegisterRuntimeFeature[HTMLRenderer](world)
	world.InsertResource(input.NewState())
	page := htmlui.New(`<style>.hidden{visibility:hidden}</style><div id="parent" class="hidden"><input id="hidden" autofocus value="edited"><button id="visible" style="visibility:visible" autofocus>Visible</button></div><button id="last">Last</button>`)
	page.Width, page.Height, page.Font = 320, 120, notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(page)
	owner := world.Spawn(bundle)
	sync := func() {
		t.Helper()
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
	}
	sync()
	hidden := htmlControlByID(t, world, owner, "hidden")
	visible := htmlControlByID(t, world, owner, "visible")
	last := htmlControlByID(t, world, owner, "last")
	focus, _ := world.GetResource[*scene.InputFocus]()
	if focus == nil || focus.Keyboard != visible {
		t.Fatalf("autofocus did not skip hidden input: %+v", focus)
	}
	if FocusControl(world, hidden) {
		t.Fatal("programmatic focus accepted hidden input")
	}
	state, _ := world.GetResource[*uiEventState]()
	focusNextHTMLElement(world, state, false)
	if state.Focused != last {
		t.Fatal("tab did not advance past visible descendant")
	}
	focusNextHTMLElement(world, state, false)
	if state.Focused != visible {
		t.Fatal("tab traversal included hidden input")
	}
	parent, _ := htmlui.NodeByID(world, owner, "parent")
	parent.SetAttribute("class", "")
	sync()
	if !FocusControl(world, hidden) {
		t.Fatal("shown input could not receive focus")
	}
	parent.SetAttribute("class", "hidden")
	sync()
	if state.Focused != 0 || focus.Keyboard != 0 {
		t.Fatalf("hiding focused input did not clear focus: ui=%d keyboard=%d", state.Focused, focus.Keyboard)
	}
	if field := world.GetRef[TextField](hidden); field == nil || field.Value != "edited" || field.Focused {
		t.Fatalf("blur lost input state: %+v", field)
	}
}
