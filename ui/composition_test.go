package ui

import (
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
)

func TestControlVariantsMatchTypedStates(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(2)
	bundle.Put(scene.InputState2D{Hovered: true})
	bundle.Put(scene.InputTarget2D{})
	entity := world.Spawn(bundle)
	if !controlVariantMatches(world, entity, VariantHovered) {
		t.Fatal("hover variant did not match hovered state")
	}
	pointer := world.GetMut[scene.InputState2D](entity)
	pointer.Pressed = true
	if controlVariantMatches(world, entity, VariantHovered) || !controlVariantMatches(world, entity, VariantPressed) {
		t.Fatal("pressed state did not select the pressed variant")
	}
}

func TestTypedControlVariantsSynchronizeVisibility(t *testing.T) {
	world := ecs.NewWorld()

	controlBundle := ecs.NewBundle(4)
	controlBundle.Put(Control{})
	controlBundle.Put(scene.InputTarget2D{})
	controlBundle.Put(scene.InputState2D{Hovered: true})
	controlBundle.Put(scene.Visibility{Visible: true})
	control := world.Spawn(controlBundle)
	hoverBundle := ecs.NewBundle(1)
	hoverBundle.Put(scene.Visibility{Visible: false})
	hover := world.Spawn(hoverBundle)
	pressedBundle := ecs.NewBundle(1)
	pressedBundle.Put(scene.Visibility{Visible: false})
	pressed := world.Spawn(pressedBundle)
	if err := BindVariant(world, control, Variant[ControlVariant]{Entity: hover, State: VariantHovered, Enabled: true}); err != nil {
		t.Fatalf("bind hover visual: %v", err)
	}
	if err := BindVariant(world, control, Variant[ControlVariant]{Entity: pressed, State: VariantPressed, Enabled: true}); err != nil {
		t.Fatalf("bind pressed visual: %v", err)
	}

	syncControlVariants(world)
	assertVisibility(t, world, hover, true)
	assertVisibility(t, world, pressed, false)

	pointer := world.GetMut[scene.InputState2D](control)
	pointer.Pressed = true
	syncControlVariants(world)
	assertVisibility(t, world, hover, false)
	assertVisibility(t, world, pressed, true)

	controlVisibility := world.GetMut[scene.Visibility](control)
	controlVisibility.Visible = false
	syncControlVariants(world)
	assertVisibility(t, world, hover, false)
	assertVisibility(t, world, pressed, false)
}

func TestControlStoresExplicitSelectedState(t *testing.T) {
	world := ecs.NewWorld()
	entity := SpawnControl(world, ControlOptions{Width: 20, Height: 20}, Button{})
	SetControlSelected(world, entity, true)

	control, _ := world.Get[Control](entity)
	if !control.Selected {
		t.Fatal("selected state was not stored on the explicitly bound control")
	}
}

func TestAuthoredSelectionElementUsesLayersForMultipleLines(t *testing.T) {
	world := newUIWorld(t)
	owner := SpawnControl(world, ControlOptions{Width: 160, Height: 80}, TextField{
		Value: "ab\ncde\nfg", Anchor: 1, Caret: 8, Focused: true,
	})
	theme := themeForWorld(world, Theme{})
	value := spawnText[ValuePart](world, owner, 12, 10, "ab\ncde\nfg", theme)
	if err := BindRelation[Control](world, value, owner); err != nil {
		t.Fatalf("bind value element: %v", err)
	}

	layers := make([]scene.SpriteLayer2D, 4)
	bundle := ecs.NewBundle(5)
	bundle.Put(Relation[Control]{Owner: owner})
	bundle.Put(Part[SelectionPart]{})
	bundle.Put(scene.SpriteLayers2D{Layers: layers})
	bundle.Put(scene.Visibility{Visible: true})
	bundle.Put(scene.Size2D{Width: 1, Height: 1})
	selection := world.Spawn(bundle)

	syncAuthoredControlParts(world)
	animation, ok := world.Get[scene.SpriteLayers2D](selection)
	if !ok {
		t.Fatal("selection element lost its sprite layers")
	}
	for index := range 3 {
		layer := animation.Layers[index]
		if !layer.Visible || layer.Sprite.Width <= 0 || layer.Sprite.Height <= 0 {
			t.Fatalf("selection layer %d = %#v", index, layer)
		}
		if index > 0 && layer.Transform.Y <= animation.Layers[index-1].Transform.Y {
			t.Fatalf("selection layers are not ordered by line: %#v", animation.Layers[:3])
		}
	}
	if animation.Layers[3].Visible {
		t.Fatal("unused selection layer is visible")
	}
	assertVisibility(t, world, selection, true)

	field := world.GetMut[TextField](owner)
	field.Caret = field.Anchor
	syncAuthoredControlParts(world)
	animation, _ = world.Get[scene.SpriteLayers2D](selection)
	for index, layer := range animation.Layers {
		if layer.Visible {
			t.Fatalf("collapsed selection left layer %d visible", index)
		}
	}
	assertVisibility(t, world, selection, false)
}

