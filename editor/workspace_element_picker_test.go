package editor

import (
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
)

func particlePickerFixture(t *testing.T, root bool) (*ModuleEditor, string) {
	t.Helper()
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Particle fixture", func(mod *project.ModSpec) error {
		emitter := &project.EmitterSpec{Particles: []string{"Semi-1"}, Count: 100, Lifetime: 1, SizeMin: 1, SizeMax: 5, AlphaStart: 1}
		mod.Components = project.ModComponents{}
		mod.Elements = []project.ModElementSpec{
			{ID: "Semi-1", Components: project.ModComponents{Particle: &project.TextureSpec{Color: "#ffffff"}}},
			{ID: "Semi-2", Components: project.ModComponents{Particle: &project.TextureSpec{Color: "#ff0000"}}},
			{ID: "Semi-0", Components: project.ModComponents{Particle: &project.TextureSpec{Color: "#0000ff"}}},
			{ID: "emitter"}, {ID: "plain"},
		}
		if root {
			mod.Components.Emitter = emitter
		} else {
			mod.Elements[3].Components.Emitter = emitter
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit("main.json", "start")
	pointer := "/components/emitter"
	if !root {
		a.selectElement("start", "emitter")
		pointer = "/elements/3/components/emitter"
	}
	workspaceShellTest(t, a)
	return a, pointer
}

func TestWorkspaceParticleElementsMultiSelectSaveAndHistory(t *testing.T) {
	for _, root := range []bool{false, true} {
		name := "element"
		if root {
			name = "root"
		}
		t.Run(name, func(t *testing.T) {
			a, pointer := particlePickerFixture(t, root)
			id := workspaceFieldID(modDocumentID("button"), pointer+"/particles", "")
			countID := workspaceFieldID(modDocumentID("button"), pointer+"/count", "")
			click := func(control string) {
				t.Helper()
				a.handleEvent(htmlui.Event{ID: control, Kind: htmlui.EventClick})
				workspaceShellTest(t, a)
			}
			selectParticle := func(name string, checked bool) {
				t.Helper()
				a.handleEvent(htmlui.Event{ID: "content-element-option:" + id + ":" + name, Kind: htmlui.EventChange, Checked: checked})
				workspaceShellTest(t, a)
			}
			source := workspaceShellTest(t, a)
			if !strings.Contains(source, `class="content-element-tag"`) || strings.Contains(source, `<select id="`+id+`"`) || strings.Contains(source, `<textarea id="`+id+`"`) {
				t.Fatal("particles do not use a tag selector")
			}
			click("content-elements:" + id)
			if a.elementPicker.Field != id {
				t.Fatal("particle dropdown did not open")
			}
			for _, name := range []string{"Semi-1", "Semi-2", "Semi-0"} {
				if _, exists := a.fields["content-element-option:"+id+":"+name]; !exists {
					t.Fatalf("particle option %q missing", name)
				}
			}
			if _, exists := a.fields["content-element-option:"+id+":plain"]; exists {
				t.Fatal("element without a Particle component is selectable")
			}
			a.handleEvent(htmlui.Event{ID: countID, Kind: htmlui.EventInput, Value: "250"})
			selectParticle("Semi-2", true)
			selectParticle("Semi-0", true)
			selectParticle("Semi-0", true) // Repeated change must not toggle it off.
			click("content-element-remove:" + id + ":Semi-1")
			if a.controlValues[id] != `["Semi-2","Semi-0"]` || a.drafts[countID] != "250" || a.elementPicker.Field != id {
				t.Fatalf("selection lost a draft or closed: %q (%s)", a.controlValues[id], a.status)
			}
			selectParticle("Semi-2", false)
			selectParticle("Semi-2", true)
			a.action("save")
			workspaceShellTest(t, a)
			emitter := func() *project.EmitterSpec {
				mod := a.Document.Mods["button"].Spec
				if root {
					return mod.Components.Emitter
				}
				return mod.Elements[3].Components.Emitter
			}
			if a.hasDrafts() || emitter().Count != 250 || !slices.Equal(emitter().Particles, []string{"Semi-0", "Semi-2"}) {
				t.Fatalf("multi-selection did not save: %+v (%s)", emitter(), a.status)
			}
			a.action("undo")
			workspaceShellTest(t, a)
			if emitter().Count != 100 || !slices.Equal(emitter().Particles, []string{"Semi-1"}) {
				t.Fatalf("undo did not restore selection: %+v", emitter())
			}
			a.action("redo")
			workspaceShellTest(t, a)
			if !slices.Equal(emitter().Particles, []string{"Semi-0", "Semi-2"}) {
				t.Fatal("redo did not restore multiple particles")
			}
			click("content-element-remove:" + id + ":Semi-0")
			click("content-element-remove:" + id + ":Semi-2")
			a.action("save")
			workspaceShellTest(t, a)
			if !a.hasDrafts() || !strings.Contains(a.status, "requires a particle element") || a.controlValues[id] != "[]" {
				t.Fatalf("empty selection should remain an editable draft: %s", a.status)
			}
			click("content-elements:" + id + ":empty")
			selectParticle("Semi-1", true)
			a.action("save")
			if a.hasDrafts() || !slices.Equal(emitter().Particles, []string{"Semi-1"}) {
				t.Fatalf("empty selection could not be repaired: %s", a.status)
			}
		})
	}
}

func TestWorkspaceParticleElementsDropdownFloatsAndDismisses(t *testing.T) {
	a, pointer := particlePickerFixture(t, false)
	id := workspaceFieldID(modDocumentID("button"), pointer+"/particles", "")
	syncWorkspaceTest(t, a, a.world)
	a.handleEvent(htmlui.Event{ID: "content-elements:" + id, Kind: htmlui.EventClick})
	syncWorkspaceTest(t, a, a.world)
	root, err := dom.Parse(workspaceShellTest(t, a))
	if err != nil {
		t.Fatal(err)
	}
	var popup *dom.Node
	var find func(*dom.Node)
	find = func(node *dom.Node) {
		if node.Attributes["id"] == "content-elements-list" {
			popup = node
		}
		for _, child := range node.Children {
			find(child)
		}
	}
	find(root)
	if popup == nil || popup.Attributes["role"] != "dialog" {
		t.Fatal("dropdown missing")
	}
	for parent := popup.Parent; parent != nil; parent = parent.Parent {
		if parent.Attributes["id"] == "inspector" {
			t.Fatal("dropdown is clipped by the inspector")
		}
	}
	node, _ := htmlui.NodeByID(a.world, a.shell, "content-elements-list")
	box, ok := node.Bounds()
	if !ok || box.Width <= 0 || box.Height <= 0 || box.X < 0 || box.Y < editorTopBarHeight || box.X+box.Width > a.width || box.Y+box.Height > a.height {
		t.Fatalf("dropdown does not fit the workspace: %+v", box)
	}
	anchor := a.elementPicker.Anchor
	if abs(box.X-anchor.X) > .001 || abs(box.Width-anchor.W) > .001 || abs(box.Y-(anchor.Y+anchor.H+4)) > .001 {
		t.Fatalf("dropdown does not align with selector: popup %+v, selector %+v", box, anchor)
	}
	in := input.NewState()
	in.PressMouse(input.MouseLeft, float64(box.X+8), float64(box.Y+8))
	a.updateWorkspaceMenus(in)
	if a.elementPicker.Field != id {
		t.Fatal("clicking inside the dropdown dismissed it")
	}
	in = input.NewState()
	in.PressKey(input.KeyEscape)
	a.updateOutlinerMenu(in)
	a.updateWorkspaceMenus(in)
	if a.elementPicker.Field != "" || a.editingModule.Scene == "" || a.selectedElement != "emitter" {
		t.Fatal("Escape must close the dropdown and retain element editing")
	}
	a.action("content-elements:" + id)
	syncWorkspaceTest(t, a, a.world)
	in = input.NewState()
	in.PressMouse(input.MouseLeft, 400, 50)
	a.updateWorkspaceMenus(in)
	if a.elementPicker.Field != "" {
		t.Fatal("outside click did not dismiss the dropdown")
	}
}

func TestWorkspaceParticleElementsDropdownUsesScrolledSelectorPosition(t *testing.T) {
	a, pointer := particlePickerFixture(t, false)
	a.height, a.rightPanelWidth = 500, 420
	id := workspaceFieldID(modDocumentID("button"), pointer+"/particles", "")
	triggerID := "content-elements:" + id
	syncWorkspaceTest(t, a, a.world)
	trigger, ok := htmlui.ControlByID(a.world, a.shell, triggerID)
	if !ok {
		t.Fatal("particle dropdown trigger missing")
	}
	content, ok := ecs.Get[htmlui.ScrollContent](a.world, trigger)
	if !ok {
		t.Fatal("particle selector is not in the inspector scroll container")
	}
	scroll := ecs.GetMut[scene.UIScroll](a.world, content.Area)
	if scroll == nil || scroll.ContentSize[1] <= scroll.ViewportSize[1] {
		t.Fatal("inspector fixture does not overflow")
	}
	selector, _ := htmlui.NodeByID(a.world, a.shell, id)
	box, _ := selector.Bounds()
	offset := -min(box.Y-a.layout().Inspector.Y-60, scroll.ContentSize[1]-scroll.ViewportSize[1])
	if offset >= -40 || box.Width <= 280 {
		t.Fatalf("fixture must exercise a scrolled, wide selector: box %+v, offset %g", box, offset)
	}
	scroll.Position[1] = offset
	engineui.RegisterRuntimeFeature[engineui.HTMLRenderer](a.world)
	if err := engineui.SyncAfterLayoutSystem("").Run(a.world); err != nil {
		t.Fatal(err)
	}
	if err := scene.SyncUIGlobalTransformSystem("").Run(a.world); err != nil {
		t.Fatal(err)
	}
	global := ecs.GetRef[scene.UIGlobalTransform](a.world, trigger)
	target := ecs.GetRef[scene.InputTarget2D](a.world, trigger)
	x, y, right, bottom, ok := scene.BoundsInSpace2D(global.Matrix, scene.AnchorTopLeft, target.Width, target.Height, true)
	if !ok {
		t.Fatal("trigger has no visible bounds")
	}
	a.handleEvent(htmlui.Event{ID: triggerID, Kind: htmlui.EventClick})
	syncWorkspaceTest(t, a, a.world)
	popup, _ := htmlui.NodeByID(a.world, a.shell, "content-elements-list")
	popupBox, _ := popup.Bounds()
	wantY := box.Y + offset + box.Height + 4
	if abs(popupBox.X-box.X) > .001 || abs(popupBox.Width-box.Width) > .001 || abs(popupBox.Y-wantY) > .001 {
		t.Fatalf("dropdown ignored scrolling or selector width: popup %+v, selector %+v, offset %g", popupBox, box, offset)
	}
	in := input.NewState()
	in.PressMouse(input.MouseLeft, float64((x+right)/2), float64((y+bottom)/2))
	a.updateWorkspaceMenus(in)
	if a.elementPicker.Field != id {
		t.Fatalf("clicking the visible scrolled trigger was treated as an outside click: anchor %+v, trigger (%g,%g)-(%g,%g)", a.elementPicker.Anchor, x, y, right, bottom)
	}
	// An open dropdown follows subsequent inspector scrolling without allocating.
	scroll = ecs.GetMut[scene.UIScroll](a.world, content.Area)
	scroll.Position[1] += 16
	in = input.NewState()
	a.updateWorkspaceMenus(in)
	syncWorkspaceTest(t, a, a.world)
	popup, _ = htmlui.NodeByID(a.world, a.shell, "content-elements-list")
	popupBox, _ = popup.Bounds()
	if abs(popupBox.Y-(wantY+16)) > .001 {
		t.Fatalf("open dropdown did not follow inspector scrolling: %+v", popupBox)
	}
	if allocations := testing.AllocsPerRun(100, func() { a.updateWorkspaceMenus(in) }); allocations != 0 {
		t.Fatalf("open dropdown positioning allocated %g objects", allocations)
	}
	a.handleEvent(htmlui.Event{ID: triggerID, Kind: htmlui.EventClick})
	if a.elementPicker.Field != "" {
		t.Fatal("trigger did not close its dropdown")
	}
}

func TestWorkspaceParticleElementsWholeSelectorClick(t *testing.T) {
	for _, initiallyOpen := range []bool{false, true} {
		name := "closed"
		if initiallyOpen {
			name = "open"
		}
		t.Run(name, func(t *testing.T) {
			a, pointer := particlePickerFixture(t, false)
			id := workspaceFieldID(modDocumentID("button"), pointer+"/particles", "")
			in := input.NewState()
			ecs.InsertResource(a.world, in)
			engineui.RegisterRuntimeFeature[engineui.HTMLRenderer](a.world)
			camera := scene.SpawnCamera2D(a.world)
			ecs.GetMut[scene.Camera](a.world, camera).UpdateViewport(int(a.width), int(a.height))
			if err := scene.SyncGlobalTransformSystem("").Run(a.world); err != nil {
				t.Fatal(err)
			}
			syncWorkspaceTest(t, a, a.world)
			click := func(x, y float32) {
				t.Helper()
				if err := scene.SyncUIGlobalTransformSystem("").Run(a.world); err != nil {
					t.Fatal(err)
				}
				in.PressMouse(input.MouseLeft, float64(x), float64(y))
				if err := scene.DispatchInputEvents2DSystem("").Run(a.world); err != nil {
					t.Fatal(err)
				}
				in.EndFrame()
				in.ReleaseMouse(input.MouseLeft, float64(x), float64(y))
				if err := scene.DispatchInputEvents2DSystem("").Run(a.world); err != nil {
					t.Fatal(err)
				}
				if err := engineui.UpdateSystem("").Run(a.world); err != nil {
					t.Fatal(err)
				}
				if err := a.update(&ecs.SystemContext{World: a.world}); err != nil {
					t.Fatal(err)
				}
				in.EndFrame()
				syncWorkspaceTest(t, a, a.world)
			}
			selector, _ := htmlui.NodeByID(a.world, a.shell, id)
			box, _ := selector.Bounds()
			for _, point := range [][2]float32{
				{box.X + box.Width/2, box.Y + box.Height/2},    // Blank area.
				{box.X + 12, box.Y + box.Height/2},             // Tag name.
				{box.X + 1, box.Y + box.Height/2},              // Left border.
				{box.X + box.Width - 12, box.Y + box.Height/2}, // Arrow.
			} {
				wasOpen := a.elementPicker.Field != ""
				click(point[0], point[1])
				if (a.elementPicker.Field != "") == wasOpen {
					t.Fatalf("click at (%g, %g) did not toggle the dropdown once", point[0], point[1])
				}
			}
			if initiallyOpen {
				click(box.X+box.Width/2, box.Y+box.Height/2)
			}
			remove, _ := htmlui.NodeByID(a.world, a.shell, "content-element-remove:"+id+":Semi-1")
			removeBox, _ := remove.Bounds()
			click(removeBox.X+removeBox.Width/2, removeBox.Y+removeBox.Height/2)
			if a.drafts[id] != "[]" || (a.elementPicker.Field != "") != initiallyOpen {
				t.Fatalf("remove button changed dropdown state or failed to remove: %q, %q", a.elementPicker.Field, a.drafts[id])
			}
		})
	}
}
