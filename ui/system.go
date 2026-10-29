package ui

import (
	"cmp"
	"strings"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/internal/smallsort"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

type tabParentChild struct {
	parent ecs.Entity
	child  ecs.Entity
}

func compareTabParentChild(a, b tabParentChild) int {
	if byParent := cmp.Compare(a.parent, b.parent); byParent != 0 {
		return byParent
	}
	return cmp.Compare(a.child, b.child)
}

type tabChildRange struct {
	start int
	end   int
}

type tabHierarchyScratch struct{}
type tabHierarchy struct {
	children    []tabParentChild
	ranges      map[ecs.Entity]tabChildRange
	versions    [2]uint64
	initialized bool
}
type iconWidthsScratch struct{}
type menuWidthsScratch struct{}
type contextMenuWidthsScratch struct{}
type selectorListsScratch struct{}
type itemMetricsScratch[K any] struct{}
type focusedInputReaderTag struct{}
type htmlFocusCandidatesScratch struct{}
type buttonGroupPositionsScratch struct{}
type buttonGroupPositionKey struct {
	group ecs.Entity
	index int
}

type htmlFocusCandidate struct {
	entity   ecs.Entity
	tabIndex int
	order    uint32
}

type itemMetrics struct {
	count int
	width float32
}

type syncMetrics struct {
	menus        map[ecs.Entity]itemMetrics
	contextMenus map[ecs.Entity]itemMetrics
	selectors    map[ecs.Entity]itemMetrics
	labelWidths  map[ecs.Entity]float32
}

func FocusedInputSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "dispatch_focused_input"
	}
	return ecs.SystemConfig{Name: name, System: dispatchFocusedInput}
}

func dispatchFocusedInput(context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}
	state, ok := world.GetResource[*input.State]()
	if !ok || state == nil {
		return nil
	}
	focus, ok := world.GetResource[*scene.InputFocus]()
	if !ok || focus == nil || focus.Keyboard == 0 {
		return nil
	}
	if !world.IsAlive(focus.Keyboard) {
		focus.Keyboard = 0
		return nil
	}
	reader := context.Local[focusedInputReaderTag, ecs.MessageReader[input.Event]]()
	for event := range reader.All(world) {
		if !scene.IsInteractive(world, focus.Keyboard) || state.EventConsumed(event.Seq) || !focusedInputEventKind(event.Kind) {
			continue
		}
		dispatch := scene.FocusedInputContext{World: world, Entity: focus.Keyboard, Event: *event}
		if handler := world.GetRef[scene.FocusedInputHandler](focus.Keyboard); handler != nil && handler.Handle != nil {
			handler.Handle(&dispatch)
		}
		if world.Has[TextField](focus.Keyboard) {
			dispatch.Handled = true
		}
		if dispatch.Handled {
			state.ConsumeEvent(event.Seq)
		}
		world.WriteMessage(scene.FocusedInputEvent{Entity: focus.Keyboard, Event: *event, Handled: dispatch.Handled})
	}
	return nil
}

func focusedInputEventKind(kind input.EventKind) bool {
	switch kind {
	case input.EventKeyDown, input.EventKeyUp, input.EventTextInput, input.EventTextComposition, input.EventTextCommand, input.EventTextSelection:
		return true
	default:
		return false
	}
}

func update(context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}
	ensureTextFieldCaches(world)
	uiInputState, ok := world.GetResource[*input.State]()
	if !ok || uiInputState == nil {
		return nil
	}
	state, ok := world.GetResource[*uiEventState]()
	if !ok || state == nil {
		state = &uiEventState{}
		world.InsertResource(state)
	}
	pointer := uiInputState.Pointer()
	if state.Focused != 0 && !scene.IsInteractive(world, state.Focused) {
		focus(world, state, 0)
	}

	resetFrameState(world)
	keyboardHandled := false
	if handler := world.GetRef[TextFieldKeyHandler](state.Focused); handler != nil && handler.Handle != nil {
		keyboardHandled = handler.Handle(uiInputState)
	}
	if !keyboardHandled {
		updateHTMLKeyboard(world, state.Focused, uiInputState)
	}
	pressDispatched := false
	for event := range state.InputEvents.All(world) {
		if event.Entity != 0 && !scene.IsInteractive(world, event.Entity) {
			continue
		}
		if event.Kind == scene.InputEventPress {
			pressDispatched = true
			if !isContextMenuPick(world, event.Entity) {
				closeContextMenus(world)
			}
			closeOtherSelectors(world, event.Entity)
		}
		if event.Handled {
			continue
		}
		updateVirtualJoystickInput(world, event)
		uiTarget := world.Has[scene.UIGlobalTransform](event.Entity) || world.Has[Control](event.Entity)
		switch event.Kind {
		case scene.InputEventWheel:
			handled := updateTextareaWheel(world, event.Entity, event.WheelX, event.WheelY)
			if !handled {
				handled = updateFocusedNumberInputWheel(world, state.Focused, event.WheelY)
			}
			if handled {
				uiInputState.ConsumePointerWheel()
			}
		case scene.InputEventPress:
			if progress, ok := sliderProgressOwner(world, event.Entity); ok {
				updateSliderProgressValue(world, progress, event.X, event.Y)
			}
			if isSelectorPick(world, event.Entity) {
				activate(world, event.Entity)
				state.ActivatedOnPress = event.Entity
			}
			focused := focusTarget(world, event.Entity)
			selected := false
			scrolling := focused != 0 && world.Has[Textarea](focused) && beginTextareaScrollDrag(world, focused, event.X, event.Y)
			if !scrolling {
				focus(world, state, focused)
			}
			if scrolling {
				resetInputClick(state)
				setSelecting(world, focused, false)
			} else if uiInputState.Pointer().Device == input.PointerDeviceMouse && !world.Has[NumberInputStep](event.Entity) {
				selected = selectOnInputDoubleClick(world, state, focused, event.X, event.Y, time.Now())
			} else {
				resetInputClick(state)
			}
			if focused != 0 && !scrolling {
				if selected {
					field, _ := world.Get[TextField](focused)
					uiInputState.SetTextInputSelection(field.Anchor, field.Caret)
					setSelecting(world, focused, false)
				} else {
					setCaretFromInput(world, focused, event.X, event.Y, false)
					setSelecting(world, focused, true)
				}
			}
		case scene.InputEventMove:
			if progress, ok := sliderProgressOwner(world, event.Entity); ok {
				updateSliderProgressValue(world, progress, event.X, event.Y)
			}
			if state.Focused != 0 && textFieldSelecting(world, state.Focused) {
				setCaretFromInput(world, state.Focused, event.X, event.Y, true)
			}
		case scene.InputEventRelease:
			setSelecting(world, state.Focused, false)
		case scene.InputEventTap:
			if state.ActivatedOnPress == event.Entity {
				state.ActivatedOnPress = 0
			} else {
				activate(world, event.Entity)
			}
		case scene.InputEventCancel:
			state.ActivatedOnPress = 0
			setSelecting(world, state.Focused, false)
		}
		if uiTarget {
			event.Handled = true
		}
	}
	if !keyboardHandled && uiInputState.KeyPressed(input.KeyTab) &&
		!uiInputState.KeyHeld(input.KeyLeftControl) && !uiInputState.KeyHeld(input.KeyRightControl) &&
		!uiInputState.KeyHeld(input.KeyLeftSuper) && !uiInputState.KeyHeld(input.KeyRightSuper) &&
		!uiInputState.KeyHeld(input.KeyLeftAlt) && !uiInputState.KeyHeld(input.KeyRightAlt) {
		textarea, ok := world.Get[Textarea](state.Focused)
		if !ok || !textarea.CaptureTab {
			focusNextHTMLElement(world, state, uiInputState.KeyHeld(input.KeyLeftShift) || uiInputState.KeyHeld(input.KeyRightShift))
		}
	}
	if pointer.Pressed && !pressDispatched {
		closeContextMenus(world)
		closeOtherSelectors(world, 0)
	}
	updateTextareaScrollDrags(world, uiInputState)
	if !keyboardHandled {
		editFocused(world, state.Focused, uiInputState)
	}
	if pointer.Down && (pointer.DeltaX != 0 || pointer.DeltaY != 0) && state.Focused != 0 && textFieldSelecting(world, state.Focused) {
		setCaretFromInput(world, state.Focused, pointer.X, pointer.Y, true)
	}
	return nil
}

func updateTextareaWheel(world *ecs.World, entity ecs.Entity, wheelX, wheelY float32) bool {
	if wheelX == 0 && wheelY == 0 {
		return false
	}
	geometry, ok := textareaScrollGeometryFor(world, entity)
	if !ok {
		return false
	}
	textarea := world.GetMut[Textarea](entity)
	if textarea == nil {
		return false
	}
	// Chromium chains a wheel its target cannot absorb to the nearest scrollable
	// ancestor, so a direction already at its limit must stay unhandled.
	step := scrollWheelStep(textarea.WheelStep)
	nextX := min(max(textarea.ScrollX-wheelX*step, 0), max(geometry.contentWidth-geometry.viewportWidth, 0))
	if textarea.DisableHorizontalScroll {
		nextX = 0
	}
	nextY := min(max(textarea.ScrollY-wheelY*step, 0), max(geometry.contentHeight-geometry.viewportHeight, 0))
	if nextX == textarea.ScrollX && nextY == textarea.ScrollY {
		return false
	}
	textarea.ScrollX, textarea.ScrollY = nextX, nextY
	return true
}

func updateFocusedNumberInputWheel(world *ecs.World, entity ecs.Entity, wheelY float32) bool {
	if entity == 0 || wheelY == 0 {
		return false
	}
	config, ok := world.Get[NumberInput](entity)
	if !ok {
		return false
	}
	field := world.GetMut[TextField](entity)
	if field == nil {
		return false
	}
	direction := 1
	if wheelY < 0 {
		direction = -1
	}
	next, changed := stepNumberInputValue(field.Value, config, direction)
	if !changed {
		return false
	}
	field.Value = next
	field.Caret = runeCount(next)
	if input := world.GetMut[NumberInput](entity); input != nil {
		input.Changed = true
	}
	world.WriteMessage(ChangedEvent[string]{Entity: entity, Value: next})
	return true
}

func isSelectorPick(world *ecs.World, entity ecs.Entity) bool {
	if entity == 0 {
		return false
	}
	return world.Has[Selector](entity) || world.Has[SelectorItem](entity)
}

func syncWidgets(context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}
	theme, ok := world.GetResource[Theme]()
	if ok {
		theme = themeWithDefaults(theme)
	} else {
		theme = DefaultTheme()
	}
	skin := skinFor(world, theme)
	ensureTextFieldCaches(world)
	syncCaretBlink(world)
	syncButtonGroups(world, skin)
	prepareScrollAreas(world)
	syncNumberInputs(world)
	syncMenuIndicators(world, skin)
	labelWidths := syncText(world, theme)
	metrics := collectSyncMetrics(world)
	metrics.labelWidths = labelWidths
	syncMenus(world, metrics)
	syncContextMenus(world, metrics)
	prepareSelectors(world, metrics)
	syncTabs(world, skin)
	syncSelectorItems(world, metrics)
	applyUISyncVisuals(world, skin, metrics)
	return nil
}

func syncAfterLayout(context *ecs.SystemContext) error {
	world := context.World
	if world == nil {
		return nil
	}
	theme, ok := world.GetResource[Theme]()
	if ok {
		theme = themeWithDefaults(theme)
	} else {
		theme = DefaultTheme()
	}
	skin := skinFor(world, theme)
	finalizeScrollAreas(world)
	metrics := collectSyncMetrics(world)
	metrics.labelWidths = collectButtonLabelWidths(world)
	syncSelectorItems(world, metrics)
	applyUISyncVisuals(world, skin, metrics)
	return nil
}

func applyUISyncVisuals(world *ecs.World, skin Skin, metrics syncMetrics) {
	syncPanels(world, skin)
	syncControls(world, skin)
	syncVirtualJoysticks(world, skin)
	syncJoinedEdges(world, skin)
	syncProgress(world, skin)
	syncImageGridSurfaces(world, skin)
	syncAuthoredTextPositions(world)
	syncSurfaceElements(world, skin, metrics)
	syncTextVisibility(world)
	syncSurfaceElementVisibility(world)
	syncControlVariants(world)
	syncAuthoredControlParts(world)
	syncInputBlockers(world)
	syncCornerRadii(world)
}
func syncInputBlockers(world *ecs.World) {
	state := world.FrameValue[syncBlockerStateTag, componentSyncState]()
	versions := [4]uint64{world.ComponentVersion[scene.InputBlocker2D](), world.ComponentVersion[scene.Size2D](), world.ComponentVersion[scene.Text2D](), world.ComponentVersion[ScrollArea]()}
	if state.initialized && state.versions == versions {
		return
	}
	for entity, row := range world.Query[scene.InputBlocker2D]().All {
		access := row

		blocker := *access.Ref()
		if area := world.GetRef[ScrollArea](entity); area != nil {
			blocker.Width, blocker.Height = area.Width, area.Height
		} else if text := world.GetRef[scene.Text2D](entity); text != nil {
			blocker.Width, blocker.Height, _ = scene.MeasureText2D(text)
		} else if size := world.GetRef[scene.Size2D](entity); size != nil {
			blocker.Width, blocker.Height = size.Width, size.Height
		}
		ecs.CommitComponent(access, blocker)
	}
	versions[0] = world.ComponentVersion[scene.InputBlocker2D]()
	state.versions, state.initialized = versions, true
}

