package onix

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui/html"
)

func TestAppNewHTMLDescendantsApplyScrollInSameFrame(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	if err := InstallSystems[UIHTML](app); err != nil {
		t.Fatal(err)
	}
	if _, err := app.LoadModules(appModulePlan(t, `{}`), module.Registry{}, module.Options{}); err != nil {
		t.Fatal(err)
	}
	surface := html.New(`<div id="scroll" style="height:80px;overflow:auto"><div style="height:120px"></div><button id="child" hidden style="height:30px;overflow:hidden">child</button><div style="height:300px"></div></div>`)
	surface.Width, surface.Height = 200, 100
	spawner := ecs.NewSpawner1[html.Surface](app.World)
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
	ecs.GetMut[scene.UIScroll](app.World, area).Position[1] = -100
	sync()
	child, _ := html.NodeByID(app.World, owner, "child")
	child.SetHidden(false)
	sync()
	bounds, ok := child.Bounds()
	if !ok {
		t.Fatal("new child has no layout")
	}
	found := false
	for entity, access := range ecs.Query[scene.Text2D](app.World).All {
		if access.Ref().Value != "child" {
			continue
		}
		found = true
		transform := ecs.GetRef[scene.UIGlobalTransform](app.World, entity)
		y := bounds.Y + ecs.GetRef[scene.UIScroll](app.World, area).Position.Y()
		if transform == nil || transform.Matrix[13] < y || transform.Matrix[13] >= y+bounds.Height {
			t.Fatalf("new child did not inherit scroll in the layout frame: bounds=%+v scrollY=%v transform=%+v", bounds, y-bounds.Y, transform)
		}
	}
	if !found {
		t.Fatal("new child was excluded from the layout frame's query scope")
	}
}
