package ui

import (
	"slices"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/input"
	"github.com/onix-engine/onix/scene"
	htmlui "github.com/onix-engine/onix/ui/html"
)

type htmlControlState struct {
	RenderRevision uint64
	SpacePressed   bool
	FocusVisible   bool
}

type htmlEventReaders struct {
	Events          ecs.MessageReader[scene.InputEvent]
	Focus           ecs.MessageReader[FocusChangedEvent]
	PressedSelector ecs.Entity
}

type htmlEventReadersTag struct{}
type htmlControlsScratch struct{}
type htmlFocusRebuildState struct {
	Field        TextField
	Focusable    htmlui.Focusable
	Textarea     Textarea
	ControlState htmlControlState
	Entity       ecs.Entity
	Ordinal      int
	HasField     bool
	HasTextarea  bool
	HasState     bool
	FocusVisible bool
}
type htmlSelectorRuntime struct {
	Options       []htmlui.SelectOption
	Items         []ecs.Entity
	Elements      []ecs.Entity
	List          ecs.Entity
	Shape         scene.Shape2D
	Width, Height float32
}
type htmlSelectorRetirements struct {
	entities []ecs.Entity
	bound    bool
}
type htmlSelectorElement struct {
	Owner ecs.Entity
}
type htmlSelectorPopup struct {
	Radius float32
}
type htmlAutofocusApplied struct{}

func SetHTMLControlValue(world *ecs.World, control ecs.Entity, value string) bool {
	return htmlui.SetControlValue(world, control, value)
}

func SetHTMLControlChecked(world *ecs.World, control ecs.Entity, checked bool) bool {
	if !htmlui.SetControlChecked(world, control, checked) {
		return false
	}
	if current, ok := world.Get[htmlui.Control](control); ok {
		refreshHTMLControlText(world, control, current)
	}
	return true
}

func SetHTMLControlSelected(world *ecs.World, control ecs.Entity, selected int) bool {
	if !htmlui.SetControlSelected(world, control, selected) {
		return false
	}
	syncHTMLSelectorState(world, control)
	return true
}

func SetHTMLControlOptionSelected(world *ecs.World, control ecs.Entity, option int, selected bool) bool {
	if !htmlui.SetControlOptionSelected(world, control, option, selected) {
		return false
	}
	syncHTMLSelectorState(world, control)
	return true
}

func SetHTMLControlSelectionRange(world *ecs.World, control ecs.Entity, anchor, focus int) bool {
	if !htmlui.SetControlSelectionRange(world, control, anchor, focus) {
		return false
	}
	syncHTMLSelectorState(world, control)
	return true
}

func SetHTMLEditableValue(world *ecs.World, editable ecs.Entity, value string) bool {
	return htmlui.SetEditableValue(world, editable, value)
}

func SetHTMLEditableSelection(world *ecs.World, editable ecs.Entity, anchor, focus int) bool {
	return htmlui.SetEditableSelection(world, editable, anchor, focus)
}

func ResetHTMLForm(world *ecs.World, owner ecs.Entity, formID string) bool {
	if !htmlui.HasForm(world, owner, formID) {
		return false
	}
	event := htmlui.PublishEvent(world, htmlui.Event{Entity: owner, ID: formID, FormID: formID, Kind: htmlui.EventReset, Bubbles: true, Cancelable: true})
	if event.DefaultPrevented || !htmlui.ResetForm(world, owner, formID) {
		return false
	}
	syncHTMLControlsAfterReset(world, owner)
	return true
}

func SyncHTMLSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "sync_html"
	}
	return ecs.SystemConfig{Name: name, System: syncHTMLRuntime}
}

func SyncHTML(world *ecs.World) error {
	return SyncHTMLSystem("").Run(world)
}

// RefreshHTMLScope includes newly created UI children in the current scene query scope.
type RefreshHTMLScope func()

func refreshUIChildren(world *ecs.World) {
	if refresh, ok := world.GetResource[RefreshHTMLScope](); ok {
		refresh()
	}
}

func syncHTMLRuntime(context *ecs.SystemContext) error {
	if context == nil || context.World == nil {
		return nil
	}
	request, _ := context.World.GetResource[*scene.RenderRequest]()
	needsFrame := request != nil && htmlui.NeedsFrame(context.World)
	state, preserve := captureHTMLFocusRebuildState(context.World)
	if err := htmlui.SyncSystem("").System(context); err != nil {
		return err
	}
	if needsFrame {
		request.Request()
	}
	replacedFocus := preserve && (!context.World.IsAlive(state.Entity) || !context.World.Has[htmlui.Focusable](state.Entity))
	if replacedFocus {
		// Enrich the replacement before restoring native focus and composition.
		refreshUIChildren(context.World)
	}
	enrichHTMLControls(context.World)
	if replacedFocus {
		restoreHTMLFocusRebuildState(context.World, state)
	}
	if inputState, ok := context.World.GetResource[*uiEventState](); ok && inputState != nil && inputState.Focused != 0 && !scene.IsInteractive(context.World, inputState.Focused) {
		focus(context.World, inputState, 0)
	}
	autofocusHTMLElement(context.World)
	return nil
}

