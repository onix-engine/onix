package ui

import (
	"fmt"
	"sort"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

type CompositionNodeKind uint8

const (
	CompositionGroup CompositionNodeKind = iota
	CompositionSprite
	CompositionText
)

// CompositionNode describes one static, visual node available to a control.
// Behavior is selected exclusively by components installed on the control root.
type CompositionNode struct {
	Kind          CompositionNodeKind
	X, Y          float32
	Width, Height float32
	Text          string
	Visible       bool
}

// ControlComposition contains the static nodes that may form control visuals.
type ControlComposition map[ecs.Entity]CompositionNode

// BindControlComposition assigns static descendants to the behavior components
// already installed on owner. bindButton binds nested button roots when needed.
func BindControlComposition(world *ecs.World, composition ControlComposition, owner ecs.Entity, bindButton func(ecs.Entity) error) error {
	node, ok := composition[owner]
	if !ok {
		return nil
	}
	if world.Has[TextField](owner) {
		return composition.bindTextField(world, owner, node, bindButton)
	}
	switch {
	case world.Has[Button](owner):
		return composition.bindButton(world, owner, node)
	case world.Has[Toggle[bool]](owner):
		return composition.bindCheckbox(world, owner)
	case world.Has[Selector](owner):
		return composition.bindSelector(world, owner, node, bindButton)
	case world.Has[Progress](owner):
		return composition.bindProgress(world, owner, bindButton)
	case world.Has[ScrollArea](owner):
		return composition.bindScrollArea(world, owner)
	default:
		return nil
	}
}

func (composition ControlComposition) bindCompositionPart[P any](world *ecs.World, entity, owner ecs.Entity, state ControlVariant) error {
	if err := BindRelation[Control](world, entity, owner); err != nil {
		return err
	}
	if err := BindPart[P](world, entity); err != nil {
		return err
	}
	if world.Has[scene.Text2D](entity) {
		if err := BindRelation[scene.Text2D](world, entity, owner); err != nil {
			return err
		}
	}
	if world.Has[scene.UIBackground2D](entity) || world.Has[scene.UIImage2D](entity) || world.Has[scene.UIBorder2D](entity) {
		if err := BindRelation[SurfaceStyle](world, entity, owner); err != nil {
			return err
		}
	}
	enabled := true
	if node, ok := composition[entity]; ok {
		enabled = node.Visible
	}
	return BindVariant(world, owner, Variant[ControlVariant]{Entity: entity, State: state, Enabled: enabled})
}

// UnbindControlCompositionPart removes composition state from one static node.
func UnbindControlCompositionPart(world *ecs.World, entity ecs.Entity) {
	unbindControlCompositionPart(world, entity, true)
}

func unbindControlCompositionPart(world *ecs.World, entity ecs.Entity, detachVariant bool) {
	relation, ok := world.Get[Relation[Control]](entity)
	if !ok {
		return
	}
	if world.Has[Part[FillPart]](entity) {
		if progress := world.GetMut[Progress](relation.Owner); progress != nil && progress.Fill == entity {
			progress.Fill = 0
		}
	}
	if world.Has[Part[ThumbPart]](entity) {
		if progress := world.GetMut[Progress](relation.Owner); progress != nil && progress.Thumb == entity {
			progress.Thumb = 0
		}
	}
	clipFlags := 0
	if world.Has[Part[SelectorListPart]](entity) {
		clipFlags |= 1
	}
	if world.Has[Part[ScrollTrackHorizontalPart]](entity) ||
		world.Has[Part[ScrollTrackVerticalPart]](entity) ||
		world.Has[Part[ScrollThumbHorizontalPart]](entity) ||
		world.Has[Part[ScrollThumbVerticalPart]](entity) {
		clipFlags |= 2
	}
	if detachVariant {
		UnbindVariant[ControlVariant](world, relation.Owner, entity)
	}
	compositionRemovalFor(world, clipFlags).Apply(entity)
}

// UnbindControl removes the behavior installed by BindControl from an
// existing entity. Authored transform, layout, and visual components are
// intentionally left to the caller; this function only tears down UI control
// state and the relations used by static control compositions.
func UnbindControl(world *ecs.World, entity ecs.Entity) {
	if world == nil || entity == 0 {
		return
	}
	parts := world.FrameScratch[controlPartsScratch, ecs.Entity]()
	for part, access := range world.Query[Relation[Control]]().All {
		relation := access.Get()

		if relation.Owner == entity {
			parts.Values = append(parts.Values, part)
		}
	}
	for _, part := range parts.Values {
		unbindControlCompositionPart(world, part, false)
	}
	parts.ReleaseUnused()

	controlRemovalFor(world).Apply(entity)
}

func (composition ControlComposition) directChildren(world *ecs.World, owner ecs.Entity) []ecs.Entity {
	children := make([]ecs.Entity, 0, 8)
	for entity := range composition {
		parent, ok := world.Get[scene.Parent](entity)
		if ok && parent.Entity == owner {
			children = append(children, entity)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i] < children[j] })
	return children
}

