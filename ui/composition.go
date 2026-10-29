package ui

import (
	"fmt"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

// Variant associates an entity with one strongly typed owner state.
type Variant[T comparable] struct {
	Entity  ecs.Entity
	State   T
	Enabled bool
}

// Variants stores the authored entities controlled by one state domain.
type Variants[T comparable] struct {
	Items []Variant[T]
}

type ControlVariant uint8

const (
	VariantAlways ControlVariant = iota
	VariantInputNormal
	VariantUnfocusedNormal
	VariantClosedNormal
	VariantHovered
	VariantUnfocusedHovered
	VariantClosedHovered
	VariantPressed
	VariantFocused
	VariantChecked
	VariantOpen
	VariantOpenEnabled
)

// Relation links a control part to its owner. T identifies the relation's
// domain, while semantic purpose is represented by a separate Part marker.
type Relation[T any] struct {
	Owner ecs.Entity
}

// Part is a zero-sized, strongly typed semantic marker.
type Part[T any] struct{}

// Indexed adds an ordinal only to part kinds that have multiple instances.
type Indexed[T any] struct {
	Value int
}

type (
	BodyPart                  struct{}
	TitlePart                 struct{}
	LabelPart                 struct{}
	IconPart                  struct{}
	ValuePart                 struct{}
	ItemPart                  struct{}
	TrackPart                 struct{}
	KnobPart                  struct{}
	FillPart                  struct{}
	ThumbPart                 struct{}
	PanelPart                 struct{}
	CaretPart                 struct{}
	SelectionPart             struct{}
	TreeTextPart              struct{}
	TreeIndicatorTextPart     struct{}
	TreeIndicatorImagePart    struct{}
	IncrementPart             struct{}
	DecrementPart             struct{}
	SelectorListPart          struct{}
	FrameBorderPart           struct{}
	TabItemBorderPart         struct{}
	NumberInputBorderPart     struct{}
	NumberStepIconPart        struct{}
	SelectorIconPart          struct{}
	SelectorPanelPart         struct{}
	SelectorPanelBorderPart   struct{}
	SelectorScrollTrackPart   struct{}
	SelectorScrollThumbPart   struct{}
	MenuPanelPart             struct{}
	ContextMenuPanelPart      struct{}
	ScrollTrackHorizontalPart struct{}
	ScrollTrackVerticalPart   struct{}
	ScrollThumbHorizontalPart struct{}
	ScrollThumbVerticalPart   struct{}
	VirtualJoystickTrackPart  struct{}
	VirtualJoystickKnobPart   struct{}
)

type ControlOptions struct {
	X, Y          float32
	Width, Height float32
	StackIndex    int
	Disabled      bool
	PassThrough   bool
}

type controlBinding interface {
	bindControl(*ecs.World, ecs.Entity, ControlOptions) error
}

func (control TextControl[T]) bindControl(world *ecs.World, entity ecs.Entity, opts ControlOptions) error {
	_, multiline := any(*new(T)).(Textarea)
	password := control.Password && !multiline
	field := newTextField(control.Value, control.Placeholder, password, control.MaxRunes, control.FontSize, control.DPI)
	if err := world.Insert(entity, field); err != nil {
		return err
	}
	switch any(*new(T)).(type) {
	case Input:
		world.Remove[Textarea](entity)
		if err := world.Insert(entity, Input{}); err != nil {
			return err
		}
		if err := world.Insert(entity, scene.ClipRect2D{Width: opts.Width, Height: opts.Height}); err != nil {
			return err
		}
		if password {
			return world.Insert(entity, textFieldMaskCache{})
		}
		world.Remove[textFieldMaskCache](entity)
	case Textarea:
		world.Remove[Input](entity)
		world.Remove[textFieldMaskCache](entity)
		if err := world.Insert(entity, Textarea{Wrap: control.Wrap, Padding: control.Padding}); err != nil {
			return err
		}
		return world.Insert(entity, scene.ClipRect2D{Width: opts.Width, Height: opts.Height})
	}
	return nil
}

func insertControlBinding[T any](world *ecs.World, entity ecs.Entity, opts ControlOptions, component T) error {
	if binding, ok := any(component).(controlBinding); ok {
		return binding.bindControl(world, entity, opts)
	}
	return world.Insert(entity, component)
}

// SpawnControl creates a behavior-only control root. Parts are ordinary
// child entities linked by the behavior that owns them.
func SpawnControl[T any](world *ecs.World, opts ControlOptions, component T) ecs.Entity {
	entity := spawnInteractiveAnchor(world, opts.X, opts.Y, opts.Width, opts.Height, opts.StackIndex, opts.Disabled, opts.PassThrough)
	_ = insertControlBinding(world, entity, opts, component)
	return entity
}

// BindControl installs reusable control behavior on an existing static entity.
func BindControl[T any](world *ecs.World, entity ecs.Entity, opts ControlOptions, component T) error {
	if world == nil || entity == 0 {
		return fmt.Errorf("UI control target is invalid")
	}
	if !world.Has[Control](entity) {
		if err := world.Insert(entity, Control{}); err != nil {
			return err
		}
	}
	target := scene.InputTarget2D{
		Width: opts.Width, Height: opts.Height,
		Disabled: opts.Disabled, PassThrough: opts.PassThrough,
	}
	if err := world.Insert(entity, target); err != nil {
		return err
	}
	if !world.Has[scene.InputState2D](entity) {
		if err := world.Insert(entity, scene.InputState2D{}); err != nil {
			return err
		}
	}
	if err := world.Insert(entity, scene.Size2D{Width: opts.Width, Height: opts.Height}); err != nil {
		return err
	}
	if err := insertControlBinding(world, entity, opts, component); err != nil {
		return err
	}
	setUIStackIndex(world, entity, opts.StackIndex)
	return nil
}

func BindVariant[T comparable](world *ecs.World, owner ecs.Entity, variant Variant[T]) error {
	if variant.Entity == 0 {
		return fmt.Errorf("UI variant entity is zero")
	}
	if variants := world.GetMut[Variants[T]](owner); variants != nil {
		for index := range variants.Items {
			if variants.Items[index].Entity == variant.Entity {
				variants.Items[index] = variant
				return nil
			}
		}
		variants.Items = append(variants.Items, variant)
		return nil
	}
	return world.Insert(owner, Variants[T]{Items: []Variant[T]{variant}})
}

func UnbindVariant[T comparable](world *ecs.World, owner, entity ecs.Entity) {
	variants := world.GetMut[Variants[T]](owner)
	if variants == nil {
		return
	}
	for index := range variants.Items {
		if variants.Items[index].Entity != entity {
			continue
		}
		variants.Items = append(variants.Items[:index], variants.Items[index+1:]...)
		break
	}
	if len(variants.Items) == 0 {
		world.Remove[Variants[T]](owner)
	}
}

func BindRelation[T any](world *ecs.World, entity, owner ecs.Entity) error {
	return world.Insert(entity, Relation[T]{Owner: owner})
}

func BindPart[T any](world *ecs.World, entity ecs.Entity) error {
	if world.Has[Part[T]](entity) {
		return nil
	}
	return world.Insert(entity, Part[T]{})
}

func FindPart[P, T any](world *ecs.World, owner ecs.Entity) (ecs.Entity, bool) {
	result := partEntities[P, T](world)[owner]
	return result, result != 0
}

type partIndex[P, T any] struct {
	entities    map[ecs.Entity]ecs.Entity
	versions    [2]uint64
	initialized bool
}

// Cache entities, never component pointers. Column migration changes these
// versions too, preserving the first match in the current query order.
func partEntities[P, T any](world *ecs.World) map[ecs.Entity]ecs.Entity {
	index := world.FrameValue[partIndex[P, T], partIndex[P, T]]()
	versions := [2]uint64{world.ComponentVersion[Relation[T]](), world.ComponentVersion[Part[P]]()}
	if index.initialized && index.versions == versions {
		return index.entities
	}
	if index.entities == nil {
		index.entities = make(map[ecs.Entity]ecs.Entity)
	}
	clear(index.entities)
	for entity, access := range world.Query[Relation[T]](ecs.With[Part[P]]()).All {
		relation := access.Get()

		if _, exists := index.entities[relation.Owner]; !exists {
			index.entities[relation.Owner] = entity
		}
	}
	index.versions, index.initialized = versions, true
	return index.entities
}

func syncVariants[T comparable](world *ecs.World, matches func(*ecs.World, ecs.Entity, T) bool) {
	for entity, access := range world.Query[Variants[T]]().All {
		variants := access.Get()

		ownerVisible := hierarchyVisible(world, entity)
		for _, variant := range variants.Items {
			setVisible(world, variant.Entity, ownerVisible && variant.Enabled && matches(world, entity, variant.State))
		}
	}
}

func syncControlVariants(world *ecs.World) {
	syncVariants(world, controlVariantMatches)
}

func controlVariantMatches(world *ecs.World, entity ecs.Entity, variant ControlVariant) bool {
	pointer := world.GetRef[scene.InputState2D](entity)
	target := world.GetRef[scene.InputTarget2D](entity)
	field := world.GetRef[TextField](entity)
	toggle := world.GetRef[Toggle[bool]](entity)
	menu := world.GetRef[Menu](entity)
	selector := world.GetRef[Selector](entity)
	hovered := pointer != nil && pointer.Hovered
	pressed := pointer != nil && pointer.Pressed
	disabled := target != nil && target.Disabled
	focused := field != nil && field.Focused
	checked := toggle != nil && toggle.Value
	open := menu != nil && menu.Open || selector != nil && selector.Open
	switch variant {
	case VariantAlways:
		return true
	case VariantInputNormal:
		return !hovered && !pressed
	case VariantUnfocusedNormal:
		return !hovered && !pressed && !focused
	case VariantClosedNormal:
		return !hovered && !pressed && !open
	case VariantHovered:
		return hovered && !pressed && !disabled
	case VariantUnfocusedHovered:
		return hovered && !pressed && !focused && !disabled
	case VariantClosedHovered:
		return hovered && !pressed && !open && !disabled
	case VariantPressed:
		return pressed && !disabled
	case VariantFocused:
		return focused && !disabled
	case VariantChecked:
		return checked
	case VariantOpen:
		return open
	case VariantOpenEnabled:
		return open && !disabled
	default:
		return false
	}
}
