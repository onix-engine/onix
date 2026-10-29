package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func syncAuthoredTextPositions(world *ecs.World) {
	for entity, access := range world.Query3[Relation[Control], scene.Text2D, scene.UITransform]().All {
		if !func(entity ecs.Entity, elementAccess ecs.ComponentAccess[Relation[Control]], textAccess ecs.ComponentAccess[scene.Text2D], transformAccess ecs.ComponentAccess[scene.UITransform]) bool {
			element, text := elementAccess.Ref(), textAccess.Ref()
			nextTransform := *transformAccess.Ref()
			transform := &nextTransform
			defer func() { ecs.CommitComponent(transformAccess, nextTransform) }()
			if !world.Has[Part[ValuePart]](entity) {
				return true
			}
			textarea, ok := world.Get[Textarea](element.Owner)
			if ok {
				if control, exists := controlViewFor(world, element.Owner); exists {
					syncTextareaVisual(world, entity, element.Owner, &textarea, text, transform, control)
					world.SetIfChanged(element.Owner, textarea)
				}
			}
			return true
		}(entity, access.A, access.B, access.C) {
			break
		}
	}
}

func syncAuthoredControlParts(world *ecs.World) {
	ensureTextFieldCaches(world)
	for entity, access := range world.Query3[Relation[Control], scene.UITransform, scene.Sprite2D]().All {
		if !func(entity ecs.Entity, elementAccess ecs.ComponentAccess[Relation[Control]], transformAccess ecs.ComponentAccess[scene.UITransform], spriteAccess ecs.ComponentAccess[scene.Sprite2D]) bool {
			element := elementAccess.Ref()
			nextTransform, nextSprite := *transformAccess.Ref(), *spriteAccess.Ref()
			transform, sprite := &nextTransform, &nextSprite
			defer func() {
				ecs.CommitComponent(transformAccess, nextTransform)
				ecs.CommitComponent(spriteAccess, nextSprite)
			}()
			switch {
			case world.Has[Part[CaretPart]](entity):
				syncAuthoredCaret(world, entity, element.Owner, transform, sprite)
			case world.Has[Part[FillPart]](entity):
				syncAuthoredProgressFill(world, element.Owner, sprite)
			case isScrollPart(world, entity):
				syncAuthoredScrollElement(world, entity, element.Owner, transform, sprite)
			}
			return true
		}(entity, access.A, access.B, access.C) {
			break
		}
	}
	for entity, access := range world.Query3[Relation[Control], scene.SpriteLayers2D, scene.Visibility]().All {
		elementAccess := access.A
		layersAccess := access.B
		visibilityAccess := access.C

		if world.Has[Part[SelectionPart]](entity) {
			syncAuthoredSelectionLayers(world, entity, elementAccess.Ref().Owner, layersAccess, visibilityAccess)
		}
	}
}

func syncAuthoredCaret(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, sprite *scene.Sprite2D) {
	r, ok := textCaretRect(world, owner)
	if ok {
		transform.Translation.X.Value, transform.Translation.Y.Value = r.X, r.Y
		sprite.Width, sprite.Height = 1, r.H
	}
	if visibility, exists := world.Get[scene.Visibility](entity); exists {
		visibility.Visible = visibility.Visible && ok && caretVisible(world)
		world.SetIfChanged(entity, visibility)
	}
}

func syncAuthoredSelectionLayers(world *ecs.World, entity, owner ecs.Entity, layersAccess ecs.ComponentAccess[scene.SpriteLayers2D], visibilityAccess ecs.ComponentAccess[scene.Visibility]) {
	text, textTransform, ok := textFieldElement(world, owner)
	if !ok {
		ecs.CommitComponent(visibilityAccess, scene.Visibility{})
		return
	}
	value := textFieldValue(world, owner)
	if field, exists := world.Get[TextField](owner); exists && field.Password {
		value = maskedTextFieldValue(world, owner, field)
	}
	start, end, focused := textFieldSelection(world, owner)
	viewportHeight := textFieldViewportHeight(world, owner)
	visibleLayers := 0
	layers := layersAccess.Ref()
	changed := false
	for slot := range layers.Layers {
		r, exists := textSelectionRect(text, textTransform, value, start, end, slot, viewportHeight)
		layer := layers.Layers[slot]
		layer.Visible = focused && exists
		if exists {
			layer.Transform.X, layer.Transform.Y = r.X, r.Y
			layer.Sprite.Width, layer.Sprite.Height = r.W, r.H
			visibleLayers++
		}
		if layer != layers.Layers[slot] {
			if !changed {
				layers = layersAccess.Mut()
				changed = true
			}
			layers.Layers[slot] = layer
		}
	}
	if changed {
		layers.InvalidateMatrices()
	}
	visibility := visibilityAccess.Get()
	visibility.Visible = visibility.Visible && focused && visibleLayers > 0
	ecs.CommitComponent(visibilityAccess, visibility)
	world.SetIfChanged(entity, scene.Size2D{Width: 1, Height: 1})
}

