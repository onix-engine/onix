package scene

import (
	"github.com/diiyw/onix/ecs"
	"testing"
)

func TestPickingIDResolvesLiveEntityWithoutPerFrameMap(t *testing.T) {
	world := ecs.NewWorld()
	entity := world.Spawn(ecs.NewBundle(17))
	id := entity.PickingID()
	if id == 0 {
		t.Fatal("live entity received the background picking ID")
	}
	resolved, ok := world.EntityByPickingID(id)
	if !ok || resolved != entity {
		t.Fatalf("picking ID %d resolved to %v, %v; want %v, true", id, resolved, ok, entity)
	}
	if _, ok := world.EntityByPickingID(0); ok {
		t.Fatal("background picking ID resolved to an entity")
	}
	if !world.Despawn(entity) {
		t.Fatal("despawn failed")
	}
	if _, ok := world.EntityByPickingID(id); ok {
		t.Fatal("despawned entity remained visible to picking")
	}
}

func TestPickingIDUsesEntitySlotBits(t *testing.T) {
	entity := ecs.Entity(uint64(0xfeedbeef)<<32 | 0x12345678)
	if got := entity.PickingID(); got != 0x12345678 {
		t.Fatalf("picking ID = %#x, want %#x", got, uint32(0x12345678))
	}
}