func (composition ControlComposition) bindButton(world *ecs.World, owner ecs.Entity, node CompositionNode) error {
	bodies := make([]ecs.Entity, 0, 3)
	for _, child := range composition.directChildren(world, owner) {
		childNode := composition[child]
		switch childNode.Kind {
		case CompositionSprite:
			if compositionBodySprite(node, childNode) {
				bodies = append(bodies, child)
			} else if err := composition.bindCompositionPart[IconPart](world, child, owner, VariantAlways); err != nil {
				return err
			}
		case CompositionText:
			switch {
			case world.Has[NumberInputStep](owner):
				if err := composition.bindCompositionPart[IconPart](world, child, owner, VariantAlways); err != nil {
					return err
				}
			case world.Has[SelectorItem](owner):
				if err := composition.bindCompositionPart[ItemPart](world, child, owner, VariantAlways); err != nil {
					return err
				}
			default:
				if err := composition.bindCompositionPart[LabelPart](world, child, owner, VariantAlways); err != nil {
					return err
				}
			}
		}
	}
	return composition.bindBodies(world, owner, bodies, buttonBodyStates)
}

type bodyStateSet uint8

const (
	buttonBodyStates bodyStateSet = iota
	textFieldBodyStates
	selectorBodyStates
)

func (composition ControlComposition) bindBodies(world *ecs.World, owner ecs.Entity, bodies []ecs.Entity, states bodyStateSet) error {
	for index, body := range bodies {
		state := VariantAlways
		if len(bodies) > 1 {
			switch states {
			case buttonBodyStates:
				state = []ControlVariant{VariantUnfocusedNormal, VariantHovered, VariantPressed}[min(index, 2)]
			case textFieldBodyStates:
				state = []ControlVariant{VariantUnfocusedNormal, VariantUnfocusedHovered, VariantFocused}[min(index, 2)]
			case selectorBodyStates:
				state = []ControlVariant{VariantClosedNormal, VariantClosedHovered, VariantOpenEnabled}[min(index, 2)]
			}
		}
		if err := composition.bindCompositionPart[BodyPart](world, body, owner, state); err != nil {
			return err
		}
	}
	return nil
}

func compositionBodySprite(owner, child CompositionNode) bool {
	return child.Kind == CompositionSprite && child.Width == owner.Width && child.Height == owner.Height && child.X == 0 && child.Y == 0
}

func compositionSelectorBodySprite(owner, child CompositionNode) bool {
	return child.Kind == CompositionSprite && child.Width == owner.Width && child.Height > 0 && child.X == 0 && child.Y == 0
}

func (composition ControlComposition) bindTextField(world *ecs.World, owner ecs.Entity, node CompositionNode, bindButton func(ecs.Entity) error) error {
	bodies := make([]ecs.Entity, 0, 3)
	decorations := make([]ecs.Entity, 0, 2)
	steps := make([]ecs.Entity, 0, 2)
	for _, child := range composition.directChildren(world, owner) {
		childNode := composition[child]
		switch childNode.Kind {
		case CompositionSprite:
			if compositionBodySprite(node, childNode) {
				bodies = append(bodies, child)
			} else {
				decorations = append(decorations, child)
			}
		case CompositionText:
			if err := composition.bindCompositionPart[ValuePart](world, child, owner, VariantAlways); err != nil {
				return err
			}
		case CompositionGroup:
			if world.Has[NumberInput](owner) {
				steps = append(steps, child)
			}
		}
	}
	if err := composition.bindBodies(world, owner, bodies, textFieldBodyStates); err != nil {
		return err
	}
	if len(decorations) > 0 {
		if err := composition.bindCompositionPart[SelectionPart](world, decorations[0], owner, VariantFocused); err != nil {
			return err
		}
		if err := linkCompositionSelection(world, decorations[0]); err != nil {
			return err
		}
	}
	if len(decorations) > 1 {
		if err := composition.bindCompositionPart[CaretPart](world, decorations[1], owner, VariantFocused); err != nil {
			return err
		}
	}
	for index, step := range steps {
		if bindButton == nil {
			return fmt.Errorf("UI control composition requires a nested button binder")
		}
		if err := bindButton(step); err != nil {
			return err
		}
		direction := -1
		if index == 0 {
			direction = 1
			if err := composition.bindCompositionPart[IncrementPart](world, step, owner, VariantAlways); err != nil {
				return err
			}
		} else if err := composition.bindCompositionPart[DecrementPart](world, step, owner, VariantAlways); err != nil {
			return err
		}
		if err := world.Insert(step, NumberInputStep{Input: owner, Direction: direction}); err != nil {
			return err
		}
	}
	return nil
}

