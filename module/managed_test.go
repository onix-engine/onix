package module

import (
	"errors"
	"slices"
	"testing"
)

type managedProbe struct {
	active              bool
	transitions, closes int
}

func (p *managedProbe) Activate(active bool) error { p.active = active; p.transitions++; return nil }
func (p *managedProbe) Close() error               { p.closes++; return nil }

func TestSceneManagedResourcesAndHooksFollowForegroundLifetime(t *testing.T) {
	plan, registry, _ := managerPlan(t)
	probe := &managedProbe{}
	var hooks []string
	delete(registry.scenes, "main.json")
	must(t, RegisterScene(&registry, "main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{
		Install: func(c *Context, _ *Signal) error { hooks = append(hooks, "install"); return Own(c, probe) },
		Enter:   func(*Context, *Signal, SceneStartup) error { hooks = append(hooks, "enter"); return nil },
		Exit:    func(*Context, *Signal) error { hooks = append(hooks, "exit"); return nil },
		Suspend: func(*Context, *Signal) error { hooks = append(hooks, "suspend"); return nil },
		Resume:  func(*Context, *Signal, SceneStartup) error { hooks = append(hooks, "resume"); return nil },
		Destroy: func(*Context, *Signal) error { hooks = append(hooks, "destroy"); return nil },
	}))
	r := newFixture(t, plan, registry, Options{})
	main, err := r.Manager().StartEntry()
	must(t, err)
	if !probe.active || probe.transitions != 1 {
		t.Fatal("scene resource was not activated")
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "game.json", Overlay: true}))
	must(t, r.Dispatch())
	if probe.active || probe.closes != 0 {
		t.Fatal("overlay did not suspend scene resource")
	}
	must(t, r.Manager().Activate(main, SceneActivationOptions{}))
	must(t, r.Dispatch())
	if !probe.active || probe.transitions != 3 {
		t.Fatal("resumption did not activate scene resource")
	}
	must(t, main.Dispose())
	if probe.closes != 1 {
		t.Fatal("scene resource was not closed")
	}
	want := []string{"install", "enter", "exit", "suspend", "resume", "exit", "destroy"}
	if !slices.Equal(hooks, want) {
		t.Fatalf("lifecycle = %v, want %v", hooks, want)
	}
}

func TestModuleManagedResourcesFollowCommittedSceneLifetime(t *testing.T) {
	plan, _ := compileFixture(t, map[string]string{
		"resources/project.json":       `{"entry":"main.json","scenes":["main.json","game.json","broken.json"]}`,
		"resources/scenes/main.json":   `{"modules":[{"id":"a","mod":"speaker"},{"id":"b","mod":"speaker"}]}`,
		"resources/scenes/game.json":   `{"modules":[]}`,
		"resources/scenes/broken.json": `{"modules":[],"behavior":"Broken"}`,
		"broken.go":                    "package main\ntype Broken struct{}\n",
		"speaker/mod.json":             `{}`,
	})
	var probes []*managedProbe
	registry := Registry{}
	must(t, RegisterMod(&registry, "speaker", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error { p := &managedProbe{}; probes = append(probes, p); return Own(c, p) }}))
	must(t, RegisterScene(&registry, "broken.json", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(*Context, *Signal) error { return errors.New("failed startup") }}))
	r := newFixture(t, plan, registry, Options{})
	main, err := r.Manager().StartEntry()
	must(t, err)
	if !probes[0].active || !probes[1].active {
		t.Fatal("entry resources were not activated")
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "broken.json", Retain: true}))
	must(t, r.Dispatch())
	if !probes[0].active || probes[0].transitions != 1 {
		t.Fatal("failed preparation changed the active scene")
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "game.json", Retain: true}))
	must(t, r.Dispatch())
	if probes[0].active || probes[0].closes != 0 {
		t.Fatal("retained resource was not suspended")
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "main.json"}))
	must(t, r.Dispatch())
	if !probes[0].active || probes[0].transitions != 3 {
		t.Fatal("resuming recreated or lost the resource")
	}
	a := &main.modules[0]
	extra := &managedProbe{}
	must(t, Own(a, extra))
	Disown(a, extra)
	must(t, a.Dispose())
	if probes[0].closes != 1 || probes[1].closes != 0 || extra.closes != 0 {
		t.Fatal("resource disposal crossed instance ownership")
	}
	must(t, r.Close())
	if probes[0].closes != 1 || probes[1].closes != 1 {
		t.Fatal("resource was leaked or closed twice")
	}
}
