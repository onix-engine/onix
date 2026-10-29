package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
)

func TestBindControlCompositionAssignsStaticButtonParts(t *testing.T) {
	world := ecs.NewWorld()
	root := world.Spawn(ecs.NewBundle(0))
	if err := BindControl(world, root, ControlOptions{Width: 120, Height: 40}, Button{}); err != nil {
		t.Fatal(err)
	}
	body := world.Spawn(ecs.NewBundle(0))
	label := world.Spawn(ecs.NewBundle(0))
	for _, child := range []ecs.Entity{body, label} {
		if err := world.Insert(child, scene.Parent{Entity: root}); err != nil {
			t.Fatal(err)
		}
	}
	if err := world.Insert(label, scene.Text2D{Value: "Run"}); err != nil {
		t.Fatal(err)
	}
	composition := ControlComposition{
		root:  {Kind: CompositionGroup, Width: 120, Height: 40, Visible: true},
		body:  {Kind: CompositionSprite, Width: 120, Height: 40, Visible: true},
		label: {Kind: CompositionText, Text: "Run", Visible: true},
	}
	if err := BindControlComposition(world, composition, root, nil); err != nil {
		t.Fatal(err)
	}
	if !world.Has[Part[BodyPart]](body) || !world.Has[Part[LabelPart]](label) {
		t.Fatal("static button parts were not assigned")
	}
	relation, ok := world.Get[Relation[scene.Text2D]](label)
	if !ok || relation.Owner != root {
		t.Fatalf("label relation = %#v", relation)
	}
	variants, ok := world.Get[Variants[ControlVariant]](root)
	if !ok || len(variants.Items) != 2 {
		t.Fatalf("button variants = %#v", variants)
	}

	UnbindControlCompositionPart(world, label)
	if world.Has[Part[LabelPart]](label) || world.Has[Relation[scene.Text2D]](label) {
		t.Fatal("static label composition was not removed")
	}
}

