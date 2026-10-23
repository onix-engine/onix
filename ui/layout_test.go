package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestSyncUILayoutHorizontalPositionsChildren(t *testing.T) {
	world := ecs.NewWorld()
	parentBundle := ecs.NewBundle(4)
	ecs.Put(&parentBundle, scene.NewUITransform())
	ecs.Put(&parentBundle, scene.Size2D{Width: 200, Height: 80})
	ecs.Put(&parentBundle, NewLayout())
	ecs.Put(&parentBundle, scene.AnchorTopLeft)
	parent := world.Spawn(parentBundle)
	child := func(width float32) ecs.Entity {
		bundle := ecs.NewBundle(5)
		ecs.Put(&bundle, scene.NewUITransform())
		ecs.Put(&bundle, scene.Size2D{Width: width, Height: 20})
		ecs.Put(&bundle, scene.AnchorTopLeft)
		ecs.Put(&bundle, scene.Parent{Entity: parent})
		return world.Spawn(bundle)
	}
	one, two := child(30), child(40)
	value := ecs.GetMut[Layout](world, parent)
	value.Type, value.PaddingLeft, value.PaddingTop, value.SpacingX = LayoutHorizontal, 10, 5, 7
	if err := SyncLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	first, _ := ecs.Get[scene.UITransform](world, one)
	second, _ := ecs.Get[scene.UITransform](world, two)
	if got := first.Translation.Resolve(scene.UIResolveContext{}); got.X() != 10 || got.Y() != 5 {
		t.Fatalf("first child position = %v, want (10, 5)", got)
	}
	if got := second.Translation.Resolve(scene.UIResolveContext{}); got.X() != 47 || got.Y() != 5 {
		t.Fatalf("second child position = %v, want (47, 5)", got)
	}
}

func TestSyncUILayoutUsesAuthoredSiblingOrder(t *testing.T) {
	world := ecs.NewWorld()
	parentBundle := ecs.NewBundle(4)
	ecs.Put(&parentBundle, scene.NewUITransform())
	ecs.Put(&parentBundle, scene.Size2D{Width: 100, Height: 40})
	ecs.Put(&parentBundle, scene.AnchorTopLeft)
	ecs.Put(&parentBundle, Layout{Type: LayoutHorizontal, AutoAlignment: true})
	parent := world.Spawn(parentBundle)
	spawnChild := func(order int, width float32) ecs.Entity {
		bundle := ecs.NewBundle(6)
		ecs.Put(&bundle, scene.NewUITransform())
		ecs.Put(&bundle, scene.Size2D{Width: width, Height: 10})
		ecs.Put(&bundle, scene.AnchorTopLeft)
		ecs.Put(&bundle, scene.Parent{Entity: parent})
		ecs.Put(&bundle, scene.SiblingOrder(order))
		return world.Spawn(bundle)
	}
	second := spawnChild(1, 10)
	first := spawnChild(0, 20)
	if err := SyncLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	firstTransform, _ := ecs.Get[scene.UITransform](world, first)
	secondTransform, _ := ecs.Get[scene.UITransform](world, second)
	if x := firstTransform.Translation.Resolve(scene.UIResolveContext{}).X(); x != 0 {
		t.Fatalf("authored first child x = %v, want 0", x)
	}
	if x := secondTransform.Translation.Resolve(scene.UIResolveContext{}).X(); x != 20 {
		t.Fatalf("authored second child x = %v, want 20", x)
	}
}

