package onix

import (
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui/html"
)

func TestIdleFlushesHTMLChangesAfterLayoutBeforeTimer(t *testing.T) {
	for _, name := range []string{"DOM", "component", "surface"} {
		t.Run(name, func(t *testing.T) {
			app := NewApp()
			defer app.Destroy()
			app.SetRenderMode(RenderOnDemand)
			app.SetIdleEnabled(true)
			app.SetTargetFPS(0)
			now := time.Unix(10, 0)
			app.now = func() time.Time { return now }
			if err := InstallSystems[UIHTML](app); err != nil {
				t.Fatal(err)
			}
			page := html.New(`<div id="label">before</div>`)
			page.Width, page.Height = 200, 100
			bundle := ecs.NewBundle(1)
			ecs.Put(&bundle, page)
			owner := app.World.Spawn(bundle)
			var root ecs.Entity
			if name == "component" {
				definition, err := html.Define[struct{}, string](html.NewTemplate(`<div id="label" data-ref="label"></div>`), "", html.Behavior[struct{}, string]{
					Render: func(c html.RenderContext[struct{}, string], view *html.View) { view.Text("label", *c.State) },
				})
				if err != nil {
					t.Fatal(err)
				}
				root, err = html.MountComponent(app.World, owner, definition, struct{}{}, "before", 1)
				if err != nil {
					t.Fatal(err)
				}
			}
			late := false
			if err := app.AddSystem(StagePostUpdate, ecs.SystemConfig{
				Name: "late_html_change", After: []string{"sync_ui_global_transform_after_layout"},
				System: func(*ecs.SystemContext) error {
					if late {
						late = false
						switch name {
						case "component":
							html.UpdateComponentState[struct{}, string](app.World, root, func(value *string) { *value = "after" })
						case "surface":
							ecs.GetMut[html.Surface](app.World, owner).Value = `<div id="label">after</div>`
						default:
							node, _ := html.NodeByID(app.World, owner, "label")
							node.SetText("after")
						}
						app.RequestRender()
					}
					return nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			step := func() bool {
				t.Helper()
				rendered, err := app.Step()
				if err != nil {
					t.Fatal(err)
				}
				now = now.Add(time.Millisecond)
				return rendered
			}
			for range 3 {
				app.RequestRender()
				step()
			}
			deadline := now.Add(5 * time.Second)
			app.WakeAt(deadline)
			late = true
			app.RequestRender()
			step()
			if _, idle := app.idleDeadline(); idle {
				t.Fatal("native event loop may sleep with unpublished HTML changes")
			}
			if !step() {
				t.Fatal("HTML update slept until the unrelated five-second refresh")
			}
			painted := false
			for _, access := range ecs.Query[scene.Text2D](app.World).All {
				painted = painted || access.Get().Value == "after"
			}
			if !painted {
				t.Fatal("follow-up frame did not publish updated text")
			}
			if step() {
				t.Fatal("settled HTML kept the idle app rendering")
			}
			if got, idle := app.idleDeadline(); !idle || got != deadline {
				t.Fatalf("idle=%v deadline=%v, want original timer %v", idle, got, deadline)
			}
		})
	}
}

func TestIdlePublishesFirstFrameSurfaceLifecycle(t *testing.T) {
	for _, change := range []string{"create", "show", "remove", "despawn"} {
		t.Run(change, func(t *testing.T) {
			app := NewApp()
			defer app.Destroy()
			app.SetRenderMode(RenderOnDemand)
			app.SetIdleEnabled(true)
			app.SetTargetFPS(0)
			now := time.Unix(10, 0)
			app.now = func() time.Time { return now }
			if err := InstallSystems[UIHTML](app); err != nil {
				t.Fatal(err)
			}
			page := html.New(`<div>firstframe</div>`)
			page.Width, page.Height = 200, 100
			bundle := ecs.NewBundle(2)
			if change != "create" {
				ecs.Put(&bundle, page)
			}
			ecs.Put(&bundle, scene.Visibility{Visible: change != "show"})
			owner := app.World.Spawn(bundle)
			changed := false
			if err := app.AddSystem(StagePostUpdate, ecs.SystemConfig{
				Name: "first_layout_surface_change", After: []string{"sync_ui_global_transform_after_layout"},
				System: func(*ecs.SystemContext) error {
					if changed {
						return nil
					}
					changed = true
					switch change {
					case "create":
						return ecs.Insert(app.World, owner, page)
					case "show":
						ecs.GetMut[scene.Visibility](app.World, owner).Visible = true
					case "remove":
						ecs.Remove[html.Surface](app.World, owner)
					case "despawn":
						app.World.Despawn(owner)
					}
					return nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			app.WakeAt(now.Add(5 * time.Second))
			if _, err := app.Step(); err != nil {
				t.Fatal(err)
			}
			if _, idle := app.idleDeadline(); idle {
				t.Fatal("first frame suspended before publishing the surface change")
			}
			now = now.Add(time.Millisecond)
			if rendered, err := app.Step(); err != nil || !rendered {
				t.Fatalf("follow-up frame: rendered=%v err=%v", rendered, err)
			}
			texts := 0
			for entity, access := range ecs.Query[scene.Text2D](app.World).All {
				if access.Get().Value == "firstframe" {
					texts++
					if !scene.IsVisible(app.World, entity) {
						t.Fatal("first frame text remained hidden")
					}
				}
			}
			removed := change == "remove" || change == "despawn"
			if removed && texts != 0 || !removed && texts != 1 {
				t.Fatalf("published texts=%d for surface %s", texts, change)
			}
			if _, idle := app.idleDeadline(); !idle {
				t.Fatal("settled surface did not return to idle")
			}
		})
	}
}

func TestIdleFlushesScrollPaintAfterLayout(t *testing.T) {
	for _, test := range []struct {
		name string
		fps  int
	}{{"unlimited", 0}, {"60fps", 60}} {
		t.Run(test.name, func(t *testing.T) {
			fps := test.fps
			app := NewApp()
			defer app.Destroy()
			app.SetRenderMode(RenderOnDemand)
			app.SetIdleEnabled(true)
			app.SetTargetFPS(fps)
			now := time.Unix(10, 0)
			app.now = func() time.Time { return now }
			if err := InstallSystems[UIHTML](app); err != nil {
				t.Fatal(err)
			}
			page := html.New(`<div id="scroll" style="width:180px;height:80px;overflow:auto"><div style="height:1000px"></div><div style="height:20px">tail</div></div>`)
			page.Width, page.Height = 200, 100
			bundle := ecs.NewBundle(1)
			ecs.Put(&bundle, page)
			owner := app.World.Spawn(bundle)
			step := func() bool {
				t.Helper()
				rendered, err := app.Step()
				if err != nil {
					t.Fatal(err)
				}
				return rendered
			}
			step()
			node, _ := html.NodeByID(app.World, owner, "scroll")
			area, ok := node.ScrollContainer()
			if !ok {
				t.Fatal("missing scroll container")
			}
			late := true
			if err := app.AddSystem(StagePostUpdate, ecs.SystemConfig{
				Name: "late_scroll", After: []string{"sync_ui_global_transform_after_layout"},
				System: func(*ecs.SystemContext) error {
					if late {
						late = false
						ecs.GetMut[scene.UIScroll](app.World, area).Position[1] = -940
						app.RequestRender()
					}
					return nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			deadline := now.Add(5 * time.Second)
			app.WakeAt(deadline)
			now = now.Add(20 * time.Millisecond)
			step()
			if _, idle := app.idleDeadline(); idle {
				t.Fatal("native loop may sleep before mounting newly visible scroll content")
			}
			if fps != 0 {
				now = now.Add(time.Millisecond)
				if step() {
					t.Fatal("follow-up frame bypassed the FPS limit")
				}
				if _, idle := app.idleDeadline(); idle {
					t.Fatal("throttled follow-up frame lost its render request")
				}
			}
			now = now.Add(20 * time.Millisecond)
			if !step() {
				t.Fatal("scroll paint waited for another input event")
			}
			painted := false
			for _, text := range ecs.Query[scene.Text2D](app.World).All {
				painted = painted || text.Ref().Value == "tail"
			}
			if !painted {
				t.Fatal("newly visible text remained unmounted")
			}
			if got, idle := app.idleDeadline(); !idle || got != deadline {
				t.Fatalf("settled scroll did not return to original deadline: idle=%v deadline=%v", idle, got)
			}
		})
	}
}
