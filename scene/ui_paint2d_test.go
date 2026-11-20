package scene

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
)

func TestUIPaint2DRetainsFragmentsAndTracksDamage(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	host := entities[0]
	world.GetMut[UIGlobalTransform](host).clip = clipRect2D{valid: true, minX: 0, minY: 0, maxX: 30, maxY: 1}
	beforeEntities := world.EntityCount()
	fragment := UIShapeFragment2D{StackOffset: 2, X: 2, Y: 1, Width: 5, Height: 2, Shape: RoundedRectShape2D(graphics.Black, 0)}
	SetUIShapeFragments2D(world, host, []UIShapeFragment2D{fragment})
	incrementalFrame(t, world, canvas)
	state := RenderWorldFor(world)
	tree := world.ScopeValue[renderTree2D, renderTree2D]()
	if len(tree.items) != 101 || world.EntityCount() != beforeEntities {
		t.Fatalf("fragment should add one render item and no entities, got %d items", len(tree.items))
	}
	var part renderPart2D
	found := false
	for _, item := range tree.items {
		if item.entity == host && item.sortIndex == 2 {
			part, found = item.part, true
			break
		}
	}
	if !found {
		t.Fatal("host's fragment did not enter the retained tree")
	}
	item := tree.itemForPart(part)
	if item.entity != host || item.sortIndex != 2 || item.transform[12] != 2 || item.transform[13] != 1 || !item.clip.valid || item.clip.maxX != 30 || item.paint == nil {
		t.Fatalf("fragment lost stack, transform, clip, or retained paint: %+v", item)
	}
	oldPaint := *item.paint
	SetUIShapeFragmentFill2D(world, host, 0, graphics.White)
	incrementalFrame(t, world, canvas)
	item = tree.itemForPart(part)
	if stats := state.Preparation2D; stats.Extracted != 2 || stats.Sorted || !tree.damageValid || item.paint.contentHash == oldPaint.contentHash {
		t.Fatalf("local fragment edit rebuilt the scene or missed its damage: %+v", stats)
	}
	transform := world.GetMut[UIGlobalTransform](host)
	transform.Matrix = mgl32.Translate3D(4, -1, 0)
	transform.clip.maxX, transform.clip.maxY = 20, 10
	incrementalFrame(t, world, canvas)
	item = tree.itemForPart(part)
	if state.Preparation2D.PropertiesUpdated != 2 || item.transform[12] != 6 || item.transform[13] != 0 || item.clip.maxX != 20 {
		t.Fatal("fragment did not follow the host's translation and updated clip")
	}
	if allocs := testing.AllocsPerRun(30, func() {
		SetUIShapeFragmentFill2D(world, host, 0, graphics.Black)
		incrementalFrame(t, world, canvas)
		SetUIShapeFragmentFill2D(world, host, 0, graphics.White)
		incrementalFrame(t, world, canvas)
	}); allocs != 0 {
		t.Fatalf("warm fragment edits allocated %g times", allocs)
	}
	SetUIShapeFragments2D(world, host, nil)
	incrementalFrame(t, world, canvas)
	if len(tree.items) != 100 || world.EntityCount() != beforeEntities {
		t.Fatal("fragment removal left stale paint or retired its host")
	}
}

func TestRetainedPaintListTracksSlotDamageAndStructuralEdits(t *testing.T) {
	world, canvas, _, _ := incrementalRenderFixture(t, 32, false)
	list := new(UIPaintList2D)
	box := UIBoxPaint2D{Shape: RoundedRectShape2D(graphics.Black, 0), Width: 8, Height: 8, Order: 1}
	first := list.SetBox(0, box)
	box.Order, box.X = 2, 12
	second := list.SetBox(0, box)
	owner := world.Spawn(ecs.NewBundle(0))
	if err := world.Insert(owner, list); err != nil {
		t.Fatal(err)
	}
	frame := func() *renderTree2D {
		incrementalFrame(t, world, canvas)
		return world.ScopeValue[renderTree2D, renderTree2D]()
	}
	frame()
	update := func(id uint32, value UIBoxPaint2D) {
		binding := world.GetMut[*UIPaintList2D](owner)
		(*binding).SetBox(id, value)
	}
	box.Shape.Fill = graphics.White
	box.X += 3
	update(second, box)
	tree := frame()
	if tree.stats.Extracted != 1 || tree.stats.Sorted || !tree.damageValid || len(tree.damage) != 1 {
		t.Fatalf("sparse damage: %+v, %v", tree.stats, tree.damage)
	}
	item := tree.itemForPart(tree.damage[0].part)
	if item.paintSlot != second || item.transform[12] != 15 || item.snapshot[Sprite2D]().Shape.Fill != graphics.White || *item.paint != preparePaintItem2D(item, canvas.Device()) {
		t.Fatalf("wrong slot snapshot: %+v", item)
	}
	// Edit and remove before extracting. The reused ID must target the new box.
	list.RemoveBox(first)
	box.Order, box.X = 3, 24
	reused := list.SetBox(0, box)
	if reused != first {
		t.Fatal("slot was not reused")
	}
	list.SetOrder(second, box.Stack, 4)
	world.GetMut[*UIPaintList2D](owner)
	tree = frame()
	positions := tree.nodes[owner].paintList.slots
	a, b := positions[first-1].shape, positions[second-1].shape
	if a == 0 || b <= a || tree.items[a-1].transform[12] != 24 {
		t.Fatalf("incorrect rebuilt slots: %+v", positions)
	}
	box.X = 27
	update(first, box)
	tree = frame()
	if tree.stats.Extracted != 1 || tree.stats.Sorted || tree.items[a-1].transform[12] != 27 {
		t.Fatalf("edit after slot reuse: %+v", tree.stats)
	}
}