func TestSyncUILayoutResizeChildrenFillsAvailableSpace(t *testing.T) {
	world := ecs.NewWorld()
	parentBundle := ecs.NewBundle(4)
	ecs.Put(&parentBundle, scene.NewUITransform())
	ecs.Put(&parentBundle, scene.Size2D{Width: 100, Height: 40})
	ecs.Put(&parentBundle, scene.AnchorTopLeft)
	ecs.Put(&parentBundle, Layout{Type: LayoutHorizontal, ResizeMode: LayoutResizeChildren})
	parent := world.Spawn(parentBundle)
	for range 2 {
		bundle := ecs.NewBundle(5)
		ecs.Put(&bundle, scene.NewUITransform())
		ecs.Put(&bundle, scene.Size2D{Width: 20, Height: 10})
		ecs.Put(&bundle, scene.AnchorTopLeft)
		ecs.Put(&bundle, scene.Parent{Entity: parent})
		world.Spawn(bundle)
	}
	if err := SyncLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	widths := make([]float32, 0, 2)
	for child, access := range ecs.Query[scene.Parent](world).All {
		relation := access.Get()

		if relation.Entity != parent {
			continue
		}
		size, _ := ecs.Get[scene.Size2D](world, child)
		widths = append(widths, size.Width)
	}
	if len(widths) != 2 || widths[0] != 50 || widths[1] != 50 {
		t.Fatalf("resize-children widths = %v, want [50 50]", widths)
	}
}

func TestSyncUILayoutWidgetStretchesAndResolvesPercentMargins(t *testing.T) {
	world := ecs.NewWorld()
	parentBundle := ecs.NewBundle(3)
	ecs.Put(&parentBundle, scene.NewUITransform())
	ecs.Put(&parentBundle, scene.Size2D{Width: 200, Height: 100})
	ecs.Put(&parentBundle, scene.AnchorTopLeft)
	parent := world.Spawn(parentBundle)
	childBundle := ecs.NewBundle(6)
	ecs.Put(&childBundle, scene.NewUITransform())
	ecs.Put(&childBundle, scene.Size2D{Width: 20, Height: 10})
	ecs.Put(&childBundle, scene.AnchorTopLeft)
	ecs.Put(&childBundle, scene.Parent{Entity: parent})
	ecs.Put(&childBundle, scene.InputTarget2D{Width: 20, Height: 10})
	ecs.Put(&childBundle, scene.InputBlocker2D{Width: 20, Height: 10})
	ecs.Put(&childBundle, Widget{Enabled: true, AlignLeft: true, AlignRight: true, AlignTop: true, Left: scene.Percent(10), Right: scene.Px(20), Top: scene.Px(8)})
	child := world.Spawn(childBundle)
	if err := SyncLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	size, _ := ecs.Get[scene.Size2D](world, child)
	transform, _ := ecs.Get[scene.UITransform](world, child)
	if size.Width != 160 {
		t.Fatalf("widget width = %v, want 160", size.Width)
	}
	target, _ := ecs.Get[scene.InputTarget2D](world, child)
	if target.Width != 160 || target.Height != 10 {
		t.Fatalf("widget input target = (%v, %v), want (160, 10)", target.Width, target.Height)
	}
	blocker, _ := ecs.Get[scene.InputBlocker2D](world, child)
	if blocker.Width != 160 || blocker.Height != 10 {
		t.Fatalf("widget input blocker = (%v, %v), want (160, 10)", blocker.Width, blocker.Height)
	}
	position := transform.Translation.Resolve(scene.UIResolveContext{})
	if position.X() != 20 || position.Y() != 8 {
		t.Fatalf("widget position = %v, want (20, 8)", position)
	}
}

func TestSyncUILayoutWidgetAlignOnceAndWindowResize(t *testing.T) {
	world := ecs.NewWorld()
	parentBundle := ecs.NewBundle(3)
	ecs.Put(&parentBundle, scene.NewUITransform())
	ecs.Put(&parentBundle, scene.Size2D{Width: 100, Height: 50})
	ecs.Put(&parentBundle, scene.AnchorTopLeft)
	parent := world.Spawn(parentBundle)
	childBundle := ecs.NewBundle(5)
	ecs.Put(&childBundle, scene.NewUITransform())
	ecs.Put(&childBundle, scene.Size2D{Width: 10, Height: 10})
	ecs.Put(&childBundle, scene.AnchorTopLeft)
	ecs.Put(&childBundle, scene.Parent{Entity: parent})
	ecs.Put(&childBundle, Widget{Enabled: true, AlignRight: true, Right: scene.Px(4), AlignMode: WidgetAlignOnce})
	child := world.Spawn(childBundle)
	if err := SyncLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	widget, _ := ecs.Get[Widget](world, child)
	if !widget.Enabled {
		t.Fatal("align-once alignment changed the authored enabled state")
	}
	transform := ecs.GetMut[scene.UITransform](world, child)
	transform.Translation.X = scene.Px(20)
	if err := SyncLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if x := transform.Translation.Resolve(scene.UIResolveContext{}).X(); x != 20 {
		t.Fatalf("align-once moved on second pass to %v", x)
	}
	widgetMut := ecs.GetMut[Widget](world, child)
	widgetMut.Right = scene.Px(8)
	if err := SyncLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if x := transform.Translation.Resolve(scene.UIResolveContext{}).X(); x != 82 {
		t.Fatalf("align-once did not re-align after its constraint changed, got %v", x)
	}
	widgetMut.AlignMode = WidgetAlignOnWindowResize
	if err := SyncLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if x := transform.Translation.Resolve(scene.UIResolveContext{}).X(); x != 82 {
		t.Fatalf("window resize mode did not re-align, got %v", x)
	}
}

