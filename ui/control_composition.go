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
	if ecs.Has[TextField](world, owner) {
		return composition.bindTextField(world, owner, node, bindButton)
	}
	switch {
	case ecs.Has[Button](world, owner):
		return composition.bindButton(world, owner, node)
	case ecs.Has[Toggle[bool]](world, owner):
		return composition.bindCheckbox(world, owner)
	case ecs.Has[Selector](world, owner):
		return composition.bindSelector(world, owner, node, bindButton)
	case ecs.Has[Progress](world, owner):
		return composition.bindProgress(world, owner, bindButton)
	case ecs.Has[ScrollArea](world, owner):
		return composition.bindScrollArea(world, owner)
	default:
		return nil
	}
}

func bindCompositionPart[P any](world *ecs.World, composition ControlComposition, entity, owner ecs.Entity, state ControlVariant) error {
	if err := BindRelation[Control](world, entity, owner); err != nil {
		return err
	}
	if err := BindPart[P](world, entity); err != nil {
		return err
	}
	if ecs.Has[scene.Text2D](world, entity) {
		if err := BindRelation[scene.Text2D](world, entity, owner); err != nil {
			return err
		}
	}
	if ecs.Has[scene.UIBackground2D](world, entity) || ecs.Has[scene.UIImage2D](world, entity) || ecs.Has[scene.UIBorder2D](world, entity) {
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
	relation, ok := ecs.Get[Relation[Control]](world, entity)
	if !ok {
		return
	}
	if ecs.Has[Part[FillPart]](world, entity) {
		if progress := ecs.GetMut[Progress](world, relation.Owner); progress != nil && progress.Fill == entity {
			progress.Fill = 0
		}
	}
	if ecs.Has[Part[ThumbPart]](world, entity) {
		if progress := ecs.GetMut[Progress](world, relation.Owner); progress != nil && progress.Thumb == entity {
			progress.Thumb = 0
		}
	}
	clipFlags := 0
	if ecs.Has[Part[SelectorListPart]](world, entity) {
		clipFlags |= 1
	}
	if ecs.Has[Part[ScrollTrackHorizontalPart]](world, entity) ||
		ecs.Has[Part[ScrollTrackVerticalPart]](world, entity) ||
		ecs.Has[Part[ScrollThumbHorizontalPart]](world, entity) ||
		ecs.Has[Part[ScrollThumbVerticalPart]](world, entity) {
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
	parts := ecs.FrameScratch[controlPartsScratch, ecs.Entity](world)
	for part, access := range ecs.Query[Relation[Control]](world).All {
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
		parent, ok := ecs.Get[scene.Parent](world, entity)
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
			} else if err := bindCompositionPart[IconPart](world, composition, child, owner, VariantAlways); err != nil {
				return err
			}
		case CompositionText:
			switch {
			case ecs.Has[NumberInputStep](world, owner):
				if err := bindCompositionPart[IconPart](world, composition, child, owner, VariantAlways); err != nil {
					return err
				}
			case ecs.Has[SelectorItem](world, owner):
				if err := bindCompositionPart[ItemPart](world, composition, child, owner, VariantAlways); err != nil {
					return err
				}
			default:
				if err := bindCompositionPart[LabelPart](world, composition, child, owner, VariantAlways); err != nil {
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
		if err := bindCompositionPart[BodyPart](world, composition, body, owner, state); err != nil {
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
			if err := bindCompositionPart[ValuePart](world, composition, child, owner, VariantAlways); err != nil {
				return err
			}
		case CompositionGroup:
			if ecs.Has[NumberInput](world, owner) {
				steps = append(steps, child)
			}
		}
	}
	if err := composition.bindBodies(world, owner, bodies, textFieldBodyStates); err != nil {
		return err
	}
	if len(decorations) > 0 {
		if err := bindCompositionPart[SelectionPart](world, composition, decorations[0], owner, VariantFocused); err != nil {
			return err
		}
		if err := linkCompositionSelection(world, decorations[0]); err != nil {
			return err
		}
	}
	if len(decorations) > 1 {
		if err := bindCompositionPart[CaretPart](world, composition, decorations[1], owner, VariantFocused); err != nil {
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
			if err := bindCompositionPart[IncrementPart](world, composition, step, owner, VariantAlways); err != nil {
				return err
			}
		} else if err := bindCompositionPart[DecrementPart](world, composition, step, owner, VariantAlways); err != nil {
			return err
		}
		if err := ecs.Insert(world, step, NumberInputStep{Input: owner, Direction: direction}); err != nil {
			return err
		}
	}
	return nil
}

func linkCompositionSelection(world *ecs.World, entity ecs.Entity) error {
	if ecs.Has[scene.SpriteLayers2D](world, entity) {
		return nil
	}
	sprite, ok := ecs.Get[scene.Sprite2D](world, entity)
	if !ok {
		return nil
	}
	anchor, _ := ecs.Get[scene.Anchor](world, entity)
	layers := make([]scene.SpriteLayer2D, 16)
	for index := range layers {
		layers[index] = scene.SpriteLayer2D{ID: selectionLayerIDs[index], Order: index, Sprite: sprite, Anchor: anchor}
	}
	value := scene.SpriteLayers2D{Layers: layers}
	value.InvalidateMatrices()
	cache := ecs.FrameValue[selectionPatchCache, selectionPatchCache](world)
	if !cache.initialized {
		cache.patch = ecs.NewPatch2[scene.SpriteLayers2D, scene.Size2D](world, ecs.Type[scene.Sprite2D]())
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
			if err := bindCompositionPart[LabelPart](world, composition, child, owner, VariantAlways); err != nil {
				return err
			}
		}
	}
	states := []ControlVariant{VariantInputNormal, VariantHovered, VariantPressed}
	for index, sprite := range sprites {
		if index >= len(states) {
			if err := bindCompositionPart[KnobPart](world, composition, sprite, owner, VariantChecked); err != nil {
				return err
			}
			continue
		}
		if err := bindCompositionPart[TrackPart](world, composition, sprite, owner, states[index]); err != nil {
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
			} else if err := bindCompositionPart[IconPart](world, composition, child, owner, VariantAlways); err != nil {
				return err
			}
		case CompositionText:
			if err := bindCompositionPart[LabelPart](world, composition, child, owner, VariantAlways); err != nil {
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
	if err := bindCompositionPart[SelectorListPart](world, composition, list, owner, VariantOpen); err != nil {
		return err
	}
	setVisible(world, list, false)
	setUIStackIndex(world, list, selectorPopupStackBase)
	width, _, _ := ControlBounds(world, owner)
	selector, _ := ecs.Get[Selector](world, owner)
	if err := ecs.Insert4(world, list, SelectorList{Selector: owner},
		scene.ClipRect2D{Width: width, Height: selector.ListHeight},
		scene.Size2D{Width: width, Height: selector.ListHeight},
		scene.UIScroll{ConsumeWheel: true, Vertical: true}); err != nil {
		return err
	}
	itemIndex := 0
	for _, child := range composition.directChildren(world, list) {
		switch composition[child].Kind {
		case CompositionSprite:
			if err := bindCompositionPart[PanelPart](world, composition, child, owner, VariantOpen); err != nil {
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
			if err := bindCompositionPart[ItemPart](world, composition, child, owner, VariantAlways); err != nil {
				return err
			}
			if err := ecs.Insert(world, child, SelectorItem{Owner: owner, Index: itemIndex, Label: composition.itemLabel(world, child)}); err != nil {
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
			if err := bindCompositionPart[ThumbPart](world, composition, child, owner, VariantAlways); err != nil {
				return err
			}
			if progress := ecs.GetMut[Progress](world, owner); progress != nil {
				progress.Thumb = child
			}
			if err := ecs.Insert(world, child, ProgressThumb{Progress: owner}); err != nil {
				return err
			}
		}
	}
	if len(sprites) > 0 {
		if err := bindCompositionPart[TrackPart](world, composition, sprites[0], owner, VariantAlways); err != nil {
			return err
		}
	}
	if len(sprites) > 1 {
		fill := sprites[1]
		if err := bindCompositionPart[FillPart](world, composition, fill, owner, VariantAlways); err != nil {
			return err
		}
		if progress := ecs.GetMut[Progress](world, owner); progress != nil {
			progress.Fill = fill
		}
		if err := ecs.Insert(world, fill, Relation[Progress]{Owner: owner}); err != nil {
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
			err = bindCompositionPart[ScrollTrackHorizontalPart](world, composition, entity, owner, VariantAlways)
		case 1:
			err = bindCompositionPart[ScrollTrackVerticalPart](world, composition, entity, owner, VariantAlways)
		case 2:
			err = bindCompositionPart[ScrollThumbHorizontalPart](world, composition, entity, owner, VariantAlways)
		case 3:
			err = bindCompositionPart[ScrollThumbVerticalPart](world, composition, entity, owner, VariantAlways)
		}
		if err != nil {
			return err
		}
		if index < 4 {
			if err := ecs.Insert(world, entity, scene.ClipExempt2D{}); err != nil {
				return err
			}
		}
	}
	return nil
}
