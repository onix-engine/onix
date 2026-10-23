package editor

import (
	"net/url"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

func pickerInspectorScroll(t *testing.T, a *ModuleEditor, control ecs.Entity) scene.UIScroll {
	t.Helper()
	content, ok := ecs.Get[htmlui.ScrollContent](a.world, control)
	if !ok {
		// Short inspectors still have an overflow viewport, but controls need
		// ScrollContent only when there is an actual offset to apply.
		for entity := control; entity != 0; {
			if scroll, ok := ecs.Get[scene.UIScroll](a.world, entity); ok {
				return scroll
			}
			parent, ok := ecs.Get[scene.Parent](a.world, entity)
			if !ok {
				break
			}
			entity = parent.Entity
		}
		t.Fatal("Add Component has no inspector scroll container")
	}
	scroll, ok := ecs.Get[scene.UIScroll](a.world, content.Area)
	if !ok {
		t.Fatal("inspector scroll container is missing")
	}
	return scroll
}

func TestWorkspaceComponentPickerFloatsAndDismisses(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{})
	syncWorkspaceTest(t, a, a.world)
	trigger, _ := htmlui.NodeByID(a.world, a.shell, "content-picker")
	control, _ := htmlui.ControlByID(a.world, a.shell, "content-picker")
	content := pickerInspectorScroll(t, a, control)
	before := content.ContentSize
	a.handleEvent(htmlui.Event{ID: "content-picker", Kind: htmlui.EventClick})
	syncWorkspaceTest(t, a, a.world)
	root, err := dom.Parse(workspaceShellTest(t, a))
	if err != nil {
		t.Fatal(err)
	}
	var menu *dom.Node
	var findMenu func(*dom.Node)
	findMenu = func(node *dom.Node) {
		if node.Attributes["id"] == "content-picker-list" {
			menu = node
			return
		}
		for _, child := range node.Children {
			findMenu(child)
		}
	}
	findMenu(root)
	if menu == nil || menu.Attributes["role"] != "menu" {
		t.Fatal("component picker is not a context menu")
	}
	for parent := menu.Parent; parent != nil; parent = parent.Parent {
		if parent.Attributes["id"] == "inspector" {
			t.Fatal("component menu is inside the inspector scroll container")
		}
	}
	if _, hidden := menu.Attr("hidden"); hidden {
		t.Fatal("opened menu is hidden")
	}
	content = pickerInspectorScroll(t, a, control)
	if content.ContentSize != before {
		t.Fatalf("opening the menu changed inspector content size: %v -> %v", before, content.ContentSize)
	}
	node, _ := htmlui.NodeByID(a.world, a.shell, "content-picker-list")
	box, ok := node.Bounds()
	if !ok || box.Width <= 0 || box.Height <= 0 || box.X < 0 || box.Y < editorTopBarHeight || box.X+box.Width > a.width || box.Y+box.Height > a.height {
		t.Fatalf("menu does not fit in the workspace: %+v", box)
	}
	button, _ := trigger.Bounds()
	if box.Y < button.Y+button.Height && box.Y+box.Height > button.Y {
		t.Fatal("menu covers its trigger")
	}
	in := input.NewState()
	in.PressMouse(input.MouseLeft, float64(box.X+8), float64(box.Y+8))
	a.updateWorkspaceMenus(in)
	if !a.componentPickerOpen {
		t.Fatal("click inside menu dismissed it before selection")
	}
	in = input.NewState()
	in.PressKey(input.KeyEscape)
	a.updateOutlinerMenu(in)
	a.updateWorkspaceMenus(in)
	if a.componentPickerOpen || a.editingModule.Scene == "" || a.selectedElement != "face" {
		t.Fatal("Escape must dismiss the menu and keep editing the element")
	}
	a.action("content-picker")
	syncWorkspaceTest(t, a, a.world)
	in = input.NewState()
	in.PressMouse(input.MouseLeft, 400, 50)
	a.updateWorkspaceMenus(in)
	if a.componentPickerOpen {
		t.Fatal("outside click did not dismiss the menu")
	}
}