func TestSyncUILayoutGridDirections(t *testing.T) {
	world := ecs.NewWorld()
	parentBundle := ecs.NewBundle(4)
	ecs.Put(&parentBundle, scene.NewUITransform())
	ecs.Put(&parentBundle, scene.Size2D{Width: 100, Height: 100})
	ecs.Put(&parentBundle, scene.AnchorTopLeft)
	ecs.Put(&parentBundle, Layout{Type: LayoutGrid, CellSize: mgl32.Vec2{20, 20}, Constraint: LayoutConstraintFixedColumn, ConstraintNum: 2})
	parent := world.Spawn(parentBundle)
	children := make([]ecs.Entity, 0, 3)
	for range 3 {
		bundle := ecs.NewBundle(4)
		ecs.Put(&bundle, scene.NewUITransform())
		ecs.Put(&bundle, scene.Size2D{Width: 10, Height: 10})
		ecs.Put(&bundle, scene.AnchorTopLeft)
		ecs.Put(&bundle, scene.Parent{Entity: parent})
		children = append(children, world.Spawn(bundle))
	}
	if err := SyncLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	for index, child := range children {
		transform, _ := ecs.Get[scene.UITransform](world, child)
		position := transform.Translation.Resolve(scene.UIResolveContext{})
		wantX, wantY := float32(index%2*20), float32(index/2*20)
		if position.X() != wantX || position.Y() != wantY {
			t.Fatalf("child %d position = %v, want (%v, %v)", index, position, wantX, wantY)
		}
	}
}

func TestSyncUILayoutReverseDirections(t *testing.T) {
	world := ecs.NewWorld()
	parentBundle := ecs.NewBundle(4)
	ecs.Put(&parentBundle, scene.NewUITransform())
	ecs.Put(&parentBundle, scene.Size2D{Width: 100, Height: 80})
	ecs.Put(&parentBundle, scene.AnchorTopLeft)
	ecs.Put(&parentBundle, Layout{Type: LayoutHorizontal, HorizontalDirection: LayoutRightToLeft, PaddingRight: 5, SpacingX: 3})
	parent := world.Spawn(parentBundle)
	children := make([]ecs.Entity, 0, 2)
	for _, width := range []float32{20, 10} {
		bundle := ecs.NewBundle(4)
		ecs.Put(&bundle, scene.NewUITransform())
		ecs.Put(&bundle, scene.Size2D{Width: width, Height: 10})
		ecs.Put(&bundle, scene.AnchorTopLeft)
		ecs.Put(&bundle, scene.Parent{Entity: parent})
		children = append(children, world.Spawn(bundle))
	}
	if err := SyncLayoutSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	one, _ := ecs.Get[scene.UITransform](world, children[0])
	two, _ := ecs.Get[scene.UITransform](world, children[1])
	if x := one.Translation.Resolve(scene.UIResolveContext{}).X(); x != 75 {
		t.Fatalf("right-to-left first x = %v, want 75", x)
	}
	if x := two.Translation.Resolve(scene.UIResolveContext{}).X(); x != 62 {
		t.Fatalf("right-to-left second x = %v, want 62", x)
	}
}