func TestAuthoredControlSyncDoesNotAllocateAfterWarmup(t *testing.T) {
	world := newUIWorld(t)
	owner := SpawnControl(world, ControlOptions{Width: 160, Height: 80}, TextField{
		Value: "ab\ncde\nfg", Anchor: 1, Caret: 8, Focused: true,
	})
	theme := themeForWorld(world, Theme{})
	value := spawnText[ValuePart](world, owner, 12, 10, "ab\ncde\nfg", theme)
	if err := BindRelation[Control](world, value, owner); err != nil {
		t.Fatal(err)
	}

	bundle := ecs.NewBundle(5)
	bundle.Put(Relation[Control]{Owner: owner})
	bundle.Put(Part[SelectionPart]{})
	bundle.Put(scene.SpriteLayers2D{Layers: make([]scene.SpriteLayer2D, 4)})
	bundle.Put(scene.Visibility{Visible: true})
	bundle.Put(scene.Size2D{Width: 1, Height: 1})
	selection := world.Spawn(bundle)
	if err := BindVariant(world, owner, Variant[ControlVariant]{
		Entity: selection, State: VariantFocused, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	schedule := ecs.NewSchedule("authored_control_sync_allocations")
	if err := schedule.Add(SyncSystem("")); err != nil {
		t.Fatal(err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatal(err)
	}

	if allocations := testing.AllocsPerRun(100, func() {
		if err := schedule.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("authored control sync allocated %.0f objects, want 0", allocations)
	}
}

func TestAuthoredTextareaSyncsRenderedCaretToIME(t *testing.T) {
	world := newUIWorld(t)
	owner, caret := spawnAuthoredTextarea(t, world)
	uiState := &uiEventState{}
	world.InsertResource(uiState)
	focus(world, uiState, owner)

	if err := SyncSystem("").Run(world); err != nil {
		t.Fatalf("sync authored textarea: %v", err)
	}
	syncTransforms(t, world)
	if err := SyncTextInputSystem("").Run(world); err != nil {
		t.Fatalf("sync text input: %v", err)
	}

	transform, _ := world.Get[scene.UIGlobalTransform](caret)
	sprite, _ := world.Get[scene.Sprite2D](caret)
	anchor, _ := world.Get[scene.Anchor](caret)
	minX, minY, maxX, maxY, ok := scene.BoundsInSpace2D(transform.Matrix, anchor, sprite.Width, sprite.Height, true)
	if !ok {
		t.Fatal("authored caret has no rendered bounds")
	}
	want := input.TextInputRect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
	uiInputState, _ := world.GetResource[*input.State]()
	if got := uiInputState.TextInputConfig().Caret; got != want {
		t.Fatalf("IME caret = %+v, want authored caret bounds %+v", got, want)
	}
}

func TestAuthoredTextareaCaretBlinks(t *testing.T) {
	world := newUIWorld(t)
	clock := scene.NewTime()
	world.InsertResource(clock)
	request := &scene.RenderRequest{}
	world.InsertResource(request)
	owner, caret := spawnAuthoredTextarea(t, world)
	field := world.GetMut[TextField](owner)
	field.Focused = true

	for _, test := range []struct {
		elapsed time.Duration
		visible bool
	}{
		{elapsed: 0, visible: true},
		{elapsed: 500 * time.Millisecond, visible: false},
		{elapsed: time.Second, visible: true},
	} {
		clock.Set(0, test.elapsed, 0)
		if err := SyncSystem("").Run(world); err != nil {
			t.Fatalf("sync authored textarea at %s: %v", test.elapsed, err)
		}
		visibility, _ := world.Get[scene.Visibility](caret)
		if visibility.Visible != test.visible {
			t.Fatalf("authored caret visibility at %s = %v, want %v", test.elapsed, visibility.Visible, test.visible)
		}
		if test.elapsed > 0 && !request.Take() {
			t.Fatalf("authored caret phase change at %s did not request a render", test.elapsed)
		}
	}
}

func spawnAuthoredTextarea(t *testing.T, world *ecs.World) (ecs.Entity, ecs.Entity) {
	t.Helper()
	owner := SpawnControl(world, ControlOptions{X: 20, Y: 30, Width: 180, Height: 80}, TextControl[Textarea]{Value: "ab\ncd"})
	theme := themeForWorld(world, Theme{})
	value := spawnText[ValuePart](world, owner, 12, 10, "ab\ncd", theme)
	selection := spawnAuthoredTextFieldSprite(world, owner, 0, 0, 1, 1)
	caret := spawnAuthoredTextFieldSprite(world, owner, 12, 10, 1, 24)
	composition := ControlComposition{
		owner:     {Kind: CompositionGroup, Width: 180, Height: 80, Visible: true},
		value:     {Kind: CompositionText, Text: "ab\ncd", Visible: true},
		selection: {Kind: CompositionSprite, Width: 1, Height: 1, Visible: true},
		caret:     {Kind: CompositionSprite, X: 12, Y: 10, Width: 1, Height: 24, Visible: true},
	}
	if err := BindControlComposition(world, composition, owner, nil); err != nil {
		t.Fatalf("bind authored textarea: %v", err)
	}
	if !world.Has[Part[CaretPart]](caret) || !world.Has[Relation[Control]](caret) {
		t.Fatal("authored textarea caret was not bound")
	}
	return owner, caret
}

func spawnAuthoredTextFieldSprite(world *ecs.World, parent ecs.Entity, x, y, width, height float32) ecs.Entity {
	transform := scene.UITransformFromXY(x, y)
	bundle := ecs.NewBundle(7)
	bundle.Put(transform)
	bundle.Put(scene.UIGlobalTransform{})
	bundle.Put(scene.Visibility{Visible: true})
	bundle.Put(scene.AnchorTopLeft)
	bundle.Put(scene.Parent{Entity: parent})
	bundle.Put(scene.Sprite2D{Width: width, Height: height, Alpha: 1})
	return world.Spawn(bundle)
}

func assertVisibility(t *testing.T, world *ecs.World, entity ecs.Entity, want bool) {
	t.Helper()
	visibility, ok := world.Get[scene.Visibility](entity)
	if !ok || visibility.Visible != want {
		t.Fatalf("entity %d visibility = %#v, want %v", entity, visibility, want)
	}
}
