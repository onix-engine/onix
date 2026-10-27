package html

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

func TestHTMLHoverReconcilesOnlyChangedRows(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>button{width:100px;height:20px}button:hover{background:#445566;color:red}</style><button id="first">First</button><button id="second">Second</button>`+strings.Repeat(`<div>Unchanged</div>`, 100))
	state := ecs.GetRef[htmlRenderState](world, owner)
	for range 2 {
		for _, id := range []string{"first", "second"} {
			control := incrementalControl(t, world, owner, id)
			baseline := world.ChangeTick()
			SetControlHover(world, control, true)
			incrementalSync(t, world)
			window := world.ChangesSince(baseline)
			changed := 0
			for range ecs.Query[scene.Text2D](world, ecs.Changed[scene.Text2D](&window)).All {
				changed++
			}
			if changed > 2 {
				t.Fatalf("hover refreshed %d text entities, want at most two rows", changed)
			}
			if state.SyncStats.Layouts != 0 || state.SyncStats.ReconciledBoxes > 4 || state.SyncStats.Styled > 4 || state.SyncStats.SnapshottedControls > 2 {
				t.Fatalf("hover updated unrelated branches: %+v", state.SyncStats)
			}
			for entity, access := range ecs.Query[ControlText](world).All {
				if access.Ref().Control == control && ecs.GetRef[scene.Text2D](world, entity).Color != graphics.ColorFromRGB(255, 0, 0) {
					t.Fatal("hover did not update the row text color")
				}
			}
		}
	}
	first, second := incrementalControl(t, world, owner, "first"), incrementalControl(t, world, owner, "second")
	if allocs := testing.AllocsPerRun(100, func() {
		for _, control := range [...]ecs.Entity{first, second} {
			SetControlHover(world, control, true)
			if err := Sync(world); err != nil {
				panic(err)
			}
		}
	}); allocs != 0 {
		t.Fatalf("warm row hover allocated %g times", allocs)
	}
}