func captureHTMLFocusRebuildState(world *ecs.World) (htmlFocusRebuildState, bool) {
	input, ok := world.GetResource[*uiEventState]()
	if !ok || input == nil || input.Focused == 0 {
		return htmlFocusRebuildState{}, false
	}
	entity := input.Focused
	focusable, ok := world.Get[htmlui.Focusable](entity)
	if !ok {
		return htmlFocusRebuildState{}, false
	}
	ordinal := 0
	if focusable.ID == "" {
		lookup := htmlFocusLookup(world)
		var found bool
		ordinal, found = lookup.ordinals[entity]
		if !found {
			ordinal = len(lookup.groups[htmlFocusGroup{owner: focusable.Owner, tag: focusable.Tag, kind: focusable.Type}])
		}
	}
	state := htmlFocusRebuildState{Entity: entity, Focusable: focusable, Ordinal: ordinal, FocusVisible: input.FocusVisible}
	state.Field, state.HasField = world.Get[TextField](entity)
	if state.HasField && !state.Field.Focused {
		return htmlFocusRebuildState{}, false
	}
	state.Textarea, state.HasTextarea = world.Get[Textarea](entity)
	state.ControlState, state.HasState = world.Get[htmlControlState](entity)
	return state, true
}

func restoreHTMLFocusRebuildState(world *ecs.World, state htmlFocusRebuildState) {
	entity, found := matchingHTMLFocusable(world, state.Focusable, state.Ordinal)
	if !found {
		if input, exists := world.GetResource[*uiEventState](); exists && input != nil {
			if input.Focused == state.Entity {
				input.Focused = 0
			}
			if input.ActivatedOnPress == state.Entity {
				input.ActivatedOnPress = 0
			}
			if input.LastInputClick == state.Entity {
				input.LastInputClick = 0
			}
		}
		if focus, exists := world.GetResource[*scene.InputFocus](); exists && focus != nil && focus.Keyboard == state.Entity {
			focus.Keyboard = 0
		}
		if capture, exists := world.GetResource[*scene.InputCapture2D](); exists && capture != nil {
			if capture.Target == state.Entity {
				capture.Target = 0
			}
			if capture.Receiver == state.Entity {
				capture.Receiver = 0
			}
		}
		if uiInputState, exists := world.GetResource[*input.State](); exists && uiInputState != nil {
			uiInputState.BlurTextInput()
		}
		return
	}
	field := world.GetMut[TextField](entity)
	if state.HasField && (field == nil) {
		return
	}
	if state.HasField {
		authored := *field
		*field = state.Field
		field.Placeholder, field.Password, field.MaxRunes = authored.Placeholder, authored.Password, authored.MaxRunes
		field.FontSize, field.DPI = authored.FontSize, authored.DPI
		if field.Buffer == nil {
			field.Value, field.CommittedValue = authored.Value, authored.CommittedValue
		}
		length := field.Text().Runes()
		field.Caret = max(0, min(field.Caret, length))
		field.Anchor = max(0, min(field.Anchor, length))

	}
	if state.HasTextarea {
		if textarea := world.GetMut[Textarea](entity); textarea != nil {
			wrap, padding, barSize, disableHorizontalScroll := textarea.Wrap, textarea.Padding, textarea.BarSize, textarea.DisableHorizontalScroll
			*textarea = state.Textarea
			textarea.Wrap, textarea.Padding, textarea.BarSize = wrap, padding, barSize
			textarea.DisableHorizontalScroll = disableHorizontalScroll
		}
	}
	if state.HasState {
		if controlState := world.GetMut[htmlControlState](entity); controlState != nil {
			*controlState = state.ControlState
		}
	}
	if input, exists := world.GetResource[*uiEventState](); exists && input != nil {
		if input.Focused == state.Entity {
			input.Focused = entity
		}
		if input.ActivatedOnPress == state.Entity {
			input.ActivatedOnPress = entity
		}
		if input.LastInputClick == state.Entity {
			input.LastInputClick = entity
		}
	}
	if focus, exists := world.GetResource[*scene.InputFocus](); exists && focus != nil && focus.Keyboard == state.Entity {
		focus.Keyboard = entity
	}
	scene.RemapInputEntity2D(world, state.Entity, entity)
	if state.HasField {
		if uiInputState, exists := world.GetResource[*input.State](); exists && uiInputState != nil {
			selectionStart, selectionEnd := orderedSelection(field.Anchor, field.Caret)
			uiInputState.SetTextInputSelection(selectionStart, selectionEnd)
		}
	}
	if !state.HasField || field.Focused {
		htmlui.SetFocusableFocusVisible(world, entity, true, state.FocusVisible)
	}
}

func matchingHTMLFocusable(world *ecs.World, previous htmlui.Focusable, ordinal int) (ecs.Entity, bool) {
	if previous.ID == "" {
		lookup := htmlFocusLookup(world)
		entities := lookup.groups[htmlFocusGroup{owner: previous.Owner, tag: previous.Tag, kind: previous.Type}]
		if ordinal >= 0 && ordinal < len(entities) {
			return entities[ordinal], true
		}
		return 0, false
	}
	for candidate, access := range world.Query[htmlui.Focusable]().All {
		target := access.Get()

		if target.Owner != previous.Owner || target.Tag != previous.Tag || target.Type != previous.Type {
			continue
		}
		if target.ID == previous.ID {
			return candidate, true
		}
	}
	return 0, false
}

