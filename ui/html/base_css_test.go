package html

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
)

func TestBaseCSSPreservesMountedComponentStyles(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, New(`<main></main>`))
	page := world.Spawn(bundle)
	SetBaseCSS(world, page, "main { color: black; }")
	definition, err := DefineScoped[struct{}, struct{}](NewTemplate(`<button>Custom</button>`), "button { color: red; }", Behavior[struct{}, struct{}]{})
	if err != nil {
		t.Fatal(err)
	}
	component, err := MountComponent(world, page, definition, struct{}{}, struct{}{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	base := "main { color: white; }"
	if !SetBaseCSS(world, page, base) {
		t.Fatal("base did not change")
	}
	surface, _ := ecs.Get[Surface](world, page)
	if !strings.Contains(surface.CSS, definition.CSS) || !strings.Contains(surface.CSS, base) || strings.Contains(surface.CSS, "black") {
		t.Fatal("base update lost component CSS or retained old base")
	}
	if allocs := testing.AllocsPerRun(100, func() { SetBaseCSS(world, page, base) }); allocs != 0 {
		t.Fatalf("unchanged stylesheet allocated %g times", allocs)
	}
	if !UnmountComponent(world, component) {
		t.Fatal("unmount failed")
	}
	surface, _ = ecs.Get[Surface](world, page)
	if surface.CSS != base {
		t.Fatalf("unmount restored stale base: %s", surface.CSS)
	}
}
