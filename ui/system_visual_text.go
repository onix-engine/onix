package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui/html"
)

func syncFrameBorder(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, size *scene.Size2D, skin Skin) {
	transform.Translation.X.Value, transform.Translation.Y.Value = 0, 0
	frame := SurfaceStyle{}
	stackIndex := 0
	switch {
	case world.Has[ButtonGroup](owner):
		group, _ := world.Get[ButtonGroup](owner)
		size.Width, size.Height = group.Width, group.Height
		stackIndex = group.StackIndex + 3
		frame = styleFor(world, owner, skin.ButtonGroup).Frame
	case world.Has[Tab](owner):
		tab, _ := world.Get[Tab](owner)
		size.Width, size.Height = tab.Width, max(tab.HeaderHeight, 0)
		if stack, ok := world.Get[scene.UIStackIndex](owner); ok {
			stackIndex = int(stack) + 3
		}
		frame = styleFor(world, owner, skin.Tab).Frame
	default:
		size.Width, size.Height = 0, 0
	}
	setUIStackIndex(world, entity, stackIndex)
	applySurface(world, entity, size.Width, size.Height, SurfaceStyle{
		Background: scene.RoundedRectShape2D(graphics.ColorRGBA{}, 0),
		Border:     frame.Border,
	})
}