func updateHTMLRuntime(context *ecs.SystemContext) error {
	if context == nil || context.World == nil {
		return nil
	}
	world := context.World
	enrichHTMLControls(world)
	readers := context.Local[htmlEventReadersTag, htmlEventReaders]()
	for event := range readers.Events.All(world) {
		syncHTMLInputState(world, event)
		if event.Kind == scene.InputEventCancel {
			readers.PressedSelector = 0
		}
		if event.Handled || !isHTMLActivationTarget(world, event.Entity) || !scene.IsInteractive(world, event.Entity) {
			continue
		}
		switch event.Kind {
		case scene.InputEventPress:
			if control := world.GetRef[htmlui.Control](event.Entity); control != nil && control.Tag == "select" && !control.Disabled {
				readers.PressedSelector = event.Entity
				SetHTMLSelectOpen(world, event.Entity, !control.Open)
				event.Handled = true
			}
			continue
		case scene.InputEventTap:
			event.Handled = true
		default:
			continue
		}
		pressed := readers.PressedSelector
		readers.PressedSelector = 0
		if item, ok := world.Get[SelectorItem](event.Entity); ok {
			handleHTMLSelectTap(world, item)
			continue
		}
		control, ok := world.Get[htmlui.Control](event.Entity)
		if !ok || control.Disabled {
			continue
		}
		controlType := htmlControlType(control)
		if controlType == "checkbox" || controlType == "radio" {
			checked := controlType == "radio" || !control.Checked()
			if checked == control.Checked() {
				writeHTMLEvent(world, event.Entity, control, htmlui.EventClick)
				continue
			}
			if SetHTMLControlChecked(world, event.Entity, checked) {
				control, _ = world.Get[htmlui.Control](event.Entity)
				writeHTMLEvent(world, event.Entity, control, htmlui.EventClick)
				writeHTMLEvent(world, event.Entity, control, htmlui.EventInput)
				writeHTMLEvent(world, event.Entity, control, htmlui.EventChange)
			}
			continue
		}
		if control.Tag == "select" && pressed != event.Entity {
			SetHTMLSelectOpen(world, event.Entity, !control.Open)
		}
		writeHTMLEvent(world, event.Entity, control, htmlui.EventClick)
		switch htmlControlType(control) {
		case "reset":
			if !htmlui.HasControlForm(world, event.Entity) {
				break
			}
			reset := htmlui.PublishEvent(world, htmlui.Event{
				Entity: control.Owner, ID: control.FormID, FormID: control.FormID, Kind: htmlui.EventReset,
				Bubbles: true, Cancelable: true,
			})
			if !reset.DefaultPrevented && htmlui.ResetControlForm(world, event.Entity) {
				syncHTMLControlsAfterReset(world, control.Owner)
			}
		case "submit":
			writeHTMLEvent(world, event.Entity, control, htmlui.EventSubmit)
		}
	}
	for event := range readers.Focus.All(world) {
		focusable, focusableOK := world.Get[htmlui.Focusable](event.Entity)
		if !focusableOK {
			continue
		}
		control, isControl := world.Get[htmlui.Control](event.Entity)
		if !isControl {
			htmlui.SetFocusableFocusVisible(world, event.Entity, event.Focused, event.FocusVisible)
			kind := htmlui.EventBlur
			if event.Focused {
				kind = htmlui.EventFocus
			}
			world.WriteMessage(htmlui.Event{Entity: focusable.Owner, Target: event.Entity, ID: focusable.ID, Kind: kind})
			continue
		}
		if !event.Focused && control.Tag == "select" {
			SetHTMLSelectOpen(world, event.Entity, false)
		}
		if event.Focused {
			if state := world.GetMut[htmlControlState](event.Entity); state != nil {
				state.FocusVisible = event.FocusVisible || isHTMLTextControl(control)
			}
			htmlui.SetControlFocusVisible(world, event.Entity, true, event.FocusVisible)
			writeHTMLEvent(world, event.Entity, control, htmlui.EventFocus)
			continue
		}
		commitHTMLControl(world, event.Entity, control)
		if state := world.GetMut[htmlControlState](event.Entity); state != nil {
			state.FocusVisible = false
		}
		htmlui.SetControlFocus(world, event.Entity, false)
		writeHTMLEvent(world, event.Entity, control, htmlui.EventBlur)
	}
	return nil
}

func syncHTMLControlsAfterReset(world *ecs.World, owner ecs.Entity) {
	if world == nil || owner == 0 {
		return
	}
	for entity, access := range world.Query[htmlui.Control]().All {
		control := access.Get()

		if control.Owner != owner {
			continue
		}
		if control.Tag == "select" && !control.Open {
			setHTMLSelectorVisibility(world, entity, false)
		}
	}
	for _, access := range world.Query2[htmlui.ControlText, scene.Text2D]().All {
		control := world.GetRef[htmlui.Control](access.A.Ref().Control)
		if control != nil && control.Owner == owner {
			syncHTMLControlText(access.B, control)
		}
	}
	if state, ok := world.GetResource[*uiEventState](); ok && state != nil && state.Focused != 0 {
		syncTextInputFocus(world, state.Focused)
	}
}

// syncHTMLInputState applies pointer hover styles to generated HTML
// controls without invalidating the retained DOM or rebuilding the surface.
func syncHTMLInputState(world *ecs.World, event *scene.InputEvent) {
	if world == nil || event == nil {
		return
	}
	control, ok := world.Get[htmlui.Control](event.Entity)
	if !ok {
		switch event.Kind {
		case scene.InputEventEnter:
			htmlui.SetVisualHover(world, event.Entity, true)
		case scene.InputEventLeave:
			htmlui.SetVisualHover(world, event.Entity, false)
		case scene.InputEventPress:
			htmlui.SetVisualActive(world, event.Entity, true)
		case scene.InputEventRelease, scene.InputEventCancel:
			htmlui.SetVisualActive(world, event.Entity, false)
		}
		return
	}
	if control.Owner == 0 {
		return
	}
	switch event.Kind {
	case scene.InputEventEnter:
		htmlui.SetControlHover(world, event.Entity, true)
	case scene.InputEventLeave:
		htmlui.SetControlHover(world, event.Entity, false)
	case scene.InputEventPress:
		htmlui.SetControlActive(world, event.Entity, true)
	case scene.InputEventRelease, scene.InputEventCancel:
		htmlui.SetControlActive(world, event.Entity, false)
	}
}

func isHTMLActivationTarget(world *ecs.World, entity ecs.Entity) bool {
	if world.Has[htmlui.Control](entity) {
		return true
	}
	item := world.GetRef[SelectorItem](entity)
	return item != nil && world.Has[htmlui.Control](item.Owner)
}

