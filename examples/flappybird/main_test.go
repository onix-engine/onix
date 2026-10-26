package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/diiyw/onix"
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

type flappyFixture struct {
	app          *onix.App
	entry        *module.Scene
	runtime      *module.Runtime
	target       ecs.Entity
	resultTarget ecs.Entity
	result       *module.Scene
	elements     map[string]ecs.Entity
}

func loadFlappy(t *testing.T) flappyFixture {
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
	if err := OnixInstallSystems(app); err != nil {
		t.Fatal(err)
	}
	entry, err := app.LoadModules(plan, registry, OnixModuleOptions(os.DirFS(".")))
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.Modules().Manager().Preload(module.ScenePreloadRequest{Scene: "result.json"})
	if err != nil {
		t.Fatal(err)
	}
	f := flappyFixture{result: result, app: app, entry: entry, runtime: app.Modules(), elements: make(map[string]ecs.Entity)}
	previousScope := app.World.SetQueryScope(nil)
	for entity, name := range ecs.Query[scene.Name](app.World).All {
		f.elements[name.Get().Value] = entity
	}
	for entity, target := range ecs.Query[scene.InputTarget2D](app.World).All {
		if target.Get().Width == 750 {
			if scene.LayerForEntity(app.World, entity).ID == result.Generation() {
				f.resultTarget = entity
			} else {
				f.target = entity
			}
		}
	}
	app.World.SetQueryScope(previousScope)
	if f.elements["bird"] == 0 || f.target == 0 {
		t.Fatal("owned content or input target missing")
	}
	if err := scene.SyncUIGlobalTransformSystem("").Run(app.World); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f flappyFixture) dispatch(t *testing.T) {
	t.Helper()
	for range 2 {
		if err := f.runtime.Dispatch(); err != nil {
			t.Fatal(err)
		}
	}
}

func (f flappyFixture) update(t *testing.T, steps int) {
	t.Helper()
	for range steps {
		if err := f.runtime.Update(1.0 / 60); err != nil {
			t.Fatal(err)
		}
		if err := f.runtime.Dispatch(); err != nil {
			t.Fatal(err)
		}
	}
}

func (f flappyFixture) send(t *testing.T, event scene.InputEvent) {
	t.Helper()
	state, _ := ecs.GetResource[*input.State](f.app.World)
	switch event.Kind {
	case scene.InputEventPress:
		state.PressMouse(input.MouseLeft, 0, 0)
	case scene.InputEventRelease:
		state.ReleaseMouse(input.MouseLeft, 0, 0)
	case scene.InputEventKeyDown:
		state.PressKey(event.Key)
	case scene.InputEventKeyUp:
		state.ReleaseKey(event.Key)
	}
	if event.Entity == f.target && f.runtime.Manager().Active() == f.result {
		event.Entity = f.resultTarget
	}
	if err := f.runtime.HandleInput(event); err != nil {
		t.Fatal(err)
	}
}

func (f flappyFixture) visible(name string) bool {
	value, _ := ecs.Get[scene.Visibility](f.app.World, f.elements[name])
	return value.Visible && scene.IsVisible(f.app.World, f.elements[name])
}

func (f flappyFixture) position(name string) [2]float32 {
	value, _ := ecs.Get[scene.UITransform](f.app.World, f.elements[name])
	return [2]float32{value.Translation.X.Value, value.Translation.Y.Value}
}