func resetFrameState(world *ecs.World) {
	for _, row := range world.Query[TextField]().All {
		access := row

		if access.Ref().Submitted {
			access.Mut().Submitted = false
		}
	}
	for _, row := range world.Query[NumberInput]().All {
		access := row

		if access.Ref().Changed {
			access.Mut().Changed = false
		}
	}
	for _, row := range world.Query[Toggle[bool]]().All {
		access := row

		if access.Ref().Changed {
			access.Mut().Changed = false
		}
	}
	for _, row := range world.Query[Progress]().All {
		access := row

		if access.Ref().Changed {
			access.Mut().Changed = false
		}
	}
}

func sliderProgressOwner(world *ecs.World, entity ecs.Entity) (ecs.Entity, bool) {
	if progress := world.GetRef[Progress](entity); progress != nil {
		return entity, progress.Slider && !progress.Indeterminate
	}
	if thumb := world.GetRef[ProgressThumb](entity); thumb != nil {
		if progress := world.GetRef[Progress](thumb.Progress); progress != nil {
			return thumb.Progress, progress.Slider && !progress.Indeterminate
		}
	}
	return 0, false
}

func updateSliderProgressValue(world *ecs.World, entity ecs.Entity, screenX, screenY float32) {
	progress := world.GetMut[Progress](entity)
	if progress == nil || !progress.Slider || progress.Indeterminate {
		return
	}
	transform := world.GetRef[scene.UIGlobalTransform](entity)
	control, controlOK := controlViewFor(world, entity)
	if transform == nil || !controlOK || control.Width <= 0 {
		return
	}
	x, y := inputCoordsForUI(screenX, screenY)
	local := transform.Matrix.Inv().Mul4x1(mgl32.Vec4{x, y, 0, 1})
	if local.W() == 0 {
		return
	}
	ratio := min(max(local.X()/local.W()/control.Width, float32(0)), float32(1))
	value := progress.Min + (progress.Max-progress.Min)*ratio
	if progress.Value == value {
		return
	}
	progress.Value = value
	progress.Changed = true
	world.WriteMessage(ChangedEvent[float32]{Entity: entity, Value: value})
}

func focusTarget(world *ecs.World, entity ecs.Entity) ecs.Entity {
	if step := world.GetRef[NumberInputStep](entity); step != nil {
		return step.Input
	}
	if entity != 0 && world.Has[TextField](entity) {
		return entity
	}
	if entity != 0 {
		if target, ok := world.Get[html.Focusable](entity); ok && target.Pointer {
			return entity
		}
		if owner, ok := world.Get[html.FocusOwner](entity); ok {
			if target, exists := world.Get[html.Focusable](owner.Entity); exists && target.Pointer {
				return owner.Entity
			}
		}
	}
	return 0
}

func focus(world *ecs.World, state *uiEventState, entity ecs.Entity) {
	focusWithVisibility(world, state, entity, false)
}

func focusWithVisibility(world *ecs.World, state *uiEventState, entity ecs.Entity, focusVisible bool) {
	if target, ok := world.Get[html.Focusable](entity); ok {
		focusVisible = focusVisible || target.FocusVisible
	}
	if state.Focused == entity {
		state.FocusVisible = entity != 0 && focusVisible
		if entity != 0 {
			html.SetFocusableFocusVisible(world, entity, true, focusVisible)
		}
		return
	}
	if state.Focused != 0 {
		if control := world.GetMut[htmlControlState](state.Focused); control != nil {
			control.SpacePressed = false
		}
		notified := false
		if field := world.GetMut[TextField](state.Focused); field != nil {
			field.Focused = false
			field.Composition = ""
			world.WriteMessage(FocusChangedEvent{Entity: state.Focused, Focused: false})
			notified = true
		}
		if _, ok := world.Get[html.Focusable](state.Focused); ok && !notified {
			world.WriteMessage(FocusChangedEvent{Entity: state.Focused, Focused: false})
		}
	}
	state.Focused = entity
	state.FocusVisible = entity != 0 && focusVisible
	focusState, ok := world.GetResource[*scene.InputFocus]()
	if !ok || focusState == nil {
		focusState = &scene.InputFocus{}
		world.InsertResource(focusState)
	}
	focusState.Keyboard = entity
	if entity != 0 {
		notified := false
		if field := world.GetMut[TextField](entity); field != nil {
			field.Focused = true
			field.Caret = runeCount(field.Value)
			field.Anchor = field.Caret
			world.WriteMessage(FocusChangedEvent{Entity: entity, Focused: true, FocusVisible: focusVisible})
			notified = true
		}
		if _, ok := world.Get[html.Focusable](entity); ok && !notified {
			world.WriteMessage(FocusChangedEvent{Entity: entity, Focused: true, FocusVisible: focusVisible})
		}
	}
	syncTextInputFocus(world, entity)
}

func focusNextHTMLElement(world *ecs.World, state *uiEventState, reverse bool) {
	if world == nil || state == nil {
		return
	}
	candidates := world.FrameScratch[htmlFocusCandidatesScratch, htmlFocusCandidate]()
	defer candidates.ReleaseUnused()
	for entity, access := range world.Query[html.Focusable]().All {
		target := access.Get()

		if !target.Sequential || !scene.IsInteractive(world, entity) {
			continue
		}
		candidates.Values = append(candidates.Values, htmlFocusCandidate{entity: entity, tabIndex: target.TabIndex, order: target.Order})
	}
	if len(candidates.Values) == 0 {
		focusWithVisibility(world, state, 0, true)
		return
	}
	smallsort.SortFunc(candidates.Values, func(left, right htmlFocusCandidate) int {
		leftPositive, rightPositive := left.tabIndex > 0, right.tabIndex > 0
		if leftPositive != rightPositive {
			if leftPositive {
				return -1
			}
			return 1
		}
		if leftPositive && left.tabIndex != right.tabIndex {
			return cmp.Compare(left.tabIndex, right.tabIndex)
		}
		if left.order != right.order {
			return cmp.Compare(left.order, right.order)
		}
		return cmp.Compare(left.entity, right.entity)
	})
	current := -1
	for index, candidate := range candidates.Values {
		if candidate.entity == state.Focused {
			current = index
			break
		}
	}
	next := current + 1
	if reverse {
		next = current - 1
		if current < 0 {
			next = len(candidates.Values) - 1
		}
	}
	if next < 0 {
		next = len(candidates.Values) - 1
	} else if next >= len(candidates.Values) {
		next = 0
	}
	focusWithVisibility(world, state, candidates.Values[next].entity, true)
}

func autofocusHTMLElement(world *ecs.World) {
	if world == nil {
		return
	}
	state, ok := world.GetResource[*uiEventState]()
	if !ok || state == nil {
		state = &uiEventState{}
		world.InsertResource(state)
	}
	if state.Focused != 0 {
		return
	}
	var target ecs.Entity
	for entity, access := range world.Query[html.Focusable]().All {
		focusable := access.Get()

		if !focusable.AutoFocus || world.Has[htmlAutofocusApplied](focusable.Owner) || !scene.IsInteractive(world, entity) {
			continue
		}
		if target == 0 || entity < target {
			target = entity
		}
	}
	if target == 0 {
		return
	}
	if focusable, ok := world.Get[html.Focusable](target); ok {
		_ = world.Insert(focusable.Owner, htmlAutofocusApplied{})
	}
	if controlState := world.GetMut[htmlControlState](target); controlState != nil {
		controlState.FocusVisible = true
	}
	focusWithVisibility(world, state, target, true)
	html.SetFocusableFocusVisible(world, target, true, true)
}

func syncTextInputFocus(world *ecs.World, entity ecs.Entity) {
	uiInputState, ok := world.GetResource[*input.State]()
	if !ok || uiInputState == nil {
		return
	}
	if entity == 0 {
		uiInputState.BlurTextInput()
		return
	}
	field, ok := world.Get[TextField](entity)
	if !ok {
		uiInputState.BlurTextInput()
		return
	}
	if world.Has[html.Editable](entity) {
		uiInputState.FocusTextInput(input.TextInputText, true, field.Value)
		return
	}
	if world.Has[Textarea](entity) {
		uiInputState.FocusTextInput(input.TextInputText, true, field.Value)
		return
	}
	if number, ok := world.Get[NumberInput](entity); ok {
		kind := input.TextInputDecimal
		if number.Integer {
			kind = input.TextInputInteger
		}
		uiInputState.FocusTextInput(kind, false, field.Value)
		return
	}
	uiInputState.FocusTextInput(input.TextInputText, false, field.Value)
}

func activate(world *ecs.World, entity ecs.Entity) {
	if route, ok := world.Get[treeActivation](entity); ok {
		route.activate(world, entity)
		return
	}
	if button, ok := world.Get[WindowTitleBarButton](entity); ok {
		world.WriteMessage(WindowTitleBarEvent{Bar: button.Bar, Button: entity, Action: button.Action})
		return
	}
	if remove, ok := world.Get[imageGridRemove](entity); ok {
		world.WriteMessage(ImageGridEvent{Grid: remove.Grid, Index: remove.Index})
		return
	}
	if step := world.GetRef[NumberInputStep](entity); step != nil {
		if config, ok := world.Get[NumberInput](step.Input); ok {
			field := world.GetMut[TextField](step.Input)
			if field == nil {
				return
			}
			next, changed := stepNumberInputValue(field.Value, config, step.Direction)
			if changed {
				field.Value = next
				if input := world.GetMut[NumberInput](step.Input); input != nil {
					input.Changed = true
				}
				world.WriteMessage(ChangedEvent[string]{Entity: step.Input, Value: field.Value})
			}
		}
		return
	}
	if item, ok := world.Get[ContextMenuItem](entity); ok && !item.Disabled {
		if menu := world.GetMut[ContextMenu](item.Owner); menu != nil {
			menu.Open = false
			menu.Selected = item.Index
			world.WriteMessage(SelectedEvent[string]{Entity: item.Owner, Index: item.Index, Value: item.Label})
		}
		return
	}
	if item, ok := world.Get[MenuItem](entity); ok && !item.Disabled {
		if menu := world.GetMut[Menu](item.Owner); menu != nil {
			menu.Open = false
			menu.Selected = item.Index
			world.WriteMessage(SelectedEvent[string]{Entity: item.Owner, Index: item.Index, Value: item.Label})
		}
		return
	}
	if item, ok := world.Get[SelectorItem](entity); ok && !item.Disabled {
		if selector := world.GetMut[Selector](item.Owner); selector != nil {
			selector.Open = false
			selector.Selected = item.Index
			world.WriteMessage(SelectedEvent[string]{Entity: item.Owner, Index: item.Index, Value: item.Label})
		}
		return
	}
	if item := world.GetRef[TabItem](entity); item != nil && !item.Disabled {
		if tabs := world.GetMut[Tab](item.Tabs); tabs != nil {
			tabs.Selected = item.Index
			world.WriteMessage(SelectedEvent[string]{Entity: item.Tabs, Index: item.Index, Value: item.Label})
		}
		return
	}
	if menu := world.GetMut[Menu](entity); menu != nil {
		menu.Open = !menu.Open
		return
	}
	if selector := world.GetMut[Selector](entity); selector != nil {
		if target := world.GetRef[scene.InputTarget2D](entity); target != nil && target.Disabled {
			return
		}
		selector.Open = !selector.Open
		return
	}
	if toggle := world.GetMut[Toggle[bool]](entity); toggle != nil {
		// HTML checkboxes and radios are toggled by updateHTMLRuntime,
		// which also updates their live DOM state. Letting the generic
		// widget path toggle them again would immediately undo the click.
		if control, isHTML := world.Get[html.Control](entity); isHTML && control.Tag == "input" {
			typeName := strings.TrimSpace(control.Type)
			if strings.EqualFold(typeName, "checkbox") || strings.EqualFold(typeName, "radio") {
				return
			}
		}
		if target := world.GetRef[scene.InputTarget2D](entity); target != nil && target.Disabled {
			return
		}
		toggle.Value = !toggle.Value
		toggle.Changed = true
		world.WriteMessage(ChangedEvent[bool]{Entity: entity, Value: toggle.Value})
		return
	}
}

func syncPanels(world *ecs.World, skin Skin) {
	for entity, access := range world.Query2[Panel, scene.Size2D]().All {
		size := access.B.Ref()

		style := styleFor(world, entity, skin.Panel)
		applySurface(world, entity, size.Width, size.Height, style.Body)
	}
}

func syncControls(world *ecs.World, _ Skin) {
	state := world.FrameValue[syncControlStateTag, componentSyncState]()
	versions := [4]uint64{world.ComponentVersion[Control](), world.ComponentVersion[scene.InputTarget2D](), world.ComponentVersion[scene.Size2D]()}
	if state.initialized && state.versions == versions {
		return
	}
	for _, access := range world.Query3[Control, scene.InputTarget2D, scene.Size2D]().All {
		target := access.B
		size := access.C

		ecs.CommitComponent(size, scene.Size2D{Width: target.Ref().Width, Height: target.Ref().Height})
	}
	versions[2] = world.ComponentVersion[scene.Size2D]()
	state.versions, state.initialized = versions, true
}

