package ui

import (
	"strings"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/input"
	"github.com/onix-engine/onix/scene"
	notosanssc "github.com/onix-engine/onix/testdata/fonts/Noto_Sans_SC"
	"github.com/onix-engine/onix/ui/html"
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
	if !ok || systems == nil || systems.update[0].run == nil || systems.sync[0].run == nil || systems.afterLayout[0].run == nil {
		t.Fatal("widget runtime systems were not registered")
	}
	RegisterRuntimeFeature[Widgets](world)
	if systems.update[1].run != nil || systems.sync[1].run != nil || systems.afterLayout[1].run != nil {
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

func TestHTMLAndWidgetsShareOneControlPass(t *testing.T) {
	for _, order := range []string{"html first", "widgets first", "full runtime"} {
		t.Run(order, func(t *testing.T) {
			world := ecs.NewWorld()
			world.InsertResource(input.NewState())
			checkbox := SpawnCheckbox(world, CheckboxOptions{Width: 100, Height: 24})
			tap := func(entity ecs.Entity) {
				t.Helper()
				world.WriteMessage(scene.InputEvent{Entity: entity, Kind: scene.InputEventTap})
				if err := UpdateSystem("").Run(world); err != nil {
					t.Fatal(err)
				}
			}
			switch order {
			case "html first":
				RegisterRuntimeFeature[HTMLRenderer](world)
				tap(checkbox)
				if world.GetRef[Toggle[bool]](checkbox).Value {
					t.Fatal("HTML alone activated a native checkbox")
				}
				RegisterRuntimeFeature[Widgets](world)
			case "widgets first":
				RegisterRuntimeFeature[Widgets](world)
				RegisterRuntimeFeature[HTMLRenderer](world)
			case "full runtime":
				RegisterRuntimeFeature[FullRuntime](world)
			}
			RegisterRuntimeFeature[HTMLRenderer](world)
			RegisterRuntimeFeature[Widgets](world)
			systems := runtimeSystemsFor(world)
			for _, stage := range [...][runtimeSystemCapacity]runtimeStep{systems.update, systems.sync, systems.afterLayout} {
				count := 0
				for _, step := range stage {
					if step.controls {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("registered %d control passes, want 1", count)
				}
			}
			surface := html.New(`<select id="choice"><option>One</option><option>Two</option></select>`)
			surface.Width, surface.Height = 150, 60
			surface.Font = notosanssc.NotoSansSCRegular
			bundle := ecs.NewBundle(1)
			bundle.Put(surface)
			owner := world.Spawn(bundle)
			if err := Sync(world); err != nil {
				t.Fatal(err)
			}
			selectEntity := htmlControlByID(t, world, owner, "choice")
			tap(selectEntity)
			if !world.GetRef[html.Control](selectEntity).Open {
				t.Fatal("HTML select did not open on tap")
			}
			tap(checkbox)
			if !world.GetRef[Toggle[bool]](checkbox).Value {
				t.Fatal("native checkbox did not toggle on tap")
			}
		})
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
