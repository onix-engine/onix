package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

type buttonLabelWidthsScratch struct{}
type selectorLabelsScratch struct{}

func collectButtonLabelWidths(world *ecs.World) map[ecs.Entity]float32 {
	widths := world.FrameScratchMap[buttonLabelWidthsScratch, ecs.Entity, float32]()
	for _, access := range world.Query3[Relation[scene.Text2D], scene.Text2D, Part[LabelPart]]().All {
		relation := access.A.Ref()
		text := access.B.Ref()

		if _, exists := widths[relation.Owner]; !exists {
			widths[relation.Owner], _, _ = scene.MeasureTextValue2D(text, text.Value)
		}
	}
	return widths
}

func collectSelectorLabels(world *ecs.World) map[ecs.Entity]string {
	labels := world.FrameScratchMap[selectorLabelsScratch, ecs.Entity, string]()
	for _, access := range world.Query[SelectorItem]().All {
		item := access.Ref()

		if _, exists := labels[item.Owner]; exists {
			continue
		}
		if selector := world.GetRef[Selector](item.Owner); (selector != nil) && selector.Selected == item.Index {
			labels[item.Owner] = item.Label
		}
	}
	return labels
}

func selectorCurrentLabel(labels map[ecs.Entity]string, owner ecs.Entity, selector *Selector) string {
	if label, ok := labels[owner]; ok {
		return label
	}
	return selector.Label
}
