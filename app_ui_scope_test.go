package onix

import (
	"testing"

	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui/html"
)

func TestAppNewHTMLDescendantsApplyScrollInSameFrame(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	if err := app.InstallSystems[UIHTML](); err != nil {
		t.Fatal(err)
	}
	if _, err := app.LoadModules[module.FullContent](appModulePlan(t, `{}`), module.Registry{}, module.Options{}); err != nil {
		t.Fatal(err)
	}
	surface := html.New(`<div id="scroll" style="height:80px;overflow:auto"><div style="height:120px"></div><button id="child" hidden style="height:30px;overflow:hidden">child</button><div style="height:300px"></div></div>`)
	surface.Width, surface.Height = 200, 100
	spawner := app.World.NewSpawner1[html.Surface]()
	owner := spawner.Spawn(surface)
	sync := func() {
		t.Helper()
		if err := app.runStages([]Stage{StagePostUpdate}); err != nil {
			t.Fatal(err)
		}
	}
	sync()
	node, _ := html.NodeByID(app.World, owner, "scroll")
	area, ok := node.ScrollContainer()
	if !ok {
		t.Fatal("missing scroll container")
	}
	app.World.GetMut[scene.UIScroll](area).Position[1] = -100
	sync()
	child, _ := html.NodeByID(app.World, owner, "child")
	child.SetHidden(false)
	sync()
	bounds, ok := child.Bounds()
	if !ok {
		t.Fatal("new child has no layout")
	}
	found := false
	for entity, access := range app.World.Query[scene.Text2D]().All {
		if access.Ref().Value != "child" {
			continue
		}
		found = true
		transform := app.World.GetRef[scene.UIGlobalTransform](entity)
		y := bounds.Y + app.World.GetRef[scene.UIScroll](area).Position.Y()
		if transform == nil || transform.Matrix[13] < y || transform.Matrix[13] >= y+bounds.Height {
			t.Fatalf("new child did not inherit scroll in the layout frame: bounds=%+v scrollY=%v transform=%+v", bounds, y-bounds.Y, transform)
		}
	}
	if !found {
		t.Fatal("new child was excluded from the layout frame's query scope")
	}
}
