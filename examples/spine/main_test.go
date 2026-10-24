package main

import (
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/diiyw/onix"
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

var exportNames = [...]string{"coin", "goblins", "raptor", "spineboy", "stretchyman", "stretchyman-ik", "tank", "vine"}

type spineFixture struct {
	app     *onix.App
	runtime *module.Runtime
	entry   *module.Scene
	shell   ecs.Entity
	items   [8]ecs.Entity
}

func loadSpine(t *testing.T) spineFixture {
	t.Helper()
	plan, err := project.CompileProjectDirectory(".")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := OnixRegistry()
	if err != nil {
		t.Fatal(err)
	}
	app := onix.NewApp()
	t.Cleanup(app.Destroy)
	entry, err := app.LoadModules(plan, registry, OnixModuleOptions(os.DirFS(".")))
	if err != nil {
		t.Fatal(err)
	}
	f := spineFixture{app: app, runtime: app.Modules(), entry: entry}
	for entity, access := range ecs.Query[scene.Name](app.World).All {
		for index, name := range exportNames {
			if access.Get().Value == name {
				f.items[index] = entity
			}
		}
	}
	for entity := range ecs.Query[htmlui.Surface](app.World).All {
		f.shell = entity
	}
	if f.shell == 0 {
		t.Fatal("module selectors missing")
	}
	return f
}

func (f spineFixture) dispatch(t *testing.T) {
	t.Helper()
	for range 3 {
		if err := f.runtime.Dispatch(); err != nil {
			t.Fatal(err)
		}
	}
}

func (f spineFixture) selectIndex(t *testing.T, id string, index int) {
	t.Helper()
	if err := f.runtime.HandleHTMLEvent(htmlui.Event{Entity: f.shell, ID: id, Kind: htmlui.EventChange, Value: strconv.Itoa(index)}); err != nil {
		t.Fatal(err)
	}
	f.dispatch(t)
}

func (f spineFixture) assertSelected(t *testing.T, selected int) {
	t.Helper()
	for index, entity := range f.items {
		visible, ok := ecs.Get[scene.Visibility](f.app.World, entity)
		if !ok || visible.Visible != (index == selected) {
			t.Fatalf("export %s visibility = %v", exportNames[index], visible.Visible)
		}
	}
	component, _ := ecs.Get[scene.Spine2D](f.app.World, f.items[selected])
	minX, minY, maxX, maxY, valid := component.Player.AnimationBounds()
	transform, _ := ecs.Get[scene.UITransform](f.app.World, f.items[selected])
	x := (minX+maxX)*transform.Scale.X()/2 + transform.Translation.X.Value
	y := -(minY+maxY)*transform.Scale.Y()/2 + transform.Translation.Y.Value
	if !valid || math.Abs(float64(x)) > 1e-4 || math.Abs(float64(y)) > 1e-4 || (maxX-minX)*transform.Scale.X() > 900.001 || (maxY-minY)*transform.Scale.Y() > 600.001 {
		t.Fatalf("export %s does not fit in module local coordinates: center=(%v,%v)", exportNames[selected], x, y)
	}
}

func TestSpineModuleLoadsAndFitsAllExportsAndAnimations(t *testing.T) {
	f := loadSpine(t)
	for index, name := range exportNames {
		t.Run(name, func(t *testing.T) {
			f.selectIndex(t, "resource", index)
			component, ok := ecs.Get[scene.Spine2D](f.app.World, f.items[index])
			if !ok || component.Player == nil {
				t.Fatal("private player missing")
			}
			names := component.Player.Resource.Animations()
			if len(names) == 0 {
				t.Fatal("export has no animations")
			}
			for animation, name := range names {
				f.selectIndex(t, "animation", animation)
				if component.Player.Animation != name {
					t.Fatalf("animation = %q, want %q", component.Player.Animation, name)
				}
				f.assertSelected(t, index)
				component.Player.Update(.25)
				if len(component.Player.MeshBatch(mgl32.Ident4(), mgl32.Ident4()).Draws) == 0 {
					t.Fatal("animation produced no mesh")
				}
			}
			if err := ui.SyncHTML(f.app.World); err != nil {
				t.Fatal(err)
			}
			control, ok := htmlui.ControlByID(f.app.World, f.shell, "animation")
			if !ok {
				t.Fatal("animation selector did not render")
			}
			value, _ := ecs.Get[htmlui.Control](f.app.World, control)
			selector, ok := ecs.Get[ui.Selector](f.app.World, control)
			if !ok || selector.Selected != value.Selected {
				t.Fatal("HTML selection and UI selector diverged", selector.Selected, value.Selected)
			}
			if len(value.Options) != len(names) {
				t.Fatalf("selector retained old resource options: got %d, want %d", len(value.Options), len(names))
			}
			for option, value := range value.Options {
				if option < len(names) && (value.Label != names[option] || value.Disabled) {
					t.Fatalf("option %d: %v", option, value)
				}
				if option >= len(names) && !value.Disabled {
					t.Fatalf("old resource option %d remained enabled", option)
				}
			}
		})
	}
}

func TestSpineSelectorEventsAreBufferedAndWarmSelectionDoesNotAllocate(t *testing.T) {
	f := loadSpine(t)
	for index := range exportNames {
		f.selectIndex(t, "resource", index)
		component, _ := ecs.Get[scene.Spine2D](f.app.World, f.items[index])
		for animation := range component.Player.Resource.Animations() {
			f.selectIndex(t, "animation", animation)
		}
	}
	f.selectIndex(t, "resource", 0)
	if err := f.runtime.HandleHTMLEvent(htmlui.Event{Entity: f.shell, ID: "resource", Kind: htmlui.EventChange, Value: "2"}); err != nil {
		t.Fatal(err)
	}
	f.assertSelected(t, 0)
	f.dispatch(t)
	f.assertSelected(t, 2)
	step := func() {
		for _, index := range []string{"0", "2"} {
			if err := f.runtime.HandleHTMLEvent(htmlui.Event{Entity: f.shell, ID: "resource", Kind: htmlui.EventChange, Value: index}); err != nil {
				panic(err)
			}
			for range 3 {
				if err := f.runtime.Dispatch(); err != nil {
					panic(err)
				}
			}
		}
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("warm Spine selection chain allocated %g", allocations)
	}
	if err := f.runtime.HandleInput(scene.InputEvent{Kind: scene.InputEventKeyDown, Key: input.KeyDown}); err != nil {
		t.Fatal(err)
	}
	f.dispatch(t)
	f.assertSelected(t, 3)
	if err := ui.SyncHTML(f.app.World); err != nil {
		t.Fatal(err)
	}
	control, ok := htmlui.ControlByID(f.app.World, f.shell, "resource")
	selector, _ := ecs.Get[ui.Selector](f.app.World, control)
	if !ok || selector.Selected != 3 {
		t.Fatal("keyboard selection did not refresh the UI selector", selector.Selected)
	}
}

func TestSpineReusableInstancesKeepPlayersAndSlotsIndependent(t *testing.T) {
	f := loadSpine(t)
	second, err := f.runtime.MountEntry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Dispose() })
	f.dispatch(t)
	var other ecs.Entity
	for entity, name := range ecs.Query[scene.Name](f.app.World).All {
		if name.Get().Value == "coin" && entity != f.items[0] {
			other = entity
		}
	}
	firstPlayer, _ := ecs.Get[scene.Spine2D](f.app.World, f.items[0])
	otherPlayer, _ := ecs.Get[scene.Spine2D](f.app.World, other)
	if other == 0 || firstPlayer.Player == otherPlayer.Player {
		t.Fatal("reused definition shared player state")
	}
	resource, err := f.entry.SlotWriter("gallery", "resource")
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Write(project.SlotValue{Type: project.IntValue, Integer: 2}); err != nil {
		t.Fatal(err)
	}
	f.dispatch(t)
	f.assertSelected(t, 2)
	visible, _ := ecs.Get[scene.Visibility](f.app.World, other)
	if !visible.Visible {
		t.Fatal("slot update changed another instance")
	}
	f.selectIndex(t, "animation", 1)
	if err := resource.Write(project.SlotValue{Type: project.IntValue, Integer: 0}); err != nil {
		t.Fatal(err)
	}
	f.dispatch(t)
	f.assertSelected(t, 0)
}