func controlSurface(world *ecs.World, entity ecs.Entity, control *controlView, skin Skin) SurfaceStyle {
	state := controlState(control, false)
	if remove, ok := world.Get[imageGridRemove](entity); ok {
		style := styleFor(world, remove.Grid, skin.ImageGrid)
		return style.Remove.Resolve(state)
	}
	if step, ok := world.Get[NumberInputStep](entity); ok {
		style := styleFor(world, step.Input, skin.NumberInput)
		return style.Step.Resolve(state)
	}
	if thumb, ok := world.Get[ProgressThumb](entity); ok {
		style := styleFor(world, thumb.Progress, skin.Progress)
		return style.Thumb.Resolve(state)
	}
	if progress, ok := world.Get[Progress](entity); ok {
		style := styleFor(world, entity, skin.Progress)
		if !progress.Slider || progress.Indeterminate {
			state.Hovered, state.Pressed = false, false
		}
		return style.Track.Resolve(state)
	}
	if item, ok := world.Get[TabItem](entity); ok {
		tabs, _ := world.Get[Tab](item.Tabs)
		style := styleFor(world, item.Tabs, skin.Tab)
		state.Disabled = state.Disabled || item.Disabled
		surface := style.Item.Resolve(tabs.Selected == item.Index, state)
		if _, custom := world.Get[StyleOverride[TabStyle]](item.Tabs); !custom && tabs.Selected == item.Index {
			surface.Background.Fill = tabs.Highlight
		}
		return surface
	}
	if item, ok := world.Get[SelectorItem](entity); ok {
		style := styleFor(world, item.Owner, skin.Selector)
		if authored := htmlSelectStyleFor(world, item.Owner); authored != nil {
			palette := authored.Item
			palette.Normal = style.Item.Normal
			return palette.Resolve(state)
		}
		return style.Item.Resolve(state)
	}
	if item, ok := world.Get[MenuItem](entity); ok {
		style := styleFor(world, item.Owner, skin.Menu)
		return style.Item.Resolve(state)
	}
	if item, ok := world.Get[ContextMenuItem](entity); ok {
		style := styleFor(world, item.Owner, skin.ContextMenu)
		return style.Item.Resolve(state)
	}
	if item, ok := world.Get[buttonGroupItem](entity); ok {
		style := styleFor(world, item.Group, skin.ButtonGroup)
		return style.Button.Body.Resolve(state)
	}
	if _, ok := world.Get[NumberInput](entity); ok {
		field, _ := world.Get[TextField](entity)
		state.Focused = field.Focused
		style := styleFor(world, entity, skin.NumberInput)
		return style.Field.Body.Resolve(state)
	}
	if world.Has[Input](entity) || world.Has[Textarea](entity) {
		field, _ := world.Get[TextField](entity)
		state.Focused = field.Focused
		if textarea := world.GetRef[Textarea](entity); textarea != nil && textarea.scrollDragging {
			state.Pressed = false
		}
		style := styleFor(world, entity, skin.TextField)
		return style.Body.Resolve(state)
	}
	if _, ok := world.Get[Selector](entity); ok {
		style := styleFor(world, entity, skin.Selector)
		return style.Body.Resolve(state)
	}
	if _, ok := world.Get[Menu](entity); ok {
		style := styleFor(world, entity, skin.Menu)
		return style.Trigger.Body.Resolve(state)
	}
	if _, ok := world.Get[Button](entity); ok {
		variant, _ := world.Get[ButtonVariant](entity)
		style := buttonStyleFor(skin, variant, world.Has[Danger](entity))
		style = styleFor(world, entity, style)
		return style.Body.Resolve(state)
	}
	return skin.TextField.Body.Resolve(state)
}

func syncNumberInputs(world *ecs.World) {
	for entity, access := range world.Query[NumberInputStep]().All {
		step := access.Ref()

		input, ok := world.Get[NumberInput](step.Input)
		if !ok {
			continue
		}
		root, ok := controlViewFor(world, step.Input)
		if !ok {
			continue
		}
		buttonWidth := numberInputStepButtonWidth(input.StepButtonWidth)
		buttonHeight := root.Height * 0.5
		y := float32(0)
		if step.Direction < 0 {
			y = buttonHeight
		}
		setUIPosition(world, entity, root.Width-buttonWidth, y)
		SetControlBounds(world, entity, buttonWidth, buttonHeight)
		SetControlDisabled(world, entity, root.Disabled)
		world.SetIfChanged(entity, scene.Size2D{Width: buttonWidth, Height: buttonHeight})
		setUIStackIndex(world, entity, root.StackIndex+1)
	}
}