// SetHTMLSelectOpen opens or closes the picker for an HTML select.
func SetHTMLSelectOpen(world *ecs.World, entity ecs.Entity, open bool) {
	control := world.GetRef[htmlui.Control](entity)
	if control != nil && control.Tag == "select" && control.Open != open {
		world.GetMut[htmlui.Control](entity).Open = open
		setHTMLSelectorVisibility(world, entity, open)
	}
}

func handleHTMLSelectTap(world *ecs.World, item SelectorItem) {
	entity, control, exists := htmlSelectControlFor(world, item.Owner)
	if !exists || control.Disabled || item.Disabled {
		return
	}
	if control.Multiple {
		shift, shortcut := htmlSelectionModifiers(world)
		if updateHTMLMultipleSelection(world, entity, control, item.Index, shift, shortcut, false) {
			publishHTMLSelectChange(world, entity)
		}
		return
	}
	SetHTMLSelectOpen(world, entity, false)
	if item.Index != control.Selected && htmlui.SetControlSelected(world, entity, item.Index) {
		publishHTMLSelectChange(world, entity)
	}
}

func handleHTMLSelectInput(context *scene.FocusedInputContext) {
	if context == nil || context.World == nil || context.Event.Kind != input.EventKeyDown {
		return
	}
	world, entity := context.World, context.Entity
	control, ok := world.Get[htmlui.Control](entity)
	if !ok || control.Tag != "select" || !control.Multiple || control.Disabled || len(control.Options) == 0 {
		return
	}
	shift, shortcut := htmlSelectionModifiers(world)
	current := control.SelectionFocus
	if current < 0 {
		current = control.Selected
	}
	target := -1
	switch context.Event.Key {
	case input.KeyUp:
		target = enabledHTMLOption(control.Options, current, -1)
	case input.KeyDown:
		target = enabledHTMLOption(control.Options, current, 1)
	case input.KeyHome:
		target = enabledHTMLOption(control.Options, -1, 1)
	case input.KeyEnd:
		target = enabledHTMLOption(control.Options, len(control.Options), -1)
	case input.KeySpace:
		context.Consume()
		if !shortcut {
			return
		}
		if current < 0 || current >= len(control.Options) || control.Options[current].Disabled {
			current = enabledHTMLOption(control.Options, -1, 1)
		}
		if current < 0 {
			return
		}
		changed := htmlui.SetControlOptionSelected(world, entity, current, !control.Options[current].Selected())
		anchor := control.SelectionAnchor
		if anchor < 0 {
			anchor = current
		}
		htmlui.SetControlSelectionFocus(world, entity, anchor, current)
		if changed {
			publishHTMLSelectChange(world, entity)
		}
		return
	case input.KeyA:
		if !shortcut {
			return
		}
		context.Consume()
		first := enabledHTMLOption(control.Options, -1, 1)
		last := enabledHTMLOption(control.Options, len(control.Options), -1)
		if first >= 0 && last >= 0 && updateHTMLMultipleSelection(world, entity, control, last, true, false, true) {
			publishHTMLSelectChange(world, entity)
		}
		return
	default:
		return
	}
	context.Consume()
	if target < 0 {
		return
	}
	if shortcut && !shift {
		anchor := control.SelectionAnchor
		if anchor < 0 {
			anchor = current
		}
		htmlui.SetControlSelectionFocus(world, entity, anchor, target)
		return
	}
	if updateHTMLMultipleSelection(world, entity, control, target, shift, shortcut, false) {
		publishHTMLSelectChange(world, entity)
	}
}

func htmlSelectionModifiers(world *ecs.World) (shift, shortcut bool) {
	state, ok := world.GetResource[*input.State]()
	if !ok || state == nil {
		return false, false
	}
	shift = state.KeyHeld(input.KeyLeftShift) || state.KeyHeld(input.KeyRightShift)
	shortcut = state.KeyHeld(input.KeyLeftControl) || state.KeyHeld(input.KeyRightControl) ||
		state.KeyHeld(input.KeyLeftSuper) || state.KeyHeld(input.KeyRightSuper)
	return shift, shortcut
}

func enabledHTMLOption(options []htmlui.SelectOption, current, direction int) int {
	for index := current + direction; index >= 0 && index < len(options); index += direction {
		if !options[index].Disabled {
			return index
		}
	}
	return -1
}

func updateHTMLMultipleSelection(world *ecs.World, entity ecs.Entity, control htmlui.Control, target int, extend, shortcut, selectAll bool) bool {
	if target < 0 || target >= len(control.Options) || control.Options[target].Disabled {
		return false
	}
	if shortcut && !extend {
		changed := htmlui.SetControlOptionSelected(world, entity, target, !control.Options[target].Selected())
		htmlui.SetControlSelectionFocus(world, entity, target, target)
		return changed
	}
	if !extend {
		changed := control.SelectedCount != 1 || !control.Options[target].Selected()
		if !changed {
			htmlui.SetControlSelectionFocus(world, entity, target, target)
			return false
		}
		if !htmlui.SetControlSelected(world, entity, target) {
			return false
		}
		htmlui.SetControlSelectionFocus(world, entity, target, target)
		syncHTMLSelectorState(world, entity)
		return true
	}
	anchor := control.SelectionAnchor
	if selectAll {
		anchor = enabledHTMLOption(control.Options, -1, 1)
	} else if anchor < 0 {
		anchor = control.SelectionFocus
		if anchor < 0 {
			anchor = control.Selected
		}
		if anchor < 0 {
			anchor = target
		}
	}
	start, end := min(anchor, target), max(anchor, target)
	changed := false
	for index, option := range control.Options {
		if option.Disabled {
			continue
		}
		if option.Selected() != (index >= start && index <= end) {
			changed = true
			break
		}
	}
	if !changed {
		htmlui.SetControlSelectionFocus(world, entity, anchor, target)
		return false
	}
	if !htmlui.SetControlSelectionRange(world, entity, anchor, target) {
		return false
	}
	syncHTMLSelectorState(world, entity)
	return true
}