func linkCompositionSelection(world *ecs.World, entity ecs.Entity) error {
	if world.Has[scene.SpriteLayers2D](entity) {
		return nil
	}
	sprite, ok := world.Get[scene.Sprite2D](entity)
	if !ok {
		return nil
	}
	anchor, _ := world.Get[scene.Anchor](entity)
	layers := make([]scene.SpriteLayer2D, 16)
	for index := range layers {
		layers[index] = scene.SpriteLayer2D{ID: selectionLayerIDs[index], Order: index, Sprite: sprite, Anchor: anchor}
	}
	value := scene.SpriteLayers2D{Layers: layers}
	value.InvalidateMatrices()
	cache := world.FrameValue[selectionPatchCache, selectionPatchCache]()
	if !cache.initialized {
		cache.patch = world.NewPatch2[scene.SpriteLayers2D, scene.Size2D](ecs.Type[scene.Sprite2D]())
		cache.initialized = true
	}
	return cache.patch.Apply(entity, value, scene.Size2D{Width: 1, Height: 1})
}

type selectionPatchCache struct {
	patch       ecs.Patch2[scene.SpriteLayers2D, scene.Size2D]
	initialized bool
}

var selectionLayerIDs = [...]string{
	"selection-0", "selection-1", "selection-2", "selection-3",
	"selection-4", "selection-5", "selection-6", "selection-7",
	"selection-8", "selection-9", "selection-10", "selection-11",
	"selection-12", "selection-13", "selection-14", "selection-15",
}

func (composition ControlComposition) bindCheckbox(world *ecs.World, owner ecs.Entity) error {
	sprites := make([]ecs.Entity, 0, 4)
	for _, child := range composition.directChildren(world, owner) {
		switch composition[child].Kind {
		case CompositionSprite:
			sprites = append(sprites, child)
		case CompositionText:
			if err := composition.bindCompositionPart[LabelPart](world, child, owner, VariantAlways); err != nil {
				return err
			}
		}
	}
	states := []ControlVariant{VariantInputNormal, VariantHovered, VariantPressed}
	for index, sprite := range sprites {
		if index >= len(states) {
			if err := composition.bindCompositionPart[KnobPart](world, sprite, owner, VariantChecked); err != nil {
				return err
			}
			continue
		}
		if err := composition.bindCompositionPart[TrackPart](world, sprite, owner, states[index]); err != nil {
			return err
		}
	}
	return nil
}

