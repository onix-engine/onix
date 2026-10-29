package module

import (
	"testing"

	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

func TestSceneExitPolicyControlsDepartingScene(t *testing.T) {
	for _, test := range []struct {
		name     string
		policy   project.SceneExitPolicy
		retain   bool
		activate bool
		overlay  bool
		kept     bool
	}{
		{name: "keep", policy: project.SceneExitRetain, kept: true},
		{name: "destroy overrides request", policy: project.SceneExitDestroy, retain: true},
		{name: "default destroys"},
		{name: "default follows request", retain: true, kept: true},
		{name: "activate keeps", policy: project.SceneExitRetain, activate: true, kept: true},
		{name: "activate destroys", policy: project.SceneExitDestroy, retain: true, activate: true},
		{name: "overlay keeps background", policy: project.SceneExitDestroy, overlay: true, kept: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, registry, p := managerPlan(t)
			plan.Scenes[plan.Entry].Spec.ExitPolicy = test.policy
			r := newFixture(t, plan, registry, Options{})
			main, err := r.Manager().StartEntry()
			must(t, err)
			writer, err := main.SlotWriter("button", "text")
			must(t, err)
			must(t, writer.Write(project.SlotValue{Type: project.StringValue, String: "saved state"}))
			must(t, r.Dispatch())
			if test.activate {
				ready, err := r.Manager().Preload(ScenePreloadRequest{Scene: "game.json"})
				must(t, err)
				must(t, r.Manager().Activate(ready, SceneActivationOptions{Retain: test.retain}))
			} else {
				must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "game.json", Retain: test.retain, Overlay: test.overlay}))
			}
			must(t, r.Dispatch())
			if main.Alive() != test.kept || r.Manager().Active().ID() != "game.json" {
				t.Fatal("exit policy did not apply to the departing scene")
			}
			if test.kept && (main.State() != SceneSuspended || scene.IsVisible(r.world, p.buttons[0].root) != test.overlay) {
				t.Fatal("retained scene was not suspended with the expected visibility")
			}
			must(t, r.Update(1.0/60))
			if p.mainUpdates != 0 || p.gameUpdates != 1 {
				t.Fatal("departed scene continued updating")
			}
			must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "main.json"}))
			must(t, r.Dispatch())
			current := r.Manager().Active()
			if (current == main) != test.kept || current.State() != SceneActive {
				t.Fatal("return did not restore or recreate the scene as configured")
			}
			button := &current.modules[current.instances["button"]]
			text, err := button.Slot("text")
			must(t, err)
			value, err := text.Read[string]()
			must(t, err)
			if (value == "saved state") != test.kept {
				t.Fatal("return did not preserve or reset scene state as configured")
			}
		})
	}
}

func TestSceneExitPolicyRetainedSwitchesAllocateNothingWarm(t *testing.T) {
	plan, registry, _ := managerPlan(t)
	for i := range plan.Scenes {
		plan.Scenes[i].Spec.ExitPolicy = project.SceneExitRetain
	}
	r := newFixture(t, plan, registry, Options{})
	_, err := r.Manager().StartEntry()
	must(t, err)
	step := func() {
		id := project.SceneID("game.json")
		if r.Manager().Active().ID() == id {
			id = "main.json"
		}
		must(t, r.Manager().Switch(SceneSwitchRequest{Scene: id}))
		must(t, r.Dispatch())
		must(t, r.Dispatch())
	}
	for range 8 {
		step()
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("configured retained switches allocated %g times", allocations)
	}
}