func syncHTMLSelectorState(world *ecs.World, entity ecs.Entity) {
	control, ok := world.Get[htmlui.Control](entity)
	if !ok {
		return
	}
	refreshHTMLControlText(world, entity, control)
}

func publishHTMLSelectChange(world *ecs.World, entity ecs.Entity) {
	syncHTMLSelectorState(world, entity)
	control, ok := world.Get[htmlui.Control](entity)
	if !ok {
		return
	}
	writeHTMLEvent(world, entity, control, htmlui.EventInput)
	writeHTMLEvent(world, entity, control, htmlui.EventChange)
}

func enrichHTMLControls(world *ecs.World) {
	enrichHTMLControlStates(world)
	cleanupHTMLSelectorElements(world)
	enrichHTMLParts(world)
}

func enrichHTMLControlStates(world *ecs.World) {
	state, changed, full := prepareHTMLBridge[htmlControlsScratch](world, htmlControlBridgeVersions(world))
	if !changed {
		return
	}
	controls := world.FrameScratch[htmlControlsScratch, ecs.Entity]()
	defer controls.ReleaseUnused()
	query := world.Query[htmlui.Control]()
	changes := state.changes(world)
	if !full {
		query = world.Query[htmlui.Control](changes.Changed[htmlui.Control]())
	}
	for entity, access := range query.All {
		control := access.Get()

		state, exists := world.Get[htmlControlState](entity)
		// Structural HTML updates can change order without changing authored
		// control data. Keep the selector bridge current without rebuilding it.
		if exists && control.Tag == "select" {
			if selector := world.GetRef[Selector](entity); selector != nil {
				stack := htmlControlStackIndex(world, entity)
				if selector.StackIndex != stack {
					current := world.GetMut[Selector](entity)
					current.StackIndex = stack
				}
			}
		}
		if !exists || state.RenderRevision != control.RenderRevision {
			controls.Values = append(controls.Values, entity)
		}
	}
	for _, entity := range controls.Values {
		control, ok := world.Get[htmlui.Control](entity)
		if !ok {
			continue
		}
		if runtime := world.GetMut[htmlControlState](entity); runtime != nil {
			runtime.RenderRevision = control.RenderRevision
			refreshRetainedHTMLControl(world, entity, control)
			continue
		}
		runtime := htmlControlState{RenderRevision: control.RenderRevision}
		switch {
		case isHTMLTextControl(control):
			if control.Tag == "textarea" {
				textarea := Textarea{Wrap: control.Wrap}
				if computed, exists := htmlui.ControlScrollStyle(world, entity); exists {
					textarea.BarSize = computed.BarSize
					textarea.DisableHorizontalScroll = computed.DisableHorizontalScroll
					if computed.Hidden {
						textarea.BarSize = -1
					}
					if computed.HasTrackColor || computed.HasThumbColor || computed.HasThumbHoverColor {
						applyHTMLScrollColors(world, entity, computed)
					}
				}
				_ = world.Insert3(entity, Control{}, runtime, textarea)
			} else {
				_ = world.Insert3(entity, Control{}, runtime, Input{})
			}
		case control.Tag == "button" || control.Tag == "input":
			_ = world.Insert3(entity, Control{}, runtime, Button{})
		case control.Tag == "select":
			selector := Selector{ItemHeight: selectorHTMLItemHeight(control), ListHeight: selectorHTMLListHeight(control), StackIndex: htmlControlStackIndex(world, entity)}
			_ = world.Insert4(entity, Control{}, runtime, selector, scene.FocusedInputHandler{Handle: handleHTMLSelectInput})
			bindHTMLSelector(world, entity, control, mgl32.Vec2{})
			syncHTMLSelectorState(world, entity)
			setHTMLSelectorVisibility(world, entity, control.Open)
		default:
			_ = world.Insert2(entity, Control{}, runtime)
		}
	}

	state.capture(world, htmlControlBridgeVersions(world))
}

const (
	htmlSelectorDefaultItemHeight float32 = 30
	htmlSelectorPopupGap          float32 = 4
)

// HTMLSelectStyle supplies native option interaction colors for an HTML surface.
// Normal option colors continue to come from the authored select control.
type HTMLSelectStyle struct {
	Item StateStyle[SurfaceStyle]
	Text StateStyle[graphics.ColorRGBA]
}

func htmlSelectStyleFor(world *ecs.World, selector ecs.Entity) *HTMLSelectStyle {
	control := world.GetRef[htmlui.Control](selector)
	if control == nil {
		return nil
	}
	return world.GetRef[HTMLSelectStyle](control.Owner)
}

func selectorHTMLItemHeight(control htmlui.Control) float32 {
	_ = control
	return htmlSelectorDefaultItemHeight
}

func selectorHTMLListHeight(control htmlui.Control) float32 {
	count := len(control.Options)
	if count < 1 {
		return 0
	}
	return float32(count) * htmlSelectorDefaultItemHeight
}

func htmlControlStackIndex(world *ecs.World, entity ecs.Entity) int {
	if stack, ok := world.Get[scene.UIStackIndex](entity); ok {
		return int(stack)
	}
	return 0
}

