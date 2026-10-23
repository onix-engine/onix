package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestVerticalListVirtualizesLargeDataSet(t *testing.T) {
	world := newUIWorld(t)
	items := make([]int, 100_000)
	for index := range items {
		items[index] = index * 10
	}
	binds := 0
	list := SpawnList(world, ListOptions[int]{
		Width: 240, Height: 100, ItemSize: 20, Overscan: 2, Items: items,
		BindItem: func(_ *ecs.World, _ ecs.Entity, _ int, _ int) { binds++ },
	})
	itemEntities := ListItemEntities[int](world, list)
	if len(itemEntities) != 10 {
		t.Fatalf("pooled item entities = %d, want 10", len(itemEntities))
	}
	if got := world.EntityCount(); got >= len(items) {
		t.Fatalf("virtual list spawned %d entities for %d items", got, len(items))
	}
	component, _ := ecs.Get[List[int]](world, list)
	if component.First != 0 || component.Last != 10 || binds != 10 {
		t.Fatalf("initial range = [%d,%d), binds = %d", component.First, component.Last, binds)
	}
	SetScrollAreaOffset(world, list, mgl32.Vec2{0, 501})
	binds = 0
	if err := SyncListSystem[int]("").Run(world); err != nil {
		t.Fatal(err)
	}
	component, _ = ecs.Get[List[int]](world, list)
	if component.First != 23 || component.Last != 33 || binds != 10 {
		t.Fatalf("scrolled range = [%d,%d), binds = %d", component.First, component.Last, binds)
	}
	for slot, itemEntity := range itemEntities {
		item, ok := ecs.Get[ListItem[int]](world, itemEntity)
		if !ok || item.List != list || item.Index != 23+slot || item.Value != (23+slot)*10 {
			t.Fatalf("item entity %d value = %#v", slot, item)
		}
		transform, _ := ecs.Get[scene.UITransform](world, itemEntity)
		wantY := float32((23+slot)*20 - 501)
		if transform.Translation.Y.Value != wantY {
			t.Fatalf("item entity %d y = %v, want %v", slot, transform.Translation.Y.Value, wantY)
		}
	}
	areaValue, _ := ecs.Get[ScrollArea](world, list)
	metrics, _ := GetScrollAreaMetrics(world, list)
	if areaValue.ContentHeight != 2_000_000 || metrics.MaxScrollY != 1_999_900 || !metrics.VerticalVisible {
		t.Fatalf("scroll area = %#v metrics = %#v", areaValue, metrics)
	}
}

func TestHorizontalListVirtualizesAlongXAxis(t *testing.T) {
	world := newUIWorld(t)
	items := make([]int, 100_000)
	for index := range items {
		items[index] = index * 10
	}
	binds := 0
	list := SpawnList(world, ListOptions[int]{
		Width: 100, Height: 60, Axis: ListHorizontal, ItemSize: 20,
		Overscan: 2, Items: items, ScrollX: 501,
		BindItem: func(_ *ecs.World, _ ecs.Entity, _ int, _ int) { binds++ },
	})
	itemEntities := ListItemEntities[int](world, list)
	if len(itemEntities) != 10 {
		t.Fatalf("pooled item entities = %d, want 10", len(itemEntities))
	}
	if got := world.EntityCount(); got >= len(items) {
		t.Fatalf("virtual list spawned %d entities for %d items", got, len(items))
	}
	component, _ := ecs.Get[List[int]](world, list)
	if component.Axis != ListHorizontal || component.First != 23 || component.Last != 33 || binds != 10 {
		t.Fatalf("horizontal range = [%d,%d), axis = %v, binds = %d", component.First, component.Last, component.Axis, binds)
	}
	for slot, itemEntity := range itemEntities {
		index := 23 + slot
		item, ok := ecs.Get[ListItem[int]](world, itemEntity)
		if !ok || item.List != list || item.Index != index || item.Value != index*10 {
			t.Fatalf("item entity %d item = %#v", slot, item)
		}
		transform, _ := ecs.Get[scene.UITransform](world, itemEntity)
		wantX := float32(index*20 - 501)
		if transform.Translation.X.Value != wantX || transform.Translation.Y.Value != 0 {
			t.Fatalf("item entity %d position = (%v, %v), want (%v, 0)", slot, transform.Translation.X.Value, transform.Translation.Y.Value, wantX)
		}
		size, _ := ecs.Get[scene.Size2D](world, itemEntity)
		if size.Width != 20 || size.Height != 60 {
			t.Fatalf("item entity %d size = (%v, %v), want (20, 60)", slot, size.Width, size.Height)
		}
	}
	area, _ := ecs.Get[ScrollArea](world, list)
	metrics, _ := GetScrollAreaMetrics(world, list)
	if area.ContentWidth != 2_000_000 || area.ContentHeight != 0 || metrics.MaxScrollX != 1_999_900 || !metrics.HorizontalVisible || metrics.VerticalVisible {
		t.Fatalf("scroll area = %#v metrics = %#v", area, metrics)
	}
	offset, _ := GetScrollAreaOffset(world, list)
	if offset.X() != 501 || offset.Y() != 0 {
		t.Fatalf("scroll offset = %v, want (501, 0)", offset)
	}
}

