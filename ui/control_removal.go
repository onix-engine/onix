package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

var compositionPartTypes = [...]ecs.ComponentType{
	ecs.Type[Part[BodyPart]](),
	ecs.Type[Part[LabelPart]](),
	ecs.Type[Part[IconPart]](),
	ecs.Type[Part[ValuePart]](),
	ecs.Type[Part[ItemPart]](),
	ecs.Type[Part[TrackPart]](),
	ecs.Type[Part[KnobPart]](),
	ecs.Type[Part[FillPart]](),
	ecs.Type[Part[ThumbPart]](),
	ecs.Type[Part[PanelPart]](),
	ecs.Type[Part[CaretPart]](),
	ecs.Type[Part[SelectionPart]](),
	ecs.Type[Part[FrameBorderPart]](),
	ecs.Type[Part[IncrementPart]](),
	ecs.Type[Part[DecrementPart]](),
	ecs.Type[Part[NumberInputBorderPart]](),
	ecs.Type[Part[SelectorListPart]](),
	ecs.Type[Part[SelectorPanelBorderPart]](),
	ecs.Type[Part[ScrollTrackHorizontalPart]](),
	ecs.Type[Part[ScrollTrackVerticalPart]](),
	ecs.Type[Part[ScrollThumbHorizontalPart]](),
	ecs.Type[Part[ScrollThumbVerticalPart]](),
	ecs.Type[Part[VirtualJoystickTrackPart]](),
	ecs.Type[Part[VirtualJoystickKnobPart]](),
}

type controlPartsScratch struct{}

type controlRemovalCache struct {
	root             ecs.PreparedRemoval
	parts            [4]ecs.PreparedRemoval
	rootInitialized  bool
	partsInitialized [4]bool
}

func controlRemovalFor(world *ecs.World) ecs.PreparedRemoval {
	cache := ecs.FrameValue[controlRemovalCache, controlRemovalCache](world)
	if !cache.rootInitialized {
		types := append([]ecs.ComponentType{
			ecs.Type[Control](), ecs.Type[scene.InputTarget2D](), ecs.Type[scene.InputState2D](),
			ecs.Type[Variants[ControlVariant]](), ecs.Type[Button](), ecs.Type[Toggle[bool]](),
			ecs.Type[TextField](), ecs.Type[TextFieldKeyHandler](), ecs.Type[Input](), ecs.Type[Textarea](), ecs.Type[textFieldMaskCache](),
			ecs.Type[NumberInput](), ecs.Type[Selector](), ecs.Type[SelectorList](), ecs.Type[SelectorItem](),
			ecs.Type[Progress](), ecs.Type[VirtualJoystick](), ecs.Type[ProgressThumb](),
			ecs.Type[ScrollArea](), ecs.Type[ScrollContent](), ecs.Type[Content[string]](), ecs.Type[Typography](),
			ecs.Type[NumberInputStep](), ecs.Type[Relation[Control]](), ecs.Type[Relation[scene.Text2D]](),
			ecs.Type[Relation[SurfaceStyle]](), ecs.Type[Relation[Progress]](),
			ecs.Type[scene.ClipRect2D](), ecs.Type[scene.ClipExempt2D](),
		}, compositionPartTypes[:]...)
		cache.root = ecs.PrepareRemoval(world, types...)
		cache.rootInitialized = true
	}
	return cache.root
}

func compositionRemovalFor(world *ecs.World, clipFlags int) ecs.PreparedRemoval {
	cache := ecs.FrameValue[controlRemovalCache, controlRemovalCache](world)
	if !cache.partsInitialized[clipFlags] {
		types := append([]ecs.ComponentType{
			ecs.Type[NumberInputStep](), ecs.Type[SelectorList](), ecs.Type[SelectorItem](),
			ecs.Type[ProgressThumb](), ecs.Type[Relation[Progress]](), ecs.Type[Relation[Control]](),
			ecs.Type[Relation[scene.Text2D]](), ecs.Type[Relation[SurfaceStyle]](),
		}, compositionPartTypes[:]...)
		if clipFlags&1 != 0 {
			types = append(types, ecs.Type[scene.ClipRect2D]())
		}
		if clipFlags&2 != 0 {
			types = append(types, ecs.Type[scene.ClipExempt2D]())
		}
		cache.parts[clipFlags] = ecs.PrepareRemoval(world, types...)
		cache.partsInitialized[clipFlags] = true
	}
	return cache.parts[clipFlags]
}
