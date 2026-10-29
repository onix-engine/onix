package ui

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/diiyw/onix/ui/html"
)

func TestRuntimeSystemsRequireExplicitFeatures(t *testing.T) {
	world := ecs.NewWorld()
	if err := SyncSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if _, ok := world.GetResource[*runtimeSystems](); ok {
		t.Fatal("UI sync installed an implicit runtime profile")
	}

	RegisterRuntimeFeature[Widgets](world)
	systems, ok := world.GetResource[*runtimeSystems]()
	if !ok || systems == nil || systems.update[0] == nil || systems.sync[0] == nil || systems.afterLayout[0] == nil {
		t.Fatal("widget runtime systems were not registered")
	}
	RegisterRuntimeFeature[Widgets](world)
	if systems.update[1] != nil || systems.sync[1] != nil || systems.afterLayout[1] != nil {
		t.Fatal("widget runtime systems were registered twice")
	}
}

func TestFullRuntimeToolSyncDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	if err := SyncWith[FullRuntime](world); err != nil {
		t.Fatal(err)
	}
	if _, ok := world.GetResource[*runtimeSystems](); ok {
		t.Fatal("tool sync leaked its profile into scheduled runtime systems")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := SyncWith[FullRuntime](world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("full runtime tool sync allocated %.0f objects, want 0", allocations)
	}
}

func TestHTMLRendererSynchronizesHTMLComponents(t *testing.T) {
	world := ecs.NewWorld()
	component := html.New(`<div>runtime html</div>`)
	component.Width, component.Height = 160, 80
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	bundle.Put(component)
	owner := world.Spawn(bundle)

	RegisterRuntimeFeature[HTMLRenderer](world)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	found := false
	for entity, access := range world.Query[scene.Text2D]().All {
		text := access.Get()

		parent, ok := world.Get[scene.Parent](entity)
		if ok && parent.Entity == owner && strings.Contains(text.Value, "runtime") {
			found = true
		}
	}
	if !found {
		t.Fatal("HTML runtime did not create a text visual")
	}
}