func TestFlappyModuleInputAppliesOnceAcrossFixedTicks(t *testing.T) {
	for _, device := range []string{"mouse", "touch", "space"} {
		for _, steps := range []int{0, 1, 2, 5} {
			t.Run(fmt.Sprintf("%s/%d_steps", device, steps), func(t *testing.T) {
				f := loadFlappy(t)
				press, release := scene.InputEvent{Entity: f.target, Kind: scene.InputEventPress}, scene.InputEvent{Entity: f.target, Kind: scene.InputEventRelease}
				if device == "touch" {
					press.Device, release.Device = input.PointerDeviceTouch, input.PointerDeviceTouch
				}
				if device == "space" {
					press, release = scene.InputEvent{Kind: scene.InputEventKeyDown, Key: input.KeySpace}, scene.InputEvent{Kind: scene.InputEventKeyUp, Key: input.KeySpace}
				}
				f.send(t, press)
				if !f.visible("getReady") {
					t.Fatal("action delivered recursively")
				}
				f.dispatch(t)
				f.update(t, steps)
				if f.visible("getReady") || f.visible("gameOver") {
					t.Fatal("press did not start game")
				}
				f.update(t, 250)
				if !f.visible("gameOver") {
					t.Fatal("ground collision did not end game")
				}
				f.send(t, press)
				f.dispatch(t)
				if !f.visible("gameOver") {
					t.Fatal("held input restarted game")
				}
				f.send(t, release)
				f.send(t, press)
				f.dispatch(t)
				f.update(t, steps)
				if !f.visible("getReady") || f.visible("gameOver") || f.position("bird") != [2]float32{375, 470} {
					t.Fatal("restart must only reset game", f.position("bird"))
				}
				f.send(t, press)
				f.dispatch(t)
				if !f.visible("getReady") {
					t.Fatal("held restart input started game")
				}
				f.send(t, release)
				f.send(t, press)
				f.dispatch(t)
				if f.visible("getReady") {
					t.Fatal("new press did not start game")
				}
			})
		}
	}
}

func TestFlappyBirdStartSlotAndRepeatedInstancesAreIndependent(t *testing.T) {
	f := loadFlappy(t)
	second, err := f.runtime.MountEntry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Dispose() })
	f.dispatch(t)
	var secondBird ecs.Entity
	for entity, name := range ecs.Query[scene.Name](f.app.World).All {
		if name.Get().Value == "bird" && entity != f.elements["bird"] {
			secondBird = entity
		}
	}
	writer, err := f.entry.SlotWriter("simulation", "birdStart")
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(project.SlotValue{Type: project.Vec2Value, Vector: [4]float32{410, 590}}); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		f.dispatch(t)
	}
	if f.position("bird") != [2]float32{410, 590} {
		t.Fatal("slot did not reset owned simulation", f.position("bird"))
	}
	other, _ := ecs.Get[scene.UITransform](f.app.World, secondBird)
	if secondBird == 0 || other.Translation.X.Value != 375 || other.Translation.Y.Value != 470 {
		t.Fatal("slot leaked into another scene instance")
	}
}

func TestFlappyWarmSimulationAndPresentationChainDoesNotAllocate(t *testing.T) {
	f := loadFlappy(t)
	press := scene.InputEvent{Entity: f.target, Kind: scene.InputEventPress}
	release := scene.InputEvent{Entity: f.target, Kind: scene.InputEventRelease}
	step := func() {
		for _, event := range [...]scene.InputEvent{release, press} {
			if err := f.runtime.HandleInput(event); err != nil {
				panic(err)
			}
		}
		if err := f.runtime.Dispatch(); err != nil {
			panic(err)
		}
		if err := f.runtime.Update(1.0 / 60000); err != nil {
			panic(err)
		}
		if err := f.runtime.Dispatch(); err != nil {
			panic(err)
		}
	}
	for range 4 {
		step()
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("warm input/physics/frame route allocated %g", allocations)
	}
}

func TestFlappyBirdAnimationAndStableHUD(t *testing.T) {
	f := loadFlappy(t)
	animator, ok := ecs.Get[scene.Animator](f.app.World, f.elements["bird"])
	if !ok || !animator.Playing || animator.Clip == nil {
		t.Fatal("bird animation was not installed")
	}
	versions := [2]uint64{ecs.ComponentVersion[scene.Text2D](f.app.World), ecs.ComponentVersion[scene.Visibility](f.app.World)}
	f.update(t, 5)
	if versions != [2]uint64{ecs.ComponentVersion[scene.Text2D](f.app.World), ecs.ComponentVersion[scene.Visibility](f.app.World)} {
		t.Fatal("unchanged HUD marked components changed")
	}
	for _, name := range []string{"title", "status", "scoreLabel"} {
		text, ok := ecs.Get[scene.Text2D](f.app.World, f.elements[name])
		if !ok || text.Font == nil {
			t.Fatal("private HUD font missing", name)
		}
	}
}