// The border is a separate high-stack layer so step buttons cannot cover it.
func syncNumberInputBorder(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, size *scene.Size2D, skin Skin) {
	control, ok := controlViewFor(world, owner)
	if !ok {
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	transform.Translation.X.Value = 0
	transform.Translation.Y.Value = 0
	setUIStackIndex(world, entity, control.StackIndex+4)
	surface := controlSurface(world, owner, &control, skin)
	surface.Background = scene.RoundedRectShape2D(graphics.ColorRGBA{}, 0)
	surface.Image = scene.UIImage2D{}
	applySurface(world, entity, control.Width, control.Height, surface)
}

func syncText(world *ecs.World, theme Theme) map[ecs.Entity]float32 {
	syncTextContent(world, theme)
	iconWidths, labelWidths := collectTextWidths(world, theme)
	layoutTextVisuals(world, iconWidths, labelWidths)
	return labelWidths
}

func syncTextContent(world *ecs.World, theme Theme) {
	selectorLabels := collectSelectorLabels(world)
	for entity, access := range world.Query2[Relation[scene.Text2D], scene.Text2D]().All {
		if !func(entity ecs.Entity, visualAccess ecs.ComponentAccess[Relation[scene.Text2D]], textAccess ecs.ComponentAccess[scene.Text2D]) bool {
			visual := visualAccess.Ref()
			next := *textAccess.Ref()
			text := &next
			defer func() {
				previous := textAccess.Ref()
				if previous.Value != next.Value || previous.Font != next.Font || previous.FontSize != next.FontSize || previous.DPI != next.DPI || previous.Color != next.Color || previous.WrapMode != next.WrapMode || previous.WrapWidth != next.WrapWidth {
					*textAccess.Mut() = next
				}
			}()
			authored := world.Has[Relation[Control]](entity)
			if !authored {
				text.Font = theme.Font
				text.DPI = theme.DPI
				text.FontSize = theme.FontSize
				text.Color = theme.Text
				if world.Has[Danger](visual.Owner) {
					text.Color = graphics.White
				}
			}
			switch {
			case world.Has[Part[TitlePart]](entity):
				if panel := world.GetRef[Panel](visual.Owner); panel != nil {
					text.Value = panel.Title
				}
			case world.Has[Part[IconPart]](entity):
				if !authored {
					syncIconText(world, visual.Owner, text, theme)
				}
			case world.Has[Part[LabelPart]](entity):
				if !authored {
					syncLabelText(world, visual.Owner, text, theme, selectorLabels)
				} else if selector := world.GetRef[Selector](visual.Owner); selector != nil {
					text.Value = selectorCurrentLabel(selectorLabels, visual.Owner, selector)
				}
			case world.Has[Part[ValuePart]](entity):
				if authored {
					syncAuthoredValueText(world, visual.Owner, text)
				} else {
					syncValueText(world, visual.Owner, text, theme)
				}
				syncTextareaTextLayout(world, entity, visual.Owner, text)
			case world.Has[Part[ItemPart]](entity):
				if !authored {
					syncItemText(world, entity, visual.Owner, text, theme)
				} else if item := world.GetRef[SelectorItem](visual.Owner); item != nil {
					if color, ok := html.ControlTextColor(world, item.Owner); ok {
						text.Color = color
						if state, exists := controlViewFor(world, visual.Owner); exists {
							if authored := htmlSelectStyleFor(world, item.Owner); authored != nil {
								palette := authored.Text
								palette.Normal = color
								text.Color = palette.Resolve(controlState(&state, false))
							} else if (state.Selected || state.Hovered || state.Pressed) && !state.Disabled {
								text.Color = graphics.White
							}
						}
					}
				}
			}
			return true
		}(entity, access.A, access.B) {
			break
		}
	}
}

const (
	textareaTextPaddingX float32 = 12
	textareaTextPaddingY float32 = 10
)

func textareaContentPadding(textarea *Textarea) (float32, float32) {
	if textarea != nil && textarea.Padding > 0 {
		return textarea.Padding, textarea.Padding
	}
	return textareaTextPaddingX, textareaTextPaddingY
}

func textareaScrollBarSize(textarea *Textarea) float32 {
	if textarea != nil && textarea.BarSize != 0 {
		return max(textarea.BarSize, 0)
	}
	return scrollBarSize(0)
}

func syncTextareaTextLayout(world *ecs.World, entity, owner ecs.Entity, text *scene.Text2D) {
	textarea, ok := world.Get[Textarea](owner)
	if !ok || text == nil {
		return
	}
	if control, exists := world.Get[html.Control](owner); exists {
		textarea.Wrap = control.Wrap
	}
	text.WrapMode = scene.TextWrapNone
	text.WrapWidth = 0
	if !textarea.Wrap {
		return
	}
	control, ok := controlViewFor(world, owner)
	if !ok {
		return
	}
	text.WrapMode = scene.TextWrapCharacters
	if textarea.WrapTokens {
		text.WrapMode = scene.TextWrapTokens
	}
	_, _, text.WrapWidth, _ = textareaContentBox(world, entity, &textarea, control)
	if textarea.WrapWidthPercent > 0 {
		text.WrapWidth *= min(textarea.WrapWidthPercent, 100) / 100
	}
}

func collectTextWidths(world *ecs.World, theme Theme) (map[ecs.Entity]float32, map[ecs.Entity]float32) {
	icons := world.FrameScratchMap[iconWidthsScratch, ecs.Entity, float32]()
	labels := world.FrameScratchMap[buttonLabelWidthsScratch, ecs.Entity, float32]()
	for entity, access := range world.Query2[Relation[scene.Text2D], scene.Text2D]().All {
		relation := access.A.Ref()
		text := access.B.Ref()

		width, measured := float32(0), false
		measureText := func() float32 {
			if !measured {
				width, _, _ = scene.MeasureTextValue2D(text, text.Value)
				measured = true
			}
			return width
		}
		if world.Has[Part[LabelPart]](entity) {
			if _, exists := labels[relation.Owner]; !exists {
				labels[relation.Owner] = measureText()
			}
		}
		if !world.Has[Part[IconPart]](entity) {
			continue
		}
		icon := IconStyle{}
		if content := world.GetRef[IconContent](relation.Owner); content != nil {
			icon = content.Value
		} else if item := world.GetRef[TabItem](relation.Owner); item != nil {
			icon = item.Icon
		} else if selector := world.GetRef[Selector](relation.Owner); selector != nil {
			icon = selector.Icon
		} else if step := world.GetRef[NumberInputStep](relation.Owner); step != nil {
			icon = numberInputStepIcon(world, step)
		}
		if icon.Texture != nil || icon.Width > 0 {
			icons[relation.Owner] = measureIconWidth(theme, icon)
		} else {
			icons[relation.Owner] = measureText()
		}
	}
	return icons, labels
}

func layoutTextVisuals(world *ecs.World, iconWidths, labelWidths map[ecs.Entity]float32) {
	for entity, access := range world.Query2[Relation[scene.Text2D], scene.UITransform]().All {
		if !func(entity ecs.Entity, visualAccess ecs.ComponentAccess[Relation[scene.Text2D]], transformAccess ecs.ComponentAccess[scene.UITransform]) bool {
			visual := visualAccess.Ref()
			// HTML layout already assigns each label fragment its final local position.
			// Keep that geometry while allowing editable HTML fields to use the text
			// input synchronization below.
			if world.Has[html.ControlText](entity) && !world.Has[TextField](visual.Owner) {
				return true
			}
			if world.Has[Relation[Control]](entity) && !world.Has[html.ControlText](entity) {
				return true
			}
			if world.Has[html.Editable](visual.Owner) {
				return true
			}
			control, ok := controlViewFor(world, visual.Owner)
			if !ok {
				return true
			}
			nextTransform := *transformAccess.Ref()
			transform := &nextTransform
			defer func() { ecs.CommitComponent(transformAccess, nextTransform) }()
			if world.Has[TextField](visual.Owner) && !world.Has[Textarea](visual.Owner) {
				if text := world.GetRef[scene.Text2D](entity); text != nil {
					syncSingleLineTextVisual(world, entity, visual.Owner, text, transform, control)
				} else {
					centerTextVisual(world, entity, transform, control.Height)
				}
				return true
			}
			if textarea, ok := world.Get[Textarea](visual.Owner); ok && world.Has[Part[ValuePart]](entity) {
				if text := world.GetRef[scene.Text2D](entity); text != nil {
					syncTextareaVisual(world, entity, visual.Owner, &textarea, text, transform, control)
					world.SetIfChanged(visual.Owner, textarea)
				}
				return true
			}
			if world.Has[Button](visual.Owner) {
				centerTextVisual(world, entity, transform, control.Height)
				alignment, _ := world.Get[ButtonContentAlignment](visual.Owner)
				switch {
				case world.Has[Part[IconPart]](entity):
					transform.Translation.X.Value = buttonContentX(control.Width, iconWidths[visual.Owner], labelWidths[visual.Owner], alignment)
					if content := world.GetRef[Content[string]](visual.Owner); content != nil && content.Value == "" {
						transform.Translation.X.Value = (control.Width - iconWidths[visual.Owner]) * 0.5
					}
				case world.Has[Part[LabelPart]](entity):
					transform.Translation.X.Value = 14
					if iconWidths[visual.Owner] > 0 {
						transform.Translation.X.Value = buttonLabelX(control.Width, iconWidths[visual.Owner], labelWidths[visual.Owner], alignment)
					} else {
						if width, exists := labelWidths[visual.Owner]; exists {
							transform.Translation.X.Value = (control.Width - width) * 0.5
						}
					}
				}
				return true
			}
			if item := world.GetRef[TabItem](visual.Owner); item != nil {
				height := control.Height
				if tabs := world.GetRef[Tab](item.Tabs); tabs != nil {
					height = max(tabs.HeaderHeight, float32(0))
				}
				centerTextVisual(world, entity, transform, height)
				transform.Translation.X.Value = textIconX
				if world.Has[Part[IconPart]](entity) && item.Label == "" {
					transform.Translation.X.Value = (control.Width - iconWidths[visual.Owner]) * 0.5
				} else if world.Has[Part[ItemPart]](entity) && iconWidths[visual.Owner] > 0 {
					transform.Translation.X.Value += iconWidths[visual.Owner] + textIconGap
				}
			}
			if selector := world.GetRef[Selector](visual.Owner); selector != nil && world.Has[Part[IconPart]](entity) {
				centerTextVisual(world, entity, transform, control.Height)
				transform.Translation.X.Value = control.Width - iconWidths[visual.Owner] - 12
			}
			if world.Has[NumberInputStep](visual.Owner) {
				setUIStackIndex(world, entity, control.StackIndex+2)
				if world.Has[Part[IconPart]](entity) {
					centerTextVisual(world, entity, transform, control.Height)
					transform.Translation.X.Value = (control.Width - iconWidths[visual.Owner]) * 0.5
				}
			}
			if item, ok := world.Get[MenuItem](visual.Owner); ok {
				height := control.Height
				if menu, ok := world.Get[Menu](item.Owner); ok {
					height = max(menu.ItemHeight, float32(0))
				}
				centerTextVisual(world, entity, transform, height)
			}
			if item, ok := world.Get[ContextMenuItem](visual.Owner); ok {
				height := control.Height
				if menu, ok := world.Get[ContextMenu](item.Owner); ok {
					height = max(menu.ItemHeight, float32(0))
				}
				centerTextVisual(world, entity, transform, height)
			}
			if item, ok := world.Get[SelectorItem](visual.Owner); ok {
				if selector, ok := world.Get[Selector](item.Owner); ok {
					centerTextVisual(world, entity, transform, selector.ItemHeight)
				}
			}
			return true
		}(entity, access.A, access.B) {
			break
		}
	}
}

func syncSingleLineTextVisual(world *ecs.World, visualEntity, owner ecs.Entity, text *scene.Text2D, transform *scene.UITransform, control controlView) {
	if text == nil || transform == nil {
		return
	}
	htmlLayout := world.Has[html.ControlText](visualEntity)
	baseX, baseY := transform.Translation.X.Value, transform.Translation.Y.Value
	if text.WrapMode != scene.TextWrapNone || text.WrapWidth != 0 {
		if updated := world.GetMut[scene.Text2D](visualEntity); updated != nil {
			text = updated
		}
		text.WrapMode, text.WrapWidth = scene.TextWrapNone, 0
	}
	viewportWidth := max(control.Width-textareaTextPaddingX*2, 0)
	if htmlLayout {
		if size, ok := world.Get[scene.Size2D](visualEntity); ok {
			viewportWidth = max(size.Width, 0)
		}
	}
	if number, ok := world.Get[NumberInput](owner); ok {
		viewportWidth = max(control.Width-number.StepButtonWidth-textareaTextPaddingX, 0)
	}
	world.SetIfChanged(owner, scene.ClipRect2D{Width: control.Width, Height: control.Height})
	fieldValue, fieldOK := world.Get[TextField](owner)
	field := &fieldValue
	text.Index = nil
	if field.Buffer != nil {
		text.Index = field.Buffer
	}
	defer func() {
		if fieldOK {
			world.SetIfChanged(owner, fieldValue)
		}
	}()
	keepInsertionOrigin := false
	if fieldOK && !field.Password && field.scrollInitialized &&
		field.lastScrollValue != field.Value && field.lastScrollComposition == "" && field.Composition == "" &&
		field.Caret > field.lastScrollCaret {
		oldCaretByte := runeByteIndex(field.lastScrollValue, field.lastScrollCaret)
		previousPrefix := field.lastScrollValue[:oldCaretByte]
		currentPrefix := field.Value[:runeByteIndex(field.Value, field.lastScrollCaret)]
		previousSuffix := field.lastScrollValue[oldCaretByte:]
		currentSuffix := field.Value[runeByteIndex(field.Value, field.Caret):]
		keepInsertionOrigin = previousPrefix == currentPrefix && previousSuffix == currentSuffix
	}
	if htmlLayout && fieldOK {
		// HTML layout already applies the control's CSS padding. Recover the
		// unscrolled origin before updating ScrollX; the new offset is applied below.
		baseX -= field.ScrollX
	}
	contentWidth, _, measured := scene.MeasureText2D(text)
	if !measured {
		if fieldOK {
			field.ScrollX = 0
			field.lastScrollValue = field.Value
			field.lastScrollComposition = field.Composition
			field.lastScrollCaret = field.Caret
			field.lastScrollFocused = field.Focused
			field.lastScrollViewport = viewportWidth
			field.scrollInitialized = true
		}
		if htmlLayout {
			transform.Translation.X.Value = baseX
			if fieldOK {
				transform.Translation.X.Value += field.ScrollX
			}
			transform.Translation.Y.Value = baseY
		} else {
			centerSingleLineTextVisual(text, transform, control.Height)
			transform.Translation.X.Value = textareaTextPaddingX
		}
		return
	}
	if fieldOK {
		minScrollX := -max(contentWidth-viewportWidth, 0)
		field.ScrollX = max(min(field.ScrollX, 0), minScrollX)
		autoScroll := !field.scrollInitialized || field.lastScrollValue != field.Value ||
			field.lastScrollComposition != field.Composition || field.lastScrollCaret != field.Caret ||
			field.lastScrollFocused != field.Focused || field.lastScrollViewport != viewportWidth
		if autoScroll && !keepInsertionOrigin {
			caretValue, caretIndex := field.Value, field.Caret
			if field.Password {
				caretValue = maskedTextFieldPrefix(world, owner, *field, field.Caret)
				caretIndex = runeCount(caretValue)
			} else if field.Composition != "" {
				caretValue = textFieldCompositionPrefix(world, owner, *field, field.Caret)
				caretIndex = runeCount(caretValue)
			}
			if caretX, _, _, _, caretOK := scene.TextCaretRect2D(text, caretValue, caretIndex); caretOK {
				if caretX+field.ScrollX < 0 {
					field.ScrollX = -caretX
				} else if caretX+1+field.ScrollX > viewportWidth {
					field.ScrollX = viewportWidth - caretX - 1
				}
			}
		}
		field.ScrollX = max(min(field.ScrollX, 0), minScrollX)
		field.lastScrollValue = field.Value
		field.lastScrollComposition = field.Composition
		field.lastScrollCaret = field.Caret
		field.lastScrollFocused = field.Focused
		field.lastScrollViewport = viewportWidth
		field.scrollInitialized = true
	}
	if htmlLayout {
		transform.Translation.X.Value = baseX
		if fieldOK {
			transform.Translation.X.Value += field.ScrollX
		}
		transform.Translation.Y.Value = baseY
	} else {
		centerSingleLineTextVisual(text, transform, control.Height)
		transform.Translation.X.Value = textareaTextPaddingX
		if fieldOK {
			transform.Translation.X.Value += field.ScrollX
		}
	}
}

func centerTextVisual(world *ecs.World, entity ecs.Entity, transform *scene.UITransform, height float32) {
	if text := world.GetRef[scene.Text2D](entity); text != nil {
		centerSingleLineTextVisual(text, transform, height)
	}
}

func centerSingleLineTextVisual(text *scene.Text2D, transform *scene.UITransform, height float32) {
	if text == nil || transform == nil {
		return
	}
	_, lineHeight, measured := scene.MeasureTextValue2D(text, text.Value)
	if measured {
		transform.Translation.Y.Value = (height - lineHeight) * 0.5
	}
}

func textareaContentBox(world *ecs.World, textEntity ecs.Entity, textarea *Textarea, control controlView) (x, y, width, height float32) {
	if link, ok := world.Get[html.ControlText](textEntity); ok {
		return link.Content.X, link.Content.Y, max(link.Content.Width, 0), max(link.Content.Height, 0)
	}
	x, y = textareaContentPadding(textarea)
	return x, y, max(control.Width-x*2, 0), max(control.Height-y*2, 0)
}

func syncTextareaVisual(world *ecs.World, entity, owner ecs.Entity, textarea *Textarea, text *scene.Text2D, transform *scene.UITransform, control controlView) {
	if textarea == nil || text == nil || transform == nil {
		return
	}
	// Layout can resize the content box after the text-content pass.
	syncTextareaTextLayout(world, entity, owner, text)
	paddingX, paddingY, viewportWidth, viewportHeight := textareaContentBox(world, entity, textarea, control)
	world.SetIfChanged(owner, scene.ClipRect2D{Width: control.Width, Height: control.Height})
	contentWidth, contentHeight, measured := scene.MeasureText2D(text)
	if !measured {
		textarea.ScrollX, textarea.ScrollY = 0, 0
		textarea.preserveScroll = false
		transform.Translation.X.Value = paddingX
		transform.Translation.Y.Value = paddingY
		if field, ok := world.Get[TextField](owner); ok {
			textarea.lastValue, textarea.lastCaret, textarea.lastFocused = field.Value, field.Caret, field.Focused
		}
		textarea.lastViewportWidth, textarea.lastViewportHeight = viewportWidth, viewportHeight
		textarea.lastWrapWidthPercent, textarea.lastWrapTokens = textarea.WrapWidthPercent, textarea.WrapTokens
		textarea.lastWrap = textarea.Wrap
		textarea.lastFollowTail = false
		textarea.scrollInitialized = true
		return
	}
	maxScrollX := max(contentWidth-viewportWidth, 0)
	if textarea.DisableHorizontalScroll {
		maxScrollX = 0
	}
	maxScrollY := max(contentHeight-viewportHeight, 0)
	textarea.ScrollX = min(max(textarea.ScrollX, 0), maxScrollX)
	textarea.ScrollY = min(max(textarea.ScrollY, 0), maxScrollY)
	if field, ok := world.Get[TextField](owner); ok {
		text.Index = nil
		if field.Buffer != nil {
			text.Index = field.Buffer
		}
		contentChanged := !textarea.scrollInitialized || textarea.FollowTail && !textarea.lastFollowTail || textarea.lastValue != field.Value ||
			textarea.lastViewportWidth != viewportWidth || textarea.lastViewportHeight != viewportHeight || textarea.lastWrap != textarea.Wrap ||
			textarea.lastWrapWidthPercent != textarea.WrapWidthPercent || textarea.lastWrapTokens != textarea.WrapTokens
		autoScroll := contentChanged || textarea.lastCaret != field.Caret || textarea.lastFocused != field.Focused
		if textarea.FollowTail && contentChanged {
			textarea.ScrollY = maxScrollY
		} else if autoScroll && field.Focused && !textarea.scrollDragging && !textarea.preserveScroll {
			caretX, caretY, _, caretHeight, caretOK := scene.TextCaretRect2D(text, field.Value, field.Caret)
			if caretOK {
				if caretX < textarea.ScrollX {
					textarea.ScrollX = caretX
				} else if caretX+1 > textarea.ScrollX+viewportWidth {
					textarea.ScrollX = caretX + 1 - viewportWidth
				}
				if caretY < textarea.ScrollY {
					textarea.ScrollY = caretY
				} else if caretY+caretHeight > textarea.ScrollY+viewportHeight {
					textarea.ScrollY = caretY + caretHeight - viewportHeight
				}
			}
		}
		textarea.lastValue, textarea.lastCaret, textarea.lastFocused = field.Value, field.Caret, field.Focused
	}
	textarea.preserveScroll = false
	textarea.ScrollX = min(max(textarea.ScrollX, 0), maxScrollX)
	textarea.ScrollY = min(max(textarea.ScrollY, 0), maxScrollY)
	textarea.lastViewportWidth, textarea.lastViewportHeight = viewportWidth, viewportHeight
	textarea.lastWrapWidthPercent, textarea.lastWrapTokens = textarea.WrapWidthPercent, textarea.WrapTokens
	textarea.lastWrap = textarea.Wrap
	textarea.lastFollowTail = textarea.FollowTail
	textarea.scrollInitialized = true
	transform.Translation.X.Value = paddingX - textarea.ScrollX
	transform.Translation.Y.Value = paddingY - textarea.ScrollY
}

func syncAuthoredValueText(world *ecs.World, owner ecs.Entity, text *scene.Text2D) {
	field, ok := world.Get[TextField](owner)
	if !ok {
		return
	}
	text.Value = textFieldDisplay(world, owner, field)
}

func syncIconText(world *ecs.World, owner ecs.Entity, text *scene.Text2D, theme Theme) {
	if icon := world.GetRef[IconContent](owner); icon != nil {
		text.Value = iconFallback(icon.Value)
		if typography := world.GetRef[Typography](owner); typography != nil {
			text.FontSize = sizeOr(typography.FontSize, theme.FontSize)
			text.DPI = sizeOr(typography.DPI, theme.DPI)
		}
		return
	}
	if item := world.GetRef[TabItem](owner); item != nil {
		text.Value = iconFallback(item.Icon)
		return
	}
	if selector := world.GetRef[Selector](owner); selector != nil {
		text.Value = iconFallback(selector.Icon)
		return
	}
	if step := world.GetRef[NumberInputStep](owner); step != nil {
		text.Value = iconFallback(numberInputStepIcon(world, step))
	}
}

func syncLabelText(world *ecs.World, owner ecs.Entity, text *scene.Text2D, theme Theme, selectorLabels map[ecs.Entity]string) {
	if label := world.GetRef[Label](owner); label != nil {
		text.Value = label.Value
		text.Color = colorOr(label.Color, theme.Text)
		text.FontSize = sizeOr(label.FontSize, theme.FontSize)
		text.DPI = sizeOr(label.DPI, theme.DPI)
		return
	}
	if content := world.GetRef[Content[string]](owner); content != nil {
		text.Value = content.Value
		if typography := world.GetRef[Typography](owner); typography != nil {
			text.FontSize = sizeOr(typography.FontSize, theme.FontSize)
			text.DPI = sizeOr(typography.DPI, theme.DPI)
			text.Color = colorOr(typography.Color, theme.Text)
		}
		return
	}
	if selector := world.GetRef[Selector](owner); selector != nil {
		text.Value = selectorCurrentLabel(selectorLabels, owner, selector)
	}
}

func syncValueText(world *ecs.World, owner ecs.Entity, text *scene.Text2D, theme Theme) {
	field, ok := world.Get[TextField](owner)
	if !ok {
		return
	}
	text.Value = textFieldDisplay(world, owner, field)
	text.FontSize = sizeOr(field.FontSize, theme.FontSize)
	text.DPI = sizeOr(field.DPI, theme.DPI)
	if field.Value == "" && field.Composition == "" && !field.Focused {
		text.Color = theme.Muted
	}
}

func caretVisible(world *ecs.World) bool {
	clock, ok := world.GetResource[*scene.Time]()
	return !ok || clock == nil || clock.Elapsed/(500*time.Millisecond)%2 == 0
}

// CaretBlinkDelay is the next visual deadline for a focused text field. An
// on-demand host can sleep between phases instead of polling every frame.
func CaretBlinkDelay(world *ecs.World) (time.Duration, bool) {
	if !hasFocusedInput(world) {
		return 0, false
	}
	clock, ok := world.GetResource[*scene.Time]()
	if !ok || clock == nil {
		return 0, false
	}
	const interval = 500 * time.Millisecond
	return interval - clock.Elapsed%interval, true
}

func syncCaretBlink(world *ecs.World) {
	clock, ok := world.GetResource[*scene.Time]()
	if !ok || clock == nil {
		return
	}
	phase := uint64(clock.Elapsed / (500 * time.Millisecond))
	state, ok := world.GetResource[*caretBlinkState]()
	if !ok || state == nil {
		world.InsertResource(&caretBlinkState{phase: phase})
		return
	}
	if state.phase == phase {
		return
	}
	state.phase = phase
	if !hasFocusedInput(world) {
		return
	}
	if request, ok := world.GetResource[*scene.RenderRequest](); ok {
		request.Request()
	}
}

func hasFocusedInput(world *ecs.World) bool {
	focused := false
	for _, access := range world.Query[TextField]().All {
		field := access.Get()

		focused = field.Focused
		if !(!focused) {
			break
		}
	}
	return focused
}

func syncItemText(world *ecs.World, entity, owner ecs.Entity, text *scene.Text2D, theme Theme) {
	if item, ok := world.Get[MenuItem](owner); ok {
		text.Value = item.Label
		if menu, ok := world.Get[Menu](item.Owner); ok {
			setUIStackIndex(world, entity, menu.StackIndex+3)
		}
		return
	}
	if item, ok := world.Get[ContextMenuItem](owner); ok {
		text.Value = item.Label
		if menu, ok := world.Get[ContextMenu](item.Owner); ok {
			setUIStackIndex(world, entity, menu.StackIndex+3)
		}
		return
	}
	if item, ok := world.Get[SelectorItem](owner); ok {
		text.Value = item.Label
		if world.Has[Selector](item.Owner) {
			setUIStackIndex(world, entity, selectorPopupStackBase+3)
		}
		return
	}
	if item := world.GetRef[TabItem](owner); item != nil {
		text.Value = item.Label
		text.Color = theme.Muted
		if tabs := world.GetRef[Tab](item.Tabs); tabs != nil && tabs.Selected == item.Index {
			text.Color = theme.Text
		}
	}
}

func syncMenuIndicators(world *ecs.World, skin Skin) {
	for entity, _ := range world.Query[Menu]().All {
		if icon := world.GetRef[IconContent](entity); icon != nil {
			indicator := styleFor(world, entity, skin.Menu).Indicator
			if icon.Value != indicator {
				mutable := world.GetMut[IconContent](entity)
				mutable.Value = indicator
			}
		}
	}
}

func syncMenus(world *ecs.World, metrics syncMetrics) {
	widths := world.FrameScratchMap[menuWidthsScratch, ecs.Entity, float32]()
	for entity, access := range world.Query[Menu]().All {
		menu := access.Ref()

		padding := menuPadding(menu.Padding)
		contentWidth := metrics.menus[entity].width
		width := menuPanelWidth(menu.Width, padding, contentWidth)
		widths[entity] = width
		if control, ok := controlViewFor(world, entity); ok {
			SetControlBounds(world, entity, width, control.Height)
		}
		if size, ok := world.Get[scene.Size2D](entity); ok {
			size.Width = width
			world.SetIfChanged(entity, size)
		}
		if target, ok := world.Get[scene.InputTarget2D](entity); ok {
			target.Width = width
			world.SetIfChanged(entity, target)
		}
	}
	for entity, access := range world.Query2[MenuItem, scene.Visibility]().All {
		itemAccess := access.A
		visibilityAccess := access.B

		item := itemAccess.Ref()
		menu := world.GetRef[Menu](item.Owner)
		if menu == nil {
			ecs.CommitComponent(visibilityAccess, scene.Visibility{})
			continue
		}
		padding := menuPadding(menu.Padding)
		width := widths[item.Owner]
		ecs.CommitComponent(visibilityAccess, scene.Visibility{Visible: menu.Open})
		setUIPosition(world, entity, padding, menu.ItemHeight+padding+menu.ItemHeight*float32(item.Index))
		SetControlBounds(world, entity, width-padding*2, menu.ItemHeight)
		world.SetIfChanged(entity, scene.Size2D{Width: width - padding*2, Height: menu.ItemHeight})
		setUIStackIndex(world, entity, menu.StackIndex+2)
	}
	for entity, access := range world.Query2[Relation[SurfaceStyle], scene.Visibility]().All {
		if !func(entity ecs.Entity, visualAccess ecs.ComponentAccess[Relation[SurfaceStyle]], visibilityAccess ecs.ComponentAccess[scene.Visibility]) bool {
			visual := visualAccess.Ref()
			nextVisibility := *visibilityAccess.Ref()
			visibility := &nextVisibility
			defer func() { ecs.CommitComponent(visibilityAccess, nextVisibility) }()
			if world.Has[Part[MenuPanelPart]](entity) {
				menu := world.GetRef[Menu](visual.Owner)
				visibility.Visible = menu != nil && menu.Open
			}
			return true
		}(entity, access.A, access.B) {
			break
		}
	}
}

func syncContextMenus(world *ecs.World, metrics syncMetrics) {
	widths := world.FrameScratchMap[contextMenuWidthsScratch, ecs.Entity, float32]()
	for entity, access := range world.Query[ContextMenu]().All {
		menu := access.Ref()

		setVisible(world, entity, menu.Open)
		padding := menuPadding(menu.Padding)
		contentWidth := metrics.contextMenus[entity].width
		widths[entity] = menuPanelWidth(menu.Width, padding, contentWidth)
	}
	for entity, access := range world.Query[ContextMenuItem]().All {
		item := access.Ref()

		menu := world.GetRef[ContextMenu](item.Owner)
		if menu == nil {
			continue
		}
		setVisible(world, entity, menu.Open)
		padding := menuPadding(menu.Padding)
		width := widths[item.Owner]
		setUIPosition(world, entity, padding, padding+menu.ItemHeight*float32(item.Index))
		SetControlBounds(world, entity, width-padding*2, menu.ItemHeight)
		world.SetIfChanged(entity, scene.Size2D{Width: width - padding*2, Height: menu.ItemHeight})
		setUIStackIndex(world, entity, menu.StackIndex+2)
	}
}

func prepareSelectors(world *ecs.World, metrics syncMetrics) {
	for entity, access := range world.Query3[SelectorList, scene.UITransform, scene.Visibility]().All {
		if !func(entity ecs.Entity, listAccess ecs.ComponentAccess[SelectorList], transformAccess ecs.ComponentAccess[scene.UITransform], visibilityAccess ecs.ComponentAccess[scene.Visibility]) bool {
			list := listAccess.Ref()
			nextTransform, nextVisibility := transformAccess.Get(), visibilityAccess.Get()
			transform, visibility := &nextTransform, &nextVisibility
			defer func() {
				ecs.CommitComponent(transformAccess, nextTransform)
				ecs.CommitComponent(visibilityAccess, nextVisibility)
			}()
			selector := world.GetRef[Selector](list.Selector)
			control, controlOK := controlViewFor(world, list.Selector)
			if selector == nil || !controlOK {
				visibility.Visible = false
				return true
			}
			itemHeight := selectorItemHeight(selector, &control)
			metric := metrics.selectors[list.Selector]
			count := metric.count
			listHeight, upward := selectorListLayout(world, list.Selector, selector, &control, count)
			listWidth := max(control.Width, metric.width+24)
			if itemHeight*float32(count) > listHeight {
				listWidth = max(listWidth, metric.width+24+selectorScrollBarSize)
			}
			listWidth, listX := selectorListHorizontalLayout(world, list.Selector, listWidth)
			world.SetIfChanged(entity, scene.Size2D{Width: listWidth, Height: listHeight})
			if scroll, ok := world.Get[scene.UIScroll](entity); ok {
				scroll.ViewportSize = mgl32.Vec2{listWidth, listHeight}
				scroll.ContentSize = mgl32.Vec2{listWidth, itemHeight * float32(count)}
				scroll.WheelStep = itemHeight
				scroll.DisableInput = !selector.Open
				scroll.Vertical = true
				world.SetIfChanged(entity, scroll)
			}
			transform.Translation.X.Value = listX
			gap := selectorPopupGap(world, list.Selector)
			transform.Translation.Y.Value = control.Height + gap
			if popup, popupOK := world.Get[htmlSelectorPopup](entity); popupOK {
				world.SetIfChanged(entity, scene.RoundedClip2D{Width: listWidth, Height: listHeight, Radius: popup.Radius})
			}
			if upward {
				transform.Translation.Y.Value = -listHeight - gap
			}
			visibility.Visible = selector.Open && hierarchyVisible(world, list.Selector)
			world.SetIfChanged(entity, scene.UIStackIndex(selectorPopupStackBase))
			if clip, ok := world.Get[scene.ClipRect2D](entity); ok {
				clip.Width, clip.Height = listWidth, listHeight
				world.SetIfChanged(entity, clip)
			}
			return true
		}(entity, access.A, access.B, access.C) {
			break
		}
	}
}

func syncSelectorItems(world *ecs.World, metrics syncMetrics) {
	lists := world.FrameScratchMap[selectorListsScratch, ecs.Entity, ecs.Entity]()
	for entity, access := range world.Query[SelectorList]().All {
		list := access.Get()

		lists[list.Selector] = entity
	}
	for entity, access := range world.Query3[SelectorItem, scene.UITransform, scene.Visibility]().All {
		if !func(entity ecs.Entity, itemAccess ecs.ComponentAccess[SelectorItem], transformAccess ecs.ComponentAccess[scene.UITransform], visibilityAccess ecs.ComponentAccess[scene.Visibility]) bool {
			item := itemAccess.Ref()
			nextTransform, nextVisibility := *transformAccess.Ref(), *visibilityAccess.Ref()
			transform, visibility := &nextTransform, &nextVisibility
			defer func() {
				ecs.CommitComponent(transformAccess, nextTransform)
				ecs.CommitComponent(visibilityAccess, nextVisibility)
			}()
			selector := world.GetRef[Selector](item.Owner)
			control, controlOK := controlViewFor(world, item.Owner)
			listVisibility := world.GetRef[scene.Visibility](lists[item.Owner])
			if (selector == nil) || (!controlOK) || (listVisibility == nil) {
				visibility.Visible = false
				return true
			}
			itemHeight := selectorItemHeight(selector, &control)
			listSize := world.GetRef[scene.Size2D](lists[item.Owner])
			listHeight := listSize.Height
			itemWidth := listSize.Width
			if itemHeight*float32(metrics.selectors[item.Owner].count) > listHeight {
				itemWidth = max(itemWidth-selectorScrollBarSize, 0)
			}
			transform.Translation.X.Value = 0
			scrollY := float32(0)
			if listEntity, ok := lists[item.Owner]; ok {
				if scroll, exists := world.Get[scene.UIScroll](listEntity); exists {
					scrollY = -scroll.Position.Y()
				}
			}
			transform.Translation.Y.Value = float32(item.Index)*itemHeight - scrollY
			visibility.Visible = listVisibility.Visible
			SetControlBounds(world, entity, itemWidth, itemHeight)
			world.SetIfChanged(entity, scene.Size2D{Width: itemWidth, Height: itemHeight})
			setUIStackIndex(world, entity, selectorPopupStackBase+2)
			return true
		}(entity, access.A, access.B, access.C) {
			break
		}
	}
}

func syncTabs(world *ecs.World, skin Skin) {
	hasTabs := false
	for range world.Query2[Tab, scene.Size2D]().All {
		hasTabs = true
		break
	}
	if !hasTabs {
		return
	}
	hierarchy := world.FrameValue[tabHierarchyScratch, tabHierarchy]()
	versions := [2]uint64{world.ComponentVersion[scene.Parent](), world.ComponentStructureVersion[scene.Parent]()}
	if !hierarchy.initialized || hierarchy.versions != versions {
		hierarchy.children = hierarchy.children[:0]
		if hierarchy.ranges == nil {
			hierarchy.ranges = make(map[ecs.Entity]tabChildRange)
		} else {
			clear(hierarchy.ranges)
		}
		for entity, access := range world.Query[scene.Parent]().All {
			parent := access.Get()

			hierarchy.children = append(hierarchy.children, tabParentChild{parent: parent.Entity, child: entity})
		}
		smallsort.SortFunc(hierarchy.children, compareTabParentChild)
		for start := 0; start < len(hierarchy.children); {
			end := start + 1
			for end < len(hierarchy.children) && hierarchy.children[end].parent == hierarchy.children[start].parent {
				end++
			}
			hierarchy.ranges[hierarchy.children[start].parent] = tabChildRange{start: start, end: end}
			start = end
		}
		hierarchy.versions, hierarchy.initialized = versions, true
	}
	parentChildren, childRanges := hierarchy.children, hierarchy.ranges
	for entity, access := range world.Query2[Tab, scene.Size2D]().All {
		tabAccess := access.A
		sizeAccess := access.B

		tab := tabAccess.Ref()
		ecs.CommitComponent(sizeAccess, scene.Size2D{Width: tab.Width, Height: tab.Height})
		style := styleFor(world, entity, skin.Tab)
		frame := style.Frame
		frame.Border = scene.Shape2D{}
		applySurface(world, entity, tab.Width, tab.Height, frame)
		headerHeight := tab.HeaderHeight
		if headerHeight < 0 {
			headerHeight = 0
		}
		itemCount, customWidths := 0, true
		children := tabChildren(parentChildren, childRanges, entity)
		for _, relation := range children {
			if item := world.GetRef[TabItem](relation.child); item != nil {
				itemCount++
				customWidths = customWidths && item.Width > 0
			}
		}
		customWidths = customWidths && itemCount > 0
		itemWidth := tab.Width
		if itemCount > 0 && !customWidths {
			itemWidth = tab.Width / float32(itemCount)
		}
		x := float32(0)
		for _, relation := range children {
			child := relation.child
			if item := world.GetRef[TabItem](child); item != nil {
				width := itemWidth
				if customWidths {
					width = item.Width
				}
				if transform, ok := world.Get[scene.UITransform](child); ok {
					transform.Translation.X.Value, transform.Translation.Y.Value = x, 0
					world.SetIfChanged(child, transform)
				}
				SetControlBounds(world, child, width, headerHeight)
				SetControlDisabled(world, child, item.Disabled)
				world.SetIfChanged(child, scene.Size2D{Width: width, Height: headerHeight})
				x += width
			}
			if panel := world.GetRef[TabPanel](child); panel != nil {
				contentHeight := tab.Height - headerHeight
				if contentHeight < 0 {
					contentHeight = 0
				}
				if transform, ok := world.Get[scene.UITransform](child); ok {
					transform.Translation.X.Value, transform.Translation.Y.Value = 0, headerHeight
					world.SetIfChanged(child, transform)
				}
				panelStyle := styleFor(world, child, style)
				applySurface(world, child, tab.Width, contentHeight, panelStyle.Panel)
				setTabSubtreeVisible(world, parentChildren, childRanges, child, tab.Selected == panel.Index)
			}
		}
	}
}

func tabChildren(parentChildren []tabParentChild, childRanges map[ecs.Entity]tabChildRange, parent ecs.Entity) []tabParentChild {
	childRange, ok := childRanges[parent]
	if !ok {
		return nil
	}
	return parentChildren[childRange.start:childRange.end]
}

func syncButtonGroups(world *ecs.World, skin Skin) {
	positions := world.FrameScratchMap[buttonGroupPositionsScratch, buttonGroupPositionKey, float32]()
	for groupEntity, access := range world.Query[ButtonGroup]().All {
		groupAccess := access

		group := groupAccess.Ref()
		x := float32(0)
		dividerX := float32(0)
		for index, button := range group.Buttons {
			control, hasControl := controlViewFor(world, button)
			if hasControl {
				dividerX += control.Width
				positions[buttonGroupPositionKey{groupEntity, index}] = dividerX
			}
			item, ok := world.Get[buttonGroupItem](button)
			if !ok || item.Group != groupEntity {
				continue
			}
			setUIPosition(world, button, x, 0)
			if hasControl {
				SetControlBounds(world, button, control.Width, group.Height)
				setUIStackIndex(world, button, group.StackIndex)
				x += control.Width
			}
		}
		// Missing controls have no boundary; retain the group's terminal position.
		positions[buttonGroupPositionKey{groupEntity, -1}] = dividerX
		if group.Width != x {
			groupAccess.Mut().Width = x
		}
		frame := styleFor(world, groupEntity, skin.ButtonGroup).Frame
		frame.Border = scene.Shape2D{}
		applySurface(world, groupEntity, group.Width, group.Height, frame)
	}
	for entity, access := range world.Query3[buttonGroupDivider, scene.UITransform, scene.Size2D]().All {
		if !func(entity ecs.Entity, dividerAccess ecs.ComponentAccess[buttonGroupDivider], transformAccess ecs.ComponentAccess[scene.UITransform], sizeAccess ecs.ComponentAccess[scene.Size2D]) bool {
			divider := dividerAccess.Ref()
			nextTransform, nextSize := transformAccess.Get(), sizeAccess.Get()
			transform, size := &nextTransform, &nextSize
			defer func() {
				ecs.CommitComponent(transformAccess, nextTransform)
				ecs.CommitComponent(sizeAccess, nextSize)
			}()
			group, ok := world.Get[ButtonGroup](divider.Group)
			if !ok || divider.Index < 0 || divider.Index >= len(group.Buttons) {
				size.Width = 0
				size.Height = 0
				return true
			}
			x, found := positions[buttonGroupPositionKey{divider.Group, divider.Index}]
			if !found {
				x = positions[buttonGroupPositionKey{divider.Group, -1}]
			}
			inset := min(max(divider.Inset, 0), group.Height*0.5)
			transform.Translation.X.Value = x - divider.Width*0.5
			transform.Translation.Y.Value = inset
			size.Width = divider.Width
			size.Height = group.Height - inset*2
			setUIStackIndex(world, entity, group.StackIndex+2)
			surface := styleFor(world, divider.Group, skin.ButtonGroup).Divider
			if divider.CustomColor {
				surface = ColorSurface(divider.Color)
			}
			applySurface(world, entity, size.Width, size.Height, surface)
			return true
		}(entity, access.A, access.B, access.C) {
			break
		}
	}
}

func syncJoinedEdges(world *ecs.World, skin Skin) {
	for entity, access := range world.Query4[joinedEdge, scene.UITransform, scene.Size2D, scene.Visibility]().All {
		if !func(entity ecs.Entity, edgeAccess ecs.ComponentAccess[joinedEdge], transformAccess ecs.ComponentAccess[scene.UITransform], sizeAccess ecs.ComponentAccess[scene.Size2D], visibilityAccess ecs.ComponentAccess[scene.Visibility]) bool {
			edge := edgeAccess.Ref()
			nextTransform, nextSize := transformAccess.Get(), sizeAccess.Get()
			transform, size := &nextTransform, &nextSize
			defer func() {
				ecs.CommitComponent(transformAccess, nextTransform)
				ecs.CommitComponent(sizeAccess, nextSize)
			}()
			nextVisibility := visibilityAccess.Get()
			nextVisibility.Visible = hierarchyVisible(world, edge.Owner)
			ecs.CommitComponent(visibilityAccess, nextVisibility)
			control, ok := controlViewFor(world, edge.Owner)
			if !ok {
				size.Width = 0
				size.Height = 0
				return true
			}
			radius := cornerRadiusPixels(world, edge.Owner, control.Width, control.Height)
			transform.Translation.X.Value = 0
			transform.Translation.Y.Value = 0
			switch edge.Side {
			case joinedEdgeRight:
				transform.Translation.X.Value = control.Width - radius
			case joinedEdgeBottom:
				transform.Translation.Y.Value = control.Height - radius
			}
			if edge.Side == joinedEdgeTop || edge.Side == joinedEdgeBottom {
				size.Width = control.Width
				size.Height = radius
			} else {
				size.Width = radius
				size.Height = control.Height
			}
			setUIStackIndex(world, entity, control.StackIndex+1)
			surface := controlSurface(world, edge.Owner, &control, skin)
			if item := world.GetRef[TabItem](edge.Owner); item != nil {
				if tabs := world.GetRef[Tab](item.Tabs); tabs != nil && tabs.ItemBorders {
					surface.Border = scene.Shape2D{}
				}
			}
			if _, ok := world.Get[Button](edge.Owner); ok {
				var style ButtonStyle
				if item, grouped := world.Get[buttonGroupItem](edge.Owner); grouped {
					style = styleFor(world, item.Group, skin.ButtonGroup).Button
				} else if world.Has[Menu](edge.Owner) {
					style = styleFor(world, edge.Owner, skin.Menu).Trigger
				} else {
					variant, _ := world.Get[ButtonVariant](edge.Owner)
					style = buttonStyleFor(skin, variant, world.Has[Danger](edge.Owner))
					style = styleFor(world, edge.Owner, style)
				}
				surface = style.JoinedEdge.Resolve(controlState(&control, false))
			}
			applySurface(world, entity, size.Width, size.Height, surface)
			return true
		}(entity, access.A, access.B, access.C, access.D) {
			break
		}
	}
}

func syncProgress(world *ecs.World, skin Skin) {
	for entity, access := range world.Query[Progress]().All {
		progress := access.Get()

		widgetWidth := float32(0)
		widgetHeight := float32(0)
		if size := world.GetRef[scene.Size2D](entity); size != nil {
			widgetWidth = size.Width
			widgetHeight = size.Height
		}
		fillWidth := widgetWidth * progressRatio(progress)
		if progress.Indeterminate {
			fillWidth = widgetWidth * 0.35
		}
		style := styleFor(world, entity, skin.Progress)
		applySurface(world, progress.Fill, fillWidth, widgetHeight, style.Fill)
		if progress.Thumb != 0 {
			setVisible(world, progress.Thumb, progress.Slider && !progress.Indeterminate)
			thumbSize := float32(0)
			if control, exists := controlViewFor(world, progress.Thumb); exists {
				thumbSize = control.Width
			}
			setUIPosition(world, progress.Thumb, fillWidth-thumbSize*.5, (widgetHeight-thumbSize)*.5)
		}
	}
}

func syncImageGridSurfaces(world *ecs.World, skin Skin) {
	for entity, access := range world.Query2[imageGridCell, scene.Size2D]().All {
		cell := access.A.Ref()
		size := access.B.Ref()

		style := styleFor(world, cell.Grid, skin.ImageGrid)
		applySurface(world, entity, size.Width, size.Height, style.Cell)
	}
}

func syncTextVisibility(world *ecs.World) {
	for entity, access := range world.Query2[Relation[scene.Text2D], scene.Visibility]().All {
		if !func(entity ecs.Entity, visualAccess ecs.ComponentAccess[Relation[scene.Text2D]], visibilityAccess ecs.ComponentAccess[scene.Visibility]) bool {
			// Generated picker text follows its option, while DOM text retains
			// the visibility computed by the HTML renderer.
			if world.Has[Relation[Control]](entity) && !world.Has[htmlSelectorElement](entity) {
				return true
			}
			visual := visualAccess.Ref()
			nextVisibility := *visibilityAccess.Ref()
			visibility := &nextVisibility
			defer func() { ecs.CommitComponent(visibilityAccess, nextVisibility) }()
			ownerVisibility := world.GetRef[scene.Visibility](visual.Owner)
			visibility.Visible = ownerVisibility == nil || ownerVisibility.Visible
			return true
		}(entity, access.A, access.B) {
			break
		}
	}
}

func syncSurfaceElementVisibility(world *ecs.World) {
	for entity, access := range world.Query2[Relation[SurfaceStyle], scene.Visibility]().All {
		if !func(entity ecs.Entity, visualAccess ecs.ComponentAccess[Relation[SurfaceStyle]], visibilityAccess ecs.ComponentAccess[scene.Visibility]) bool {
			if world.Has[Relation[Control]](entity) {
				return true
			}
			visual := visualAccess.Ref()
			nextVisibility := *visibilityAccess.Ref()
			visibility := &nextVisibility
			defer func() { ecs.CommitComponent(visibilityAccess, nextVisibility) }()
			ownerVisibility := world.GetRef[scene.Visibility](visual.Owner)
			visibility.Visible = ownerVisibility == nil || ownerVisibility.Visible
			switch {
			case world.Has[Part[MenuPanelPart]](entity):
				menu := world.GetRef[Menu](visual.Owner)
				visibility.Visible = menu != nil && menu.Open
			case world.Has[Part[TabItemBorderPart]](entity):
				item := world.GetRef[TabItem](visual.Owner)
				side := world.GetRef[TabItemBorder](entity)
				if item != nil {
					tabs := world.GetRef[Tab](item.Tabs)
					visibility.Visible = (visibility.Visible) && (side != nil) && (tabs != nil) && (tabs.ItemBorders) && (tabs.Selected != item.Index || side.Side != TabItemBorderBottom)
				} else {
					visibility.Visible = false
				}
			case world.Has[Part[CaretPart]](entity):
				_, _, _, focused := textFieldState(world, visual.Owner)
				visibility.Visible = visibility.Visible && focused && caretVisible(world)
			case world.Has[Part[SelectionPart]](entity):
				start, end, focused := textFieldSelection(world, visual.Owner)
				visibility.Visible = visibility.Visible && focused && start != end
			}
			return true
		}(entity, access.A, access.B) {
			break
		}
	}
}

func syncSurfaceElements(world *ecs.World, skin Skin, metrics syncMetrics) {
	var valueTexts map[ecs.Entity]ecs.Entity
	var scrollTexts map[ecs.Entity]textareaScrollMetrics
	for entity, access := range world.Query3[Relation[SurfaceStyle], scene.UITransform, scene.Size2D]().All {
		if !func(entity ecs.Entity, visualAccess ecs.ComponentAccess[Relation[SurfaceStyle]], transformAccess ecs.ComponentAccess[scene.UITransform], sizeAccess ecs.ComponentAccess[scene.Size2D]) bool {
			visual := visualAccess.Ref()
			nextTransform, nextSize := *transformAccess.Ref(), *sizeAccess.Ref()
			transform, size := &nextTransform, &nextSize
			defer func() { ecs.CommitComponent(transformAccess, nextTransform) }()
			switch {
			case world.Has[Part[BodyPart]](entity):
				control, ok := controlViewFor(world, visual.Owner)
				if ok {
					transform.Translation.X.Value, transform.Translation.Y.Value = 0, 0
					setUIStackIndex(world, entity, control.StackIndex)
					surface := controlSurface(world, visual.Owner, &control, skin)
					if item := world.GetRef[TabItem](visual.Owner); item != nil {
						if tabs := world.GetRef[Tab](item.Tabs); tabs != nil && tabs.ItemBorders {
							surface.Border = scene.Shape2D{}
						}
					}
					applySurface(world, entity, control.Width, control.Height, surface)
				}
			case world.Has[Part[IconPart]](entity):
				syncIconElement(world, entity, visual.Owner, transform, size, metrics.labelWidths[visual.Owner])
			case world.Has[Part[TrackPart]](entity):
				syncCheckboxElement(world, entity, visual.Owner, false, transform, size, skin)
			case world.Has[Part[KnobPart]](entity):
				syncCheckboxElement(world, entity, visual.Owner, true, transform, size, skin)
			case world.Has[Part[FrameBorderPart]](entity):
				syncFrameBorder(world, entity, visual.Owner, transform, size, skin)
			case world.Has[Part[TabItemBorderPart]](entity):
				syncTabItemBorder(world, entity, visual.Owner, transform, size, skin)
			case world.Has[Part[NumberInputBorderPart]](entity):
				syncNumberInputBorder(world, entity, visual.Owner, transform, size, skin)
			case world.Has[Part[NumberStepIconPart]](entity):
				syncNumberInputStepIcon(world, entity, visual.Owner, transform, size)
			case world.Has[Part[SelectorIconPart]](entity):
				syncSelectorIcon(world, entity, visual.Owner, transform, size)
			case world.Has[Part[SelectorPanelPart]](entity):
				syncSelectorPanel(world, entity, visual.Owner, transform, size, skin)
			case world.Has[Part[SelectorPanelBorderPart]](entity):
				syncSelectorPanelBorder(world, entity, visual.Owner, transform, size, skin)
			case world.Has[Part[SelectorScrollTrackPart]](entity):
				syncSelectorScrollElement(world, entity, visual.Owner, false, transform, size, skin, metrics.selectors)
			case world.Has[Part[SelectorScrollThumbPart]](entity):
				syncSelectorScrollElement(world, entity, visual.Owner, true, transform, size, skin, metrics.selectors)
			case world.Has[Part[MenuPanelPart]](entity):
				syncMenuPanelElement(world, entity, visual.Owner, transform, size, skin, metrics.menus)
			case world.Has[Part[ContextMenuPanelPart]](entity):
				syncContextMenuPanelElement(world, entity, visual.Owner, transform, size, skin, metrics.contextMenus)
			case isScrollPart(world, entity):
				if scrollTexts == nil {
					scrollTexts = world.FrameScratchMap[textareaScrollMetrics, ecs.Entity, textareaScrollMetrics]()
				}
				syncScrollAreaElement(world, entity, visual.Owner, transform, size, skin, scrollTexts)
			case world.Has[Part[CaretPart]](entity):
				if valueTexts == nil {
					valueTexts = partEntities[ValuePart, scene.Text2D](world)
				}
				syncCaretElement(world, entity, visual.Owner, valueTexts[visual.Owner], transform, size, skin)
			case world.Has[Part[SelectionPart]](entity):
				if valueTexts == nil {
					valueTexts = partEntities[ValuePart, scene.Text2D](world)
				}
				indexed, _ := world.Get[Indexed[SelectionPart]](entity)
				syncSelectionElement(world, entity, visual.Owner, valueTexts[visual.Owner], indexed.Value, transform, size, skin)
			}
			return true
		}(entity, access.A, access.B, access.C) {
			break
		}
	}
}

func prepareScrollAreas(world *ecs.World) {
	for entity, access := range world.Query2[ScrollArea, scene.ClipRect2D]().All {
		area := access.A.Ref()
		clip := access.B.Ref()

		nextClip := *clip
		prepareScrollArea(world, entity, area, &nextClip)
		world.SetIfChanged(entity, nextClip)
	}
}

func prepareScrollArea(world *ecs.World, entity ecs.Entity, area *ScrollArea, clip *scene.ClipRect2D) {
	metrics := measureScrollArea(area.Width, area.Height, area.ContentWidth, area.ContentHeight, area.BarSize)
	if clip != nil {
		clip.X, clip.Y = 0, 0
		clip.Width, clip.Height = metrics.ViewportWidth, metrics.ViewportHeight
	}
	scrollValue, scrollOK := world.Get[scene.UIScroll](entity)
	if !scrollOK {
		return
	}
	scroll := &scrollValue
	world.SetIfChanged(entity, scene.Size2D{Width: area.Width, Height: area.Height})
	scroll.ViewportSize = mgl32.Vec2{metrics.ViewportWidth, metrics.ViewportHeight}
	scroll.ContentSize = mgl32.Vec2{area.ContentWidth, area.ContentHeight}
	scroll.WheelStep = scrollWheelStep(area.WheelStep)
	scroll.DisableInput = area.Controlled || !hierarchyVisible(world, entity)
	scroll.ScrollbarSize = scrollBarSize(area.BarSize)
	scroll.ScrollbarMinThumbSize = scrollMinThumbSize(area.MinThumbSize)
	scroll.DisableDrag = area.DisableDrag
	scroll.Horizontal = metrics.HorizontalVisible
	scroll.Vertical = metrics.VerticalVisible
	world.SetIfChanged(entity, scrollValue)
}

func finalizeScrollAreas(world *ecs.World) {
	for entity, access := range world.Query2[ScrollContent, scene.UITransform]().All {
		if !func(_ ecs.Entity, contentAccess ecs.ComponentAccess[ScrollContent], transformAccess ecs.ComponentAccess[scene.UITransform]) bool {
			content := contentAccess.Ref()
			nextTransform := *transformAccess.Ref()
			transform := &nextTransform
			defer func() { ecs.CommitComponent(transformAccess, nextTransform) }()
			offset, ok := GetScrollAreaOffset(world, content.Area)
			if !ok {
				return true
			}
			transform.Translation.X.Value = content.X - offset.X()
			transform.Translation.Y.Value = content.Y - offset.Y()
			return true
		}(entity, access.A, access.B) {
			break
		}
	}
	for entity, access := range world.Query2[html.ScrollContent, scene.UITransform]().All {
		if !func(_ ecs.Entity, contentAccess ecs.ComponentAccess[html.ScrollContent], transformAccess ecs.ComponentAccess[scene.UITransform]) bool {
			content := contentAccess.Ref()
			nextTransform := *transformAccess.Ref()
			transform := &nextTransform
			defer func() { ecs.CommitComponent(transformAccess, nextTransform) }()
			offset, ok := scene.GetUIScrollOffset(world, content.Area)
			if !ok {
				return true
			}
			transform.Translation.X.Value = content.X + offset.X()
			transform.Translation.Y.Value = content.Y + offset.Y()
			return true
		}(entity, access.A, access.B) {
			break
		}
	}
	html.SyncScrollbars(world)
}

func progressRatio(progress Progress) float32 {
	minValue := progress.Min
	maxValue := progress.Max
	if maxValue == minValue {
		maxValue = minValue + 1
	}
	ratio := (progress.Value - minValue) / (maxValue - minValue)
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}

func syncCheckboxElement(world *ecs.World, entity, owner ecs.Entity, knob bool, transform *scene.UITransform, size *scene.Size2D, skin Skin) {
	control, ok := controlViewFor(world, owner)
	if !ok {
		return
	}
	toggle := world.GetRef[Toggle[bool]](owner)
	if toggle == nil {
		return
	}
	boxSize := checkboxBoxSize(control.Height)
	boxY := (control.Height - boxSize) * 0.5
	style := styleFor(world, owner, skin.Checkbox)
	state := controlState(&control, false)
	if !knob {
		transform.Translation.X.Value = 0
		transform.Translation.Y.Value = boxY
		size.Width = boxSize
		size.Height = boxSize
		setUIStackIndex(world, entity, control.StackIndex)
		applySurface(world, entity, boxSize, boxSize, style.Track.Resolve(toggle.Value, state))
	} else {
		if !toggle.Value {
			size.Width = 0
			size.Height = 0
			applySurface(world, entity, 0, 0, SurfaceStyle{})
			return
		}
		markSize := boxSize - 6
		if markSize < 10 {
			markSize = 10
		}
		transform.Translation.X.Value = 3
		transform.Translation.Y.Value = boxY + (boxSize-markSize)*0.5
		size.Width = markSize
		size.Height = markSize
		setUIStackIndex(world, entity, control.StackIndex+1)
		applySurface(world, entity, size.Width, size.Height, style.Mark.Resolve(state))
	}
}

func syncContextMenuPanelElement(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, size *scene.Size2D, skin Skin, metrics map[ecs.Entity]itemMetrics) {
	menu := world.GetRef[ContextMenu](owner)
	if menu == nil {
		return
	}
	transform.Translation.X.Value = 0
	transform.Translation.Y.Value = 0
	padding := menuPadding(menu.Padding)
	metric := metrics[owner]
	contentWidth := metric.width
	height := itemPanelHeight(metric.count, menu.ItemHeight, padding)
	size.Width = menuPanelWidth(menu.Width, padding, contentWidth)
	size.Height = height
	setUIStackIndex(world, entity, menu.StackIndex+1)
	style := styleFor(world, owner, skin.ContextMenu)
	applySurface(world, entity, size.Width, size.Height, style.Popup)
}

func syncMenuPanelElement(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, size *scene.Size2D, skin Skin, metrics map[ecs.Entity]itemMetrics) {
	menu := world.GetRef[Menu](owner)
	if menu == nil {
		return
	}
	padding := menuPadding(menu.Padding)
	metric := metrics[owner]
	contentWidth := metric.width
	transform.Translation.X.Value = 0
	transform.Translation.Y.Value = menu.ItemHeight
	size.Width = menuPanelWidth(menu.Width, padding, contentWidth)
	size.Height = itemPanelHeight(metric.count, menu.ItemHeight, padding)
	setUIStackIndex(world, entity, menu.StackIndex+1)
	style := styleFor(world, owner, skin.Menu)
	applySurface(world, entity, size.Width, size.Height, style.Popup)
}

func syncScrollAreaElement(world *ecs.World, entity, owner ecs.Entity, transform *scene.UITransform, size *scene.Size2D, skin Skin, scrollTexts map[ecs.Entity]textareaScrollMetrics) {
	if textarea := world.GetRef[Textarea](owner); textarea != nil {
		syncTextareaScrollElement(world, entity, owner, textarea, transform, size, skin, scrollTexts)
		return
	}
	area := world.GetRef[ScrollArea](owner)
	if area == nil {
		return
	}
	metrics := measureScrollArea(area.Width, area.Height, area.ContentWidth, area.ContentHeight, area.BarSize)
	offset, _ := GetScrollAreaOffset(world, owner)
	barSize := scrollBarSize(area.BarSize)
	thumbBarSize := scrollThumbBarSize(area.ThumbBarSize, area.BarSize)
	hoverBarSize := scrollHoverBarSize(area.HoverBarSize, area.ThumbBarSize, area.BarSize)
	barStackIndex := area.StackIndex
	if area.ScrollBarStackIndex != 0 {
		barStackIndex = area.ScrollBarStackIndex
	}
	style := styleFor(world, owner, skin.Scroll)
	switch {
	case world.Has[Part[ScrollTrackHorizontalPart]](entity):
		transform.Translation.X.Value = 0
		transform.Translation.Y.Value = max(area.Height-barSize, 0)
		size.Width = visibleLength(metrics.HorizontalVisible, metrics.ViewportWidth)
		size.Height = visibleLength(metrics.HorizontalVisible, barSize)
		setUIStackIndex(world, entity, barStackIndex)
		applySurface(world, entity, size.Width, size.Height, style.Track)
	case world.Has[Part[ScrollTrackVerticalPart]](entity):
		transform.Translation.X.Value = max(area.Width-barSize, 0)
		transform.Translation.Y.Value = 0
		size.Width = visibleLength(metrics.VerticalVisible, barSize)
		size.Height = visibleLength(metrics.VerticalVisible, metrics.ViewportHeight)
		setUIStackIndex(world, entity, barStackIndex)
		applySurface(world, entity, size.Width, size.Height, style.Track)
	case world.Has[Part[ScrollThumbHorizontalPart]](entity):
		thumbHeight := thumbBarSize
		if area.HoverHorizontal || area.DraggingHorizontal {
			thumbHeight = hoverBarSize
		}
		thumbWidth := scrollThumbSize(metrics.ViewportWidth, metrics.ViewportWidth, area.ContentWidth, area.MinThumbSize)
		transform.Translation.X.Value = scrollThumbOffset(metrics.ViewportWidth, thumbWidth, offset.X(), metrics.MaxScrollX)
		transform.Translation.Y.Value = max(area.Height-barSize, 0) + (barSize-thumbHeight)*0.5
		size.Width = visibleLength(metrics.HorizontalVisible, thumbWidth)
		size.Height = visibleLength(metrics.HorizontalVisible, thumbHeight)
		setUIStackIndex(world, entity, barStackIndex+1)
		state := ControlState{Hovered: area.HoverHorizontal || area.DraggingHorizontal, Pressed: area.DraggingHorizontal}
		applySurface(world, entity, size.Width, size.Height, style.Thumb.Resolve(state))
	case world.Has[Part[ScrollThumbVerticalPart]](entity):
		thumbWidth := thumbBarSize
		if area.HoverVertical || area.DraggingVertical {
			thumbWidth = hoverBarSize
		}
		thumbHeight := scrollThumbSize(metrics.ViewportHeight, metrics.ViewportHeight, area.ContentHeight, area.MinThumbSize)
		transform.Translation.X.Value = max(area.Width-barSize, 0) + (barSize-thumbWidth)*0.5
		transform.Translation.Y.Value = scrollThumbOffset(metrics.ViewportHeight, thumbHeight, offset.Y(), metrics.MaxScrollY)
		size.Width = visibleLength(metrics.VerticalVisible, thumbWidth)
		size.Height = visibleLength(metrics.VerticalVisible, thumbHeight)
		setUIStackIndex(world, entity, barStackIndex+1)
		state := ControlState{Hovered: area.HoverVertical || area.DraggingVertical, Pressed: area.DraggingVertical}
		applySurface(world, entity, size.Width, size.Height, style.Thumb.Resolve(state))
	}
}

type textareaScrollMetrics struct {
	textareaScrollGeometry
	control   controlView
	hoverAxis uint8
	valid     bool
}

func measureTextareaScroll(world *ecs.World, owner ecs.Entity) textareaScrollMetrics {
	control, ok := controlViewFor(world, owner)
	geometry, measured := textareaScrollGeometryFor(world, owner)
	if !ok || !measured {
		return textareaScrollMetrics{}
	}
	metrics := textareaScrollMetrics{textareaScrollGeometry: geometry, control: control, valid: true}
	if state, ok := world.GetResource[*input.State](); ok && state != nil {
		pointer := state.Pointer()
		if x, y, ok := textareaLocalPoint(world, owner, pointer.X, pointer.Y); ok {
			metrics.hoverAxis = textareaScrollAxisAt(geometry, x, y)
		}
	}
	return metrics
}

func syncTextareaScrollElement(world *ecs.World, entity, owner ecs.Entity, textarea *Textarea, transform *scene.UITransform, size *scene.Size2D, skin Skin, scrollTexts map[ecs.Entity]textareaScrollMetrics) {
	metrics, exists := scrollTexts[owner]
	if !exists {
		metrics = measureTextareaScroll(world, owner)
		scrollTexts[owner] = metrics
	}
	if !metrics.valid {
		applySurface(world, entity, 0, 0, SurfaceStyle{})
		return
	}
	control := metrics.control
	contentWidth, contentHeight := metrics.contentWidth, metrics.contentHeight
	trackX, trackY := metrics.trackX, metrics.trackY
	viewportWidth, viewportHeight := metrics.viewportWidth, metrics.viewportHeight
	horizontal, vertical := metrics.horizontal, metrics.vertical
	barSize := metrics.barSize
	trackWidth, trackHeight := metrics.trackWidth, metrics.trackHeight
	style := styleFor(world, owner, skin.Scroll)
	barStackIndex := control.StackIndex + 3
	if world.Has[Part[ScrollThumbHorizontalPart]](entity) || world.Has[Part[ScrollThumbVerticalPart]](entity) {
		barStackIndex++
	}
	setUIStackIndex(world, entity, barStackIndex)
	switch {
	case world.Has[Part[ScrollTrackHorizontalPart]](entity):
		transform.Translation.X.Value = trackX
		transform.Translation.Y.Value = metrics.horizontalY
		size.Width = visibleLength(horizontal, trackWidth)
		size.Height = visibleLength(horizontal, barSize)
		applySurface(world, entity, size.Width, size.Height, style.Track)
	case world.Has[Part[ScrollTrackVerticalPart]](entity):
		transform.Translation.X.Value = metrics.verticalX
		transform.Translation.Y.Value = trackY
		size.Width = visibleLength(vertical, barSize)
		size.Height = visibleLength(vertical, trackHeight)
		applySurface(world, entity, size.Width, size.Height, style.Track)
	case world.Has[Part[ScrollThumbHorizontalPart]](entity):
		state := ControlState{Hovered: metrics.hoverAxis == textareaScrollAxisHorizontal, Pressed: textarea.scrollDragging && textarea.scrollDragAxis == textareaScrollAxisHorizontal}
		inset := min(max(barSize*.2, 1), 3)
		if state.Hovered || state.Pressed {
			inset = 0
		}
		thumbWidth := scrollThumbSize(trackWidth, viewportWidth, contentWidth, 20)
		transform.Translation.X.Value = trackX + scrollThumbOffset(trackWidth, thumbWidth, textarea.ScrollX, max(contentWidth-viewportWidth, 0))
		transform.Translation.Y.Value = metrics.horizontalY + inset
		size.Width = visibleLength(horizontal, thumbWidth)
		size.Height = visibleLength(horizontal, max(barSize-inset*2, 0))
		applySurface(world, entity, size.Width, size.Height, style.Thumb.Resolve(state))
	case world.Has[Part[ScrollThumbVerticalPart]](entity):
		state := ControlState{Hovered: metrics.hoverAxis == textareaScrollAxisVertical, Pressed: textarea.scrollDragging && textarea.scrollDragAxis == textareaScrollAxisVertical}
		inset := min(max(barSize*.2, 1), 3)
		if state.Hovered || state.Pressed {
			inset = 0
		}
		thumbHeight := scrollThumbSize(trackHeight, viewportHeight, contentHeight, 20)
		transform.Translation.X.Value = metrics.verticalX + inset
		transform.Translation.Y.Value = trackY + scrollThumbOffset(trackHeight, thumbHeight, textarea.ScrollY, max(contentHeight-viewportHeight, 0))
		size.Width = visibleLength(vertical, max(barSize-inset*2, 0))
		size.Height = visibleLength(vertical, thumbHeight)
		applySurface(world, entity, size.Width, size.Height, style.Thumb.Resolve(state))
	}
}

func syncCornerRadii(world *ecs.World) {
	for entity, access := range world.Query2[CornerRadius, scene.Size2D]().All {
		radiusAccess := access.A
		sizeAccess := access.B

		radius, size := radiusAccess.Ref(), sizeAccess.Ref()
		pixels := radius.pixels(size.Width, size.Height)
		if background, ok := world.Get[scene.UIBackground2D](entity); ok && background.Shape.Kind != 0 {
			background.Shape.Radius = pixels
			world.SetIfChanged(entity, background)
		}
		if border, ok := world.Get[scene.UIBorder2D](entity); ok && border.Shape.Kind != 0 {
			border.Shape.Radius = pixels
			world.SetIfChanged(entity, border)
		}
	}
}

func cornerRadiusPixels(world *ecs.World, entity ecs.Entity, width, height float32) float32 {
	radius, ok := world.Get[CornerRadius](entity)
	if !ok {
		return 0
	}
	return radius.pixels(width, height)
}

func visibleLength(visible bool, size float32) float32 {
	if !visible {
		return 0
	}
	return size
}

func itemPanelHeight(count int, itemHeight, padding float32) float32 {
	return itemHeight*float32(count) + padding*2
}

func collectSyncMetrics(world *ecs.World) syncMetrics {
	metrics := syncMetrics{
		menus:        collectItemMetricCounts[Menu](world),
		contextMenus: collectItemMetricCounts[ContextMenu](world),
		selectors:    collectItemMetricCounts[Selector](world),
	}
	for entity, access := range world.Query2[Relation[scene.Text2D], scene.Text2D]().All {
		visual := access.A.Ref()
		text := access.B.Ref()

		itemPart := world.Has[Part[ItemPart]](entity)
		menuOwner := itemMetricOwner[Menu](world, entity, visual.Owner, itemPart, true, metrics.menus)
		contextOwner := itemMetricOwner[ContextMenu](world, entity, visual.Owner, itemPart, false, metrics.contextMenus)
		selectorOwner := itemMetricOwner[Selector](world, entity, visual.Owner, itemPart, false, metrics.selectors)
		if menuOwner != 0 || contextOwner != 0 || selectorOwner != 0 {
			// Measurement may refresh Text2D's internal cache; it is not a semantic edit.
			if width, _, ok := scene.MeasureTextValue2D(text, text.Value); ok {
				updateItemMetricWidth(metrics.menus, menuOwner, width)
				updateItemMetricWidth(metrics.contextMenus, contextOwner, width)
				updateItemMetricWidth(metrics.selectors, selectorOwner, width)
			}
		}
	}
	return metrics
}

func collectItemMetricCounts[K any](world *ecs.World) map[ecs.Entity]itemMetrics {
	metrics := world.FrameScratchMap[itemMetricsScratch[K], ecs.Entity, itemMetrics]()
	for entity, _ := range world.Query[K]().All {
		metrics[entity] = itemMetrics{}
	}
	for _, access := range world.Query[Item[K]]().All {
		item := access.Ref()

		metric := metrics[item.Owner]
		metric.count++
		metrics[item.Owner] = metric
	}
	return metrics
}

func itemMetricOwner[K any](world *ecs.World, entity, owner ecs.Entity, itemPart, includeOwnerLabel bool, metrics map[ecs.Entity]itemMetrics) ecs.Entity {
	if itemPart {
		if item := world.GetRef[Item[K]](owner); item != nil {
			return item.Owner
		}
	} else if includeOwnerLabel && world.Has[Part[LabelPart]](entity) {
		if _, ok := metrics[owner]; ok {
			return owner
		}
	}
	return 0
}

func updateItemMetricWidth(metrics map[ecs.Entity]itemMetrics, owner ecs.Entity, width float32) {
	if owner != 0 {
		metric := metrics[owner]
		metric.width = max(metric.width, width)
		metrics[owner] = metric
	}
}

func menuPanelWidth(minWidth, padding, contentWidth float32) float32 {
	return max(minWidth, contentWidth+menuTextPaddingX*2+padding*2)
}
