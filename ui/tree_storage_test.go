package ui

import (
	"reflect"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
)

func TestUILayoutTopologyStorage(t *testing.T) {
	world := ecs.NewWorld()
	spawn := world.NewSpawner3[Layout, scene.UITransform, scene.Parent]()
	var parent ecs.Entity
	for i := range 10000 {
		if i%16 == 0 {
			parent = 0
		}
		parent = spawn.Spawn(Layout{}, scene.NewUITransform(), scene.Parent{Entity: parent})
	}
	scene.HierarchyFor(world)
	if err := UpdateLayout(world); err != nil {
		t.Fatal(err)
	}
	scratch := world.FrameValue[uiLayoutScratchTag, uiLayoutScratch]()
	value := reflect.ValueOf(scratch).Elem()
	bytes := value.Type().Size()
	for i := range value.NumField() {
		field := value.Field(i)
		if field.Kind() == reflect.Slice {
			bytes += uintptr(field.Cap()) * field.Type().Elem().Size()
		}
	}
	t.Logf("layout topology struct and slice capacity: %d B; maps and per-container size snapshots excluded", bytes)
	if bytes > 400000 {
		t.Fatalf("layout topology retained %d B for 10000 nodes", bytes)
	}
}