func syncTabItemBorder(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, size *scene.Size2D, skin Skin) {
	item := world.GetRef[TabItem](owner)
	side := world.GetRef[TabItemBorder](entity)
	control, controlOK := controlViewFor(world, owner)
	if (item == nil) || (side == nil) || (!controlOK) {
		size.Width, size.Height = 0, 0
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	tabs := world.GetRef[Tab](item.Tabs)
	if tabs == nil || !tabs.ItemBorders {
		size.Width, size.Height = 0, 0
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	surface := controlSurface(world, owner, &control, skin)
	borderWidth := surface.Border.BorderWidth
	if borderWidth <= 0 || surface.Border.Border.A == 0 {
		size.Width, size.Height = 0, 0
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	width, height := control.Width, control.Height
	switch side.Side {
	case TabItemBorderTop:
		transform.Translation.X.Value, transform.Translation.Y.Value = 0, 0
		size.Width, size.Height = width, borderWidth
	case TabItemBorderRight:
		transform.Translation.X.Value, transform.Translation.Y.Value = width-borderWidth, 0
		size.Width, size.Height = borderWidth, height
	case TabItemBorderBottom:
		transform.Translation.X.Value, transform.Translation.Y.Value = 0, height-borderWidth
		size.Width, size.Height = width, borderWidth
	case TabItemBorderLeft:
		transform.Translation.X.Value, transform.Translation.Y.Value = 0, 0
		size.Width, size.Height = borderWidth, height
	default:
		size.Width, size.Height = 0, 0
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	setUIStackIndex(world, entity, control.StackIndex+3)
	applySurface(world, entity, size.Width, size.Height, ColorSurface(surface.Border.Border))
}

func syncIconElement(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, size *scene.Size2D, labelWidth float32) {
	control, ok := controlViewFor(world, owner)
	if !ok {
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	icon := IconStyle{}
	label := ""
	if iconContent := world.GetRef[IconContent](owner); iconContent != nil {
		icon = iconContent.Value
		if content := world.GetRef[Content[string]](owner); content != nil {
			label = content.Value
		}
	} else if item := world.GetRef[TabItem](owner); item != nil {
		icon, label = item.Icon, item.Label
	}
	width, height := iconTextureSize(icon)
	if icon.Texture == nil || width <= 0 || height <= 0 {
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	x := textIconX
	if label == "" {
		x = (control.Width - width) * 0.5
	} else if alignment, ok := world.Get[ButtonContentAlignment](owner); ok {
		x = buttonContentX(control.Width, width, labelWidth, alignment)
	}
	transform.Translation.X.Value = x
	transform.Translation.Y.Value = (control.Height - height) * 0.5
	image := scene.NewUIImage2D(icon.Texture)
	applySurface(world, entity, width, height, SurfaceStyle{Image: image})
	setUIStackIndex(world, entity, control.StackIndex+2)
}

func syncSelectorIcon(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, visualSize *scene.Size2D) {
	selector := world.GetRef[Selector](owner)
	control, controlOK := controlViewFor(world, owner)
	if selector == nil || !controlOK {
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	width, height := iconTextureSize(selector.Icon)
	if selector.Icon.Texture == nil || width <= 0 || height <= 0 {
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	transform.Translation.X.Value = control.Width - width - 12
	transform.Translation.Y.Value = (control.Height - height) * 0.5
	visualSize.Width = width
	visualSize.Height = height
	setUIStackIndex(world, entity, control.StackIndex+2)
	image := scene.NewUIImage2D(selector.Icon.Texture)
	applySurface(world, entity, visualSize.Width, visualSize.Height, SurfaceStyle{Image: image})
}

func syncSelectorPanel(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, size *scene.Size2D, skin Skin) {
	list := world.GetRef[SelectorList](owner)
	if list == nil {
		return
	}
	selector := world.GetRef[Selector](list.Selector)
	listSize := world.GetRef[scene.Size2D](owner)
	if selector == nil || listSize == nil {
		return
	}
	transform.Translation.X.Value, transform.Translation.Y.Value = 0, 0
	*size = *listSize
	setUIStackIndex(world, entity, selectorPopupStackBase+1)
	style := styleFor(world, list.Selector, skin.Selector)
	applySurface(world, entity, size.Width, size.Height, style.Popup)
}

func syncSelectorPanelBorder(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, size *scene.Size2D, skin Skin) {
	list := world.GetRef[SelectorList](owner)
	if list == nil {
		return
	}
	selector := world.GetRef[Selector](list.Selector)
	listSize := world.GetRef[scene.Size2D](owner)
	if selector == nil || listSize == nil {
		return
	}
	transform.Translation.X.Value, transform.Translation.Y.Value = 0, 0
	*size = *listSize
	setUIStackIndex(world, entity, selectorPopupStackBase+4)
	style := styleFor(world, list.Selector, skin.Selector)
	applySurface(world, entity, size.Width, size.Height, SurfaceStyle{
		Background: scene.RoundedRectShape2D(graphics.ColorRGBA{}, 0),
		Border:     style.Popup.Border,
	})
}

const (
	selectorScrollBarSize      float32 = 8
	selectorScrollThumbSize    float32 = 6
	selectorScrollMinThumbSize float32 = 20
)

func syncSelectorScrollElement(world *ecs.World, entity, owner ecs.Entity, thumb bool, transform *scene.UITransform, size *scene.Size2D, skin Skin, counts map[ecs.Entity]itemMetrics) {
	list := world.GetRef[SelectorList](owner)
	if list == nil {
		return
	}
	selector := world.GetRef[Selector](list.Selector)
	control, controlOK := controlViewFor(world, list.Selector)
	if selector == nil || !controlOK {
		return
	}
	contentHeight := selectorItemHeight(selector, &control) * float32(counts[list.Selector].count)
	listSize := world.GetRef[scene.Size2D](owner)
	listHeight := listSize.Height
	visible := contentHeight > listHeight && listHeight > 0
	style := skin.Scroll
	if !thumb {
		transform.Translation.X.Value = listSize.Width - selectorScrollBarSize
		transform.Translation.Y.Value = 0
		size.Width = visibleLength(visible, selectorScrollBarSize)
		size.Height = visibleLength(visible, listHeight)
		setUIStackIndex(world, entity, selectorPopupStackBase+3)
		applySurface(world, entity, size.Width, size.Height, style.Track)
	} else {
		maxScroll := max(contentHeight-listHeight, 0)
		thumbHeight := scrollThumbSize(listHeight, listHeight, contentHeight, selectorScrollMinThumbSize)
		transform.Translation.X.Value = listSize.Width - (selectorScrollBarSize+selectorScrollThumbSize)*.5
		scrollY := float32(0)
		if scroll, exists := world.Get[scene.UIScroll](owner); exists {
			scrollY = -scroll.Position.Y()
		}
		transform.Translation.Y.Value = scrollThumbOffset(listHeight, thumbHeight, scrollY, maxScroll)
		size.Width = visibleLength(visible, selectorScrollThumbSize)
		size.Height = visibleLength(visible, thumbHeight)
		setUIStackIndex(world, entity, selectorPopupStackBase+4)
		applySurface(world, entity, size.Width, size.Height, style.Thumb.Normal)
	}
}

func syncCaretElement(world *ecs.World, entity, owner, valueEntity ecs.Entity, transform *scene.UITransform, size *scene.Size2D, skin Skin) {
	value, composition, caret, focused := textFieldState(world, owner)
	field, fieldExists := world.Get[TextField](owner)
	valueText := world.GetRef[scene.Text2D](valueEntity)
	valueTransform, transformOK := world.Get[scene.UITransform](valueEntity)
	if valueText == nil || !transformOK || valueText.Font == nil {
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}

	caret = max(0, caret) // TextCaretRect2D clamps while locating the caret.
	caretValue := value
	caretIndex := caret
	if fieldExists && field.Password {
		caretValue = maskedTextFieldPrefix(world, owner, field, caret)
		caretIndex = runeCount(caretValue)
	} else if composition != "" {
		if !fieldExists {
			field = TextField{Value: value, Composition: composition, Caret: caret, Focused: focused}
		}
		caretValue = textFieldCompositionPrefix(world, owner, field, caret)
		caretIndex = runeCount(caretValue)
	}
	caretX, caretY, caretWidth, caretHeight, ok := scene.TextCaretRect2D(valueText, caretValue, caretIndex)
	if !ok {
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	transform.Translation.X.Value = valueTransform.Translation.X.Value + caretX
	transform.Translation.Y.Value = valueTransform.Translation.Y.Value + caretY
	size.Width = caretWidth
	size.Height = caretHeight
	applySurface(world, entity, size.Width, size.Height, textFieldStyleFor(world, owner, skin).Caret)
	if control, ok := controlViewFor(world, owner); ok {
		setUIStackIndex(world, entity, control.StackIndex+3)
	}
}

type selectionRect struct{ X, Y, W, H float32 }

func syncSelectionElement(world *ecs.World, entity, owner, valueEntity ecs.Entity, index int, transform *scene.UITransform, size *scene.Size2D, skin Skin) {
	value, _, _, _ := textFieldState(world, owner)
	start, end, _ := textFieldSelection(world, owner)
	valueText := world.GetRef[scene.Text2D](valueEntity)
	valueTransform, transformOK := world.Get[scene.UITransform](valueEntity)
	if valueText == nil || !transformOK || valueText.Font == nil {
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	if field, ok := world.Get[TextField](owner); ok && field.Password {
		value = maskedTextFieldValue(world, owner, field)
	}
	r, ok := textSelectionRect(valueText, &valueTransform, value, start, end, index, textFieldViewportHeight(world, owner))
	if !ok {
		size.Width, size.Height = 0, 0
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	transform.Translation.X.Value, transform.Translation.Y.Value = r.X, r.Y
	size.Width, size.Height = r.W, r.H
	applySurface(world, entity, size.Width, size.Height, textFieldStyleFor(world, owner, skin).Selection)
	if control, ok := controlViewFor(world, owner); ok {
		setUIStackIndex(world, entity, control.StackIndex+1)
	}
}

// textSelectionRect returns the rectangle of the slot-th painted row of a
// selection. Rows come from the text layout, so soft-wrapped lines and hard
// newlines share the caret's geometry, and rows outside the scroll window are
// skipped without consuming a selection surface.
func textSelectionRect(text *scene.Text2D, transform *scene.UITransform, value string, start, end, slot int, viewportHeight float32) (selectionRect, bool) {
	if start == end || slot < 0 {
		return selectionRect{}, false
	}
	start, end = orderedSelection(start, end)
	runeCount := utf8.RuneCountInString(value)
	start, end = max(0, min(start, runeCount)), max(0, min(end, runeCount))
	if start == end {
		return selectionRect{}, false
	}
	top := -transform.Translation.Y.Value
	cull, bottom := viewportHeight > 0, top+viewportHeight
	lineCount := scene.TextLineCount2D(text)
	matched, byteCursor, runeCursor := 0, 0, 0
	for index := range lineCount {
		lineStartByte, lineEndByte, rect, ok := scene.TextLineRange2D(text, index)
		if !ok || lineStartByte < byteCursor || lineEndByte > len(value) {
			return selectionRect{}, false
		}
		runeCursor += utf8.RuneCountInString(value[byteCursor:lineStartByte])
		byteCursor = lineStartByte
		lineStart, lineEnd := runeCursor, runeCursor+utf8.RuneCountInString(value[lineStartByte:lineEndByte])
		if lineStart >= end {
			return selectionRect{}, false
		}
		selectionStart, selectionEnd := max(start, lineStart), min(end, lineEnd)
		if selectionStart >= selectionEnd || cull && (rect.Y+rect.Height <= top || rect.Y >= bottom) {
			continue
		}
		if matched != slot {
			matched++
			continue
		}
		selectionStartByte, selectionEndByte := lineStartByte, lineEndByte
		if selectionStart > lineStart {
			selectionStartByte = runeByteIndex(value, selectionStart)
		}
		if selectionEnd < lineEnd {
			selectionEndByte = runeByteIndex(value, selectionEnd)
		}
		prefixWidth, _, _ := scene.MeasureTextValue2D(text, value[lineStartByte:selectionStartByte])
		selectionWidth, _, _ := scene.MeasureTextValue2D(text, value[selectionStartByte:selectionEndByte])
		return selectionRect{X: transform.Translation.X.Value + rect.X + prefixWidth, Y: transform.Translation.Y.Value + rect.Y,
			W: max(selectionWidth, 1), H: rect.Height}, true
	}
	return selectionRect{}, false
}

// textFieldViewportHeight is the scroll window a selection can paint inside.
func textFieldViewportHeight(world *ecs.World, owner ecs.Entity) float32 {
	if control, ok := world.Get[html.Control](owner); ok && control.PaddingBox.Height > 0 {
		return control.PaddingBox.Height
	}
	if size, ok := world.Get[scene.Size2D](owner); ok {
		return size.Height
	}
	return 0
}

func textFieldSelection(world *ecs.World, owner ecs.Entity) (int, int, bool) {
	if field, ok := world.Get[TextField](owner); ok {
		start, end := orderedSelection(field.Anchor, field.Caret)
		return start, end, field.Focused
	}
	return 0, 0, false
}

func textFieldState(world *ecs.World, owner ecs.Entity) (value, composition string, caret int, focused bool) {
	if field, ok := world.Get[TextField](owner); ok {
		return field.Value, field.Composition, field.Caret, field.Focused
	}
	return "", "", 0, false
}

type textFieldMaskCache struct {
	mask                         string
	valueRunes, compositionRunes int
	valid                        bool
}

type textFieldDisplayCache struct {
	value, placeholder, composition  string
	caret                            int
	focused                          bool
	display, prefix                  string
	valid, displayValid, prefixValid bool
}

type textFieldMaskMissingScratch struct{}
type textFieldDisplayMissingScratch struct{}

func ensureTextFieldCaches(world *ecs.World) {
	masks := world.FrameScratch[textFieldMaskMissingScratch, ecs.Entity]()
	displays := world.FrameScratch[textFieldDisplayMissingScratch, ecs.Entity]()
	defer masks.ReleaseUnused()
	defer displays.ReleaseUnused()
	for entity, access := range world.Query[TextField]().All {
		field := access.Ref()

		if field.Password {
			if !world.Has[textFieldMaskCache](entity) {
				masks.Values = append(masks.Values, entity)
			}
		} else if field.Composition != "" && !world.Has[textFieldDisplayCache](entity) {
			displays.Values = append(displays.Values, entity)
		}
	}
	// Structural changes must happen after the query has closed.
	for _, entity := range masks.Values {
		_ = world.Insert(entity, textFieldMaskCache{})
	}
	for _, entity := range displays.Values {
		_ = world.Insert(entity, textFieldDisplayCache{})
	}
}

func textFieldMask(world *ecs.World, owner ecs.Entity, field TextField) textFieldMaskCache {
	valueRunes := runeCount(field.Value)
	compositionRunes := runeCount(field.Composition)
	cache, ok := world.Get[textFieldMaskCache](owner)
	if ok && cache.valid && cache.valueRunes == valueRunes && cache.compositionRunes == compositionRunes {
		return cache
	}
	total := valueRunes + compositionRunes
	if len(cache.mask) < total {
		cache.mask = strings.Repeat("*", total)
	}
	cache.valueRunes = valueRunes
	cache.compositionRunes = compositionRunes
	cache.valid = true
	if current := world.GetMut[textFieldMaskCache](owner); current != nil {
		*current = cache
	}
	return cache
}

func maskedTextFieldValue(world *ecs.World, owner ecs.Entity, field TextField) string {
	cache := textFieldMask(world, owner, field)
	return cache.mask[:cache.valueRunes]
}

func maskedTextFieldPrefix(world *ecs.World, owner ecs.Entity, field TextField, caret int) string {
	cache := textFieldMask(world, owner, field)
	caret = max(0, min(caret, cache.valueRunes))
	return cache.mask[:caret+cache.compositionRunes]
}

func textFieldDisplay(world *ecs.World, owner ecs.Entity, field TextField) string {
	if !field.Password {
		if field.Composition != "" {
			if cache := textFieldDisplayCacheRef(world, owner, field, field.Caret); cache != nil && cache.displayValid {
				return cache.display
			}
			if cache := textFieldDisplayCacheMut(world, owner, field, field.Caret); cache != nil {
				if !cache.displayValid {
					cache.display = inputDisplay(field.Value, field.Placeholder, false, field.Focused, field.Composition, field.Caret)
					cache.displayValid = true
				}
				return cache.display
			}
		}
		return inputDisplay(field.Value, field.Placeholder, false, field.Focused, field.Composition, field.Caret)
	}
	if field.Value == "" && field.Composition == "" && !field.Focused {
		return field.Placeholder
	}
	cache := textFieldMask(world, owner, field)
	return cache.mask[:cache.valueRunes+cache.compositionRunes]
}

func textFieldDisplayCacheMatches(cache *textFieldDisplayCache, field TextField, caret int) bool {
	return cache.valid && cache.value == field.Value && cache.placeholder == field.Placeholder && cache.composition == field.Composition && cache.caret == caret && cache.focused == field.Focused
}

func textFieldDisplayCacheRef(world *ecs.World, owner ecs.Entity, field TextField, caret int) *textFieldDisplayCache {
	cache := world.GetRef[textFieldDisplayCache](owner)
	if cache == nil || !textFieldDisplayCacheMatches(cache, field, caret) {
		return nil
	}
	return cache
}

func textFieldDisplayCacheMut(world *ecs.World, owner ecs.Entity, field TextField, caret int) *textFieldDisplayCache {
	cache := world.GetMut[textFieldDisplayCache](owner)
	if cache == nil {
		return nil
	}
	if !textFieldDisplayCacheMatches(cache, field, caret) {
		*cache = textFieldDisplayCache{
			value: field.Value, placeholder: field.Placeholder, composition: field.Composition,
			caret: caret, focused: field.Focused, valid: true,
		}
	}
	return cache
}

func textFieldCompositionPrefix(world *ecs.World, owner ecs.Entity, field TextField, caret int) string {
	caret = max(0, min(caret, runeCount(field.Value)))
	if field.Composition == "" {
		return field.Value[:runeByteIndex(field.Value, caret)]
	}
	if cache := textFieldDisplayCacheRef(world, owner, field, caret); cache != nil && cache.prefixValid {
		return cache.prefix
	}
	if cache := textFieldDisplayCacheMut(world, owner, field, caret); cache != nil {
		if !cache.prefixValid {
			byteIndex := runeByteIndex(field.Value, caret)
			cache.prefix = field.Value[:byteIndex] + field.Composition
			cache.prefixValid = true
		}
		return cache.prefix
	}
	byteIndex := runeByteIndex(field.Value, caret)
	return field.Value[:byteIndex] + field.Composition
}

func runeCount(value string) int {
	return utf8.RuneCountInString(value)
}

func runeByteIndex(value string, target int) int {
	if target <= 0 {
		return 0
	}
	index := 0
	for byteIndex := range value {
		if index == target {
			return byteIndex
		}
		index++
	}
	return len(value)
}

func runeSlice(value string, start, end int) string {
	start, end = orderedSelection(start, end)
	return value[runeByteIndex(value, start):runeByteIndex(value, end)]
}

func syncNumberInputStepIcon(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, size *scene.Size2D) {
	step := world.GetRef[NumberInputStep](owner)
	ok := step != nil
	if !ok || step == nil {
		return
	}
	control, ok := controlViewFor(world, owner)
	if !ok {
		return
	}
	icon := numberInputStepIcon(world, step)
	if icon.Texture == nil {
		size.Width = 0
		size.Height = 0
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	width, height := fitIconSize(icon, control.Width-4, control.Height-4)
	transform.Translation.X.Value = (control.Width - width) * 0.5
	transform.Translation.Y.Value = (control.Height - height) * 0.5
	size.Width = width
	size.Height = height
	image := scene.NewUIImage2D(icon.Texture)
	applySurface(world, entity, width, height, SurfaceStyle{Image: image})
	setUIStackIndex(world, entity, control.StackIndex+2)
}

func numberInputStepIcon(world *ecs.World, step *NumberInputStep) IconStyle {
	input, ok := world.Get[NumberInput](step.Input)
	if !ok {
		return IconStyle{}
	}
	if step.Direction > 0 {
		return input.UpIcon
	}
	return input.DownIcon
}