func bindHTMLSelector(world *ecs.World, owner ecs.Entity, control htmlui.Control, position mgl32.Vec2) {
	if world == nil || owner == 0 || !world.IsAlive(owner) || world.Has[htmlSelectorRuntime](owner) {
		return
	}
	retirements := world.FrameValue[htmlSelectorRetirements, htmlSelectorRetirements]()
	if !retirements.bound {
		world.OnComponentRemove[htmlSelectorRuntime](retireHTMLSelector)
		retirements.bound = true
	}
	controlView, ok := world.Get[scene.InputTarget2D](owner)
	if !ok {
		return
	}
	itemHeight := selectorHTMLItemHeight(control)
	listHeight := selectorHTMLListHeight(control)
	theme := themeForWorld(world, Theme{})
	if html, htmlOK := world.Get[htmlui.Surface](control.Owner); htmlOK {
		if html.Font != nil {
			theme.Font = html.Font
		}
		if html.FontSize > 0 {
			theme.FontSize = html.FontSize
		}
		if html.DPI > 0 {
			theme.DPI = html.DPI
		}
	}
	if text, ok := htmlui.ControlTextStyle(world, owner); ok {
		if text.Font != nil {
			theme.Font = text.Font
		}
		theme.FontSize, theme.DPI = text.FontSize, text.DPI
	}
	popupFill := theme.Surface
	popupRadius := float32(0)
	if sprite, spriteOK := world.Get[scene.Sprite2D](owner); spriteOK {
		if sprite.Shape.Fill.A > 0 {
			popupFill = sprite.Shape.Fill
		}
		popupRadius = sprite.Shape.Radius
	}
	// The native select picker is a separate surface below (or above) the control.
	list := spawnAnchor(world, 0, controlView.Height+htmlSelectorPopupGap)
	_ = world.Insert(list, scene.Parent{Entity: owner})
	// The select control clips its text and indicator; the popup must escape
	// that clip while retaining its own viewport clip below.
	_ = world.Insert(list, scene.ClipExempt2D{})
	_ = world.Insert(list, SelectorList{Selector: owner})
	_ = world.Insert(list, htmlSelectorPopup{Radius: popupRadius})
	_ = world.Insert(list, htmlSelectorElement{Owner: owner})
	_ = world.Insert(list, scene.Size2D{Width: controlView.Width, Height: listHeight})
	_ = world.Insert(list, scene.ClipRect2D{Width: controlView.Width, Height: listHeight})
	_ = world.Insert(list, scene.RoundedClip2D{Width: controlView.Width, Height: listHeight, Radius: popupRadius})
	_ = world.Insert(list, scene.UIScroll{Position: position, ViewportSize: mgl32.Vec2{controlView.Width, listHeight}, ContentSize: mgl32.Vec2{controlView.Width, itemHeight * float32(len(control.Options))}, WheelStep: itemHeight, Vertical: true, ConsumeWheel: true, DisableInput: true})
	setUIStackIndex(world, list, selectorPopupStackBase)
	setVisible(world, list, false)
	// HTML controls are styled by the authored page, so keep the generated
	// selector popup on the same surface instead of falling back to the world theme.
	selectorStyle := formSkinFor(world, theme).Selector
	var popupBorder graphics.ColorRGBA
	if sprite, spriteOK := world.Get[scene.Sprite2D](owner); spriteOK && sprite.Shape.Border.A > 0 {
		popupBorder = sprite.Shape.Border
	} else {
		popupBorder = alphaColor(theme.Muted, 0.48)
	}
	selectorStyle.Popup = ColorSurface(popupFill).WithBorder(
		scene.OutlinedRoundedRectShape2D(graphics.ColorRGBA{}, popupBorder, 0, 1),
	)
	selectorStyle.Item.Normal = ColorSurface(popupFill)
	_ = world.Insert(owner, StyleOverride[SelectorStyle]{Value: selectorStyle})
	elements := make([]ecs.Entity, 0, len(control.Options)*3+5)
	elements = append(elements, list)
	panel := spawnSurfacePart[SelectorPanelPart](world, list, 0, 0, controlView.Width, listHeight, popupFill, selectorPopupStackBase+1)
	elements = append(elements, panel)
	setCornerRadius(world, panel, CornerRadius(popupRadius))
	_ = world.Insert(panel, htmlSelectorElement{Owner: owner})
	setVisible(world, panel, false)
	border := spawnSurfacePart[SelectorPanelBorderPart](world, list, 0, 0, controlView.Width, listHeight, graphics.ColorRGBA{}, selectorPopupStackBase+4)
	elements = append(elements, border)
	setCornerRadius(world, border, CornerRadius(popupRadius))
	_ = world.Insert(border, htmlSelectorElement{Owner: owner})
	setVisible(world, border, false)
	track := spawnSurfacePart[SelectorScrollTrackPart](world, list, 0, 0, 0, 0, graphics.ColorRGBA{}, selectorPopupStackBase+3)
	thumb := spawnSurfacePart[SelectorScrollThumbPart](world, list, 0, 0, 0, 0, graphics.ColorRGBA{}, selectorPopupStackBase+4)
	for _, part := range [...]ecs.Entity{track, thumb} {
		elements = append(elements, part)
		setCornerRadius(world, part, CornerRadius(popupRadius))
		_ = world.Insert(part, htmlSelectorElement{Owner: owner})
		setVisible(world, part, false)
	}
	items := make([]ecs.Entity, 0, len(control.Options))
	for index, option := range control.Options {
		item := spawnInteractiveAnchor(world, 0, float32(index)*itemHeight, controlView.Width, itemHeight, selectorPopupStackBase+2, option.Disabled, false)
		// Track the option background with its anchor and label: resizing a
		// retained select must retire all three before rebuilding the popup.
		body := spawnSurfacePart[BodyPart](world, item, 0, 0, controlView.Width, itemHeight, popupFill, selectorPopupStackBase+2)
		elements = append(elements, item, body)
		_ = world.Insert(body, htmlSelectorElement{Owner: owner})
		_ = world.Insert(item, scene.Parent{Entity: list})
		_ = world.Insert(item, htmlui.FocusOwner{Entity: owner})
		_ = world.Insert(item, SelectorItem{Owner: owner, Index: index, Label: option.Label, Disabled: option.Disabled})
		_ = world.Insert(item, htmlSelectorElement{Owner: owner})
		text := spawnText[ItemPart](world, item, 12, 0, option.Label, theme)
		elements = append(elements, text)
		_ = world.Insert(text, htmlSelectorElement{Owner: owner})
		// HTML popup text is marked as authored below so the HTML font and color
		// survive the generic text sync. Keep it above the popup panel explicitly;
		// authored text skips syncItemText, which normally assigns this layer.
		setUIStackIndex(world, text, selectorPopupStackBase+3)
		// Mark popup text as authored so the generic text sync keeps the HTML
		// surface's font instead of replacing it with the world theme font.
		_ = world.Insert(text, Relation[Control]{Owner: item})
		centerTextVertically(world, text, itemHeight)
		setVisible(world, item, false)
		setVisible(world, body, false)
		setVisible(world, text, false)
		items = append(items, item)
	}
	sprite, _ := world.Get[scene.Sprite2D](owner)
	_ = world.Insert(owner, htmlSelectorRuntime{List: list, Items: items, Elements: elements, Options: control.Options, Width: controlView.Width, Height: controlView.Height, Shape: sprite.Shape})
}