func textCaretRect(world *ecs.World, owner ecs.Entity) (selectionRect, bool) {
	text, transform, ok := textFieldElement(world, owner)
	if !ok || text.Font == nil {
		return selectionRect{}, false
	}
	value, composition, caret, focused := textFieldState(world, owner)
	field, fieldExists := world.Get[TextField](owner)
	caret = max(0, min(caret, runeCount(value)))
	if !fieldExists {
		field = TextField{Value: value, Composition: composition, Caret: caret, Focused: focused}
	}
	caretValue := value
	if field.Password {
		caretValue = maskedTextFieldPrefix(world, owner, field, caret)
	} else if composition != "" {
		caretValue = textFieldCompositionPrefix(world, owner, field, caret)
	}
	x, y, width, height, measured := scene.TextCaretRect2D(text, caretValue, runeCount(caretValue))
	if !measured {
		return selectionRect{}, false
	}
	return selectionRect{
		X: transform.Translation.X.Value + x,
		Y: transform.Translation.Y.Value + y,
		W: width,
		H: height,
	}, true
}

func textFieldElement(world *ecs.World, owner ecs.Entity) (*scene.Text2D, *scene.UITransform, bool) {
	entity, ok := FindPart[ValuePart, scene.Text2D](world, owner)
	if !ok || !world.Has[Relation[Control]](entity) {
		return nil, nil, false
	}
	text := world.GetRef[scene.Text2D](entity)
	transform := world.GetRef[scene.UITransform](entity)
	return text, transform, text != nil && transform != nil
}

func syncAuthoredProgressFill(world *ecs.World, owner ecs.Entity, sprite *scene.Sprite2D) {
	progress := world.GetRef[Progress](owner)
	control, controlOK := controlViewFor(world, owner)
	if progress == nil || !controlOK {
		return
	}
	width := control.Width * progressRatio(*progress)
	if progress.Indeterminate {
		width = control.Width * .35
	}
	sprite.Width, sprite.Height = width, control.Height
}

func isScrollPart(world *ecs.World, entity ecs.Entity) bool {
	return world.Has[Part[ScrollTrackHorizontalPart]](entity) ||
		world.Has[Part[ScrollTrackVerticalPart]](entity) ||
		world.Has[Part[ScrollThumbHorizontalPart]](entity) ||
		world.Has[Part[ScrollThumbVerticalPart]](entity)
}

func syncAuthoredScrollElement(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, sprite *scene.Sprite2D) {
	area := world.GetRef[ScrollArea](owner)
	if area == nil {
		return
	}
	metrics := measureScrollArea(area.Width, area.Height, area.ContentWidth, area.ContentHeight, area.BarSize)
	offset, _ := GetScrollAreaOffset(world, owner)
	barSize := scrollBarSize(area.BarSize)
	thumbBarSize := scrollThumbBarSize(area.ThumbBarSize, area.BarSize)
	hoverBarSize := scrollHoverBarSize(area.HoverBarSize, area.ThumbBarSize, area.BarSize)
	visible := false
	switch {
	case world.Has[Part[ScrollTrackHorizontalPart]](entity):
		visible = metrics.HorizontalVisible
		transform.Translation.X.Value, transform.Translation.Y.Value = 0, max(area.Height-barSize, 0)
		sprite.Width, sprite.Height = metrics.ViewportWidth, barSize
	case world.Has[Part[ScrollTrackVerticalPart]](entity):
		visible = metrics.VerticalVisible
		transform.Translation.X.Value, transform.Translation.Y.Value = max(area.Width-barSize, 0), 0
		sprite.Width, sprite.Height = barSize, metrics.ViewportHeight
	case world.Has[Part[ScrollThumbHorizontalPart]](entity):
		visible = metrics.HorizontalVisible
		height := thumbBarSize
		if area.HoverHorizontal || area.DraggingHorizontal {
			height = hoverBarSize
		}
		width := scrollThumbSize(metrics.ViewportWidth, metrics.ViewportWidth, area.ContentWidth, area.MinThumbSize)
		transform.Translation.X.Value = scrollThumbOffset(metrics.ViewportWidth, width, offset.X(), metrics.MaxScrollX)
		transform.Translation.Y.Value = max(area.Height-barSize, 0) + (barSize-height)*.5
		sprite.Width, sprite.Height = width, height
	case world.Has[Part[ScrollThumbVerticalPart]](entity):
		visible = metrics.VerticalVisible
		width := thumbBarSize
		if area.HoverVertical || area.DraggingVertical {
			width = hoverBarSize
		}
		height := scrollThumbSize(metrics.ViewportHeight, metrics.ViewportHeight, area.ContentHeight, area.MinThumbSize)
		transform.Translation.X.Value = max(area.Width-barSize, 0) + (barSize-width)*.5
		transform.Translation.Y.Value = scrollThumbOffset(metrics.ViewportHeight, height, offset.Y(), metrics.MaxScrollY)
		sprite.Width, sprite.Height = width, height
	}
	if visibility, ok := world.Get[scene.Visibility](entity); ok {
		visibility.Visible = visibility.Visible && visible
		world.SetIfChanged(entity, visibility)
	}
}
