package ui

import (
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
	"github.com/onix-engine/onix/ui/html"
)

func syncForms(context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}
	ensureTextFieldCaches(world)
	syncCaretBlink(world)
	prepareScrollAreas(world)
	theme := controlTheme(world)
	syncHTMLFormText(world)
	metrics := formSelectorMetrics(world)
	prepareSelectors(world, metrics)
	syncFormVisuals(world, formSkinFor(world, theme), metrics)
	return nil
}

func syncFormsAfterLayout(context *ecs.SystemContext) error {
	if world := context.World; world != nil {
		finalizeScrollAreas(world)
		syncFormVisuals(world, formSkinFor(world, controlTheme(world)), formSelectorMetrics(world))
	}
	return nil
}

func syncFormVisuals(world *ecs.World, skin *Skin, metrics syncMetrics) {
	syncSelectorItems(world, metrics)
	syncControls(world)
	syncAuthoredTextPositions(world)
	syncControlSurfaces(world, skin, metrics, syncFormPart)
	syncTextVisibility(world)
	syncSurfaceElementVisibility(world)
	syncInputBlockers(world)
	syncCornerRadii(world)
}

func formSelectorMetrics(world *ecs.World) syncMetrics {
	counts := collectItemMetricCounts[Selector](world)
	for _, row := range world.Query2[Relation[scene.Text2D], scene.Text2D](ecs.With[Part[ItemPart]]()).All {
		owner := row.A.Ref().Owner
		if item := world.GetRef[SelectorItem](owner); item != nil {
			text := row.B.Ref()
			if width, _, ok := scene.MeasureTextValue2D(text, text.Text()); ok {
				updateItemMetricWidth(counts, item.Owner, width)
			}
		}
	}
	return syncMetrics{selectors: counts}
}

func syncFormPart(world *ecs.World, entity, owner ecs.Entity, transform scene.UITransform, _ scene.Size2D, skin *Skin, _ syncMetrics) (scene.UITransform, bool) {
	if !world.Has[Part[BodyPart]](entity) {
		return transform, false
	}
	item := world.GetRef[SelectorItem](owner)
	control, ok := controlViewFor(world, owner)
	if item != nil && ok {
		transform.Translation.X.Value, transform.Translation.Y.Value = 0, 0
		surface := selectorItemSurface(world, item.Owner, skin, controlState(&control, false))
		setUIStackIndex(world, entity, control.StackIndex)
		applySurface(world, entity, control.Width, control.Height, surface)
	}
	return transform, true
}

// DOM layout positions static labels and options. Only editable form values
// need the shared text-field geometry after their content has been updated.
func syncHTMLFormText(world *ecs.World) {
	for entity, row := range world.Query2[Relation[scene.Text2D], scene.Text2D](ecs.With[Relation[Control]]()).All {
		owner := row.A.Ref().Owner
		next := row.B.Get()
		syncAuthoredTextContent(world, entity, owner, &next, nil)
		commitControlText(row.B, next)
		if !world.Has[html.ControlText](entity) || !world.Has[TextField](owner) || world.Has[html.Editable](owner) {
			continue
		}
		transform, ok := world.Get[scene.UITransform](entity)
		if !ok {
			continue
		}
		if control, ok := controlViewFor(world, owner); ok && layoutFieldText(world, entity, owner, &transform, control) {
			world.SetIfChanged(entity, transform)
		}
	}
}

func applyHTMLScrollColors(world *ecs.World, entity ecs.Entity, computed html.ScrollStyle) {
	scrollStyle := formSkinFor(world, Theme{}).Scroll
	if computed.HasTrackColor {
		scrollStyle.Track = ColorSurface(computed.TrackColor)
	}
	if computed.HasThumbColor {
		thumb := ColorSurface(computed.ThumbColor)
		scrollStyle.Thumb.Normal = thumb
		scrollStyle.Thumb.Hovered = &thumb
	}
	if computed.HasThumbHoverColor {
		hovered := ColorSurface(computed.ThumbHoverColor)
		scrollStyle.Thumb.Hovered = &hovered
	}
	_ = world.Insert(entity, StyleOverride[ScrollStyle]{Value: scrollStyle})
}