func TestWorkspaceComponentPickerScrollsAboveInspector(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{40, 20}}})
	a.height = 500
	syncWorkspaceTest(t, a, a.world)
	trigger, _ := htmlui.ControlByID(a.world, a.shell, "content-picker")
	inspectorContent, _ := ecs.Get[htmlui.ScrollContent](a.world, trigger)
	inspectorBefore, _ := ecs.Get[scene.UIScroll](a.world, inspectorContent.Area)
	a.action("content-picker")
	syncWorkspaceTest(t, a, a.world)
	if err := scene.SyncUIGlobalTransformSystem("").Run(a.world); err != nil {
		t.Fatal(err)
	}
	control, ok := htmlui.ControlByID(a.world, a.shell, "content-add:button%2Fmod.json|/elements/0/components/layout")
	if !ok {
		t.Fatal("missing Layout menu item")
	}
	content, _ := ecs.Get[htmlui.ScrollContent](a.world, control)
	menuBefore, ok := ecs.Get[scene.UIScroll](a.world, content.Area)
	if !ok || !menuBefore.Vertical || menuBefore.ContentSize[1] <= menuBefore.ViewportSize[1] {
		t.Fatalf("component menu is not scrollable: %+v", menuBefore)
	}
	node, _ := htmlui.NodeByID(a.world, a.shell, "content-picker-list")
	box, _ := node.Bounds()
	in := input.NewState()
	ecs.InsertResource(a.world, in)
	in.MoveMouse(float64(box.X+20), float64(box.Y+20))
	in.ScrollMouse(0, -1)
	if err := scene.UIScrollSystem("").Run(a.world); err != nil {
		t.Fatal(err)
	}
	menuAfter, _ := ecs.Get[scene.UIScroll](a.world, content.Area)
	inspectorAfter, _ := ecs.Get[scene.UIScroll](a.world, inspectorContent.Area)
	if menuAfter.Position[1] != -48 || inspectorAfter.Position != inspectorBefore.Position {
		t.Fatalf("wheel scrolled the wrong container: menu %v, inspector %v", menuAfter.Position, inspectorAfter.Position)
	}
	in.EndFrame()
	in.PressMouse(input.MouseLeft, float64(box.X+box.Width-4), float64(box.Y+20))
	if err := scene.UIScrollSystem("").Run(a.world); err != nil {
		t.Fatal(err)
	}
	in.EndFrame()
	in.MoveMouse(float64(box.X+box.Width-4), float64(box.Y+50))
	if err := scene.UIScrollSystem("").Run(a.world); err != nil {
		t.Fatal(err)
	}
	menuDragged, _ := ecs.Get[scene.UIScroll](a.world, content.Area)
	inspectorAfter, _ = ecs.Get[scene.UIScroll](a.world, inspectorContent.Area)
	if menuDragged.Position[1] >= menuAfter.Position[1] || inspectorAfter.Position != inspectorBefore.Position {
		t.Fatalf("thumb drag scrolled the wrong container: menu %v, inspector %v", menuDragged.Position, inspectorAfter.Position)
	}
}

func TestWorkspaceTrailAndMaterialAreIndependentComponents(t *testing.T) {
	a := contentEditorFixture(t, project.ModComponents{Sprite: &project.ModSpriteSpec{Color: "#ffffff", Size: []float32{40, 20}}})
	target, base := modDocumentID("button"), "/elements/0/components"
	for _, name := range [...]string{"trail", "material"} {
		a.action("content-picker")
		workspaceShellTest(t, a)
		id := "content-add:" + url.PathEscape(string(target)) + "|" + base + "/" + name
		a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventClick})
		if a.componentPickerOpen {
			t.Fatal("component selection did not close the menu")
		}
	}
	source := workspaceShellTest(t, a)
	for _, name := range [...]string{"trail", "material"} {
		if !strings.Contains(source, `id="body:content:`+base+`/`+name+`"`) || strings.Contains(source, base+"/sprite/"+name) {
			t.Fatalf("%s is not an independent Details section", name)
		}
	}
	a.handleEvent(htmlui.Event{ID: workspaceFieldID(target, base+"/trail/lifetime", ""), Kind: htmlui.EventInput, Value: "0.8"})
	a.action("save")
	if a.hasDrafts() {
		t.Fatalf("component edits failed to save: %s", a.status)
	}
	reloaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	components := reloaded.Mods["button"].Spec.Elements[0].Components
	if components.Trail == nil || components.Trail.Lifetime != .8 || components.Trail.MaxSamples != 24 || components.Material == nil {
		t.Fatalf("independent components did not reload: %+v", components)
	}
	a.handleEvent(htmlui.Event{ID: "remove:" + url.PathEscape(string(target)) + "|" + base + "/sprite", Kind: htmlui.EventClick})
	a.action("save")
	components = a.Document.Mods["button"].Spec.Elements[0].Components
	if components.Sprite != nil || components.Trail == nil || components.Material == nil {
		t.Fatalf("removing Sprite removed its sibling components: %+v (%s)", components, a.status)
	}
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "remove:" + url.PathEscape(string(target)) + "|" + base + "/trail", Kind: htmlui.EventClick})
	a.action("save")
	components = a.Document.Mods["button"].Spec.Elements[0].Components
	if components.Trail != nil || components.Material == nil {
		t.Fatal("removing Trail changed Material")
	}
	a.action("undo")
	if a.Document.Mods["button"].Spec.Elements[0].Components.Trail == nil {
		t.Fatal("undo did not restore the independent Trail component")
	}
}
