package html

import (
	"slices"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
)

func trackOptionalForTest[T any](writer *htmlEntityWriter, slot htmlComponentSlot[T], removed *[]int) {
	var value T
	writer.putOptional(slot, value)
	writer.world.OnComponentRemove[T](func(*ecs.World, ecs.Entity, *T) {
		*removed = append(*removed, int(slot))
	})
}

func TestHTMLReconcileOptionalComponents(t *testing.T) {
	world := ecs.NewWorld()
	spawner := world.NewSpawner1[scene.Size2D]()
	entity := spawner.Spawn(scene.Size2D{Width: 17})
	writer := htmlEntityWriter{world: world, entity: entity}
	var removed []int
	trackOptionalForTest(&writer, spriteSlot, &removed)
	trackOptionalForTest(&writer, spriteLayersSlot, &removed)
	trackOptionalForTest(&writer, clipRectSlot, &removed)
	trackOptionalForTest(&writer, inputTargetSlot, &removed)
	trackOptionalForTest(&writer, inputBlockerSlot, &removed)
	trackOptionalForTest(&writer, focusableSlot, &removed)
	trackOptionalForTest(&writer, editableSlot, &removed)
	trackOptionalForTest(&writer, editableNodeSlot, &removed)
	trackOptionalForTest(&writer, controlTextSlot, &removed)
	trackOptionalForTest(&writer, controlTextNodeSlot, &removed)
	trackOptionalForTest(&writer, scrollSlot, &removed)
	trackOptionalForTest(&writer, scrollbarSlot, &removed)
	trackOptionalForTest(&writer, clipExemptSlot, &removed)
	trackOptionalForTest(&writer, materialShaderSlot, &removed)
	trackOptionalForTest(&writer, elementSurfaceSlot, &removed)
	trackOptionalForTest(&writer, opacitySurfaceSlot, &removed)
	if writer.optional != 0xffff {
		t.Fatalf("component mask %#x", writer.optional)
	}
	writer.oldOptional = writer.optional
	writer.removeMissingHTML()
	if len(removed) != 0 {
		t.Fatalf("retained components triggered cleanup %v", removed)
	}
	writer.optional = 0x5555
	writer.removeMissingHTML()
	if !slices.Equal(removed, []int{1, 3, 5, 7, 9, 11, 13, 15}) {
		t.Fatalf("removed slots %v", removed)
	}
	removed = removed[:0]
	writer.oldOptional, writer.optional = writer.optional, 0
	writer.removeMissingHTML()
	if !slices.Equal(removed, []int{0, 2, 4, 6, 8, 10, 12, 14}) {
		t.Fatalf("remaining slots %v", removed)
	}
	if size, ok := world.Get[scene.Size2D](entity); !ok || size.Width != 17 {
		t.Fatal("retained size was lost")
	}
}
