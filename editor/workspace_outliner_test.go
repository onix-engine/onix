package editor

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/diiyw/onix/window"
)

func TestWorkspaceOutlinerDiscoversScenesAndSelectsRepeatedInstanceIDs(t *testing.T) {
	a := workspaceEditorFixture(t)
	file := project.ScenePath("game ui.json")
	if err := writeWorkspaceFile(a.Document.Absolute(file), []byte(`{"modules":[{"id":"start","mod":"button","children":[{"id":"child","mod":"empty"}]}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.Refresh(); err != nil {
		t.Fatal(err)
	}
	a.changed()
	source := workspaceShellTest(t, a)
	if len(a.Document.Scenes) != 2 || len(a.rows) != 4 || !strings.Contains(source, `id="tree-select:game%20ui.json/start"`) || strings.Contains(source, `id="scene-select"`) || strings.Contains(source, `id="create-scene"`) {
		t.Fatal("outliner did not show scanned scenes with distinct instance controls")
	}
	if strings.Contains(source, `id="scene-toggle:`) || !strings.Contains(source, `class="scene-tree-heading"`) || !strings.Contains(source, `<img alt="" src="onix-leftbar/arrow.svg"><span>game ui</span></button>`) {
		t.Fatal("scene arrow and name are not part of one tree control")
	}
	if !strings.Contains(source, `<img alt="" src="onix-leftbar/component.svg"><span>start</span></button>`) || strings.Contains(source, `id="tree-select:game%20ui.json/child"`) || strings.Contains(source, `>Button</span>`) || strings.Contains(source, `>start · button</span>`) {
		t.Fatal("module rows do not show their icon and instance ID")
	}
	a.handleEvent(htmlui.Event{ID: "tree-select:game%20ui.json/start", Kind: htmlui.EventClick})
	if a.Document.Active != "game ui.json" || a.Selected != "start" {
		t.Fatal("selection resolved the wrong scene")
	}
	if source := workspaceShellTest(t, a); len(a.rows) != 5 || !strings.Contains(source, `id="select:child"`) || !strings.Contains(source, `<img alt="" src="onix-leftbar/component.svg"><span>child</span></button>`) {
		t.Fatal("clicking an inactive scene's parent module did not expand its children")
	}
	a.time++
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: workspaceFieldID(sceneDocumentID(a.Document.Active), "/modules/0/placement/position", "X"), Kind: htmlui.EventInput, Value: "88"})
	a.handleEvent(htmlui.Event{ID: "tree-select:main.json/start", Kind: htmlui.EventClick})
	if a.pendingAction == "" || a.Document.Active != "game ui.json" {
		t.Fatal("navigation discarded a draft")
	}
	a.action("discard-pending")
	if a.Document.Active != "main.json" || a.Selected != "start" {
		t.Fatal("deferred tree selection lost scene ownership")
	}
	a.handleEvent(htmlui.Event{ID: "scene:game%20ui.json", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	if a.Document.Active != "game ui.json" || a.Selected != "" || len(a.rows) != 4 {
		t.Fatal("selecting a scene did not expand its modules")
	}
	a.handleEvent(htmlui.Event{ID: "scene:game%20ui.json", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	if len(a.rows) != 3 {
		t.Fatal("collapsed scene still exposed its modules")
	}
	a.handleEvent(htmlui.Event{ID: "scene:game%20ui.json", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	if len(a.rows) != 4 {
		t.Fatal("clicking the collapsed scene did not expand its modules")
	}
}

func TestWorkspaceOutlinerResourceSelectionCollapseAndDelete(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := os.MkdirAll(a.Document.Absolute("button/fonts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"button/face.png", "button/fonts/ui.ttf"} {
		if err := writeWorkspaceFile(a.Document.Absolute(file), []byte("resource")); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Document.EditMod("button", "Add tree content", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{{ID: "face"}}
		mod.Resources = map[string]project.ModResourceSpec{
			"ui-font": {Kind: "font", Path: "fonts/ui.ttf"},
			"face":    {Kind: "image", Path: "face.png"},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if source := workspaceShellTest(t, a); strings.Contains(source, `id="resource:main.json/start/`) {
		t.Fatal("scene mode exposed definition resources")
	}
	a.enterModuleEdit("main.json", "start")
	source := workspaceShellTest(t, a)
	fontID := "resource:main.json/start/ui-font"
	groupID := "tree-group:main.json/start/resources"
	if !strings.Contains(source, `id="`+fontID+`"`) || !strings.Contains(source, `id="element:main.json/start/face"`) || !strings.Contains(source, `id="resource:main.json/start/face"`) {
		t.Fatal("elements and resources did not coexist with distinct tree controls")
	}
	fieldID := workspaceFieldID(modDocumentID("button"), "/components/size", "Width")
	a.handleEvent(htmlui.Event{ID: fieldID, Kind: htmlui.EventInput, Value: "77"})
	a.handleEvent(htmlui.Event{ID: fontID, Kind: htmlui.EventClick})
	if a.pendingAction != fontID || a.selectedResource != "" {
		t.Fatal("resource selection discarded a module draft")
	}
	a.action("discard-pending")
	source = workspaceShellTest(t, a)
	if a.selectedResource != "ui-font" || a.selectedElement != "" || !strings.Contains(source, "Path within module: fonts/ui.ttf") || strings.Contains(source, `id="definition-identity-title"`) {
		t.Fatal("resource selection did not show its own Details")
	}
	a.handleEvent(htmlui.Event{ID: groupID, Kind: htmlui.EventClick})
	if source = workspaceShellTest(t, a); strings.Contains(source, `id="`+fontID+`"`) || !strings.Contains(source, `id="element:main.json/start/face"`) {
		t.Fatal("resource collapse hid elements or left resource rows visible")
	}
	a.handleEvent(htmlui.Event{ID: groupID, Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	a.contextScene, a.contextParent, a.contextMod, a.contextResource = "main.json", "start", "button", "ui-font"
	a.action("outliner-delete-resource")
	if _, ok := a.Document.Mods["button"].Spec.Resources["ui-font"]; ok || a.selectedResource != "" || a.Document.Instance("start") == nil {
		t.Fatal("resource deletion did not clear its selection or deleted its module")
	}
	a.action("undo")
	if source = workspaceShellTest(t, a); !strings.Contains(source, `id="`+fontID+`"`) {
		t.Fatal("undo did not restore the resource row")
	}
	a.handleEvent(htmlui.Event{ID: fontID, Kind: htmlui.EventClick})
	a.action("edit.delete")
	if _, ok := a.Document.Mods["button"].Spec.Resources["ui-font"]; ok || a.Document.Instance("start") == nil {
		t.Fatal("Delete key removed the module instead of the selected resource")
	}
}

func TestWorkspaceOutlinerResourceSubmenuTargetsRightClickedModule(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.CreateSceneModule("child", "empty", "start"); err != nil {
		t.Fatal(err)
	}
	file := a.Document.Absolute("empty/font.otf")
	if err := writeWorkspaceFile(file, []byte("font")); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit("main.json", "start")
	a.expandModule(workspaceModuleKey{Scene: "main.json", ID: "child"})
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	point := func(id string) (float64, float64) {
		t.Helper()
		node, ok := htmlui.NodeByID(world, a.shell, id)
		if !ok {
			t.Fatalf("missing control %s", id)
		}
		box, ok := node.Bounds()
		if !ok {
			t.Fatalf("missing bounds for %s", id)
		}
		return float64(box.X + 8), float64(box.Y + box.Height/2)
	}
	x, y := point("tree-group:main.json/child/resources")
	in := input.NewState()
	in.PressMouse(input.MouseRight, x, y)
	a.updateOutlinerMenu(in)
	if !a.contextOpen || a.contextParent != "child" || a.contextMod != "empty" || a.Selected != "start" {
		t.Fatal("resource group context did not retain the clicked module's ownership")
	}
	syncWorkspaceTest(t, a, world)
	x, y = point("outliner-add-resources")
	in = input.NewState()
	in.MoveMouse(x, y)
	a.updateOutlinerMenu(in)
	if !a.resourceMenuOpen {
		t.Fatal("hovering Add Resources did not open the submenu")
	}
	syncWorkspaceTest(t, a, world)
	for _, value := range workspaceResourceTypes {
		if _, ok := htmlui.NodeByID(world, a.shell, "outliner-add-resource:"+value.Kind); !ok {
			t.Fatalf("submenu is missing %s", value.Kind)
		}
	}
	x, y = point("outliner-add-resource:font")
	in = input.NewState()
	in.PressMouse(input.MouseLeft, x, y)
	a.updateOutlinerMenu(in)
	if !a.contextOpen || !a.resourceMenuOpen {
		t.Fatal("clicking the submenu dismissed the parent before dispatch")
	}
	a.openNativeFile = func(options window.FileDialogOptions) (string, error) {
		if options.DefaultPath != filepath.Dir(file) || options.Filters[0].Name != "Font" {
			t.Fatalf("picker targeted the selected module instead of the clicked one: %+v", options)
		}
		return file, nil
	}
	a.handleEvent(htmlui.Event{ID: "outliner-add-resource:font", Kind: htmlui.EventClick})
	if resource := a.Document.Mods["empty"].Spec.Resources["font"]; resource.Kind != "font" || resource.Path != "font.otf" || a.Selected != "child" || a.selectedResource != "font" || a.contextOpen {
		t.Fatalf("submenu add failed: %+v (%s)", resource, a.status)
	}
	if source := workspaceShellTest(t, a); !strings.Contains(source, `id="resource:main.json/child/font"`) {
		t.Fatal("added resource did not appear under its owner")
	}
}

func TestWorkspaceOutlinerAddsAndDeletesSlotsInTheirOwningModule(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.CreateSceneModule("child", "empty", "start"); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit("main.json", "start")
	a.contextScene, a.contextParent, a.contextMod = "main.json", "child", "empty"
	a.handleEvent(htmlui.Event{ID: "outliner-new-slot", Kind: htmlui.EventClick})
	source := workspaceShellTest(t, a)
	if !a.slotCreating || a.Selected != "child" || !strings.Contains(source, `id="slot-create-title"`) || a.selectedMod() != "empty" || a.Local != "button" {
		t.Fatal("Add Slot did not open Details for its owning module")
	}
	a.handleEvent(htmlui.Event{ID: "add/slot/name", Kind: htmlui.EventInput, Value: "count~1"})
	a.handleEvent(htmlui.Event{ID: "add/slot/value", Kind: htmlui.EventInput, Value: `{"type":"int","default":2,"writable":true}`})
	a.handleEvent(htmlui.Event{ID: "add:slot", Kind: htmlui.EventClick})
	source = workspaceShellTest(t, a)
	slotID := "slot:main.json/child/count~1"
	if a.slotCreating || a.selectedSlot != "count~1" || !strings.Contains(source, `id="`+slotID+`"`) || !strings.Contains(source, `id="slot-details-title"`) {
		t.Fatalf("new slot did not appear in the tree and Details: %s", a.status)
	}
	id := workspaceFieldID(modDocumentID("empty"), "/slots/count~01/default", "")
	if _, ok := a.fields[id]; !ok {
		t.Fatal("slot Details targeted the root module or failed to escape its name")
	}
	a.handleEvent(htmlui.Event{ID: id, Kind: htmlui.EventInput, Value: "3"})
	a.action("save")
	if slot := a.Document.Mods["empty"].Spec.Slots["count~1"]; slot.Type != "int" || string(slot.Default) != "3" {
		t.Fatalf("slot creation or editing failed: %+v (%s)", slot, a.status)
	}
	if _, ok := a.Document.Mods["button"].Spec.Slots["count~1"]; ok {
		t.Fatal("creating a slot in a child modified the edited root")
	}
	a.action("edit.delete")
	if len(a.Document.Mods["empty"].Spec.Slots) != 0 || a.Document.Instance("child") == nil || a.selectedSlot != "" {
		t.Fatal("slot deletion removed its module or left the selection behind")
	}
	a.action("undo")
	if source = workspaceShellTest(t, a); !strings.Contains(source, `id="`+slotID+`"`) {
		t.Fatal("undo did not restore the slot")
	}
	a.handleEvent(htmlui.Event{ID: slotID, Kind: htmlui.EventClick})
	a.contextSlot = "count~1"
	a.action("outliner-delete-slot")
	if len(a.Document.Mods["empty"].Spec.Slots) != 0 {
		t.Fatal("context menu deletion did not remove the slot")
	}
}

func TestWorkspaceOutlinerModulesToggleNestedChildrenAndKeepSceneState(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := writeWorkspaceFile(a.Document.Absolute(project.ScenePath("second.json")), []byte(`{"modules":[{"id":"start","mod":"button","children":[{"id":"child","mod":"empty"}]}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.Refresh(); err != nil {
		t.Fatal(err)
	}
	a.Document.Instance("start").Children = []project.ModInstanceSpec{{ID: "child", Mod: "empty", Children: []project.ModInstanceSpec{{ID: "grandchild", Mod: "empty"}}}}
	if source := workspaceShellTest(t, a); len(a.rows) != 4 || strings.Contains(source, `id="select:child"`) || strings.Contains(source, `id="tree-select:second.json/child"`) || len(a.collapsedModules) != 0 {
		t.Fatal("tree did not default to one expanded level without storing per-module state")
	}
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); !strings.Contains(source, `id="select:child"`) || strings.Contains(source, `id="select:grandchild"`) {
		t.Fatal("expanding a module also expanded its descendants")
	}
	a.handleEvent(htmlui.Event{ID: "select:child", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); !strings.Contains(source, `id="select:grandchild"`) {
		t.Fatal("nested module did not expand on first click")
	}
	a.time++
	a.handleEvent(htmlui.Event{ID: "select:child", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); strings.Contains(source, `id="select:grandchild"`) || a.Selected != "child" {
		t.Fatal("clicking a nested parent did not select it and hide its descendants")
	}
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); strings.Contains(source, `id="select:child"`) || len(a.rows) != 4 {
		t.Fatal("collapsed parent still exposed descendants or hid siblings")
	}
	a.time++
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); !strings.Contains(source, `id="select:child"`) || strings.Contains(source, `id="select:grandchild"`) {
		t.Fatal("expanding a parent lost its child's collapsed state")
	}
	a.handleEvent(htmlui.Event{ID: "select:child", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); !strings.Contains(source, `id="select:grandchild"`) {
		t.Fatal("clicking a collapsed nested parent did not expand it")
	}
	a.handleEvent(htmlui.Event{ID: "select:grandchild", Kind: htmlui.EventClick})
	if a.Selected != "grandchild" || len(a.collapsedModules) != 2 {
		t.Fatal("leaf selection created collapse state")
	}
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "scene:second.json", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); strings.Contains(source, `id="select:child"`) {
		t.Fatal("another scene's modules did not start collapsed")
	}
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); !strings.Contains(source, `id="select:child"`) {
		t.Fatal("same instance ID in another scene inherited collapse state")
	}
	a.handleEvent(htmlui.Event{ID: "scene:main.json", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); strings.Contains(source, `id="select:child"`) {
		t.Fatal("scene navigation lost module collapse state")
	}
	a.Selected = "start"
	workspaceShellTest(t, a)
	field := workspaceFieldID(sceneDocumentID(a.Document.Active), "/modules/0/placement/position", "X")
	a.handleEvent(htmlui.Event{ID: field, Kind: htmlui.EventInput, Value: "88"})
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	if a.pendingAction != "select:start" || !a.collapsedModules[workspaceModuleKey{Scene: "main.json", ID: "start"}] {
		t.Fatal("pending draft did not defer module expansion")
	}
	a.action("discard-pending")
	if source := workspaceShellTest(t, a); !strings.Contains(source, `id="select:grandchild"`) || a.pendingAction != "" {
		t.Fatal("deferred module click did not expand descendants")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		a.toggleModule("start")
		a.toggleModule("start")
	}); allocs != 0 {
		t.Fatalf("toggling an existing module allocated %g", allocs)
	}
}

func TestWorkspaceOutlinerContextMenuCreatesSceneAndNestedModule(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	node, ok := htmlui.NodeByID(world, a.shell, "select:start")
	if !ok {
		t.Fatal("missing instance row")
	}
	bounds, ok := node.Bounds()
	if !ok {
		t.Fatal("missing row bounds")
	}
	in := input.NewState()
	in.PressMouse(input.MouseRight, float64(bounds.X+8), float64(bounds.Y+bounds.Height/2))
	a.updateOutlinerMenu(in)
	if !a.contextOpen || a.contextScene != "main.json" || a.contextParent != "start" {
		t.Fatal("right click did not target the module")
	}
	a.handleEvent(htmlui.Event{ID: "outliner-new-module", Kind: htmlui.EventClick})
	syncWorkspaceTest(t, a, world)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("module-create", "module-name"), "newmodule", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("module-create", "instance-name"), "child", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, "module-create-confirm", "", htmlui.EventClick)
	if a.dialog != "" || a.Selected != "child" || len(a.Document.Instance("start").Children) != 1 || a.Document.Mods["newmodule"] == nil {
		t.Fatalf("module creation failed: %s", a.status)
	}
	if err := a.Document.Undo(); err != nil {
		t.Fatal(err)
	}
	if a.Document.Instance("child") != nil || !a.Document.Mods["newmodule"].Deleted {
		t.Fatal("creation was not atomic")
	}
	if err := a.Document.Redo(); err != nil {
		t.Fatal(err)
	}
	a.action("outliner-new-scene")
	syncWorkspaceTest(t, a, world)
	workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("scene-create", "scene-name"), "scene2", htmlui.EventInput)
	workspaceDialogTestEvent(t, a, world, "scene-create-confirm", "", htmlui.EventClick)
	if a.Document.Active != "scene2.json" || a.dialog != "" {
		t.Fatalf("scene creation failed: %s", a.status)
	}
	if err := a.Document.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Document.Absolute(project.ScenePath("scene2.json"))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Document.Absolute("scene2.json")); !os.IsNotExist(err) {
		t.Fatal("scene was saved outside resources/scenes")
	}
	loaded, err := LoadWorkspaceDocument(a.Document.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Scenes["scene2.json"] == nil || loaded.Scenes["main.json"].Spec.Modules[0].Children[0].Mod != "newmodule" {
		t.Fatal("saved hierarchy did not reload")
	}
}

func TestWorkspaceOutlinerDuplicatesRightClickedInstance(t *testing.T) {
	for _, inactiveScene := range []bool{false, true} {
		sceneID := project.SceneID("main.json")
		if inactiveScene {
			sceneID = "extra.json"
		}
		t.Run(string(sceneID), func(t *testing.T) {
			a := workspaceEditorFixture(t)
			if inactiveScene {
				if err := a.Document.CreateScene(sceneID); err != nil {
					t.Fatal(err)
				}
				if err := a.Document.EditScene(sceneID, "Extra scene fixture", func(spec *project.SceneSpec) error {
					spec.Modules = []project.ModInstanceSpec{{ID: "start", Mod: "empty", Children: []project.ModInstanceSpec{{ID: "again", Mod: "empty"}}}}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			} else if err := a.Document.MoveInstance("again", "start", 0); err != nil {
				t.Fatal(err)
			}
			a.selectInstance("other")
			a.expandModule(workspaceModuleKey{Scene: sceneID, ID: "start"})
			world := ecs.NewWorld()
			syncWorkspaceTest(t, a, world)
			control := "select:again"
			if inactiveScene {
				control = "tree-select:extra.json/again"
			}
			node, ok := htmlui.NodeByID(world, a.shell, control)
			if !ok {
				t.Fatal("missing source instance row")
			}
			bounds, ok := node.Bounds()
			if !ok {
				t.Fatal("missing source instance bounds")
			}
			in := input.NewState()
			in.PressMouse(input.MouseRight, float64(bounds.X+8), float64(bounds.Y+bounds.Height/2))
			a.updateOutlinerMenu(in)
			syncWorkspaceTest(t, a, world)
			menu, ok := htmlui.NodeByID(world, a.shell, "outliner-duplicate-module")
			if !ok {
				t.Fatal("missing duplicate context action")
			}
			if box, ok := menu.Bounds(); !ok || box.Height == 0 {
				t.Fatal("duplicate context action is hidden for a module row")
			}
			a.handleEvent(htmlui.Event{ID: "outliner-duplicate-module", Kind: htmlui.EventClick})
			syncWorkspaceTest(t, a, world)
			if a.dialog != "instance-duplicate" || a.contextOpen || a.Selected != "other" {
				t.Fatal("opening duplication changed selection or failed to open the Copy ID dialog")
			}
			history := len(a.Document.undo)
			workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("instance-duplicate", "instance-name"), "start", htmlui.EventInput)
			workspaceDialogTestEvent(t, a, world, "instance-duplicate-confirm", "", htmlui.EventClick)
			if a.dialog != "instance-duplicate" || a.dialogError == "" || len(a.Document.undo) != history {
				t.Fatal("an occupied Copy ID closed the dialog or changed the scene")
			}
			workspaceDialogTestEvent(t, a, world, htmlDialogFieldID("instance-duplicate", "instance-name"), " copied ", htmlui.EventInput)
			workspaceDialogTestEvent(t, a, world, "instance-duplicate-confirm", "", htmlui.EventClick)
			children := a.Document.Instance("start").Children
			if a.dialog != "" || a.Document.Active != sceneID || a.Selected != "copied" || len(children) != 2 || children[0].ID != "again" || children[1].ID != "copied" || children[0].Mod != children[1].Mod || len(a.Document.undo) != history+1 {
				t.Fatalf("duplication did not target the clicked sibling in its scene: %s", a.status)
			}
			a.action("undo")
			if a.Document.Instance("copied") != nil {
				t.Fatal("undo did not remove the copied instance")
			}
			a.action("redo")
			a.contextParent = "copied"
			a.handleEvent(htmlui.Event{ID: "outliner-delete-module", Kind: htmlui.EventClick})
			if a.Document.Instance("copied") != nil || a.Document.Instance("again") == nil || a.Document.Mods[children[0].Mod].Deleted {
				t.Fatal("existing Delete Module action did not remove only the copied instance")
			}
		})
	}
}

func TestWorkspaceOutlinerDoubleClickEntersModuleEditingAndContextEditsElements(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Add elements", func(mod *project.ModSpec) error {
		mod.Elements = []project.ModElementSpec{{ID: "face"}, {ID: "glow", Parent: "face"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Document.CreateScene("extra.json"); err != nil {
		t.Fatal(err)
	}
	workspaceShellTest(t, a)
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	a.time += .2
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	if a.editingModule != (workspaceModuleKey{Scene: "main.json", ID: "start"}) || a.Selected != "start" || a.Local != "button" {
		t.Fatalf("double click did not enter module editing: %s", a.status)
	}
	source := workspaceShellTest(t, a)
	if !strings.Contains(source, `id="element:main.json/start/face"`) || strings.Contains(source, `id="element:main.json/start/glow"`) {
		t.Fatal("definition elements did not default to one visible level")
	}
	if strings.Contains(source, `id="scene:extra.json"`) || strings.Contains(source, `id="select:again"`) || strings.Contains(source, `id="element:main.json/again/face"`) {
		t.Fatal("editing space is not exclusive to the double-clicked module")
	}
	a.time += .1
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); strings.Contains(source, `id="element:main.json/start/face"`) {
		t.Fatal("collapsing the edited module root hid nothing")
	}
	a.time += .1
	a.handleEvent(htmlui.Event{ID: "select:start", Kind: htmlui.EventClick})
	if source := workspaceShellTest(t, a); !strings.Contains(source, `id="element:main.json/start/face"`) || a.Local != "button" {
		t.Fatal("expanding the edited module root or module canvas preview failed")
	}
	a.contextScene, a.contextParent, a.contextMod, a.contextElement = "main.json", "start", "button", ""
	a.handleEvent(htmlui.Event{ID: "outliner-new-element", Kind: htmlui.EventClick})
	if elements := a.modElements("button"); len(elements) != 3 || elements[2].ID != "element1" {
		t.Fatalf("context add element failed: %+v (%s)", elements, a.status)
	}
	a.contextElement = "face"
	a.handleEvent(htmlui.Event{ID: "outliner-delete-element", Kind: htmlui.EventClick})
	if remaining := a.modElements("button"); len(remaining) != 1 || remaining[0].ID != "element1" {
		t.Fatalf("element delete did not prune descendants: %+v (%s)", remaining, a.status)
	}
	a.contextElement, a.contextMod = "", ""
	a.contextParent = "other"
	a.handleEvent(htmlui.Event{ID: "outliner-delete-module", Kind: htmlui.EventClick})
	if a.Document.Instance("other") != nil {
		t.Fatalf("context delete module failed: %s", a.status)
	}
	workspaceShellTest(t, a)
	in := input.NewState()
	in.PressKey(input.KeyEscape)
	a.updateOutlinerMenu(in)
	if a.editingModule != (workspaceModuleKey{}) || a.Local != "" {
		t.Fatal("Escape did not exit module editing")
	}
	if source := workspaceShellTest(t, a); strings.Contains(source, `id="element:main.json/start/face"`) || !strings.Contains(source, `id="scene:extra.json"`) {
		t.Fatal("scene tree did not return to the all-scenes view")
	}
}

func TestWorkspaceOutlinerSceneNavigationDecodesDeferredFileName(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.CreateScene("game ui.json"); err != nil {
		t.Fatal(err)
	}
	a.Selected = "start"
	a.collapsedScenes = map[project.SceneID]bool{"game ui.json": true}
	workspaceShellTest(t, a)
	a.drafts[workspaceFieldID(sceneDocumentID(a.Document.Active), "/modules/0/placement/position", "X")] = "90"
	a.handleEvent(htmlui.Event{ID: "scene:" + url.PathEscape("game ui.json"), Kind: htmlui.EventClick})
	a.action("save-pending")
	if a.Document.Active != "game ui.json" || a.pendingAction != "" || a.collapsedScenes["game ui.json"] {
		t.Fatalf("deferred scene filename was not decoded: %s", a.status)
	}
}

func TestWorkspaceOutlinerNewChildInheritsSpaceAndFailedCreationIsAtomic(t *testing.T) {
	w := workspaceFixture(t)
	if err := w.EditMod("button", "Use UI space", func(spec *project.ModSpec) error {
		spec.Space = project.CoordinateSpaceUI
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.CreateSceneModule("child", "newui", "start"); err != nil {
		t.Fatal(err)
	}
	if w.Mods["newui"].Spec.Space != project.CoordinateSpaceUI {
		t.Fatal("new child did not inherit the parent's space")
	}
	history := len(w.undo)
	if err := w.CreateSceneModule("child", "invalid", "start"); err == nil {
		t.Fatal("duplicate instance was accepted")
	}
	if w.Mods["invalid"] != nil || len(w.undo) != history {
		t.Fatal("failed creation left a definition or undo entry")
	}
	if err := w.CreateSceneModule("reused", "button", "start"); err != nil {
		t.Fatal(err)
	}
	if len(w.Mods) != 3 || w.Instance("reused").Mod != "button" {
		t.Fatal("existing definition was not reused")
	}
}