func (composition ControlComposition) bindSelector(world *ecs.World, owner ecs.Entity, node CompositionNode, bindButton func(ecs.Entity) error) error {
	bodies := make([]ecs.Entity, 0, 3)
	var list ecs.Entity
	for _, child := range composition.directChildren(world, owner) {
		childNode := composition[child]
		switch childNode.Kind {
		case CompositionSprite:
			if compositionSelectorBodySprite(node, childNode) {
				bodies = append(bodies, child)
			} else if err := composition.bindCompositionPart[IconPart](world, child, owner, VariantAlways); err != nil {
				return err
			}
		case CompositionText:
			if err := composition.bindCompositionPart[LabelPart](world, child, owner, VariantAlways); err != nil {
				return err
			}
		case CompositionGroup:
			list = child
		}
	}
	if err := composition.bindBodies(world, owner, bodies, selectorBodyStates); err != nil {
		return err
	}
	if len(bodies) > 0 {
		body := composition[bodies[0]]
		SetControlBounds(world, owner, body.Width, body.Height)
	}
	if list == 0 {
		return nil
	}
	// Authored selector lists are hidden in the editor. Visibility is runtime
	// state for this part, so it must not disable the open variant.
	listNode := composition[list]
	listNode.Visible = true
	composition[list] = listNode
	if err := composition.bindCompositionPart[SelectorListPart](world, list, owner, VariantOpen); err != nil {
		return err
	}
	setVisible(world, list, false)
	setUIStackIndex(world, list, selectorPopupStackBase)
	width, _, _ := ControlBounds(world, owner)
	selector, _ := world.Get[Selector](owner)
	if err := world.Insert4(list, SelectorList{Selector: owner},
		scene.ClipRect2D{Width: width, Height: selector.ListHeight},
		scene.Size2D{Width: width, Height: selector.ListHeight},
		scene.UIScroll{ConsumeWheel: true, Vertical: true}); err != nil {
		return err
	}
	itemIndex := 0
	for _, child := range composition.directChildren(world, list) {
		switch composition[child].Kind {
		case CompositionSprite:
			if err := composition.bindCompositionPart[PanelPart](world, child, owner, VariantOpen); err != nil {
				return err
			}
			setVisible(world, child, false)
		case CompositionGroup:
			if bindButton == nil {
				return fmt.Errorf("UI control composition requires a nested button binder")
			}
			if err := bindButton(child); err != nil {
				return err
			}
			if err := composition.bindCompositionPart[ItemPart](world, child, owner, VariantAlways); err != nil {
				return err
			}
			if err := world.Insert(child, SelectorItem{Owner: owner, Index: itemIndex, Label: composition.itemLabel(world, child)}); err != nil {
				return err
			}
			itemIndex++
		}
	}
	return nil
}

func (composition ControlComposition) itemLabel(world *ecs.World, item ecs.Entity) string {
	for _, child := range composition.directChildren(world, item) {
		node := composition[child]
		if node.Kind == CompositionText {
			return node.Text
		}
	}
	return ""
}

func (composition ControlComposition) bindProgress(world *ecs.World, owner ecs.Entity, bindButton func(ecs.Entity) error) error {
	sprites := make([]ecs.Entity, 0, 2)
	for _, child := range composition.directChildren(world, owner) {
		switch composition[child].Kind {
		case CompositionSprite:
			sprites = append(sprites, child)
		case CompositionGroup:
			if bindButton == nil {
				return fmt.Errorf("UI control composition requires a nested button binder")
			}
			if err := bindButton(child); err != nil {
				return err
			}
			if err := composition.bindCompositionPart[ThumbPart](world, child, owner, VariantAlways); err != nil {
				return err
			}
			if progress := world.GetMut[Progress](owner); progress != nil {
				progress.Thumb = child
			}
			if err := world.Insert(child, ProgressThumb{Progress: owner}); err != nil {
				return err
			}
		}
	}
	if len(sprites) > 0 {
		if err := composition.bindCompositionPart[TrackPart](world, sprites[0], owner, VariantAlways); err != nil {
			return err
		}
	}
	if len(sprites) > 1 {
		fill := sprites[1]
		if err := composition.bindCompositionPart[FillPart](world, fill, owner, VariantAlways); err != nil {
			return err
		}
		if progress := world.GetMut[Progress](owner); progress != nil {
			progress.Fill = fill
		}
		if err := world.Insert(fill, Relation[Progress]{Owner: owner}); err != nil {
			return err
		}
	}
	return nil
}

func (composition ControlComposition) bindScrollArea(world *ecs.World, owner ecs.Entity) error {
	parts := composition.directChildren(world, owner)
	for index, entity := range parts {
		if composition[entity].Kind != CompositionSprite {
			continue
		}
		var err error
		switch index {
		case 0:
			err = composition.bindCompositionPart[ScrollTrackHorizontalPart](world, entity, owner, VariantAlways)
		case 1:
			err = composition.bindCompositionPart[ScrollTrackVerticalPart](world, entity, owner, VariantAlways)
		case 2:
			err = composition.bindCompositionPart[ScrollThumbHorizontalPart](world, entity, owner, VariantAlways)
		case 3:
			err = composition.bindCompositionPart[ScrollThumbVerticalPart](world, entity, owner, VariantAlways)
		}
		if err != nil {
			return err
		}
		if index < 4 {
			if err := world.Insert(entity, scene.ClipExempt2D{}); err != nil {
				return err
			}
		}
	}
	return nil
}