func htmlSelectControlFor(world *ecs.World, selector ecs.Entity) (ecs.Entity, htmlui.Control, bool) {
	if selector == 0 {
		return 0, htmlui.Control{}, false
	}
	value, ok := world.Get[htmlui.Control](selector)
	return selector, value, ok && value.Tag == "select"
}

func setHTMLSelectorVisibility(world *ecs.World, selector ecs.Entity, visible bool) {
	runtime, ok := world.Get[htmlSelectorRuntime](selector)
	if !ok {
		return
	}
	setVisible(world, runtime.List, visible)
	for _, entity := range runtime.Elements {
		if world.IsAlive(entity) {
			setVisible(world, entity, visible)
		}
	}
}

func retireHTMLSelector(world *ecs.World, _ ecs.Entity, runtime *htmlSelectorRuntime) {
	retirements := world.FrameValue[htmlSelectorRetirements, htmlSelectorRetirements]()
	retirements.entities = append(retirements.entities, runtime.Elements...)
}

func cleanupHTMLSelectorElements(world *ecs.World) {
	retirements := world.FrameValue[htmlSelectorRetirements, htmlSelectorRetirements]()
	for _, entity := range retirements.entities {
		world.Despawn(entity)
	}
	retirements.entities = retirements.entities[:0]
}

func refreshHTMLControlText(world *ecs.World, controlEntity ecs.Entity, control htmlui.Control) {
	for _, access := range world.Query2[htmlui.ControlText, scene.Text2D]().All {
		if access.A.Ref().Control == controlEntity {
			syncHTMLControlText(access.B, &control)
		}
	}
}

func syncHTMLControlText(text ecs.ComponentAccess[scene.Text2D], control *htmlui.Control) {
	value := ""
	if control.Tag == "input" && htmlControlType(*control) == "checkbox" && control.Checked() {
		value = "✓"
	}
	if control.Tag == "select" && control.Selected >= 0 && control.Selected < len(control.Options) {
		value = control.Options[control.Selected].Label
	}
	if text.Ref().Value != value {
		text.Mut().Value = value
	}
}

func isHTMLTextControl(control htmlui.Control) bool {
	if control.Tag == "textarea" {
		return true
	}
	if control.Tag != "input" {
		return false
	}
	switch htmlControlType(control) {
	case "button", "checkbox", "color", "file", "hidden", "image", "radio", "range", "reset", "submit":
		return false
	default:
		return true
	}
}

func htmlControlType(control htmlui.Control) string {
	typeValue := strings.ToLower(strings.TrimSpace(control.Type))
	if typeValue != "" {
		return typeValue
	}
	if control.Tag == "button" {
		return "submit"
	}
	return "text"
}

func writeHTMLEvent(world *ecs.World, target ecs.Entity, control htmlui.Control, kind htmlui.EventKind) {
	bubbles := kind != htmlui.EventFocus && kind != htmlui.EventBlur
	event := htmlui.Event{
		Entity: control.Owner, Target: target, ID: control.ID, FormID: control.FormID, Name: control.Name, Kind: kind, Value: htmlui.ControlValue(world, target),
		Checked: control.Checked(), Selected: control.Selected, SelectedCount: control.SelectedCount, Bubbles: bubbles,
	}
	event.Cancelable = kind == htmlui.EventSubmit
	htmlui.PublishEvent(world, event)
}

func dispatchHTMLEditableBeforeInput(world *ecs.World, target ecs.Entity, inputType, data string) bool {
	editable, ok := world.Get[htmlui.Editable](target)
	if !ok {
		return false
	}
	field := world.GetRef[TextField](target)
	if field == nil {
		return false
	}
	event := htmlui.PublishEvent(world, htmlui.Event{
		Entity: editable.Owner, Target: target, ID: editable.ID, Kind: htmlui.EventBeforeInput,
		Value: field.Text().String(), InputType: inputType, Data: data, Anchor: field.Anchor, Focus: field.Caret,
		Bubbles: true, Cancelable: true,
	})
	return event.DefaultPrevented
}