func TestListReusesItemEntitiesWhenItemsAreReplaced(t *testing.T) {
	world := newUIWorld(t)
	built := 0
	bound := map[int]string{}
	unbound := 0
	list := SpawnList(world, ListOptions[string]{
		Width: 100, Height: 40, ItemSize: 20, Overscan: 1, Items: []string{"A", "B"},
		BuildItem: func(_ *ecs.World, _ ecs.Entity) { built++ },
		BindItem:  func(_ *ecs.World, _ ecs.Entity, index int, value string) { bound[index] = value },
		UnbindItem: func(_ *ecs.World, _ ecs.Entity) {
			unbound++
		},
	})
	itemEntities := ListItemEntities[string](world, list)
	if built != 5 || len(itemEntities) != 5 {
		t.Fatalf("built item entities = %d, handles = %d, want 5", built, len(itemEntities))
	}
	if unbound != len(itemEntities) {
		t.Fatalf("initial unbinds = %d, want %d", unbound, len(itemEntities))
	}
	for index, itemEntity := range itemEntities {
		visibility, _ := ecs.Get[scene.Visibility](world, itemEntity)
		if visibility.Visible != (index < 2) {
			t.Fatalf("item entity %d visibility = %v", index, visibility.Visible)
		}
	}
	source := []string{"C", "D", "E", "F", "G", "H"}
	if !SetListItems(world, list, source) {
		t.Fatal("SetListItems rejected a list of the expected type")
	}
	source[0] = "changed"
	bound = map[int]string{}
	if err := SyncListSystem[string]("").Run(world); err != nil {
		t.Fatal(err)
	}
	component, _ := ecs.Get[List[string]](world, list)
	if built != 5 || component.Items[0] != "C" || component.First != 0 || component.Last != 5 {
		t.Fatalf("reused list = %#v, built = %d", component, built)
	}
	if len(bound) != 5 || bound[0] != "C" || bound[4] != "G" {
		t.Fatalf("bound items = %#v", bound)
	}
	if unbound != len(itemEntities) {
		t.Fatalf("growing the list unexpectedly unbound an item entity: %d", unbound)
	}
	for _, itemEntity := range itemEntities {
		if visibility, _ := ecs.Get[scene.Visibility](world, itemEntity); !visibility.Visible {
			t.Fatal("a reusable item entity remained hidden after growing the item set")
		}
	}

	if !SetListItems[string](world, list, nil) {
		t.Fatal("SetListItems rejected an empty replacement")
	}
	if err := SyncListSystem[string]("").Run(world); err != nil {
		t.Fatal(err)
	}
	if unbound != len(itemEntities)*2 {
		t.Fatalf("unbinds after clearing = %d, want %d", unbound, len(itemEntities)*2)
	}
}

func TestListClampsToLastItems(t *testing.T) {
	world := newUIWorld(t)
	items := make([]int, 20)
	list := SpawnList(world, ListOptions[int]{Width: 100, Height: 40, ItemSize: 10, Overscan: 1, Items: items})
	SetScrollAreaOffset(world, list, mgl32.Vec2{0, 1_000})
	if err := SyncListSystem[int]("").Run(world); err != nil {
		t.Fatal(err)
	}
	component, _ := ecs.Get[List[int]](world, list)
	if component.First != 13 || component.Last != 20 {
		t.Fatalf("last range = [%d,%d), want [13,20)", component.First, component.Last)
	}
	offset, _ := GetScrollAreaOffset(world, list)
	if offset.Y() != 160 {
		t.Fatalf("scroll y = %v, want 160", offset.Y())
	}
}

func TestBindListUsesExistingScrollArea(t *testing.T) {
	world := newUIWorld(t)
	area := SpawnScrollArea(world, ScrollAreaOptions{Width: 120, Height: 40})
	before := 0
	for range ecs.Query[ScrollArea](world).All {
		before++
	}

	if !BindList(world, area, ListOptions[int]{Items: []int{1, 2, 3}, ItemSize: 20}) {
		t.Fatal("BindList rejected an existing scroll area")
	}
	if _, ok := ecs.Get[List[int]](world, area); !ok {
		t.Fatal("existing scroll area was not bound as a list")
	}
	after := 0
	for range ecs.Query[ScrollArea](world).All {
		after++
	}
	if after != before {
		t.Fatalf("scroll area count = %d after binding, want %d", after, before)
	}
}

func TestListSyncWithItemLifecycleDoesNotAllocateAfterWarmup(t *testing.T) {
	for _, test := range []struct {
		name string
		axis ListAxis
	}{
		{name: "vertical", axis: ListVertical},
		{name: "horizontal", axis: ListHorizontal},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newUIWorld(t)
			items := []int{1, 2, 3}
			listEntity := SpawnList(world, ListOptions[int]{
				Width: 100, Height: 40, Axis: test.axis, ItemSize: 20, Items: items,
				BindItem:   func(*ecs.World, ecs.Entity, int, int) {},
				UnbindItem: func(*ecs.World, ecs.Entity) {},
			})
			system := SyncListSystem[int]("")
			if err := system.Run(world); err != nil {
				t.Fatal(err)
			}

			show := false
			allocations := testing.AllocsPerRun(100, func() {
				list := ecs.GetMut[List[int]](world, listEntity)
				if show {
					list.Items = items
				} else {
					list.Items = list.Items[:0]
				}
				show = !show
				if err := system.Run(world); err != nil {
					panic(err)
				}
			})
			if allocations != 0 {
				t.Fatalf("list item lifecycle allocated %v objects per sync, want 0", allocations)
			}
		})
	}
}