func TestBindControlCompositionUsesSelectorBodyBoundsForInput(t *testing.T) {
	world := newUIWorld(t)
	root := spawnAnchor(world, 10, 10)
	if err := BindControl(world, root, ControlOptions{Width: 120, Height: 112}, Selector{ItemHeight: 36, ListHeight: 72}); err != nil {
		t.Fatal(err)
	}

	composition := ControlComposition{root: {Kind: CompositionGroup, Width: 120, Height: 112, Visible: true}}
	spawnNode := func(parent ecs.Entity, node CompositionNode) ecs.Entity {
		entity := spawnAnchor(world, node.X, node.Y)
		if err := world.Insert(entity, scene.Parent{Entity: parent}); err != nil {
			t.Fatal(err)
		}
		composition[entity] = node
		return entity
	}
	bodies := make([]ecs.Entity, 3)
	for index := range bodies {
		bodies[index] = spawnNode(root, CompositionNode{Kind: CompositionSprite, Width: 120, Height: 40, Visible: index == 0})
	}
	label := spawnNode(root, CompositionNode{Kind: CompositionText, X: 12, Y: 10, Text: "A", Visible: true})
	if err := world.Insert(label, scene.NewText2D("A")); err != nil {
		t.Fatal(err)
	}
	list := spawnNode(root, CompositionNode{Kind: CompositionGroup, Y: 40, Width: 120, Height: 72, Visible: false})
	setVisible(world, list, false)
	panel := spawnNode(list, CompositionNode{Kind: CompositionSprite, Width: 120, Height: 72, Visible: true})
	for index, value := range []string{"A", "B"} {
		item := spawnNode(list, CompositionNode{Kind: CompositionGroup, Y: float32(index * 36), Width: 120, Height: 36, Visible: true})
		itemLabel := spawnNode(item, CompositionNode{Kind: CompositionText, X: 12, Y: 8, Text: value, Visible: true})
		if err := world.Insert(itemLabel, scene.NewText2D(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := BindControlComposition(world, composition, root, func(entity ecs.Entity) error {
		node := composition[entity]
		return BindControl(world, entity, ControlOptions{Width: node.Width, Height: node.Height}, Button{})
	}); err != nil {
		t.Fatal(err)
	}

	width, height, _ := ControlBounds(world, root)
	if width != 120 || height != 40 {
		t.Fatalf("selector bounds = (%v,%v), want (120,40)", width, height)
	}
	listVisibility, _ := world.Get[scene.Visibility](list)
	if listVisibility.Visible {
		t.Fatal("authored selector list is visible while closed")
	}
	panelVisibility, _ := world.Get[scene.Visibility](panel)
	if panelVisibility.Visible {
		t.Fatal("authored selector panel is visible while closed")
	}
	for _, body := range bodies {
		if !world.Has[Part[BodyPart]](body) {
			t.Fatalf("selector state sprite %d was not bound as a body", body)
		}
	}

	if err := runUIFrame(world); err != nil {
		t.Fatal(err)
	}
	state, _ := world.GetResource[*input.State]()
	state.PressMouse(input.MouseLeft, 20, 20)
	state.ReleaseMouse(input.MouseLeft, 20, 20)
	if err := runUIUpdate(world); err != nil {
		t.Fatal(err)
	}
	selector, _ := world.Get[Selector](root)
	if !selector.Open {
		t.Fatal("selector did not open from its authored body")
	}
	if err := runUIFrame(world); err != nil {
		t.Fatal(err)
	}
	panelVisibility, _ = world.Get[scene.Visibility](panel)
	if !panelVisibility.Visible {
		t.Fatal("authored selector panel is hidden while open")
	}
	state.EndFrame()
	state.PressMouse(input.MouseLeft, 20, 96)
	state.ReleaseMouse(input.MouseLeft, 20, 96)
	if err := runUIUpdate(world); err != nil {
		t.Fatal(err)
	}
	selector, _ = world.Get[Selector](root)
	if selector.Open || selector.Selected != 1 {
		t.Fatalf("selector state after choosing B = %+v", selector)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatal(err)
	}
	panelVisibility, _ = world.Get[scene.Visibility](panel)
	if panelVisibility.Visible {
		t.Fatal("authored selector panel remained visible after selection")
	}
}

func TestBindControlExpandsGenericTextControlMode(t *testing.T) {
	world := ecs.NewWorld()
	entity := world.Spawn(ecs.NewBundle(0))
	opts := ControlOptions{Width: 180, Height: 80}
	if err := BindControl(world, entity, opts, TextControl[Input]{
		Value: "Onix", Placeholder: "Name", Password: true, MaxRunes: 80,
	}); err != nil {
		t.Fatal(err)
	}
	field, ok := world.Get[TextField](entity)
	if !ok || field.Value != "Onix" || field.Caret != 4 || field.Placeholder != "Name" || field.MaxRunes != 80 {
		t.Fatalf("input field = %+v, ok=%v", field, ok)
	}
	if !world.Has[Input](entity) || world.Has[Textarea](entity) || !world.Has[textFieldMaskCache](entity) {
		t.Fatal("single-line text control components were not installed")
	}

	if err := BindControl(world, entity, opts, TextControl[Textarea]{Placeholder: "Notes", Password: true}); err != nil {
		t.Fatal(err)
	}
	field, _ = world.Get[TextField](entity)
	if field.Password {
		t.Fatal("textarea retained single-line password behavior")
	}
	if world.Has[Input](entity) || !world.Has[Textarea](entity) || world.Has[textFieldMaskCache](entity) {
		t.Fatal("multiline text control components were not installed")
	}
	clip, ok := world.Get[scene.ClipRect2D](entity)
	if !ok || clip.Width != opts.Width || clip.Height != opts.Height {
		t.Fatalf("textarea clip = %+v, ok=%v", clip, ok)
	}
}
