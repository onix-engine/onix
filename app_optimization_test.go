package onix

import (
	"slices"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestUpdateTransformIsPropagatedBeforeRender(t *testing.T) {
	app := NewApp()
	if err := InstallSystems[Transforms](app); err != nil {
		t.Fatal(err)
	}
	spawner := ecs.NewSpawner1[scene.Transform](app.World)
	entity := spawner.Spawn(scene.NewTransform())
	if err := app.AddSystem(StageUpdate, ecs.SystemConfig{Name: "move", System: func(context *ecs.SystemContext) error {
		pose := ecs.GetMut[scene.Transform](context.World, entity)
		pose.Translation[0] = 42
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := app.AddSystem(StageRender, ecs.SystemConfig{Name: "observe", After: []string{"sync_render_transform"}, System: func(context *ecs.SystemContext) error {
		pose, ok := ecs.Get[scene.GlobalTransform](context.World, entity)
		if !ok || pose.Matrix[12] != 42 {
			t.Fatalf("render observed stale transform: %+v", pose)
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Step(); err != nil {
		t.Fatal(err)
	}
}

func TestPhysicsSystemSetsShareTransformPropagation(t *testing.T) {
	app := NewApp()
	if err := InstallSystems[Physics2D](app); err != nil {
		t.Fatal(err)
	}
	if err := InstallSystems[Physics3D](app); err != nil {
		t.Fatal(err)
	}
	order, err := app.Schedule(StageFixedPostUpdate).Order()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(order, []string{"sync_physics_transform"}) {
		t.Fatalf("duplicate propagation: %v", order)
	}
}

func TestLateUITransformIsPropagatedBeforeExtraction(t *testing.T) {
	app := NewApp()
	if err := InstallSystems[UIWidgets](app); err != nil {
		t.Fatal(err)
	}
	if err := InstallSystems[Render2D](app); err != nil {
		t.Fatal(err)
	}
	spawner := ecs.NewSpawner1[scene.UITransform](app.World)
	entity := spawner.Spawn(scene.NewUITransform())
	if err := app.AddSystem(StagePostUpdate, ecs.SystemConfig{Name: "late_ui_move", After: []string{"sync_ui_global_transform_after_layout"}, System: func(context *ecs.SystemContext) error {
		pose := ecs.GetMut[scene.UITransform](context.World, entity)
		pose.Translation.X = scene.Px(42)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := app.AddSystem(StageRender, ecs.SystemConfig{Name: "observe_ui", After: []string{"sync_render_ui_transform"}, Before: []string{"extract_cameras"}, System: func(context *ecs.SystemContext) error {
		pose, ok := ecs.Get[scene.UIGlobalTransform](context.World, entity)
		if !ok || pose.Matrix[12] != 42 {
			t.Fatalf("render observed stale UI transform: %+v", pose)
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Step(); err != nil {
		t.Fatal(err)
	}
}

func TestRenderSynchronizationOrderAcrossInstallationOrders(t *testing.T) {
	for _, rendererFirst := range []bool{false, true} {
		app := NewApp()
		if rendererFirst {
			if err := InstallSystems[Render2D](app); err != nil {
				t.Fatal(err)
			}
		}
		if err := InstallSystems[UIWidgets](app); err != nil {
			t.Fatal(err)
		}
		if !rendererFirst {
			if err := InstallSystems[Render2D](app); err != nil {
				t.Fatal(err)
			}
		}
		order, err := app.Schedule(StageRender).Order()
		if err != nil {
			t.Fatal(err)
		}
		world, ui, cameras := slices.Index(order, "sync_render_transform"), slices.Index(order, "sync_render_ui_transform"), slices.Index(order, "extract_cameras")
		if world < 0 || ui <= world || cameras <= ui {
			t.Fatalf("renderer first=%v, order=%v", rendererFirst, order)
		}
	}
}
