package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/input"
	"github.com/onix-engine/onix/project"
	htmlui "github.com/onix-engine/onix/ui/html"
)

func TestWorkspaceOutlinerDeletesRightClickedElementAfterScroll(t *testing.T) {
	for _, offset := range []float32{0, -560} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			a := workspaceEditorFixture(t)
			if err := a.Document.EditMod("button", "Tree fixture", func(mod *project.ModSpec) error {
				mod.Elements = nil
				for i := range 48 {
					mod.Elements = append(mod.Elements, project.ModElementSpec{ID: fmt.Sprintf("node%d", i)})
				}
				mod.Elements = append(mod.Elements, project.ModElementSpec{ID: "child", Parent: "node22"})
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			a.enterModuleEdit(a.Document.Active, "start")
			a.selectElement("start", "node0")
			world := ecs.NewWorld()
			syncWorkspaceTest(t, a, world)
			scroll := a.outlinerScroll(world)
			if scroll == nil {
				t.Fatal("missing tree scroll container")
			}
			scroll.Position[1] = offset
			target := "node2"
			if offset != 0 {
				target = "node22"
			}
			node, ok := htmlui.NodeByID(world, a.shell, "element:main.json/start/"+target)
			if !ok {
				t.Fatal("missing target row")
			}
			box, ok := node.Bounds()
			if !ok {
				t.Fatal("missing target bounds")
			}
			in := input.NewState()
			in.PressMouse(input.MouseRight, float64(box.X+12), float64(box.Y+box.Height/2+offset))
			a.updateOutlinerMenu(in)
			if !a.contextOpen || a.contextElement != target || a.selectedElement != "node0" {
				t.Fatalf("right click targeted %q, want %q (selection %q)", a.contextElement, target, a.selectedElement)
			}
			syncWorkspaceTest(t, a, world)
			if scroll := a.outlinerScroll(world); scroll == nil || scroll.Position[1] != offset {
				t.Fatal("opening the context menu lost the tree scroll position")
			}
			a.handleEvent(htmlui.Event{ID: "outliner-delete-element", Kind: htmlui.EventClick})
			for _, element := range a.modElements("button") {
				if element.ID == target || target == "node22" && element.ID == "child" {
					t.Fatalf("deleted subtree still contains %q", element.ID)
				}
			}
			want := 48
			if target == "node22" {
				want--
			}
			if got := len(a.modElements("button")); got != want {
				t.Fatalf("deletion changed unrelated nodes: got %d elements, want %d", got, want)
			}
			a.action("undo")
			if len(a.modElements("button")) != 49 {
				t.Fatal("undo did not restore the subtree")
			}
		})
	}
}

func TestWorkspaceOutlinerElementsExpandOneLevelAndKeepInstanceState(t *testing.T) {
	a := workspaceEditorFixture(t)
	if err := a.Document.EditMod("button", "Nested elements", func(mod *project.ModSpec) error {
		// Siblings need not be contiguous in the module definition.
		mod.Elements = []project.ModElementSpec{
			{ID: "root"}, {ID: "child", Parent: "root"}, {ID: "leaf", Parent: "child"},
			{ID: "other"}, {ID: "sibling", Parent: "root"},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit(a.Document.Active, "start")
	revision, history := a.Document.Revision, len(a.Document.undo)
	attributes := func(source, id string) string {
		t.Helper()
		position := strings.Index(source, `id="element:main.json/start/`+id+`"`)
		if position < 0 {
			t.Fatalf("element %s is hidden", id)
		}
		start := strings.LastIndex(source[:position], "<button")
		end := position + strings.Index(source[position:], ">")
		return source[start:end]
	}
	check := func(expanded string, visible, hidden []string) {
		t.Helper()
		source := workspaceShellTest(t, a)
		for _, id := range visible {
			if !strings.Contains(source, `id="element:main.json/start/`+id+`"`) {
				t.Fatalf("element %s is hidden", id)
			}
		}
		for _, id := range hidden {
			if strings.Contains(source, `id="element:main.json/start/`+id+`"`) {
				t.Fatalf("element %s is unexpectedly visible", id)
			}
		}
		root := attributes(source, "root")
		if !strings.Contains(root, `aria-expanded="`+expanded+`"`) {
			t.Fatal("root expansion state is missing or wrong", root)
		}
	}
	click := func(id string) {
		a.time++ // Each call is a single click, outside the double-click window.
		a.handleEvent(htmlui.Event{ID: "element:main.json/start/" + id, Kind: htmlui.EventClick})
	}
	check("false", []string{"root", "other"}, []string{"child", "leaf", "sibling"})
	if len(a.expandedElements) != 0 {
		t.Fatal("default rendering stored expansion state for every element")
	}
	click("root")
	check("true", []string{"root", "child", "sibling", "other"}, []string{"leaf"})
	click("child")
	check("true", []string{"leaf"}, nil)
	click("root")
	check("false", []string{"root", "other"}, []string{"child", "leaf", "sibling"})
	click("root")
	check("true", []string{"child", "leaf", "sibling"}, nil)
	click("leaf")
	source := workspaceShellTest(t, a)
	leaf := attributes(source, "leaf")
	if strings.Contains(leaf, "aria-expanded") || len(a.expandedElements) != 2 || a.selectedElement != "leaf" {
		t.Fatal("leaf selection created expansion state or lost selection")
	}
	if strings.Index(source, `id="element:main.json/start/child"`) > strings.Index(source, `id="element:main.json/start/sibling"`) {
		t.Fatal("sibling order changed")
	}
	if a.Document.Revision != revision || len(a.Document.undo) != history {
		t.Fatal("expansion changed authored content")
	}
	a.enterModuleEdit(a.Document.Active, "again")
	source = workspaceShellTest(t, a)
	if !strings.Contains(source, `id="element:main.json/again/root"`) || strings.Contains(source, `id="element:main.json/again/child"`) {
		t.Fatal("another instance inherited expansion state")
	}
	a.enterModuleEdit(a.Document.Active, "start")
	check("true", []string{"leaf"}, nil)
}

func TestWorkspaceOutlinerElementTreeIndexReusesMemory(t *testing.T) {
	elements := []project.ModElementSpec{{ID: "child", Parent: "root"}, {ID: "root"}, {ID: "other"}, {ID: "leaf", Parent: "child"}}
	var tree workspaceElementTree
	tree.reset(elements)
	if allocations := testing.AllocsPerRun(100, func() { tree.reset(elements) }); allocations != 0 {
		t.Fatalf("rebuilding the element tree index allocated %g", allocations)
	}
}

func TestWorkspaceOutlinerParentElementDoubleClickOpensAnimation(t *testing.T) {
	a := animationEditorFixture(t)
	if err := a.Document.EditMod("button", "Add child", func(mod *project.ModSpec) error {
		mod.Elements = append(mod.Elements, project.ModElementSpec{ID: "child", Parent: "face"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.enterModuleEdit(a.Document.Active, "start")
	action := "element:main.json/start/face"
	a.time = 1
	a.handleEvent(htmlui.Event{ID: action, Kind: htmlui.EventClick})
	a.time = 1.2
	a.handleEvent(htmlui.Event{ID: action, Kind: htmlui.EventClick})
	if !a.animation.Open || !strings.Contains(workspaceShellTest(t, a), `id="element:main.json/start/child"`) {
		t.Fatal("double click did not open Animation or collapsed the parent")
	}
}