func publishHTMLTextInput(world *ecs.World, target ecs.Entity, inputType, data string) {
	field := world.GetRef[TextField](target)
	if field == nil {
		return
	}
	if editable, ok := world.Get[htmlui.Editable](target); ok {
		htmlui.PublishEvent(world, htmlui.Event{
			Entity: editable.Owner, Target: target, ID: editable.ID, Kind: htmlui.EventInput, Value: field.Text().String(),
			InputType: inputType, Data: data, Anchor: field.Anchor, Focus: field.Caret, Bubbles: true,
		})
	} else if control, ok := world.Get[htmlui.Control](target); ok {
		if field.Buffer != nil {
			htmlui.PublishEvent(world, htmlui.Event{Entity: control.Owner, Target: target, ID: control.ID, FormID: control.FormID, Name: control.Name, Kind: htmlui.EventInput, Snapshot: field.Source, Document: true, Bubbles: true})
		} else {
			writeHTMLEvent(world, target, control, htmlui.EventInput)
		}
	}
}

func commitHTMLControl(world *ecs.World, target ecs.Entity, control htmlui.Control) {
	field := world.GetRef[TextField](target)
	if field == nil || field.CommittedValue == field.Text().String() {
		return
	}
	world.GetMut[TextField](target).CommittedValue = field.Text().String()
	writeHTMLEvent(world, target, control, htmlui.EventChange)
}

// BlurHTMLControl removes focus from a native HTML control, including its text
// input and IME state, like HTMLElement.blur().
func BlurHTMLControl(world *ecs.World, entity ecs.Entity) {
	if !world.Has[htmlui.Control](entity) {
		return
	}
	if state, ok := world.GetResource[*uiEventState](); ok && state != nil && state.Focused == entity {
		focus(world, state, 0)
	}
}

// HTML buttons activate with Enter on keydown and Space on keyup. Select
// navigation changes the live selection without sending scene-level commands.
func updateHTMLKeyboard(world *ecs.World, entity ecs.Entity, inputState *input.State) {
	c, ok := world.Get[htmlui.Control](entity)
	if !ok || c.Disabled || inputState.Composing() {
		return
	}
	kind := htmlControlType(c)
	if c.Tag == "select" && !c.Multiple {
		index := -1
		switch {
		case inputState.KeyPressed(input.KeyDown):
			index = enabledHTMLOption(c.Options, c.Selected, 1)
		case inputState.KeyPressed(input.KeyUp):
			index = enabledHTMLOption(c.Options, c.Selected, -1)
		case inputState.KeyPressed(input.KeyHome):
			index = enabledHTMLOption(c.Options, -1, 1)
		case inputState.KeyPressed(input.KeyEnd):
			index = enabledHTMLOption(c.Options, len(c.Options), -1)
		case inputState.KeyPressed(input.KeyEscape):
			SetHTMLSelectOpen(world, entity, false)
		case inputState.KeyPressed(input.KeyEnter), inputState.KeyPressed(input.KeySpace):
			scene.DispatchInputEvent(world, scene.InputEvent{Entity: entity, Kind: scene.InputEventTap})
		}
		if index >= 0 && index != c.Selected && htmlui.SetControlSelected(world, entity, index) {
			publishHTMLSelectChange(world, entity)
		}
		return
	}
	button := c.Tag == "button" || kind == "button" || kind == "submit" || kind == "reset"
	if !button && kind != "checkbox" && kind != "radio" {
		return
	}
	state := world.GetMut[htmlControlState](entity)
	if state == nil {
		return
	}
	if inputState.KeyPressed(input.KeySpace) {
		state.SpacePressed = true
	}
	space := inputState.KeyReleased(input.KeySpace) && state.SpacePressed
	if inputState.KeyReleased(input.KeySpace) {
		state.SpacePressed = false
	}
	if space || button && inputState.KeyPressed(input.KeyEnter) {
		scene.DispatchInputEvent(world, scene.InputEvent{Entity: entity, Kind: scene.InputEventTap})
	}
}

// Refresh authored data while preserving the live interaction components of a
// retained control. A select popup is regenerated only when its content or
// geometry changes; the select itself keeps its identity and keyboard state.
func refreshRetainedHTMLControl(world *ecs.World, entity ecs.Entity, control htmlui.Control) {
	if textarea := world.GetMut[Textarea](entity); textarea != nil {
		textarea.Wrap = control.Wrap
		if computed, exists := htmlui.ControlScrollStyle(world, entity); exists {
			textarea.BarSize = computed.BarSize
			textarea.DisableHorizontalScroll = computed.DisableHorizontalScroll
			if computed.Hidden {
				textarea.BarSize = -1
			}
			applyHTMLScrollColors(world, entity, computed)
		}
	}
	if control.Tag != "select" {
		return
	}
	target, _ := world.Get[scene.InputTarget2D](entity)
	sprite, _ := world.Get[scene.Sprite2D](entity)
	var scrollPosition mgl32.Vec2
	if runtime, exists := world.Get[htmlSelectorRuntime](entity); exists && (runtime.Width != target.Width || runtime.Height != target.Height || runtime.Shape != sprite.Shape || !slices.Equal(runtime.Options, control.Options)) {
		if scroll := world.GetRef[scene.UIScroll](runtime.List); scroll != nil {
			scrollPosition = scroll.Position
		}
		world.Discard(entity, ecs.Type[htmlSelectorRuntime]())
	}
	if selector := world.GetMut[Selector](entity); selector != nil {
		selector.ItemHeight, selector.ListHeight = selectorHTMLItemHeight(control), selectorHTMLListHeight(control)
		selector.StackIndex = htmlControlStackIndex(world, entity)
	}
	bindHTMLSelector(world, entity, control, scrollPosition)
	syncHTMLSelectorState(world, entity)
	setHTMLSelectorVisibility(world, entity, control.Open)
}
