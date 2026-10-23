package html

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
	onixtext "github.com/diiyw/onix/text"
	"github.com/go-gl/mathgl/mgl32"
)

// Surface is a retained HTML surface. Value may contain regular HTML and inline
// <style> elements. CSS contains an optional stylesheet applied to the value.
// Width and Height use UI pixels; zero resolves to the current viewport size.
type Surface struct {
	Value  string
	CSS    string
	Width  float32
	Height float32
	Font   *onixtext.Font
	Images *Images
	// BoldFont is used for text whose computed font weight is 600 or heavier.
	BoldFont    *onixtext.Font
	FontSize    float64
	DPI         float64
	StackIndex  int
	PassThrough bool
}

// Layout is the computed size of an HTML surface.
type Layout struct {
	Width  float32
	Height float32
}

// ScrollStyle contains the scrollbar values computed for an HTML node.
// Has*Color distinguishes authored colors (including transparent) from auto. BarSize
// matches CSS overflow scrollbars. Hidden records scrollbar-width:none,
// which suppresses painting while scrolling remains available.
type ScrollStyle struct {
	BarSize                 float32
	DisableHorizontalScroll bool
	Hidden                  bool
	HasTrackColor           bool
	HasThumbColor           bool
	HasThumbHoverColor      bool
	TrackColor              graphics.ColorRGBA
	ThumbColor              graphics.ColorRGBA
	ThumbHoverColor         graphics.ColorRGBA
}

// ScrollbarPart is one retained visual of a CSS overflow scrollbar.
// Axis is 0 for horizontal and 1 for vertical; Kind is track or thumb.
// The part is kept separate from UIScroll so input and painting can evolve
// independently while the entity identity remains stable across HTML syncs.
type ScrollbarPart struct {
	Area ecs.Entity
	Axis uint8
	Kind uint8
}

const (
	ScrollbarHorizontal uint8 = iota
	ScrollbarVertical
)

const (
	ScrollbarTrack uint8 = iota
	ScrollbarThumb
	ScrollbarArrowStart
	ScrollbarArrowEnd
)

// NodeScrollStyle returns scrollbar styling computed for an element. It
// supports the standard scrollbar-width and scrollbar-color declarations plus
// the track/thumb overrides used by native UI widgets.
func NodeScrollStyle(world *ecs.World, owner ecs.Entity, id string) (ScrollStyle, bool) {
	if world == nil || owner == 0 || id == "" {
		return ScrollStyle{}, false
	}
	state, ok := ecs.Get[htmlRenderState](world, owner)
	if !ok || state.Root == nil || state.Styles == nil {
		return ScrollStyle{}, false
	}
	node := lookupHTMLNode(&state, id)
	computed, ok := state.Styles.Get(node)
	if !ok {
		return ScrollStyle{}, false
	}
	return htmlScrollStyle(computed), true
}

// ControlScrollStyle returns the scrollbar style computed for a generated
// form control. Unlike NodeScrollStyle, it also supports controls without
// an id attribute.
func ControlScrollStyle(world *ecs.World, control ecs.Entity) (ScrollStyle, bool) {
	if world == nil || control == 0 {
		return ScrollStyle{}, false
	}
	value, ok := ecs.Get[Control](world, control)
	if !ok || value.Owner == 0 {
		return ScrollStyle{}, false
	}
	state, ok := ecs.Get[htmlRenderState](world, value.Owner)
	if !ok || state.Root == nil || state.Styles == nil {
		return ScrollStyle{}, false
	}
	node, ok := htmlControlNode(world, value.Owner, control, value)
	if !ok {
		return ScrollStyle{}, false
	}
	computed, ok := state.Styles.Get(node)
	if !ok {
		return ScrollStyle{}, false
	}
	return htmlScrollStyle(computed), true
}

func htmlScrollStyle(computed *style.ComputedStyle) ScrollStyle {
	result := ScrollStyle{
		BarSize:            htmlOverflowScrollbarSize(computed),
		HasTrackColor:      computed.ScrollbarTrackColor.Kind == style.ValueColor,
		HasThumbColor:      computed.ScrollbarThumbColor.Kind == style.ValueColor,
		HasThumbHoverColor: computed.ScrollbarThumbHoverColor.Kind == style.ValueColor,
		TrackColor:         htmlColor(computed.ScrollbarTrackColor),
		ThumbColor:         htmlColor(computed.ScrollbarThumbColor),
		ThumbHoverColor:    htmlColor(computed.ScrollbarThumbHoverColor),
	}
	result.Hidden = computed.ScrollbarWidth.Kind == style.ValueKeyword && computed.ScrollbarWidth.Keyword == "none"
	result.DisableHorizontalScroll = computed.OverflowX.Kind == style.ValueKeyword && (computed.OverflowX.Keyword == "hidden" || computed.OverflowX.Keyword == "clip")
	return result
}

// ControlTextStyle returns the computed font and foreground for native control text.
func ControlTextStyle(world *ecs.World, control ecs.Entity) (scene.Text2D, bool) {
	value := ecs.GetRef[Control](world, control)
	if value == nil {
		return scene.Text2D{}, false
	}
	state := ecs.GetRef[htmlRenderState](world, value.Owner)
	surface := ecs.GetRef[Surface](world, value.Owner)
	if state == nil || state.Styles == nil || surface == nil {
		return scene.Text2D{}, false
	}
	node, ok := htmlControlNode(world, value.Owner, control, *value)
	if !ok {
		return scene.Text2D{}, false
	}
	computed, ok := state.Styles.Get(node)
	if !ok {
		return scene.Text2D{}, false
	}
	text := scene.NewText2D("")
	text.Font = htmlFontForControl(world, control, surface.Font, surface.BoldFont, computed)
	text.FontSize, text.DPI = fragmentFontSize(computed), surface.DPI
	text.Color = htmlColor(computed.Color)
	return text, true
}

// ControlTextColor returns the computed CSS foreground of a generated control.
func ControlTextColor(world *ecs.World, control ecs.Entity) (graphics.ColorRGBA, bool) {
	value := ecs.GetRef[Control](world, control)
	if value == nil {
		return graphics.ColorRGBA{}, false
	}
	state := ecs.GetRef[htmlRenderState](world, value.Owner)
	if state == nil || state.Styles == nil {
		return graphics.ColorRGBA{}, false
	}
	node, ok := htmlControlNode(world, value.Owner, control, *value)
	if !ok {
		return graphics.ColorRGBA{}, false
	}
	computed, ok := state.Styles.Get(node)
	if !ok {
		return graphics.ColorRGBA{}, false
	}
	return htmlColor(computed.Color), true
}

// ControlTextTabWidth resolves CSS tab-size against the control's current font.
func ControlTextTabWidth(world *ecs.World, control ecs.Entity, text *scene.Text2D) (float32, bool) {
	value, ok := ecs.Get[Control](world, control)
	if !ok || text == nil {
		return 0, false
	}
	state := ecs.GetRef[htmlRenderState](world, value.Owner)
	if state == nil || state.Styles == nil {
		return 0, false
	}
	node, ok := htmlControlNode(world, value.Owner, control, value)
	if !ok {
		return 0, false
	}
	computed, ok := state.Styles.Get(node)
	if !ok {
		return 0, false
	}
	return htmlTextTabWidth(text, computed), true
}

// Control identifies a form control generated by an HTML surface. The
// generated entity remains stable while its DOM node and control type survive.
type Control struct {
	// RenderRevision changes when authored control data or geometry is synchronized.
	RenderRevision uint64
	Owner          ecs.Entity
	ID             string
	FormID         string
	Name           string
	Tag            string
	Type           string
	Value          string
	// DefaultValue reflects the authored value attribute or textarea text.
	DefaultValue string
	// DirtyValue prevents authored default changes from replacing the live value.
	DirtyValue  bool
	Placeholder string
	MaxLength   int
	Disabled    bool
	ReadOnly    bool
	// Wrap reports whether the textarea's computed white-space permits soft wrapping.
	Wrap bool
	// PaddingBox is the area inside CSS borders, in control-local coordinates.
	// Native scrollbars run along its edges independently of text padding.
	PaddingBox scene.TextRangeRect2D
	// DefaultChecked reflects the authored checked content attribute.
	DefaultChecked bool
	// DirtyChecked prevents checked attribute changes from replacing live checkedness.
	DirtyChecked bool
	// Checked is the live state of an input checkbox.
	Checked bool
	// Selected is the live option index of a select, or -1 when no option is selected.
	Selected int
	// SelectedCount is the number of live selected options.
	SelectedCount int
	// DefaultSelected is the option selected by the authored attributes.
	DefaultSelected int
	// DirtySelected reports whether any option has dirty live selectedness.
	DirtySelected bool
	// Multiple reflects the select multiple content attribute.
	Multiple bool
	// SelectionAnchor is the fixed option used for an extended user selection.
	SelectionAnchor int
	// SelectionFocus is the option currently navigated by keyboard or pointer.
	SelectionFocus int
	// Options contains the authored options of a select in document order.
	Options []SelectOption
	// Open reports whether a select popup is currently open.
	Open bool
}

// Editable is the live plaintext and Selection state of a contenteditable
// editing host. Anchor and Focus are rune offsets in Value.
type Editable struct {
	RenderRevision uint64
	Owner          ecs.Entity
	ID             string
	Value          string
	Anchor         int
	Focus          int
}

// EditableText links the flattened live text visual of an editing host.
type EditableText struct {
	Editable ecs.Entity
}

// EditableVisual links caret and selection visuals to an editing host.
type EditableVisual struct {
	Editable  ecs.Entity
	Selection bool
	Index     int
}

type htmlEditableNodeRef struct {
	Node *dom.Node
	Text ecs.Entity
}

// Focusable describes an HTML element's focus behavior independently from
// whether the element is a form control.
type Focusable struct {
	Owner        ecs.Entity
	ID           string
	Tag          string
	Type         string
	TabIndex     int
	Order        uint32
	Sequential   bool
	Pointer      bool
	AutoFocus    bool
	FocusVisible bool
}

// FocusOwner links a rendered descendant to the HTML element that should
// receive pointer focus without changing the descendant's event target.
type FocusOwner struct {
	Entity ecs.Entity
}

// SelectOption is one authored option of an HTML select control.
type SelectOption struct {
	Value           string
	Label           string
	Disabled        bool
	Selected        bool
	DefaultSelected bool
	DirtySelected   bool
}

// Option is kept as a concise name for callers that inspect select options.
type Option = SelectOption

// ControlText links a generated text visual to its form control.
type ControlText struct {
	Control ecs.Entity
	// Content is the CSS content box in the control's local coordinates.
	Content scene.TextRangeRect2D
}

// ControlTextFont selects a native control's text face independently of its
// surface font. Attach it to the control so HTML reconciliation uses the same
// face for text, line height and CSS tab-size as the native widget.
type ControlTextFont struct{ Font *onixtext.Font }

type htmlControlTextNode struct {
	Node *dom.Node
}

type htmlControlNodeRef struct {
	Node *dom.Node
}

type htmlCheckboxMark struct {
	Control ecs.Entity
}

type htmlSelectIndicator struct {
	Control ecs.Entity
}

// ControlVisualKind identifies generated form-control visuals whose
// geometry and style are supplied by the UI runtime.
type ControlVisualKind uint8

const (
	ControlVisualCaret ControlVisualKind = iota
	ControlVisualScrollTrackHorizontal
	ControlVisualScrollTrackVertical
	ControlVisualScrollThumbHorizontal
	ControlVisualScrollThumbVertical
)

// ControlVisual links a generated caret, selection, or scrollbar visual
// to its form control. Selection remains separate because textareas reserve
// several indexed selection surfaces.
type ControlVisual struct {
	Control   ecs.Entity
	Kind      ControlVisualKind
	Selection bool
	Index     int
}

// EventKind identifies a browser-style event emitted by an HTML control.
type EventKind string

const (
	EventClick       EventKind = "click"
	EventBeforeInput EventKind = "beforeinput"
	EventInput       EventKind = "input"
	EventChange      EventKind = "change"
	EventSubmit      EventKind = "submit"
	EventReset       EventKind = "reset"
	EventFocus       EventKind = "focus"
	EventBlur        EventKind = "blur"
)

// Event is emitted for generated interactive HTML elements. Entity is the
// HTML surface, while Target is the generated input entity when present.
type Event struct {
	Entity   ecs.Entity
	Target   ecs.Entity
	ID       string
	FormID   string
	Name     string
	Kind     EventKind
	Value    string
	Checked  bool
	Selected int
	// SelectedCount allows allocation-free multiple-select event inspection.
	SelectedCount int
	// InputType identifies the beforeinput/input editing operation.
	InputType        string
	Data             string
	Bubbles          bool
	Cancelable       bool
	DefaultPrevented bool
	Anchor           int
	Focus            int
}

// ActionContext is delivered synchronously before an HTML default action.
// PreventDefault only affects cancelable events.
type ActionContext struct {
	Event   Event
	Current ecs.Entity
}

func (c *ActionContext) PreventDefault() {
	if c != nil && c.Event.Cancelable {
		c.Event.DefaultPrevented = true
	}
}

// ActionHandler observes cancelable HTML events before their default
// action. Handlers may be installed on the generated target or HTML surface.
type ActionHandler struct {
	Handle func(*ActionContext)
}

// PointerEvent is delivered to a callback bound directly to an HTML node.
// Kind contains the pointer lifecycle event; a click callback receives the
// same event with Kind set to InputEventTap.
type PointerEvent struct {
	Target  Node
	Kind    scene.InputEventKind
	X       float32
	Y       float32
	DeltaX  float32
	DeltaY  float32
	WheelX  float32
	WheelY  float32
	Handled bool
}

// PointerHandler observes pointer lifecycle events for one HTML node.
type PointerHandler func(*PointerEvent)

// ClickHandler observes click (tap) events for one HTML node.
type ClickHandler func(*PointerEvent)

// Consume prevents the input event from bubbling to an ancestor.
func (e *PointerEvent) Consume() {
	if e != nil {
		e.Handled = true
	}
}

type htmlNodeEventBinding struct {
	Pointer PointerHandler
	Click   ClickHandler
}

type htmlPointerEventArenaTag struct{}

type htmlPointerEventArena struct {
	Events []PointerEvent
	Depth  int
}

type htmlActionArenaTag struct{}

type htmlActionArena struct {
	contexts []ActionContext
	depth    int
}

// DispatchAction synchronously dispatches an event to its generated target
// and then its HTML surface. It performs no allocation after ECS warmup.
func DispatchAction(world *ecs.World, event Event) Event {
	if world == nil {
		return event
	}
	arena := ecs.FrameValue[htmlActionArenaTag, htmlActionArena](world)
	index := len(arena.contexts)
	arena.contexts = append(arena.contexts, ActionContext{Event: event})
	arena.depth++
	defer func() {
		arena.depth--
		if arena.depth == 0 {
			clear(arena.contexts)
			arena.contexts = arena.contexts[:0]
		}
	}()
	context := &arena.contexts[index]
	if event.Target != 0 {
		dispatchHTMLActionHandler(world, context, event.Target)
	}
	if event.Entity != 0 && event.Entity != event.Target {
		dispatchHTMLActionHandler(world, context, event.Entity)
	}
	context.Event = dispatchHTMLComponentAction(world, context.Event)
	return context.Event
}

func dispatchHTMLActionHandler(world *ecs.World, context *ActionContext, entity ecs.Entity) {
	if !world.IsAlive(entity) {
		return
	}
	handler := ecs.GetRef[ActionHandler](world, entity)
	if handler == nil || handler.Handle == nil {
		return
	}
	context.Current = entity
	handler.Handle(context)
}

// ImageProvider resolves an img src into an engine texture. Applications
// can install one as a resource when HTML pages reference project assets.
type ImageProvider interface {
	ResolveHTMLImage(source string) (*graphics.Texture, bool)
}

// Images binds an immutable image provider to one surface. Its owner resolves
// relative URLs without exposing a global asset namespace.
type Images struct{ Provider ImageProvider }

// New creates an HTML component with browser-like default text metrics.
func New(value string) Surface {
	return Surface{Value: value, FontSize: 16, DPI: 72}
}

// Bounds returns the current synchronized surface size.
func Bounds(world *ecs.World, entity ecs.Entity) (Layout, bool) {
	size, ok := ecs.Get[scene.Size2D](world, entity)
	return Layout{Width: size.Width, Height: size.Height}, ok
}

// SetControlHover updates CSS :hover styles for a generated control in
// place. It intentionally avoids changing the retained DOM or render
// revision, so pointer movement cannot rebuild the HTML control tree.
func SetControlHover(world *ecs.World, control ecs.Entity, hovered bool) bool {
	if world == nil || control == 0 {
		return false
	}
	value, ok := ecs.Get[Control](world, control)
	if !ok || value.Owner == 0 {
		return false
	}
	renderState := ecs.GetMut[htmlRenderState](world, value.Owner)
	if renderState == nil || renderState.Root == nil || renderState.Stylesheet == nil {
		return false
	}
	node, nodeOK := htmlControlNode(world, value.Owner, control, value)
	if !nodeOK {
		return false
	}
	return setHTMLInputTarget(world, value.Owner, renderState, &renderState.Input.Hovered, node, hovered)
}

// SetVisualHover updates :hover for a retained non-control HTML visual.
func SetVisualHover(world *ecs.World, visual ecs.Entity, hovered bool) bool {
	owner, node, state, ok := htmlVisualInputNode(world, visual)
	if !ok {
		return false
	}
	return setHTMLInputTarget(world, owner, state, &state.Input.Hovered, node, hovered)
}

// SetControlFocus updates CSS :focus styles for a generated control in
// place. Focus changes are routed through the UI runtime, so the retained DOM
// and generated control entities remain stable while their styles update.
func SetControlFocus(world *ecs.World, control ecs.Entity, focused bool) bool {
	return SetFocusableFocusVisible(world, control, focused, false)
}

// SetControlFocusVisible updates :focus and records whether the current
// input modality should also match :focus-visible.
func SetControlFocusVisible(world *ecs.World, control ecs.Entity, focused, focusVisible bool) bool {
	return SetFocusableFocusVisible(world, control, focused, focusVisible)
}

// SetFocusableFocusVisible updates :focus for a generated HTML focus target.
func SetFocusableFocusVisible(world *ecs.World, target ecs.Entity, focused, focusVisible bool) bool {
	if world == nil || target == 0 || focused && !scene.IsInteractive(world, target) {
		return false
	}
	value, ok := ecs.Get[Focusable](world, target)
	if !ok || value.Owner == 0 {
		return false
	}
	renderState := ecs.GetMut[htmlRenderState](world, value.Owner)
	if renderState == nil || renderState.Root == nil || renderState.Stylesheet == nil {
		return false
	}
	child, nodeOK := ecs.Get[htmlChild](world, target)
	if !nodeOK || child.Node == nil {
		return false
	}
	node := child.Node
	if !focused && renderState.Input.Focused != node {
		return true
	}
	renderState.PreviousInput = renderState.Input
	renderState.Input.FocusVisible = focused && (focusVisible || value.FocusVisible)
	if focused {
		renderState.Input.Focused = node
	} else {
		renderState.Input.Focused = nil
	}
	if renderState.PreviousInput == renderState.Input {
		return true
	}
	return recomputeHTMLStyles(world, value.Owner, renderState, nil, &renderState.PreviousInput)
}

// SetControlActive updates CSS :active for the duration of a pointer
// activation. The active target's element ancestors match as they do in HTML.
func SetControlActive(world *ecs.World, control ecs.Entity, active bool) bool {
	if world == nil || control == 0 {
		return false
	}
	value, ok := ecs.Get[Control](world, control)
	if !ok || value.Owner == 0 || value.Disabled {
		return false
	}
	renderState := ecs.GetMut[htmlRenderState](world, value.Owner)
	if renderState == nil || renderState.Root == nil || renderState.Stylesheet == nil {
		return false
	}
	node, ok := htmlControlNode(world, value.Owner, control, value)
	if !ok {
		return false
	}
	return setHTMLInputTarget(world, value.Owner, renderState, &renderState.Input.Active, node, active)
}

// SetVisualActive updates :active for a retained non-control HTML visual.
func SetVisualActive(world *ecs.World, visual ecs.Entity, active bool) bool {
	owner, node, state, ok := htmlVisualInputNode(world, visual)
	if !ok {
		return false
	}
	return setHTMLInputTarget(world, owner, state, &state.Input.Active, node, active)
}

func htmlVisualInputNode(world *ecs.World, visual ecs.Entity) (ecs.Entity, *dom.Node, *htmlRenderState, bool) {
	if world == nil || visual == 0 {
		return 0, nil, nil, false
	}
	child, ok := ecs.Get[htmlChild](world, visual)
	if !ok || child.Owner == 0 || child.Node == nil {
		return 0, nil, nil, false
	}
	node := child.Node
	for node != nil && node.Type != dom.ElementNode {
		node = node.Parent
	}
	state := ecs.GetMut[htmlRenderState](world, child.Owner)
	if state == nil || node == nil || state.Stylesheet == nil {
		return 0, nil, nil, false
	}
	return child.Owner, node, state, true
}

func setHTMLInputTarget(world *ecs.World, owner ecs.Entity, state *htmlRenderState, target **dom.Node, node *dom.Node, enabled bool) bool {
	if state == nil || target == nil {
		return false
	}
	oldTarget := *target
	newTarget := node
	if !enabled {
		if oldTarget != node {
			return true
		}
		newTarget = nil
	}
	if oldTarget == newTarget {
		return true
	}
	state.PreviousInput = state.Input
	*target = newTarget
	return recomputeHTMLStyles(world, owner, state, nil, &state.PreviousInput)
}

// SetControlChecked updates the live checkedness of a checkbox or radio
// without changing its authored checked content attribute.
func SetControlChecked(world *ecs.World, control ecs.Entity, checked bool) bool {
	if world == nil || control == 0 {
		return false
	}
	value, ok := ecs.Get[Control](world, control)
	inputType := strings.ToLower(strings.TrimSpace(value.Type))
	if !ok || value.Tag != "input" || inputType != "checkbox" && inputType != "radio" || value.Owner == 0 {
		return false
	}
	state := ecs.GetMut[htmlRenderState](world, value.Owner)
	if state == nil || state.Root == nil {
		return false
	}
	node := state.NodeByID[value.ID]
	if node == nil {
		node = lookupHTMLNode(state, value.ID)
	}
	if value.ID == "" {
		if ref, refOK := ecs.Get[htmlControlNodeRef](world, control); refOK {
			node = ref.Node
		}
	}
	if node == nil {
		return false
	}
	rootCount := 0
	if inputType == "radio" && checked && value.Name != "" {
		for entity, access := range ecs.Query[Control](world).All {
			candidate := access.Get()

			if entity == control || candidate.Owner != value.Owner || candidate.Tag != "input" ||
				!strings.EqualFold(strings.TrimSpace(candidate.Type), "radio") || candidate.Name != value.Name || !candidate.Checked {
				continue
			}
			if candidateNode, exists := htmlControlNode(world, value.Owner, entity, candidate); exists {
				if !dom.SameRadioGroup(node, candidateNode) {
					continue
				}
				if current := ecs.GetMut[Control](world, entity); current != nil {
					current.Checked = false
					current.DirtyChecked = true
				}
				candidateNode.States.Set(dom.StateChecked, false)
				candidateNode.States.Set(dom.StateDirtyChecked, true)
				state.StateRoots[rootCount] = candidateNode
				rootCount++
			}
			if !(rootCount < len(state.StateRoots)-1) {
				break
			}
		}
	}
	if current := ecs.GetMut[Control](world, control); current != nil {
		current.Checked = checked
		current.DirtyChecked = true
	}
	node.States.Set(dom.StateChecked, checked)
	node.States.Set(dom.StateDirtyChecked, true)
	state.StateRoots[rootCount] = node
	rootCount++
	return recomputeHTMLControlStyles(world, value.Owner, state, state.StateRoots[:rootCount])
}

// SetControlValue updates an editable control's live value without
// changing its authored value attribute or textarea child text.
func SetControlValue(world *ecs.World, control ecs.Entity, value string) bool {
	if world == nil || control == 0 {
		return false
	}
	current := ecs.GetMut[Control](world, control)
	ok := current != nil
	if !ok || current == nil || current.Owner == 0 || current.Tag != "input" && current.Tag != "textarea" {
		return false
	}
	owner := current.Owner
	current.Value = value
	current.DirtyValue = true
	state := ecs.GetMut[htmlRenderState](world, owner)
	if state == nil || state.Root == nil {
		return true
	}
	node, ok := htmlControlNode(world, owner, control, *current)
	if !ok {
		return true
	}
	wasShown := dom.IsPlaceholderShown(node)
	dom.SetLiveValue(node, value, true)
	shown := dom.IsPlaceholderShown(node)
	if !dom.SupportsPlaceholder(node) {
		return true
	}
	if shown == wasShown || state.Stylesheet == nil {
		return true
	}
	state.StateRoots[0] = node
	return recomputeHTMLControlStyles(world, owner, state, state.StateRoots[:1])
}

// SetControlSelected updates a select option without rebuilding its HTML
// tree. It returns false for an invalid or disabled option.
func SetControlSelected(world *ecs.World, control ecs.Entity, selected int) bool {
	if world == nil || control == 0 {
		return false
	}
	value, ok := ecs.Get[Control](world, control)
	if !ok || value.Tag != "select" || value.Owner == 0 || selected < -1 || selected >= len(value.Options) {
		return false
	}
	state := ecs.GetMut[htmlRenderState](world, value.Owner)
	if state == nil || state.Root == nil {
		return false
	}
	node := state.NodeByID[value.ID]
	if node == nil {
		node = lookupHTMLNode(state, value.ID)
	}
	if value.ID == "" {
		if ref, refOK := ecs.Get[htmlControlNodeRef](world, control); refOK {
			node = ref.Node
		}
	}
	if node == nil {
		return false
	}
	dom.SetSelectedIndex(node, selected)
	if current := ecs.GetMut[Control](world, control); current != nil {
		syncHTMLSelectOptionStates(node, current)
	}
	state.StateRoots[0] = node
	return recomputeHTMLControlStyles(world, value.Owner, state, state.StateRoots[:1])
}

// SetControlOptionSelected updates one option's live selectedness. Unlike
// selectedIndex, this preserves other selected options on a multiple select.
func SetControlOptionSelected(world *ecs.World, control ecs.Entity, option int, selected bool) bool {
	if world == nil || control == 0 {
		return false
	}
	value, ok := ecs.Get[Control](world, control)
	if !ok || value.Tag != "select" || value.Owner == 0 || option < 0 || option >= len(value.Options) {
		return false
	}
	state := ecs.GetMut[htmlRenderState](world, value.Owner)
	if state == nil || state.Root == nil {
		return false
	}
	node, ok := htmlControlNode(world, value.Owner, control, value)
	if !ok || !dom.SetOptionSelectedAt(node, option, selected) {
		return false
	}
	if current := ecs.GetMut[Control](world, control); current != nil {
		syncHTMLSelectOptionStates(node, current)
	}
	state.StateRoots[0] = node
	return recomputeHTMLControlStyles(world, value.Owner, state, state.StateRoots[:1])
}

// SetControlSelectionRange replaces a multiple select's live selectedness
// with the inclusive range between anchor and focus. Disabled options retain
// their prior state, matching Chromium user input.
func SetControlSelectionRange(world *ecs.World, control ecs.Entity, anchor, focus int) bool {
	if world == nil || control == 0 {
		return false
	}
	value, ok := ecs.Get[Control](world, control)
	if !ok || value.Tag != "select" || !value.Multiple || value.Owner == 0 ||
		anchor < 0 || anchor >= len(value.Options) || focus < 0 || focus >= len(value.Options) {
		return false
	}
	start, end := min(anchor, focus), max(anchor, focus)
	changed := false
	for index, option := range value.Options {
		if option.Disabled {
			continue
		}
		if option.Selected != (index >= start && index <= end) {
			changed = true
			break
		}
	}
	if !changed {
		if current := ecs.GetMut[Control](world, control); current != nil {
			current.SelectionAnchor, current.SelectionFocus = anchor, focus
		}
		return true
	}
	state := ecs.GetMut[htmlRenderState](world, value.Owner)
	if state == nil || state.Root == nil {
		return false
	}
	node, ok := htmlControlNode(world, value.Owner, control, value)
	if !ok || !dom.SetSelectedRange(node, anchor, focus) {
		return false
	}
	if current := ecs.GetMut[Control](world, control); current != nil {
		syncHTMLSelectOptionStates(node, current)
		current.SelectionAnchor, current.SelectionFocus = anchor, focus
	}
	state.StateRoots[0] = node
	return recomputeHTMLControlStyles(world, value.Owner, state, state.StateRoots[:1])
}

// SetControlSelectionFocus updates the multiple-select navigation focus
// without changing selectedness. A negative anchor clears the range anchor.
func SetControlSelectionFocus(world *ecs.World, control ecs.Entity, anchor, focus int) bool {
	if world == nil || control == 0 {
		return false
	}
	current := ecs.GetMut[Control](world, control)
	if current == nil || current.Tag != "select" || !current.Multiple ||
		anchor < -1 || anchor >= len(current.Options) || focus < -1 || focus >= len(current.Options) {
		return false
	}
	current.SelectionAnchor, current.SelectionFocus = anchor, focus
	return true
}

// SetEditableValue replaces an editing host's live textContent. The DOM is
// retained and the next HTML synchronization pass recomputes layout.
func SetEditableValue(world *ecs.World, editable ecs.Entity, value string) bool {
	if world == nil || editable == 0 {
		return false
	}
	current := ecs.GetMut[Editable](world, editable)
	ref := ecs.GetMut[htmlEditableNodeRef](world, editable)
	if (current == nil) || (ref == nil) || (ref.Node == nil) || (current.Owner == 0) {
		return false
	}
	if current.Value == value {
		return true
	}
	current.Value = value
	length := utf8.RuneCountInString(value)
	current.Anchor = max(0, min(current.Anchor, length))
	current.Focus = max(0, min(current.Focus, length))
	setHTMLNodeText(ref.Node, value)
	if ref.Text != 0 {
		if text := ecs.GetMut[scene.Text2D](world, ref.Text); text != nil {
			text.Value = value
		}
	}
	Node{world: world, owner: current.Owner, node: ref.Node}.markMutation(htmlMutationStructure)
	return true
}

// SetEditableSelection updates Selection anchor/focus rune offsets without
// rebuilding or allocating.
func SetEditableSelection(world *ecs.World, editable ecs.Entity, anchor, focus int) bool {
	if world == nil || editable == 0 {
		return false
	}
	current := ecs.GetMut[Editable](world, editable)
	if current == nil {
		return false
	}
	length := utf8.RuneCountInString(current.Value)
	if anchor < 0 || focus < 0 || anchor > length || focus > length {
		return false
	}
	current.Anchor, current.Focus = anchor, focus
	return true
}

// ResetForm executes the named form's reset default action without event
// dispatch. UI callers should use ui.ResetForm for a cancelable reset event.
func ResetForm(world *ecs.World, owner ecs.Entity, formID string) bool {
	if world == nil || owner == 0 || formID == "" {
		return false
	}
	state := ecs.GetMut[htmlRenderState](world, owner)
	if state == nil || state.Root == nil {
		return false
	}
	form := lookupHTMLNode(state, formID)
	return form != nil && form.Type == dom.ElementNode && form.Data == "form" && resetHTMLFormNode(world, owner, state, form)
}

// HasForm reports whether a named form exists in an HTML surface.
func HasForm(world *ecs.World, owner ecs.Entity, formID string) bool {
	if world == nil || owner == 0 || formID == "" {
		return false
	}
	state, ok := ecs.Get[htmlRenderState](world, owner)
	if !ok || state.Root == nil {
		return false
	}
	form := lookupHTMLNode(&state, formID)
	return form != nil && form.Type == dom.ElementNode && form.Data == "form"
}

// HasControlForm reports whether a generated control has a form owner. It
// also handles unnamed ancestor forms, which have an empty FormID.
func HasControlForm(world *ecs.World, control ecs.Entity) bool {
	if world == nil || control == 0 {
		return false
	}
	value, ok := ecs.Get[Control](world, control)
	if !ok || value.Owner == 0 {
		return false
	}
	node, ok := htmlControlNode(world, value.Owner, control, value)
	return ok && dom.FormOwner(node) != nil
}

// ResetControlForm executes the reset default action for a generated
// control's form owner without event dispatch.
func ResetControlForm(world *ecs.World, control ecs.Entity) bool {
	if world == nil || control == 0 {
		return false
	}
	value, ok := ecs.Get[Control](world, control)
	if !ok || value.Owner == 0 {
		return false
	}
	node, ok := htmlControlNode(world, value.Owner, control, value)
	if !ok {
		return false
	}
	form := dom.FormOwner(node)
	state := ecs.GetMut[htmlRenderState](world, value.Owner)
	return (form != nil) && (state != nil) && (resetHTMLFormNode(world, value.Owner, state, form))
}

func resetHTMLFormNode(world *ecs.World, owner ecs.Entity, state *htmlRenderState, form *dom.Node) bool {
	if !dom.ResetForm(form) {
		return false
	}
	for entity, access := range ecs.Query[Control](world).All {
		control := access.Get()

		if control.Owner != owner {
			continue
		}
		ref, ok := ecs.Get[htmlControlNodeRef](world, entity)
		if !ok || ref.Node == nil || dom.FormOwner(ref.Node) != form {
			continue
		}
		current := ecs.GetMut[Control](world, entity)
		if current == nil {
			continue
		}
		resetHTMLControlState(ref.Node, current)
		if state.ControlSnapshots != nil {
			state.ControlSnapshots[ref.Node] = htmlControlSnapshot{Value: current.Value}
		}
	}
	state.StateRoots[0] = state.Root
	return recomputeHTMLControlStyles(world, owner, state, state.StateRoots[:1])
}

func resetHTMLControlState(node *dom.Node, control *Control) {
	if node == nil || control == nil {
		return
	}
	control.Open = false
	control.SelectionAnchor, control.SelectionFocus = -1, -1
	control.DirtyValue = false
	control.DefaultChecked = htmlNodeHasAttr(node, "checked")
	control.Checked = dom.IsChecked(node)
	control.DirtyChecked, _ = node.States.Get(dom.StateDirtyChecked)
	switch node.Data {
	case "input":
		control.DefaultValue, _ = node.Attr("value")
		control.Value = control.DefaultValue
		if control.Value == "" && (dom.HasInputType(node, "checkbox") || dom.HasInputType(node, "radio")) {
			control.Value = "on"
		}
	case "textarea":
		control.DefaultValue = htmlNodeText(node)
		control.Value = control.DefaultValue
	case "select":
		syncHTMLSelectOptionStates(node, control)
	}
}

func recomputeHTMLControlStyles(world *ecs.World, owner ecs.Entity, renderState *htmlRenderState, roots []*dom.Node) bool {
	return recomputeHTMLStyles(world, owner, renderState, roots, nil)
}

func recomputeHTMLStyles(world *ecs.World, owner ecs.Entity, renderState *htmlRenderState, roots []*dom.Node, previous *css.State) bool {
	if renderState == nil || renderState.Root == nil || renderState.Stylesheet == nil {
		return false
	}
	var styles style.Map
	var impact style.ChangeImpact
	var err error
	if previous != nil {
		styles, impact, err = style.RecomputeInputStateWithCache(renderState.Root, renderState.Stylesheet, renderState.Viewport, previous, &renderState.Input, renderState.Styles, &renderState.StyleCache)
	} else if renderState.Styles != nil {
		styles = renderState.Styles
		impact, _, err = style.RecomputeSubtreesWithChanges(renderState.Root, renderState.Stylesheet, renderState.Viewport, &renderState.Input, styles, roots, nil, &renderState.StyleCache)
	} else {
		styles, err = computeHTMLStyles(renderState, renderState.Stylesheet, renderState.Viewport)
		impact = style.ChangeRebuild
	}
	if err != nil {
		return false
	}
	renderState.Styles = styles
	syncHTMLRenderedControlStyles(world, owner, renderState, styles)
	if impact == style.ChangeRebuild {
		renderState.PendingLayout = true
	}
	if impact >= style.ChangeVisual {
		renderState.LayoutDirty = true
	}
	if renderState.LayoutDirty && (renderState.Input.Active == nil || renderState.Input.Active == renderState.Input.Focused) {
		if renderState.Key.Revision == renderState.Revision {
			renderState.Revision++
		}
		renderState.LayoutDirty = false
		renderState.PendingVisual = true
	}
	return true
}

func syncHTMLRenderedControlStyles(world *ecs.World, owner ecs.Entity, state *htmlRenderState, styles style.Map) {
	if state == nil {
		return
	}
	syncHTMLRenderedStyles(world, owner, state, styles, state.Children)
}

func syncHTMLRenderedStyles(world *ecs.World, owner ecs.Entity, state *htmlRenderState, styles style.Map, entities []ecs.Entity) {
	if world == nil || state == nil {
		return
	}
	visible := htmlVisible(world, owner)
	for _, entity := range entities {
		child, ok := ecs.Get[htmlChild](world, entity)
		if !ok || child.Owner != owner {
			continue
		}
		computed, ok := styles.Get(child.Node)
		if !ok {
			continue
		}
		if control := ecs.GetRef[Control](world, entity); (control != nil) && control.Tag == "textarea" && control.Wrap != htmlTextareaWrap(computed) {
			current := ecs.GetMut[Control](world, entity)
			current.Wrap = htmlTextareaWrap(computed)
		}
		syncHTMLPointerEvents(world, entity, computed, state.Key.PassThrough)
		syncHTMLCSSVisibility(world, entity, computed)
		nodeVisible := visible && htmlStyleVisible(computed)
		sprite := ecs.GetMut[scene.Sprite2D](world, entity)
		if sprite != nil && nodeVisible {
			if ecs.Has[htmlCanvasSurface](world, entity) {
				sprite.Shape.Fill = htmlColor(computed.BackgroundColor)
				if texture := htmlGradientTextureFor(state, computed.BackgroundImage, sprite.Width, sprite.Height); texture != nil {
					sprite.Texture, sprite.Tint, sprite.Shape = texture, graphics.White, scene.Shape2D{}
				}
			} else if ecs.Has[htmlElementSurface](world, entity) || ecs.Has[Control](world, entity) {
				if htmlNeedsRasterPaint(computed) {
					if texture := htmlPaintTextureFor(state, child.Node, computed, sprite.Width, sprite.Height); texture != nil {
						sprite.Texture, sprite.Tint, sprite.Shape = texture, graphics.White, scene.Shape2D{}
					}
				} else {
					sprite.Shape = htmlShape(computed, float64(sprite.Width), float64(sprite.Height))
					if texture := htmlGradientTextureFor(state, computed.BackgroundImage, sprite.Width, sprite.Height); texture != nil {
						sprite.Texture, sprite.Tint, sprite.Shape = texture, graphics.White, scene.Shape2D{}
					} else {
						sprite.Texture = nil
					}
				}
			} else if ecs.Has[htmlTextDecoration](world, entity) {
				sprite.Shape.Fill = htmlColor(computed.Color)
			}
			state.Textures.bind(entity, sprite.Texture)
		}
		if text := ecs.GetMut[scene.Text2D](world, entity); text != nil {
			color := computed.Color
			if link, linked := ecs.Get[ControlText](world, entity); linked {
				if control, exists := ecs.Get[Control](world, link.Control); exists && control.Value == "" && control.Placeholder != "" {
					color = computed.PlaceholderColor
				}
			}
			text.Color = htmlColor(color)
			text.TabWidth = htmlTextTabWidth(text, computed)
			text.LCD, text.Alpha = htmlTextPaint(styles, child.Node)
		}
		if mark, exists := ecs.Get[htmlCheckboxMark](world, entity); exists {
			if control, exists := ecs.Get[Control](world, mark.Control); exists {
				if sprite != nil {
					sprite.Shape = scene.CheckmarkShape2D(htmlColor(computed.Color))
				}
				if visibility := ecs.GetMut[scene.Visibility](world, entity); visibility != nil {
					visibility.Visible = nodeVisible && control.Checked && htmlUsesNativeAppearance(computed)
				}
			}
		}
		if _, exists := ecs.Get[htmlSelectIndicator](world, entity); exists {
			if image := ecs.GetMut[scene.UIImage2D](world, entity); image != nil {
				image.Tint = htmlColor(computed.Color)
			}
			if visibility := ecs.GetMut[scene.Visibility](world, entity); visibility != nil {
				visibility.Visible = nodeVisible && htmlUsesNativeAppearance(computed)
			}
		}
		syncHTMLNodeTransform(world, state, entity, child.Node, computed)
	}
}

func findHTMLNodeByIDFromRoot(world *ecs.World, owner ecs.Entity, id string) (*dom.Node, bool) {
	if id == "" {
		return nil, false
	}
	state, ok := ecs.Get[htmlRenderState](world, owner)
	if !ok {
		return nil, false
	}
	node := state.NodeByID[id]
	if node == nil {
		node = lookupHTMLNode(&state, id)
	}
	return node, node != nil
}

func lookupHTMLNode(state *htmlRenderState, id string) *dom.Node {
	if state == nil || state.Root == nil || id == "" {
		return nil
	}
	if state.IndexDirty || state.NodeByID == nil {
		syncHTMLDocumentIndex(state)
	}
	return state.NodeByID[id]
}

func htmlControlNode(world *ecs.World, owner, control ecs.Entity, value Control) (*dom.Node, bool) {
	if value.ID != "" {
		return findHTMLNodeByIDFromRoot(world, owner, value.ID)
	}
	ref, ok := ecs.Get[htmlControlNodeRef](world, control)
	return ref.Node, ok && ref.Node != nil
}

// Set updates the HTML source on an existing entity. The visual tree is
// rebuilt by SyncSystem on the next synchronization pass.
func Set(world *ecs.World, entity ecs.Entity, value string) bool {
	html := ecs.GetMut[Surface](world, entity)
	if html == nil {
		return false
	}
	html.Value = value
	return true
}

// SyncSystem synchronizes every HTML component into ordinary ECS UI
// children. Register it before SyncUIGlobalTransformSystem so generated
// children receive their global transforms in the same frame.
func SyncSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "sync_html"
	}
	return ecs.SystemConfig{Name: name, System: syncHTML}
}

// Sync runs one HTML synchronization pass immediately.
func Sync(world *ecs.World) error {
	return SyncSystem("").Run(world)
}

type htmlChild struct {
	Owner           ecs.Entity
	Node            *dom.Node
	role            htmlRenderRole
	offset          mgl32.Vec2
	transformOffset mgl32.Vec2
	scrollArea      ecs.Entity
	scrollOrigin    mgl32.Vec2
}

type htmlElementSurface struct{}
type htmlCanvasSurface struct{}
type htmlOpacitySurface struct{}

// HTML root backgrounds, or the first body's background when the root is
// transparent, paint the canvas rather than the element's content-height box.
func htmlPropagatesCanvasBackground(box *layout.Box) bool {
	if box == nil || box.Node == nil || box.Parent == nil {
		return false
	}
	if !htmlStyleVisible(box.Style) {
		return false
	}
	if box.Node.Data == "html" && box.Parent.Node.Type == dom.DocumentNode {
		return true
	}
	return box.Node.Data == "body" && box.Parent.Node.Data == "html" && box.Parent.Style.BackgroundColor.Color.A == 0 && box.Parent.Style.BackgroundImage == ""
}

type htmlTextDecoration struct{}
type htmlNodeEventTarget struct{}

// htmlOverflowClip marks the invisible UI container used to project CSS
// overflow clipping onto the retained ECS hierarchy.
type htmlOverflowClip struct{}
type htmlNodeEventHandler struct {
	Base scene.InputHandler2D
}

// ScrollContent links a generated HTML subtree to a UIScroll viewport.
// X and Y preserve the subtree's unscrolled local origin while the UI runtime
// applies the current UIScroll offset after input handling.
type ScrollContent struct {
	Area ecs.Entity
	X    float32
	Y    float32
}

type htmlNodeOffset struct {
	node   *dom.Node
	ID     string
	Offset mgl32.Vec2
}

func markHTMLChildNode(world *ecs.World, entity ecs.Entity, node *dom.Node) {
	if world == nil || entity == 0 || node == nil {
		return
	}
	if child := ecs.GetMut[htmlChild](world, entity); child != nil {
		child.Node = node
		if state := ecs.GetMut[htmlRenderState](world, child.Owner); state != nil {
			if computed, ok := state.Styles.Get(node); ok {
				syncHTMLPointerEvents(world, entity, computed, state.Key.PassThrough)
				syncHTMLCSSVisibility(world, entity, computed)
				syncHTMLNodeTransform(world, state, entity, node, computed)
			}
		}
	}
}

// syncHTMLPointerEvents excludes only this element from hit testing. Descendants
// use their own computed value, so an explicit auto overrides inherited none.
func syncHTMLPointerEvents(world *ecs.World, entity ecs.Entity, computed *style.ComputedStyle, passThrough bool) {
	passThrough = passThrough || !htmlStyleVisible(computed) || computed.PointerEvents == style.Keyword("none")
	if target := ecs.GetMut[scene.InputTarget2D](world, entity); target != nil {
		target.PassThrough = passThrough
	}
	if blocker := ecs.GetMut[scene.InputBlocker2D](world, entity); blocker != nil {
		blocker.PassThrough = passThrough
	}
}

// SetNodeOffset moves all retained visuals generated for the element with
// the supplied id. The offset is applied to each visual's local transform, so
// updating an animation does not rebuild the HTML document or its layout.
// For overlapping bindings, the last registered matching offset wins; updating
// an existing binding preserves its registration order.
func SetNodeOffset(world *ecs.World, owner ecs.Entity, id string, x, y float32) bool {
	if world == nil || owner == 0 || id == "" {
		return false
	}
	state := ecs.GetMut[htmlRenderState](world, owner)
	if state == nil || state.Root == nil {
		return false
	}
	target := lookupHTMLNode(state, id)
	if target == nil {
		return false
	}
	offset := mgl32.Vec2{x, y}
	stored := false
	for index := range state.NodeOffsets {
		if state.NodeOffsets[index].ID != id {
			continue
		}
		state.NodeOffsets[index].Offset = offset
		stored = true
		break
	}
	if !stored {
		state.NodeOffsets = append(state.NodeOffsets, htmlNodeOffset{ID: id, Offset: offset})
		if len(state.PaintCullOffsets) != 0 {
			state.Revision++
		}
	}
	prepareHTMLNodeBindings(world, state)
	return applyHTMLNodeBindings(world, state, state.Children)
}

// SetNodeScroll attaches the generated visuals for an HTML element to an
// existing UIScroll viewport. The element's children are reparented to the
// viewport and clipped by its ClipRect2D; the binding is reapplied whenever
// the HTML visual tree is rebuilt. The first registered matching scroll binding
// owns each top-level visual; control and CSS clip descendants inherit it.
func SetNodeScroll(world *ecs.World, owner ecs.Entity, id string, area ecs.Entity, originX, originY float32) bool {
	if world == nil || owner == 0 || id == "" || area == 0 || !world.IsAlive(area) {
		return false
	}
	state := ecs.GetMut[htmlRenderState](world, owner)
	if state == nil || state.Root == nil {
		return false
	}
	target := lookupHTMLNode(state, id)
	if target == nil {
		return false
	}
	binding := htmlNodeScroll{ID: id, Area: area, Origin: mgl32.Vec2{originX, originY}}
	stored := false
	for index := range state.NodeScrolls {
		if state.NodeScrolls[index].ID != id {
			continue
		}
		state.NodeScrolls[index] = binding
		stored = true
		break
	}
	if !stored {
		state.NodeScrolls = append(state.NodeScrolls, binding)
		if len(state.PaintCullOffsets) != 0 {
			state.Revision++
		}
	}
	// Suppressing a generated CSS clip changes the retained parent topology.
	if _, clipped := state.OverflowBindings[target]; clipped {
		state.PartialValid = false
	}
	prepareHTMLNodeBindings(world, state)
	return applyHTMLNodeBindings(world, state, state.Children)
}

type htmlNodeScroll struct {
	node   *dom.Node
	ID     string
	Area   ecs.Entity
	Origin mgl32.Vec2
}

type htmlBindingIndices struct{ offset, scroll int }

// Resolve IDs and registration precedence once per synchronization. Shared
// ancestor routes are then reused across all local entity batches.
func prepareHTMLNodeBindings(world *ecs.World, state *htmlRenderState) {
	clear(state.NodeBindingLocals)
	clear(state.NodeScrollBindings)
	state.NodeBindingRoutes.clear()
	if len(state.NodeOffsets) == 0 && len(state.NodeScrolls) == 0 {
		return
	}
	if state.NodeBindingLocals == nil {
		state.NodeBindingLocals = make(map[*dom.Node]htmlBindingIndices)
	}
	for i := range state.NodeOffsets {
		binding := &state.NodeOffsets[i]
		binding.node = lookupHTMLNode(state, binding.ID)
		if binding.node != nil {
			indices := state.NodeBindingLocals[binding.node]
			indices.offset = i + 1
			state.NodeBindingLocals[binding.node] = indices
		}
	}
	if len(state.NodeScrolls) != 0 && state.NodeScrollBindings == nil {
		state.NodeScrollBindings = make(map[*dom.Node]struct{})
	}
	for i := range state.NodeScrolls {
		binding := &state.NodeScrolls[i]
		binding.node = lookupHTMLNode(state, binding.ID)
		if binding.node == nil {
			continue
		}
		state.NodeScrollBindings[binding.node] = struct{}{}
		if world.IsAlive(binding.Area) {
			indices := state.NodeBindingLocals[binding.node]
			if indices.scroll == 0 {
				indices.scroll = i + 1
				state.NodeBindingLocals[binding.node] = indices
			}
		}
	}
}

// Replay only freshly emitted entities during local reconciliation. Offset
// bindings use registration order (last match wins); the first matching scroll
// binding owns the viewport. Generated control/CSS clip descendants inherit it.
func applyHTMLNodeBindings(world *ecs.World, state *htmlRenderState, entities []ecs.Entity) bool {
	if len(state.NodeOffsets) == 0 && len(state.NodeScrolls) == 0 {
		return false
	}
	inherit := func(node *dom.Node, indices htmlBindingIndices) htmlBindingIndices {
		local := state.NodeBindingLocals[node]
		indices.offset = max(indices.offset, local.offset)
		if local.scroll != 0 && (indices.scroll == 0 || local.scroll < indices.scroll) {
			indices.scroll = local.scroll
		}
		return indices
	}
	changed := false
	for _, entity := range entities {
		state.SyncStats.BindingEntities++
		child := ecs.GetMut[htmlChild](world, entity)
		ok := child != nil
		if !ok || child.Node == nil {
			continue
		}
		transform := ecs.GetMut[scene.UITransform](world, entity)
		if transform == nil {
			continue
		}
		indices := state.NodeBindingRoutes.resolve(child.Node, htmlNodeParent, inherit)
		offset := mgl32.Vec2{}
		if indices.offset != 0 {
			offset = state.NodeOffsets[indices.offset-1].Offset
		}
		parent := ecs.GetMut[scene.Parent](world, entity)
		if parent != nil && parent.Entity != child.Owner {
			if ancestor := ecs.GetRef[htmlChild](world, parent.Entity); ancestor != nil && ancestor.Owner == child.Owner && ancestor.Node != nil {
				// Nested visuals already inherit their parent's bound offset.
				// Apply only the difference in this entity's local coordinates.
				inherited := state.NodeBindingRoutes.resolve(ancestor.Node, htmlNodeParent, inherit)
				if inherited.offset != 0 {
					offset = offset.Sub(state.NodeOffsets[inherited.offset-1].Offset)
				}
			}
		}
		delta := offset.Sub(child.offset)
		if delta != (mgl32.Vec2{}) {
			transform.Translation.X.Value += delta.X()
			transform.Translation.Y.Value += delta.Y()
			if content := ecs.GetMut[ScrollContent](world, entity); content != nil {
				content.X += delta.X()
				content.Y += delta.Y()
			}
			child.offset = offset
			changed = true
		}
		if parent == nil || (parent.Entity != child.Owner && (child.scrollArea == 0 || parent.Entity != child.scrollArea)) {
			continue
		}
		if indices.scroll == 0 {
			continue
		}
		binding := state.NodeScrolls[indices.scroll-1]
		content := ecs.GetMut[ScrollContent](world, entity)
		if (content != nil) && child.scrollArea == binding.Area && child.scrollOrigin == binding.Origin {
			continue
		}
		x, y := transform.Translation.X.Value, transform.Translation.Y.Value
		if (content != nil) && child.scrollArea != 0 {
			// The transform may already contain the runtime scroll offset.
			x, y = content.X+child.scrollOrigin.X(), content.Y+child.scrollOrigin.Y()
		}
		x, y = x-binding.Origin.X(), y-binding.Origin.Y()
		value := ScrollContent{Area: binding.Area, X: x, Y: y}
		parent.Entity = binding.Area
		child.scrollArea, child.scrollOrigin = binding.Area, binding.Origin
		if scroll, ok := scene.GetUIScrollOffset(world, binding.Area); ok {
			x, y = x+scroll.X(), y+scroll.Y()
		}
		transform.Translation.X.Value, transform.Translation.Y.Value = x, y
		// Inserting a component can move the entity to another archetype;
		// finish all writes through existing component pointers first.
		if content != nil {
			*content = value
		} else {
			_ = ecs.Insert(world, entity, value)
		}
		changed = true
	}
	return changed
}

func htmlNodeWithin(node, ancestor *dom.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if current == ancestor {
			return true
		}
	}
	return false
}

type htmlRenderKey struct {
	Value         string
	CSS           string
	Width         float32
	Height        float32
	Font          *onixtext.Font
	BoldFont      *onixtext.Font
	Images        *Images
	FontSize      float64
	DPI           float64
	RasterScale   float64
	StackIndex    int
	PassThrough   bool
	Revision      uint64
	EventRevision uint64
}

type htmlStylesheetKey struct {
	CSS, EmbeddedCSS string
	FontSize         float64
}

type htmlRenderState struct {
	BoxRenderings            map[*dom.Node]*htmlBoxRendering
	EmittingBox              *htmlBoxRendering
	RenderQueue              []*htmlBoxRendering
	RemovedBoxes             []*htmlBoxRendering
	LocalReconcile           bool
	PartialValid             bool
	OwnerSpansValid          bool
	ControlStylesChanged     bool
	LayoutCache              layout.Cache
	LayoutNodes              []*dom.Node
	LayoutOrderNodes         []*dom.Node
	RestyleNodes             []*dom.Node
	LayoutTopology           bool
	StyleCache               style.MutationCache
	BoxesByNode              map[*dom.Node]*layout.Box
	PendingLayout            bool
	PendingVisual            bool
	Visible, VisibilityKnown bool
	Mutations                map[*dom.Node]htmlMutation
	MutationRoots            []*dom.Node
	MutationRootSet          map[*dom.Node]struct{}
	NodeFlags                htmlAncestorCache[*dom.Node, bool]
	NodeAncestors            htmlAncestorCache[*dom.Node, *dom.Node]
	BoxFlags                 htmlAncestorCache[*layout.Box, bool]
	StyleChanges             []style.NodeChange
	IndexDirty               bool
	EntityGroups             map[htmlRenderIdentity]*htmlEntityGroup
	RenderNodes              map[*dom.Node]*htmlNodeRendering
	RenderGeneration         uint64
	Reconciling              bool
	SyncStats                htmlSyncStats

	Key                     htmlRenderKey
	Layout                  Layout
	Rendered                bool
	Animations              map[*dom.Node]*htmlNodeAnimation
	AnimationTrackHeads     map[string]int
	TransformFrame          uint64
	TransformFrameSet       bool
	ImageProviderChanged    ecs.Tick
	Children                []ecs.Entity
	Source                  string
	EmbeddedCSS             string
	Root                    *dom.Node
	Boxes                   *layout.Box
	Stylesheet              *css.Stylesheet
	StylesheetKey           htmlStylesheetKey
	Styles                  style.Map
	Viewport                style.Viewport
	Input                   css.State
	PreviousInput           css.State
	StateRoots              [2]*dom.Node
	LayoutDirty             bool
	Revision                uint64
	EventRevision           uint64
	SnapshotGeneration      uint32
	ControlSnapshots        map[*dom.Node]htmlControlSnapshot
	FocusTargets            []htmlFocusTargetBinding
	FocusByNode             map[*dom.Node]int
	NodeOffsets             []htmlNodeOffset
	NodeScrolls             []htmlNodeScroll
	NodeScrollBindings      map[*dom.Node]struct{}
	NodeBindingLocals       map[*dom.Node]htmlBindingIndices
	NodeBindingRoutes       htmlAncestorCache[*dom.Node, htmlBindingIndices]
	OverflowBindings        map[*dom.Node]htmlOverflowClipBinding
	OverflowEntities        map[ecs.Entity]htmlOverflowClipBinding
	OverflowRoutes          map[*dom.Node]htmlOverflowClipBinding
	OverflowExtents         map[*layout.Box]htmlOverflowExtent
	PaintCullOffsets        map[*dom.Node]mgl32.Vec2
	PaintCullRoots          []*htmlBoxRendering
	PaintCullDisabled       bool
	NodeEvents              map[*dom.Node]htmlNodeEventBinding
	ComponentEvents         map[*dom.Node][]htmlComponentEventBinding
	NodeByID                map[string]*dom.Node
	EventTargets            map[*dom.Node]ecs.Entity
	Textures                htmlTextureCache
	ContainerMetricsScratch map[*dom.Node]style.ContainerMetrics
}

type htmlGradientKey struct {
	Source        string
	Width, Height int
}

type htmlControlSnapshot struct {
	Value           string
	DirtyValue      bool
	Open            bool
	SelectionAnchor int
	SelectionFocus  int
	seen            uint32
}

type htmlFocusTargetBinding struct {
	Generation uint64
	Node       *dom.Node
	Entity     ecs.Entity
	Rect       layout.Rect
}

type htmlSyncEntry struct {
	Entity ecs.Entity
	Value  Surface
}

type htmlSyncEntriesScratch struct{}

func syncHTML(context *ecs.SystemContext) error {
	if context == nil || context.World == nil {
		return nil
	}
	world := context.World
	htmlOwners(world)
	if err := syncHTMLComponents(context); err != nil {
		return err
	}

	owners := htmlOwners(world)
	for owner, children := range owners.Children {
		if world.IsAlive(owner) && ecs.Has[Surface](world, owner) && ecs.Has[htmlRenderState](world, owner) {
			continue
		}
		for _, child := range children {
			world.Despawn(child)
		}
		delete(owners.Children, owner)
		ecs.Remove[htmlRenderState](world, owner)
	}

	entries := ecs.FrameScratch[htmlSyncEntriesScratch, htmlSyncEntry](world)
	defer entries.ReleaseUnused()
	for entity, access := range ecs.Query[Surface](world).All {
		if !scene.IsActive(world, entity) {
			if state := ecs.GetMut[htmlRenderState](world, entity); state != nil {
				syncHTMLVisibility(world, entity, state)
				state.TransformFrameSet = false
			}
			continue
		}
		value := access.Get()

		entries.Values = append(entries.Values, htmlSyncEntry{Entity: entity, Value: value})
	}
	for _, entry := range entries.Values {
		if _, err := renderHTML(world, entry.Entity, entry.Value); err != nil {
			return err
		}
		if state := ecs.GetRef[htmlRenderState](world, entry.Entity); state != nil {
			state.Textures.sweep()
		}
	}
	return nil
}

func htmlSurfaceRenderKey(world *ecs.World, value Surface, state *htmlRenderState) htmlRenderKey {
	width, height := htmlViewport(world, value)
	fontValue := value.Font
	if fontValue == nil {
		fontValue, _ = ecs.GetResource[*onixtext.Font](world)
	}
	if fontValue == nil {
		fontValue = onixtext.DefaultFont()
	}
	fontSize := value.FontSize
	if fontSize <= 0 {
		fontSize = 16
	}
	dpi := value.DPI
	if dpi <= 0 {
		dpi = 72
	}
	return htmlRenderKey{
		Value: value.Value, CSS: value.CSS, Width: width, Height: height,
		Font: fontValue, BoldFont: value.BoldFont, Images: value.Images, FontSize: fontSize, DPI: dpi,
		RasterScale: htmlPaintRasterScale(),
		StackIndex:  value.StackIndex, PassThrough: value.PassThrough, Revision: state.Revision,
		EventRevision: state.EventRevision,
	}
}

func renderHTML(world *ecs.World, owner ecs.Entity, value Surface) (Layout, error) {
	state := ecs.GetMut[htmlRenderState](world, owner)
	if state == nil {
		if err := ecs.Insert(world, owner, htmlRenderState{}); err != nil {
			return Layout{}, err
		}
		state = ecs.GetMut[htmlRenderState](world, owner)
	}
	if state.Root == nil || state.Source != value.Value {
		root, embeddedCSS, err := parseHTMLDocument(value.Value)
		if err != nil {
			return Layout{}, fmt.Errorf("ecs: parse html: %w", err)
		}
		state.Textures.dispose()
		state.Source = value.Value
		state.EmbeddedCSS = embeddedCSS
		state.Root = root
		clear(state.Animations)
		state.TransformFrameSet = false
		state.StyleCache.InvalidateDefinitions()
		syncHTMLDocumentIndex(state)
		state.Input = css.State{}
		clear(state.StateRoots[:])
		clear(state.NodeOffsets)
		state.NodeOffsets = state.NodeOffsets[:0]
		clear(state.NodeScrolls)
		state.NodeScrolls = state.NodeScrolls[:0]
		clear(state.NodeEvents)
		clear(state.ComponentEvents)
		clear(state.EventTargets)
		clear(state.ControlSnapshots)
		state.PaintCullDisabled, state.PaintCullOffsets = false, nil
		state.Revision++
	}
	if state.NodeByID == nil || state.IndexDirty {
		syncHTMLDocumentIndex(state)
	}
	if clock, ok := ecs.GetResource[*scene.Time](world); ok && clock != nil {
		if state.TransformFrameSet && state.TransformFrame != clock.Frame && state.Rendered && state.Key.Value == value.Value {
			advanceHTMLAnimations(world, state, clock.Delta.Seconds())
		}
		state.TransformFrame, state.TransformFrameSet = clock.Frame, true
	}

	key := htmlSurfaceRenderKey(world, value, state)
	width, height := key.Width, key.Height
	fontValue, fontSize, dpi := key.Font, key.FontSize, key.DPI
	state.SyncStats = htmlSyncStats{}
	imageProviderChanged := htmlImageProviderChanged(world)
	if state != nil && state.Rendered && state.Key == key && state.ImageProviderChanged == imageProviderChanged {
		if !state.PaintCullDisabled && htmlPaintCullChanged(world, state) {
			reconcileHTMLPaintCull(world, owner, state, fontValue, value.BoldFont, dpi, value.StackIndex, value.PassThrough, htmlVisible(world, owner))
		}
		syncHTMLVisibility(world, owner, state)
		return state.Layout, nil
	}
	if state != nil && state.Rendered && sameHTMLRenderKeyExceptEvents(state.Key, key) && state.ImageProviderChanged == imageProviderChanged {
		cleanupHTMLNodeEventTargets(world, state)
		bindHTMLNodeEvents(world, owner, state, state.Boxes, value.PassThrough, htmlVisible(world, owner), value.StackIndex)
		state.Key = key
		htmlOwners(world).Children[owner] = state.Children
		syncHTMLVisibility(world, owner, state)
		return state.Layout, nil
	}

	if !htmlNodeWithin(state.Input.Hovered, state.Root) {
		state.Input.Hovered = nil
	}
	if !htmlNodeWithin(state.Input.Active, state.Root) {
		state.Input.Active = nil
	}
	if !htmlNodeWithin(state.Input.Focused, state.Root) {
		state.Input.Focused = nil
		state.Input.FocusVisible = false
	}

	prepareHTMLNodeBindings(world, state)
	mutationSync := !state.PendingLayout && state.Rendered && len(state.Mutations) != 0 && sameHTMLRenderEnvironment(state.Key, key) && state.ImageProviderChanged == imageProviderChanged
	visualSync := state.PendingVisual && !state.PendingLayout && state.Rendered && sameHTMLRenderEnvironment(state.Key, key) && state.ImageProviderChanged == imageProviderChanged
	needsLayout := !visualSync
	if mutationSync {
		forceLayout := prepareHTMLMutationRoots(state)
		state.StyleChanges = state.StyleChanges[:0]
		impact, visited, err := style.RecomputeSubtreesWithChanges(state.Root, state.Stylesheet, state.Viewport, &state.Input, state.Styles, state.MutationRoots, &state.StyleChanges, &state.StyleCache)
		state.SyncStats.Styled = visited
		if err != nil {
			state.PendingLayout = true
			return Layout{}, fmt.Errorf("ecs: recompute html style: %w", err)
		}
		for _, change := range state.StyleChanges {
			if change.Impact == style.ChangeRebuild {
				state.LayoutNodes = append(state.LayoutNodes, change.Node)
			}
			state.LayoutTopology = state.LayoutTopology || change.Tree && !change.Reorder
			if change.Reorder {
				state.LayoutOrderNodes = append(state.LayoutOrderNodes, change.Node)
			}
			state.RestyleNodes = append(state.RestyleNodes, change.Node)
			if box := htmlNearestBox(state, change.Node); box != nil {
				box.InvalidatePaintHash()
			}
		}
		state.NodeAncestors.clear()
		// Interning may move a restyled node to another instance, so retained
		// boxes and fragments re-adopt the pointers the cascade just chose.
		syncHTMLAnimationChanges(state)
		state.LayoutCache.Restyle(state.RestyleNodes, state.Styles)
		// A newly transformed subtree may contain unmounted paint. Updating
		// existing sprite properties alone would leave that content missing.
		if impact < style.ChangeVisual {
			for _, change := range state.StyleChanges {
				if box := state.BoxesByNode[change.Node]; box != nil && htmlPaintWindowChanged(state, state.BoxRenderings[box.Node]) {
					impact = style.ChangeVisual
					break
				}
			}
		}
		clear(state.RestyleNodes)
		state.RestyleNodes = state.RestyleNodes[:0]
		needsLayout = forceLayout || impact == style.ChangeRebuild
		if !needsLayout && impact < style.ChangeVisual && !state.PendingVisual {
			for _, change := range state.StyleChanges {
				if rendered := state.RenderNodes[change.Node]; rendered != nil {
					syncHTMLRenderedStyles(world, owner, state, state.Styles, rendered.Entities)
					state.SyncStats.PaintUpdated += len(rendered.Entities)
				}
			}
			if state.Key.EventRevision != key.EventRevision {
				cleanupHTMLNodeEventTargets(world, state)
				bindHTMLNodeEvents(world, owner, state, state.Boxes, value.PassThrough, htmlVisible(world, owner), value.StackIndex)
			}
			state.Key = key
			syncHTMLVisibility(world, owner, state)
			htmlOwners(world).Children[owner] = state.Children
			clearHTMLMutations(state)
			return state.Layout, nil
		}
	}

	stylesheet := state.Stylesheet
	var err error
	if !mutationSync {
		stylesheetKey := htmlStylesheetKey{CSS: value.CSS, EmbeddedCSS: state.EmbeddedCSS, FontSize: fontSize}
		if stylesheet == nil || state.StylesheetKey != stylesheetKey {
			stylesheet, err = htmlStylesheet(value.CSS, state.EmbeddedCSS, fontSize)
			if err != nil {
				return Layout{}, fmt.Errorf("ecs: parse html css: %w", err)
			}
			state.StylesheetKey = stylesheetKey
		}
	}
	viewport := style.Viewport{Width: float64(width), Height: float64(height)}
	styles := state.Styles
	if !mutationSync {
		styles, err = computeHTMLStyles(state, stylesheet, viewport)
		if err != nil {
			return Layout{}, fmt.Errorf("ecs: compute html style: %w", err)
		}
		state.SyncStats.Styled = len(styles)
	} else if state.LayoutTopology {
		// Removed nodes must not remain retained by the style cache.
		withinDocument := func(node *dom.Node, inside bool) bool { return inside || node == state.Root }
		for node := range styles {
			if !state.NodeFlags.resolve(node, htmlNodeParent, withinDocument) {
				delete(styles, node)
				delete(state.Animations, node)
				delete(state.OverflowRoutes, node)
			}
		}
		state.NodeFlags.clear()
	}
	state.Stylesheet, state.Styles, state.Viewport = stylesheet, styles, viewport
	if !mutationSync {
		syncHTMLAnimations(state)
	}
	var face *font.Face
	if fontValue != nil {
		face = fontValue.Face
	}
	var boldFace *font.Face
	if value.BoldFont != nil {
		boldFace = value.BoldFont.Face
	}
	box := state.Boxes
	if needsLayout {
		box, err = state.LayoutCache.Update(state.Root, styles, face, boldFace, float64(width), float64(height), state.LayoutNodes, state.LayoutOrderNodes, state.LayoutTopology, !mutationSync)
		// Retired boxes are consumed below; do not keep removed DOM subtrees
		// alive in scratch storage while the document remains unchanged.
		defer func() {
			clear(state.LayoutCache.Removed)
			state.LayoutCache.Removed = state.LayoutCache.Removed[:0]
			clear(state.LayoutCache.Reordered)
			state.LayoutCache.Reordered = state.LayoutCache.Reordered[:0]
		}()
		state.SyncStats.LayoutWork = state.LayoutCache.Work
		if err != nil {
			return Layout{}, fmt.Errorf("ecs: layout html: %w", err)
		}
		state.SyncStats.Layouts++
	}
	if style.HasContainerQueries(stylesheet) && box != nil {
		var containerLayout bool
		box, containerLayout, err = convergeHTMLContainerQueries(state, box, face, boldFace, float64(width), float64(height))
		if err != nil {
			return Layout{}, fmt.Errorf("ecs: resolve container queries: %w", err)
		}
		needsLayout = needsLayout || containerLayout
	}

	visible := htmlVisible(world, owner)
	state.ControlStylesChanged = false
	partial := mutationSync && !state.PendingVisual && state.Key.EventRevision == key.EventRevision && reconcileHTMLPartial(world, owner, state, needsLayout, fontValue, value.BoldFont, dpi, value.StackIndex, value.PassThrough, visible)
	if !partial {
		updateHTMLControlSnapshots(world, state, state.Children)
		clear(state.EventTargets)
		state.PendingLayout = false
		state.Visible, state.VisibilityKnown = visible, true
		if state.BoxesByNode == nil {
			state.BoxesByNode = make(map[*dom.Node]*layout.Box)
		} else {
			clear(state.BoxesByNode)
		}
		state.RenderGeneration++
		state.Reconciling = true
		for _, entity := range state.Children {
			if child := ecs.GetMut[htmlChild](world, entity); child != nil {
				// Full layout rebuilds reset retained transforms to their authored
				// positions. Replay external bindings from zero for reused entities.
				child.offset = mgl32.Vec2{}
				child.scrollArea = 0
				child.scrollOrigin = mgl32.Vec2{}
			}
		}
		state.Children = state.Children[:0]
		state.FocusTargets = state.FocusTargets[:0]
		clear(state.FocusByNode)
		state.Key, state.Rendered, state.ImageProviderChanged, state.LayoutDirty = key, true, imageProviderChanged, false

		clear(state.PaintCullOffsets)
		htmlSpawnBoxes(world, owner, state, box, fontValue, value.BoldFont, dpi, value.StackIndex, value.PassThrough, visible, 0, 0, layout.Rect{})
		applyHTMLOverflowClips(world, owner, state, box, value.StackIndex, visible)
		finishHTMLReconcile(world, state)
		state.PartialValid = true
	}
	state.PendingLayout, state.PendingVisual = false, false
	state.Key, state.Rendered, state.ImageProviderChanged, state.LayoutDirty = key, true, imageProviderChanged, false
	htmlOwners(world).Children[owner] = state.Children
	clearHTMLMutations(state)
	if !partial {
		// Emission may retire an entity used as a scroll viewport.
		prepareHTMLNodeBindings(world, state)
		applyHTMLNodeBindings(world, state, state.Children)
	}
	if state.ControlStylesChanged || !partial && restoreHTMLControlSnapshots(world, state) {
		styles, recomputeErr := computeHTMLStyles(state, state.Stylesheet, state.Viewport)
		if recomputeErr == nil {
			state.Styles = styles
			syncHTMLRenderedControlStyles(world, owner, state, styles)
		}
	}
	content := box.BorderBox()
	result := Layout{Width: float32(content.Width), Height: float32(content.Height)}
	state.Boxes = box
	state.Layout = result
	if size := ecs.GetMut[scene.Size2D](world, owner); size != nil {
		size.Width, size.Height = float32(content.Width), float32(content.Height)
	} else if err := ecs.Insert(world, owner, scene.Size2D{Width: float32(content.Width), Height: float32(content.Height)}); err != nil {
		return Layout{}, err
	}
	if target := ecs.GetMut[scene.InputTarget2D](world, owner); target != nil {
		target.Width, target.Height = float32(content.Width), float32(content.Height)
	}
	return result, nil
}

func computeHTMLStyles(state *htmlRenderState, stylesheet *css.Stylesheet, viewport style.Viewport) (style.Map, error) {
	return style.ComputeWithCache(state.Root, stylesheet, viewport, &state.Input, state.Styles, &state.StyleCache)
}

func convergeHTMLContainerQueries(state *htmlRenderState, box *layout.Box, face, boldFace *font.Face, width, height float64) (*layout.Box, bool, error) {
	if state == nil || state.Root == nil || box == nil {
		return box, false, nil
	}
	laidOut := false
	for pass := 0; pass < 4; pass++ {
		metricsChanged := refreshHTMLContainerMetrics(state, box)
		if pass > 0 && !metricsChanged {
			break
		}
		state.StateRoots[0] = state.Root
		changeStart := len(state.StyleChanges)
		impact, visited, err := style.RecomputeSubtreesWithChanges(state.Root, state.Stylesheet, state.Viewport, &state.Input, state.Styles, state.StateRoots[:1], &state.StyleChanges, &state.StyleCache)
		state.SyncStats.Styled += visited
		if err != nil {
			return box, laidOut, err
		}
		state.RestyleNodes = state.RestyleNodes[:0]
		state.LayoutNodes = state.LayoutNodes[:0]
		state.LayoutOrderNodes = state.LayoutOrderNodes[:0]
		for _, change := range state.StyleChanges[changeStart:] {
			state.RestyleNodes = append(state.RestyleNodes, change.Node)
			if change.Impact == style.ChangeRebuild {
				state.LayoutNodes = append(state.LayoutNodes, change.Node)
			}
			if change.Reorder {
				state.LayoutOrderNodes = append(state.LayoutOrderNodes, change.Node)
			} else {
				state.LayoutTopology = state.LayoutTopology || change.Tree
			}
		}
		syncHTMLAnimations(state)
		state.LayoutCache.Restyle(state.RestyleNodes, state.Styles)
		if impact < style.ChangeRebuild {
			break
		}
		box, err = state.LayoutCache.Update(state.Root, state.Styles, face, boldFace, width, height, state.LayoutNodes, state.LayoutOrderNodes, state.LayoutTopology, false)
		if err != nil {
			return box, laidOut, err
		}
		state.SyncStats.LayoutWork = state.LayoutCache.Work
		state.SyncStats.Layouts++
		laidOut = true
	}
	state.Boxes = box
	return box, laidOut, nil
}

func refreshHTMLContainerMetrics(state *htmlRenderState, root *layout.Box) bool {
	if state.ContainerMetricsScratch == nil {
		state.ContainerMetricsScratch = make(map[*dom.Node]style.ContainerMetrics)
	}
	next := state.ContainerMetricsScratch
	clear(next)
	var visit func(*layout.Box)
	visit = func(box *layout.Box) {
		if box == nil {
			return
		}
		if box.Node != nil && box.Style != nil && box.Style.ContainerType.Kind == style.ValueKeyword && box.Style.ContainerType.Keyword != "normal" {
			metrics := style.ContainerMetrics{Width: box.Content.Width}
			if box.Style.ContainerType.Keyword == "size" {
				metrics.Height = box.Content.Height
			}
			next[box.Node] = metrics
		}
		for _, child := range box.Children {
			visit(child)
		}
	}
	visit(root)
	previous := state.StyleCache.ContainerMetrics
	changed := len(previous) != len(next)
	if !changed {
		for node, metrics := range next {
			if previous[node] != metrics {
				changed = true
				break
			}
		}
	}
	state.StyleCache.ContainerMetrics, state.ContainerMetricsScratch = next, previous
	return changed
}

func sameHTMLRenderKeyExceptEvents(left, right htmlRenderKey) bool {
	left.EventRevision = 0
	right.EventRevision = 0
	return left == right
}

func cleanupHTMLNodeEventTargets(world *ecs.World, state *htmlRenderState) {
	if state != nil {
		// Event-only updates may add or remove entities outside box emission.
		state.PartialValid = false
	}
	if world == nil || state == nil || len(state.Children) == 0 {
		return
	}
	children := state.Children[:0]
	for _, entity := range state.Children {
		if ecs.Has[htmlNodeEventTarget](world, entity) {
			child, ok := ecs.Get[htmlChild](world, entity)
			binding := state.NodeEvents[child.Node]
			if !ok || binding.Pointer == nil && binding.Click == nil {
				if ok {
					delete(state.EventTargets, child.Node)
				}
				if world.IsAlive(entity) {
					if handler, exists := ecs.Get[htmlNodeEventHandler](world, entity); exists {
						if current := ecs.GetMut[scene.InputHandler2D](world, entity); current != nil {
							*current = handler.Base
						}
						ecs.Remove[htmlNodeEventHandler](world, entity)
					}
				}
				world.Despawn(entity)
				continue
			}
		}
		children = append(children, entity)
	}
	state.Children = children
}

// HTML mutations rebuild generated controls; retain live state by DOM node
// identity so a replacement node cannot inherit state merely by reusing an ID.
func updateHTMLControlSnapshots(world *ecs.World, state *htmlRenderState, entities []ecs.Entity) {
	if world == nil || state == nil || len(state.ControlSnapshots) == 0 {
		return
	}
	for _, child := range entities {
		control, ok := ecs.Get[Control](world, child)
		ref, hasRef := ecs.Get[htmlControlNodeRef](world, child)
		if !ok || !hasRef || ref.Node == nil {
			continue
		}
		if snapshot, ok := state.ControlSnapshots[ref.Node]; ok {
			snapshot.Value = control.Value
			snapshot.DirtyValue = control.DirtyValue
			snapshot.Open = control.Open
			snapshot.SelectionAnchor = control.SelectionAnchor
			snapshot.SelectionFocus = control.SelectionFocus
			state.ControlSnapshots[ref.Node] = snapshot
			state.SyncStats.SnapshottedControls++
		}
	}
}

func restoreHTMLControlSnapshots(world *ecs.World, state *htmlRenderState) bool {
	if world == nil || state == nil {
		return false
	}
	state.SnapshotGeneration++
	changed := restoreHTMLControlEntities(world, state, state.Children)
	for node, snapshot := range state.ControlSnapshots {
		if snapshot.seen != state.SnapshotGeneration {
			delete(state.ControlSnapshots, node)
		}
	}
	return changed
}

func restoreHTMLControlEntities(world *ecs.World, state *htmlRenderState, entities []ecs.Entity) bool {
	changed := false
	for _, child := range entities {
		control := ecs.GetMut[Control](world, child)
		ref, hasRef := ecs.Get[htmlControlNodeRef](world, child)
		if control == nil || !hasRef || ref.Node == nil {
			continue
		}
		if state.ControlSnapshots == nil {
			state.ControlSnapshots = make(map[*dom.Node]htmlControlSnapshot)
		}
		if snapshot, exists := state.ControlSnapshots[ref.Node]; exists {
			restoreHTMLControlSnapshot(control, snapshot)
			control.Open = snapshot.Open
		}
		wasShown := dom.IsPlaceholderShown(ref.Node)
		dom.SetLiveValue(ref.Node, control.Value, control.DirtyValue)
		if wasShown != dom.IsPlaceholderShown(ref.Node) {
			changed = true
		}
		state.ControlSnapshots[ref.Node] = htmlControlSnapshot{
			Value: control.Value, DirtyValue: control.DirtyValue, Open: control.Open,
			SelectionAnchor: control.SelectionAnchor, SelectionFocus: control.SelectionFocus, seen: state.SnapshotGeneration,
		}
	}
	return changed
}

func restoreHTMLControlSnapshot(control *Control, snapshot htmlControlSnapshot) {
	if control == nil {
		return
	}
	if snapshot.DirtyValue {
		control.Value = snapshot.Value
		control.DirtyValue = true
	}
	control.Open = snapshot.Open
	control.SelectionAnchor = snapshot.SelectionAnchor
	control.SelectionFocus = snapshot.SelectionFocus
}

func htmlImageProviderChanged(world *ecs.World) ecs.Tick {
	return ecs.ResourceChangeTick[ImageProvider](world)
}

func htmlViewport(world *ecs.World, value Surface) (float32, float32) {
	width, height := value.Width, value.Height
	if viewport, ok := ecs.GetResource[scene.ViewportSize](world); ok {
		if width <= 0 {
			width = viewport.Width
		}
		if height <= 0 {
			height = viewport.Height
		}
	}
	if width <= 0 {
		if context := graphics.CurrentContext(); context != nil {
			width, _ = context.Size()
		}
	}
	if height <= 0 {
		if context := graphics.CurrentContext(); context != nil {
			_, height = context.Size()
		}
	}
	return max(width, 1), max(height, 1)
}

func parseHTMLDocument(value string) (*dom.Node, string, error) {
	root, err := dom.Parse(value)
	if err != nil {
		return nil, "", err
	}
	var builder strings.Builder
	collectHTMLStylesAndStripMetadata(&builder, root, true)
	return root, builder.String(), nil
}

func htmlStylesheet(cssText, embeddedCSS string, fontSize float64) (*css.Stylesheet, error) {
	var builder strings.Builder
	builder.WriteString(":root { font-size:")
	builder.WriteString(strconv.FormatFloat(fontSize, 'f', -1, 64))
	builder.WriteString("px; }p { margin-top: 1em; }")
	if cssText != "" {
		builder.WriteString(cssText)
	}
	builder.WriteString(embeddedCSS)
	return css.Parse(builder.String())
}

// Visit removed subtrees too: styles inside head still participate in source order.
func collectHTMLStylesAndStripMetadata(builder *strings.Builder, node *dom.Node, keep bool) {
	if node == nil {
		return
	}
	styleElement := node.Type == dom.ElementNode && node.Data == "style"
	children := node.Children[:0]
	for _, child := range node.Children {
		if child == nil {
			continue
		}
		if styleElement && child.Type == dom.TextNode {
			builder.WriteString(child.Data)
		}
		retain := keep && !(child.Type == dom.ElementNode && isHTMLMetadataElement(child.Data))
		collectHTMLStylesAndStripMetadata(builder, child, retain)
		if retain {
			children = append(children, child)
		}
	}
	clear(node.Children[len(children):])
	node.Children = children
}

func isHTMLMetadataElement(tag string) bool {
	switch tag {
	case "head", "style", "script", "title", "meta", "link", "base":
		return true
	default:
		return false
	}
}

func htmlVisible(world *ecs.World, entity ecs.Entity) bool {
	visible, ok := ecs.Get[scene.Visibility](world, entity)
	return !ok || visible.Visible
}

func setHTMLChildrenVisible(world *ecs.World, state *htmlRenderState, children []ecs.Entity, visible bool) {
	for _, child := range children {
		computed := state.Styles[htmlEntityNode(world, child)]
		nodeVisible := visible && (htmlStyleVisible(computed) || ecs.Has[htmlOverflowClip](world, child) || ecs.Has[htmlOpacitySurface](world, child))
		if visibility := ecs.GetMut[scene.Visibility](world, child); visibility != nil {
			if mark, ok := ecs.Get[htmlCheckboxMark](world, child); ok {
				control, _ := ecs.Get[Control](world, mark.Control)
				visibility.Visible = nodeVisible && control.Checked && htmlUsesNativeAppearance(computed)
			} else if ecs.Has[htmlSelectIndicator](world, child) {
				visibility.Visible = nodeVisible && htmlUsesNativeAppearance(computed)
			} else if !nodeVisible || !ecs.Has[ControlVisual](world, child) && !ecs.Has[EditableVisual](world, child) {
				visibility.Visible = nodeVisible
			}
		}
	}
}

func htmlSpawnBoxes(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, fontValue, boldFont *onixtext.Font, dpi float64, stackIndex int, passThrough, visible bool, inheritedControl, inheritedFocusOwner ecs.Entity, inheritedControlRect layout.Rect) {
	htmlSpawnBoxesPaint(world, owner, state, box, fontValue, boldFont, dpi, stackIndex, passThrough, visible, inheritedControl, inheritedFocusOwner, inheritedControlRect, false, htmlPaintWindow{})
}

// htmlSpawnBoxesPaint carries the paint suppression used by flattened opacity
// groups. The DOM boxes are still traversed for layout/event bookkeeping, but
// their individual visuals are replaced by the group's single composited image.
func htmlSpawnBoxesPaint(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, fontValue, boldFont *onixtext.Font, dpi float64, stackIndex int, passThrough, visible bool, inheritedControl, inheritedFocusOwner ecs.Entity, inheritedControlRect layout.Rect, suppressPaint bool, window htmlPaintWindow) {
	if box == nil {
		return
	}
	restyleHTMLVisualBox(state, box)
	rendered := beginHTMLBoxRendering(state, box, inheritedControl, inheritedFocusOwner)
	if state.LocalReconcile {
		window = htmlPaintWindow{}
		if box.Parent != nil {
			window = state.BoxRenderings[box.Parent.Node].Window
		}
	}
	outer := htmlPaintWindowForBox(state, box, window)
	if state.LocalReconcile && rendered.OuterWindow == outer && rendered.CullRect == box.PaddingBox() &&
		rendered.OverflowX == box.Style.OverflowX && rendered.OverflowY == box.Style.OverflowY &&
		(outer.unbounded || len(box.Style.Filters) != 0) == rendered.Window.unbounded {
		// A local paint update must not move the buffered scroll anchor unless
		// its descendants are also re-windowed. Scroll reconciliation has
		// already supplied a new window before entering this function.
		window = rendered.Window
	} else {
		window = htmlPaintWindowForClip(world, state, box, outer)
	}
	rendered.CullRect = box.PaddingBox()
	rendered.OuterWindow, rendered.Window = outer, window
	rendered.PaintBounds = htmlBoxPaintBounds(box)
	rendered.CompositeRoot = false
	entityStart := len(state.Children)
	state.BoxesByNode[box.Node] = box
	baseStack := htmlBoxStackIndex(box, stackIndex)
	paintVisible := htmlStyleVisible(box.Style)
	flattened := false
	var regularFace, boldFace *font.Face
	if fontValue != nil {
		regularFace = fontValue.Face
	}
	if boldFont != nil {
		boldFace = boldFont.Face
	}
	state.Textures.face, state.Textures.boldFace = regularFace, boldFace
	if !suppressPaint {
		if bounds, composite := htmlCompositePaintBounds(state, box, regularFace); composite {
			rendered.EffectBounds = bounds
			// A skipped group still suppresses its individual child visuals and
			// retains input/focus targets. Rasterization is deferred until needed.
			if bounds.Width <= 0 || bounds.Height <= 0 || !outer.contains(bounds) || box.Style.Opacity == style.Number(0) && !box.Style.AnimatedOpacity {
				flattened = true
			} else if entity := spawnHTMLFlattenedOpacitySurface(world, owner, state, box, baseStack, visible); entity != 0 {
				flattened = true
				markHTMLChildNode(world, entity, box.Node)
				state.Children = append(state.Children, entity)
			}
			rendered.CompositeRoot = flattened
		}
	}
	rendered.Composite = flattened || suppressPaint
	if !suppressPaint && !flattened && paintVisible && htmlPropagatesCanvasBackground(box) {
		color := htmlColor(box.Style.BackgroundColor)
		if color.A > 0 || isHTMLLinearGradient(box.Style.BackgroundImage) || box.Style.AnimatedPaint&style.AnimatedBackgroundColor != 0 {
			entity := spawnHTMLSurface(world, owner, box.Node, htmlRoleCanvas, 0, 0, float32(state.Viewport.Width), float32(state.Viewport.Height), scene.RoundedRectShape2D(color, 0), stackIndex-1, visible)
			_ = ecs.Insert(world, entity, htmlCanvasSurface{})
			markHTMLChildNode(world, entity, box.Node)
			state.Children = append(state.Children, entity)
		}
	}

	rendered.Stack = baseStack
	control, controlRect := inheritedControl, inheritedControlRect
	if control != 0 && !htmlStyleVisible(state.Styles[htmlEntityNode(world, control)]) {
		control = 0
	}
	focusOwner := inheritedFocusOwner
	if focusOwner != 0 && !htmlStyleVisible(state.Styles[htmlEntityNode(world, focusOwner)]) {
		focusOwner = 0
	}
	if !suppressPaint && !flattened && isHTMLControlNode(box.Node) {
		focusOwner = 0
		controlRect = box.BorderBox()
		control = spawnHTMLControl(world, owner, box.Node, controlRect, box.Border, htmlShape(box.Style, controlRect.Width, controlRect.Height), baseStack, visible)
		if control != 0 {
			markHTMLChildNode(world, control, box.Node)
			state.Children = append(state.Children, control)
			htmlSpawnControlVisuals(world, owner, state, control, box.Node, baseStack)
			if isHTMLCheckboxNode(box.Node) && htmlUsesNativeAppearance(box.Style) {
				if mark := spawnHTMLCheckboxMark(world, owner, control, controlRect, box.Style, baseStack+1, visible); mark != 0 {
					state.Children = append(state.Children, mark)
				}
			}
			if box.Node != nil && box.Node.Data == "select" && htmlUsesNativeAppearance(box.Style) {
				if indicator := spawnHTMLSelectIndicator(world, owner, control, controlRect, box.Style, baseStack+2, passThrough, visible); indicator != 0 {
					state.Children = append(state.Children, indicator)
				}
			}
		}
	} else if !suppressPaint && !flattened {
		if focusable, ok := htmlFocusableFromNode(owner, box.Node); ok {
			focusOwner = ensureHTMLFocusable(world, owner, state, box.Node, box.BorderBox(), focusable, baseStack, passThrough, visible)
		}
		if box.Node != nil && box.Node.Type == dom.ElementNode && box.Node.Data == "img" && paintVisible && outer.contains(box.BorderBox()) {
			fragment := layout.ImageFragment{Node: box.Node, Rect: box.BorderBox(), Style: box.Style}
			if entity, ok := spawnHTMLImage(world, owner, fragment, baseStack, visible); ok {
				markHTMLChildNode(world, entity, box.Node)
				if control != 0 {
					attachHTMLImageToControl(world, entity, control, controlRect)
				}
				if focusOwner != 0 {
					routeHTMLImageInput(world, entity, 0)
					linkHTMLFocusOwner(world, entity, focusOwner)
				}
				state.Children = append(state.Children, entity)
			}
		} else if paintVisible {
			receiver := control
			if focusOwner != 0 {
				receiver = 0
			}
			if entity, ok := spawnHTMLBoxSurface(world, owner, state, box, outer, baseStack, receiver, passThrough, visible); ok {
				markHTMLChildNode(world, entity, box.Node)
				linkHTMLFocusOwner(world, entity, focusOwner)
				state.Children = append(state.Children, entity)
			}
			if outer.contains(rendered.PaintBounds) {
				spawnHTMLBoxShadows(world, owner, state, box, baseStack, visible)
			}
			if outer.contains(box.BorderBox()) {
				spawnHTMLBorderSurfaces(world, owner, state, box, baseStack, visible)
			}
		}
	}
	if !suppressPaint && !flattened && control != 0 && isHTMLEditableControlNode(box.Node) {
		textFont := htmlFontForControl(world, control, fontValue, boldFont, box.Style)
		var textFace *font.Face
		if textFont != nil {
			textFace = textFont.Face
		}
		fragment := htmlEditableControlTextFragment(world, control, box.Node, controlRect, box.Content, box.Style, textFace)
		// Editable text stays above its selection layer (baseStack+1) so the
		// highlight never paints over the glyphs.
		spawnHTMLTextVisuals(world, owner, state, control, control, 0, controlRect, fragment, fontValue, boldFont, dpi, baseStack+2, passThrough, visible)
	} else if !suppressPaint && !flattened && spawnHTMLEditableText(world, owner, state, focusOwner, box, fontValue, boldFont, dpi, baseStack+2, passThrough, visible) {
	} else if !suppressPaint && !flattened && (control == 0 || !isHTMLCheckboxNode(box.Node)) {
		var memo htmlFocusableMemo
		for _, fragment := range box.TextFragments {
			if !htmlPaintFragmentVisible(window, fragment.Rect, fragment.Style) {
				continue
			}
			fragmentFocusOwner := focusOwner
			if control == 0 {
				fragmentFocusOwner = ensureHTMLFragmentFocusable(world, owner, state, &memo, fragment.Node, fragment.Rect, focusOwner, baseStack, passThrough, visible)
			}
			receiver := control
			if fragmentFocusOwner != 0 {
				receiver = 0
			}
			spawnHTMLTextVisuals(world, owner, state, control, receiver, fragmentFocusOwner, controlRect, fragment, fontValue, boldFont, dpi, baseStack+1, passThrough, visible)
		}
	}
	var memo htmlFocusableMemo
	for _, fragment := range box.ImageFragments {
		if suppressPaint || flattened || !htmlPaintFragmentVisible(window, fragment.Rect, fragment.Style) {
			continue
		}
		if entity, ok := spawnHTMLImage(world, owner, fragment, baseStack+1, visible); ok {
			markHTMLChildNode(world, entity, fragment.Node)
			fragmentFocusOwner := focusOwner
			if control == 0 {
				fragmentFocusOwner = ensureHTMLFragmentFocusable(world, owner, state, &memo, fragment.Node, fragment.Rect, focusOwner, baseStack, passThrough, visible)
			}
			if control != 0 {
				attachHTMLImageToControl(world, entity, control, controlRect)
			}
			if fragmentFocusOwner != 0 {
				routeHTMLImageInput(world, entity, 0)
				linkHTMLFocusOwner(world, entity, fragmentFocusOwner)
			}
			state.Children = append(state.Children, entity)
		}
	}
	for _, fragment := range box.ControlFragments {
		if suppressPaint || flattened {
			continue
		}
		controlStack := baseStack + 1
		entity := spawnHTMLControl(world, owner, fragment.Node, fragment.Rect, fragment.Border, htmlShape(fragment.Style, fragment.Rect.Width, fragment.Rect.Height), controlStack, visible)
		if entity == 0 {
			continue
		}
		markHTMLChildNode(world, entity, fragment.Node)
		state.Children = append(state.Children, entity)
		htmlSpawnControlVisuals(world, owner, state, entity, fragment.Node, controlStack)
		if isHTMLCheckboxNode(fragment.Node) && htmlUsesNativeAppearance(fragment.Style) {
			if mark := spawnHTMLCheckboxMark(world, owner, entity, fragment.Rect, fragment.Style, controlStack+1, visible); mark != 0 {
				state.Children = append(state.Children, mark)
			}
		}
		if fragment.Node != nil && fragment.Node.Data == "select" && htmlUsesNativeAppearance(fragment.Style) {
			if indicator := spawnHTMLSelectIndicator(world, owner, entity, fragment.Rect, fragment.Style, controlStack+2, passThrough, visible); indicator != 0 {
				state.Children = append(state.Children, indicator)
			}
		}
		if isHTMLEditableControlNode(fragment.Node) {
			textFont := htmlFontForControl(world, entity, fontValue, boldFont, fragment.Style)
			var textFace *font.Face
			if textFont != nil {
				textFace = textFont.Face
			}
			textFragment := htmlEditableControlTextFragment(world, entity, fragment.Node, fragment.Rect, fragment.Content, fragment.Style, textFace)
			spawnHTMLTextVisuals(world, owner, state, entity, entity, 0, fragment.Rect, textFragment, fontValue, boldFont, dpi, controlStack+2, passThrough, visible)
		} else if !isHTMLCheckboxNode(fragment.Node) {
			for _, textFragment := range fragment.TextFragments {
				spawnHTMLTextVisuals(world, owner, state, entity, entity, 0, fragment.Rect, textFragment, fontValue, boldFont, dpi, controlStack+1, passThrough, visible)
			}
		}
		for _, imageFragment := range fragment.ImageFragments {
			imageEntity, ok := spawnHTMLImage(world, owner, imageFragment, controlStack+1, visible)
			if !ok {
				continue
			}
			attachHTMLImageToControl(world, imageEntity, entity, fragment.Rect)
			state.Children = append(state.Children, imageEntity)
		}
	}
	if suppressPaint || flattened {
		if focusable, ok := htmlFocusableFromNode(owner, box.Node); ok {
			focusOwner = ensureHTMLFocusable(world, owner, state, box.Node, box.BorderBox(), focusable, baseStack, passThrough, visible)
		} else if entity := spawnHTMLNodeInputTarget(world, owner, box.Node, box.BorderBox(), baseStack, passThrough, visible, "composite"); entity != 0 {
			state.Children = append(state.Children, entity)
		}
		var memo htmlFocusableMemo
		for _, fragment := range box.TextFragments {
			fragmentFocusOwner := ensureHTMLFragmentFocusable(world, owner, state, &memo, fragment.Node, fragment.Rect, focusOwner, baseStack, passThrough, visible)
			if entity := spawnHTMLNodeInputTarget(world, owner, fragment.Node, fragment.Rect, baseStack+1, passThrough, visible, "composite"); entity != 0 {
				linkHTMLFocusOwner(world, entity, fragmentFocusOwner)
				state.Children = append(state.Children, entity)
			}
		}
	}
	rendered.Control, rendered.FocusOwner, rendered.ControlRect = control, focusOwner, controlRect
	bindHTMLBoxNodeEvents(world, owner, state, box, baseStack, passThrough, visible)
	state.EmittingBox = nil
	if state.LocalReconcile {
		return
	}
	clear(rendered.Entities)
	rendered.Entities = append(rendered.Entities[:0], state.Children[entityStart:]...)
	rendered.PaintEnd = baseStack + 4
	for _, child := range box.Children {
		htmlSpawnBoxesPaint(world, owner, state, child, fontValue, boldFont, dpi, stackIndex, passThrough, visible, control, focusOwner, controlRect, suppressPaint || flattened, window)
		rendered.PaintEnd = max(rendered.PaintEnd, state.BoxRenderings[child.Node].PaintEnd)
	}
	if htmlOverflowScrollsAxis(box.Style.OverflowX) || htmlOverflowScrollsAxis(box.Style.OverflowY) {
		rendered.PaintEnd += 2
	}
}

// bindHTMLBoxNodeEvents installs direct DOM-node callbacks on the generated
// input entity for one laid-out box. Nodes without a painted surface get
// a transparent target so their layout box remains pointer-addressable.
func bindHTMLBoxNodeEvents(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, stackIndex int, passThrough, visible bool) {
	if state == nil || box == nil || box.Node == nil || len(state.NodeEvents) == 0 {
		return
	}
	if _, bound := state.NodeEvents[box.Node]; !bound {
		return
	}
	rect := box.BorderBox()
	if rect.Width <= 0 || rect.Height <= 0 {
		return
	}
	entity := htmlNodeInputEntity(world, state, box.Node)
	if entity == 0 {
		// Keep the transparent target below descendant controls while
		// remaining above the node's own background surface.
		entity = spawnHTMLNodeEventTarget(world, owner, box.Node, rect, stackIndex-1, passThrough, visible)
		if entity != 0 {
			state.Children = append(state.Children, entity)
			if state.EventTargets == nil {
				state.EventTargets = make(map[*dom.Node]ecs.Entity)
			}
			state.EventTargets[box.Node] = entity
		}
	}
	if entity != 0 {
		installHTMLNodeEventHandler(world, owner, box.Node, entity)
	}
}

// bindHTMLNodeEvents rebinds DOM-node callbacks across a retained box tree
// after an event-only revision change. Full rebuilds bind inline while
// spawning boxes.
func bindHTMLNodeEvents(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, passThrough, visible bool, stackIndex int) {
	if world == nil || state == nil || box == nil || len(state.NodeEvents) == 0 {
		return
	}
	bindHTMLBoxNodeEvents(world, owner, state, box, htmlBoxStackIndex(box, stackIndex), passThrough, visible)
	for _, child := range box.Children {
		bindHTMLNodeEvents(world, owner, state, child, passThrough, visible, stackIndex)
	}
}

func htmlNodeInputEntity(world *ecs.World, state *htmlRenderState, node *dom.Node) ecs.Entity {
	if world == nil || state == nil || node == nil {
		return 0
	}
	if state.EventTargets != nil {
		if entity := state.EventTargets[node]; entity != 0 {
			if world.IsAlive(entity) && ecs.Has[scene.InputTarget2D](world, entity) {
				return entity
			}
			delete(state.EventTargets, node)
		}
	}
	rendered := state.RenderNodes[node]
	if rendered == nil || state.Reconciling && rendered.Generation != state.RenderGeneration {
		return 0
	}
	for _, entity := range rendered.Entities {
		child, ok := ecs.Get[htmlChild](world, entity)
		if !ok || child.Node != node || !ecs.Has[scene.InputTarget2D](world, entity) {
			continue
		}
		if state.EventTargets == nil {
			state.EventTargets = make(map[*dom.Node]ecs.Entity)
		}
		state.EventTargets[node] = entity
		return entity
	}
	return 0
}

func spawnHTMLNodeEventTarget(world *ecs.World, owner ecs.Entity, node *dom.Node, rect layout.Rect, stackIndex int, passThrough, visible bool) ecs.Entity {
	entity := spawnHTMLNodeInputTarget(world, owner, node, rect, stackIndex, passThrough, visible, "")
	if entity != 0 {
		_ = ecs.Insert(world, entity, htmlNodeEventTarget{})
	}
	return entity
}

func spawnHTMLNodeInputTarget(world *ecs.World, owner ecs.Entity, node *dom.Node, rect layout.Rect, stackIndex int, passThrough, visible bool, variant string) ecs.Entity {
	if world == nil || owner == 0 || node == nil || rect.Width <= 0 || rect.Height <= 0 {
		return 0
	}
	width, height := float32(rect.Width), float32(rect.Height)
	bundle := beginHTMLEntity(world, owner, node, htmlRoleEvent, variant, 0, 11)
	putHTML(&bundle, scene.UITransformFromXY(float32(rect.X), float32(rect.Y)))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: owner})
	putHTML(&bundle, scene.Size2D{Width: width, Height: height})
	putHTML(&bundle, scene.InputTarget2D{Width: width, Height: height, PassThrough: passThrough})
	putHTML(&bundle, scene.InputState2D{})
	putHTML(&bundle, htmlChild{Owner: owner, Node: node})
	entity := bundle.finish()
	markHTMLChildNode(world, entity, node)
	return entity
}

func installHTMLNodeEventHandler(world *ecs.World, owner ecs.Entity, node *dom.Node, entity ecs.Entity) {
	if world == nil || owner == 0 || node == nil || entity == 0 {
		return
	}
	if _, wrapped := ecs.Get[htmlNodeEventHandler](world, entity); wrapped {
		return
	}
	var previous scene.InputHandler2D
	if current, ok := ecs.Get[scene.InputHandler2D](world, entity); ok {
		previous = current
	}
	_ = ecs.Insert(world, entity, htmlNodeEventHandler{Base: previous})
	handler := scene.InputHandler2D{Handle: func(context *scene.InputEventContext) {
		if previous.Handle != nil {
			previous.Handle(context)
		}
		dispatchHTMLNodePointer(world, owner, node, context)
	}}
	if current := ecs.GetMut[scene.InputHandler2D](world, entity); current != nil {
		*current = handler
		return
	}
	_ = ecs.Insert(world, entity, handler)
}

func dispatchHTMLNodePointer(world *ecs.World, owner ecs.Entity, node *dom.Node, context *scene.InputEventContext) {
	if world == nil || owner == 0 || node == nil || context == nil || context.Event == nil {
		return
	}
	state := ecs.GetRef[htmlRenderState](world, owner)
	ok := state != nil
	if !ok || state == nil {
		return
	}
	binding, ok := state.NodeEvents[node]
	if !ok {
		return
	}
	arena := ecs.FrameValue[htmlPointerEventArenaTag, htmlPointerEventArena](world)
	index := len(arena.Events)
	arena.Events = append(arena.Events, PointerEvent{
		Target: Node{world: world, owner: owner, node: node},
		Kind:   context.Event.Kind, X: context.Event.X, Y: context.Event.Y,
		DeltaX: context.Event.DeltaX, DeltaY: context.Event.DeltaY,
		WheelX: context.Event.WheelX, WheelY: context.Event.WheelY,
		Handled: context.Handled,
	})
	arena.Depth++
	event := &arena.Events[index]
	if binding.Pointer != nil {
		binding.Pointer(event)
	}
	if event.Kind == scene.InputEventTap && binding.Click != nil {
		binding.Click(event)
	}
	if event.Handled {
		context.Consume()
	}
	arena.Depth--
	if arena.Depth == 0 {
		clear(arena.Events)
		arena.Events = arena.Events[:0]
	}
	if context.Event.Kind == scene.InputEventTap && !context.Handled {
		for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
			if _, bound := state.NodeEvents[ancestor]; !bound {
				continue
			}
			dispatchHTMLNodePointer(world, owner, ancestor, context)
			if context.Handled {
				break
			}
		}
	}
}

func linkHTMLFocusOwner(world *ecs.World, entity, focusOwner ecs.Entity) {
	if world == nil || entity == 0 || focusOwner == 0 || entity == focusOwner {
		return
	}
	_ = ecs.Insert(world, entity, FocusOwner{Entity: focusOwner})
}

func spawnHTMLFocusable(world *ecs.World, owner ecs.Entity, node *dom.Node, rect layout.Rect, focusable Focusable, stackIndex int, passThrough, visible bool) ecs.Entity {
	if world == nil || owner == 0 || node == nil || rect.Width <= 0 || rect.Height <= 0 {
		return 0
	}
	width, height := float32(rect.Width), float32(rect.Height)
	editingHost := dom.IsEditingHost(node)
	variant := ""
	if editingHost {
		variant = "editable"
	}
	bundle := beginHTMLEntity(world, owner, node, htmlRoleFocusable, variant, 0, 13)
	putHTML(&bundle, scene.UITransformFromXY(float32(rect.X), float32(rect.Y)))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: owner})
	putHTML(&bundle, scene.Size2D{Width: width, Height: height})
	putHTML(&bundle, scene.InputTarget2D{Width: width, Height: height, PassThrough: passThrough})
	putHTML(&bundle, scene.InputState2D{})
	putHTML(&bundle, focusable)
	if editingHost {
		value := htmlNodeText(node)
		length := utf8.RuneCountInString(value)
		putHTML(&bundle, Editable{RenderRevision: bundle.state.RenderGeneration, Owner: owner, ID: focusable.ID, Value: value, Anchor: length, Focus: length})
		putHTML(&bundle, htmlEditableNodeRef{Node: node})
	}
	putHTML(&bundle, htmlChild{Owner: owner, Node: node})
	entity := bundle.finish()
	markHTMLChildNode(world, entity, node)
	return entity
}

// htmlFocusableAncestorNode walks a fragment node's ancestor chain for the
// nearest focusable element, stopping at control boundaries.
func htmlFocusableAncestorNode(owner ecs.Entity, node *dom.Node) (*dom.Node, Focusable, bool) {
	for current := node; current != nil; current = current.Parent {
		if isHTMLControlNode(current) {
			return nil, Focusable{}, false
		}
		if focusable, ok := htmlFocusableFromNode(owner, current); ok {
			return current, focusable, true
		}
	}
	return nil, Focusable{}, false
}

// htmlFocusableMemo caches the focusable ancestor resolution so fragments of
// one layout box share a single ancestor walk.
type htmlFocusableMemo struct {
	node      *dom.Node
	focusNode *dom.Node
	focusable Focusable
	found     bool
}

// ensureHTMLFragmentFocusable resolves the focusable ancestor once per
// fragment node and unions every fragment rect into that focus target.
func ensureHTMLFragmentFocusable(world *ecs.World, owner ecs.Entity, state *htmlRenderState, memo *htmlFocusableMemo, node *dom.Node, rect layout.Rect, fallback ecs.Entity, stackIndex int, passThrough, visible bool) ecs.Entity {
	if memo.node != node {
		memo.node = node
		memo.focusNode, memo.focusable, memo.found = htmlFocusableAncestorNode(owner, node)
	}
	if !memo.found {
		return fallback
	}
	if !htmlStyleVisible(state.Styles[memo.focusNode]) {
		return 0
	}
	return ensureHTMLFocusable(world, owner, state, memo.focusNode, rect, memo.focusable, stackIndex, passThrough, visible)
}

func ensureHTMLFocusable(world *ecs.World, owner ecs.Entity, state *htmlRenderState, node *dom.Node, rect layout.Rect, focusable Focusable, stackIndex int, passThrough, visible bool) ecs.Entity {
	if state == nil || node == nil {
		return 0
	}
	index, exists := state.FocusByNode[node]
	if exists && state.FocusTargets[index].Generation == state.RenderGeneration {
		binding := &state.FocusTargets[index]
		binding.Rect = unionHTMLRect(binding.Rect, rect)
		updateHTMLFocusableBounds(world, binding.Entity, binding.Rect)
		return binding.Entity
	}
	entity := spawnHTMLFocusable(world, owner, node, rect, focusable, stackIndex, passThrough, visible)
	if entity != 0 {
		if state.FocusByNode == nil {
			state.FocusByNode = make(map[*dom.Node]int)
		}
		binding := htmlFocusTargetBinding{Node: node, Entity: entity, Rect: rect, Generation: state.RenderGeneration}
		if exists {
			state.FocusTargets[index] = binding
		} else {
			state.FocusByNode[node] = len(state.FocusTargets)
			state.FocusTargets = append(state.FocusTargets, binding)
		}
		state.Children = append(state.Children, entity)
		if dom.IsEditingHost(node) {
			htmlSpawnEditableVisuals(world, owner, state, entity, node, stackIndex)
		}
	}
	return entity
}

func unionHTMLRect(left, right layout.Rect) layout.Rect {
	if left.Width <= 0 || left.Height <= 0 {
		return right
	}
	if right.Width <= 0 || right.Height <= 0 {
		return left
	}
	x, y := min(left.X, right.X), min(left.Y, right.Y)
	maxX := max(left.X+left.Width, right.X+right.Width)
	maxY := max(left.Y+left.Height, right.Y+right.Height)
	return layout.Rect{X: x, Y: y, Width: maxX - x, Height: maxY - y}
}

func updateHTMLFocusableBounds(world *ecs.World, entity ecs.Entity, rect layout.Rect) {
	if transform := ecs.GetMut[scene.UITransform](world, entity); transform != nil {
		transform.Translation.X.Value = float32(rect.X)
		transform.Translation.Y.Value = float32(rect.Y)
	}
	if size := ecs.GetMut[scene.Size2D](world, entity); size != nil {
		size.Width, size.Height = float32(rect.Width), float32(rect.Height)
	}
	if target := ecs.GetMut[scene.InputTarget2D](world, entity); target != nil {
		target.Width, target.Height = float32(rect.Width), float32(rect.Height)
	}
}

func htmlSelectIndicatorSize(height float32) float32 {
	texture := defaultHTMLSelectIcon()
	if texture == nil || !texture.Valid() {
		return 0
	}
	naturalWidth, naturalHeight := float32(texture.LogicalWidth()), float32(texture.LogicalHeight())
	if naturalWidth <= 0 || naturalHeight <= 0 {
		return 0
	}
	iconSize := min(height-8, min(naturalWidth, naturalHeight))
	if iconSize <= 0 {
		iconSize = min(height, min(naturalWidth, naturalHeight))
	}
	return max(iconSize, 0)
}

func spawnHTMLSelectIndicator(world *ecs.World, owner, control ecs.Entity, rect layout.Rect, computed *style.ComputedStyle, stackIndex int, passThrough, visible bool) ecs.Entity {
	if rect.Width <= 0 || rect.Height <= 0 {
		return 0
	}
	iconSize := htmlSelectIndicatorSize(float32(rect.Height))
	if iconSize <= 0 {
		return 0
	}
	width, height := iconSize, iconSize
	x := float32(rect.Width) - width - 8
	if x < 0 {
		x = 0
	}
	y := (float32(rect.Height) - height) * 0.5
	image := scene.NewUIImage2D(defaultHTMLSelectIcon())
	image.Tint = htmlColor(computed.Color)
	bundle := beginHTMLEntity(world, owner, htmlEntityNode(world, control), htmlRoleIndicator, "", control, 11)
	putHTML(&bundle, scene.UITransformFromXY(x, y))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: control})
	putHTML(&bundle, image)
	putHTML(&bundle, scene.Size2D{Width: width, Height: height})
	putHTML(&bundle, scene.InputBlocker2D{Width: width, Height: height, Receiver: control, PassThrough: passThrough})
	putHTML(&bundle, htmlSelectIndicator{Control: control})
	putHTML(&bundle, htmlChild{Owner: owner})
	entity := bundle.finish()
	if child, ok := ecs.Get[htmlChild](world, control); ok {
		markHTMLChildNode(world, entity, child.Node)
	}
	return entity
}

func spawnHTMLCheckboxMark(world *ecs.World, owner, control ecs.Entity, rect layout.Rect, computed *style.ComputedStyle, stackIndex int, visible bool) ecs.Entity {
	if world == nil || owner == 0 || control == 0 || rect.Width <= 0 || rect.Height <= 0 {
		return 0
	}
	width, height := float32(rect.Width), float32(rect.Height)
	markSize := min(width, height) - 6
	if markSize < 10 {
		markSize = min(width, height)
	}
	if markSize <= 0 {
		return 0
	}
	nodeChecked := false
	if node, ok := ecs.Get[htmlControlNodeRef](world, control); ok && node.Node != nil {
		nodeChecked = dom.IsChecked(node.Node)
	}
	sprite := scene.NewSprite2D()
	sprite.Shape = scene.CheckmarkShape2D(htmlColor(computed.Color))
	sprite.Width, sprite.Height = markSize, markSize
	bundle := beginHTMLEntity(world, owner, htmlEntityNode(world, control), htmlRoleCheckbox, "", control, 11)
	putHTML(&bundle, scene.UITransformFromXY((width-markSize)*0.5, (height-markSize)*0.5))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible && nodeChecked})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: control})
	putHTML(&bundle, sprite)
	putHTML(&bundle, scene.Size2D{Width: markSize, Height: markSize})
	putHTML(&bundle, htmlCheckboxMark{Control: control})
	putHTML(&bundle, htmlChild{Owner: owner})
	return bundle.finish()
}

func attachHTMLImageToControl(world *ecs.World, image, control ecs.Entity, controlRect layout.Rect) {
	if world == nil || image == 0 || control == 0 {
		return
	}
	child, _ := ecs.Get[htmlChild](world, control)
	if state := ecs.GetRef[htmlRenderState](world, child.Owner); state != nil && !htmlStyleVisible(state.Styles[child.Node]) {
		return
	}
	if parent := ecs.GetMut[scene.Parent](world, image); parent != nil {
		parent.Entity = control
	}
	if transform := ecs.GetMut[scene.UITransform](world, image); transform != nil {
		transform.Translation.X.Value -= float32(controlRect.X)
		transform.Translation.Y.Value -= float32(controlRect.Y)
	}
	routeHTMLImageInput(world, image, control)
}

func routeHTMLImageInput(world *ecs.World, image, receiver ecs.Entity) {
	if target := ecs.GetMut[scene.InputTarget2D](world, image); target != nil {
		target.Receiver = receiver
	}
}

func htmlEditableControlTextFragment(world *ecs.World, control ecs.Entity, node *dom.Node, controlRect, content layout.Rect, computed *style.ComputedStyle, face *font.Face) layout.TextFragment {
	if node != nil && node.Data == "input" {
		// Native single-line inputs center their text in the border box. The
		// content box origin includes CSS padding, so centering within it leaves
		// a fractional baseline drift when the padding is asymmetric.
		lineHeight := layout.UsedLineHeight(computed, face)
		if lineHeight > 0 && controlRect.Height > 0 {
			content.Y = controlRect.Y + (controlRect.Height-lineHeight)*0.5
		}
	}
	fragment := layout.TextFragment{Node: node, Rect: content, Style: computed}
	value, ok := ecs.Get[Control](world, control)
	if !ok {
		return fragment
	}
	if value.Value != "" {
		fragment.Text = value.Value
		if strings.EqualFold(strings.TrimSpace(value.Type), "password") {
			fragment.Text = strings.Repeat("•", utf8.RuneCountInString(value.Value))
		}
		return fragment
	}
	if value.Placeholder != "" {
		fragment.Text = value.Placeholder
		placeholderStyle := *computed
		placeholderStyle.Color = placeholderStyle.PlaceholderColor
		fragment.Style = &placeholderStyle
	}
	return fragment
}

func htmlBoxDepth(box *layout.Box) int {
	depth := 0
	for box != nil {
		depth++
		box = box.Parent
	}
	return depth
}

const (
	htmlStackZStride      = 1_000_000_000
	htmlStackNestedZLimit = 1_000_000
	htmlStackOrderLimit   = 1_000_000
	htmlStackOrderStride  = htmlStackNestedZLimit*2 + 1
)

// htmlBoxStackIndex maps the DOM paint order onto the flat UI stack. The
// previous depth-only scheme let deeply nested auto-stacked content cover a
// later sibling and could not represent negative z-index values. The outermost
// numeric z-index establishes the stacking context; nested numeric values only
// order content inside that context and never escape it.
func htmlBoxStackIndex(box *layout.Box, stackIndex int) int {
	if box == nil {
		return stackIndex
	}
	contextZ, contextOrder, nestedZ := 0, 0, 0
	// An auto-positioned ancestor groups its ordinary descendants in the
	// zero-level paint phase; positioned descendants select their own DOM slot.
	positioned := 0
	hasContext := false
	var path [64]*layout.Box
	pathLen := 0
	for current := box; current != nil; current = current.Parent {
		if pathLen == len(path) {
			// HTML documents are normally shallow. Preserve correctness for an
			// unusually deep tree with a temporary spill only on that slow path.
			spill := make([]*layout.Box, 0, pathLen+1)
			spill = append(spill, path[:pathLen]...)
			for current != nil {
				spill = append(spill, current)
				current = current.Parent
			}
			pathLen = len(spill)
			for index := pathLen - 1; index >= 0; index-- {
				current := spill[index]
				if current.Style != nil && current.Style.Position.Keyword != "static" {
					positioned = htmlStackOrder(current.Node)
				}
				if !htmlBoxCreatesStackContext(current) {
					continue
				}
				z := int(max(min(current.Style.ZIndex.Number, float64(htmlStackZStride/2)), -float64(htmlStackZStride/2)))
				if !hasContext {
					contextZ, contextOrder, hasContext = z, htmlStackOrder(current.Node), true
					continue
				}
				nestedZ = max(min(nestedZ+z, htmlStackNestedZLimit), -htmlStackNestedZLimit)
			}
			if !hasContext {
				contextOrder = positioned
			}
			return stackIndex + contextZ*htmlStackZStride + contextOrder*htmlStackOrderStride + nestedZ + htmlStackOrder(box.Node)
		}
		path[pathLen] = current
		pathLen++
	}
	for index := pathLen - 1; index >= 0; index-- {
		current := path[index]
		if current.Style != nil && current.Style.Position.Keyword != "static" {
			positioned = htmlStackOrder(current.Node)
		}
		if !htmlBoxCreatesStackContext(current) {
			continue
		}
		z := int(max(min(current.Style.ZIndex.Number, float64(htmlStackZStride/2)), -float64(htmlStackZStride/2)))
		if !hasContext {
			contextZ, contextOrder = z, htmlStackOrder(current.Node)
			hasContext = true
			continue
		}
		nestedZ += z
		nestedZ = max(min(nestedZ, htmlStackNestedZLimit), -htmlStackNestedZLimit)
	}
	if !hasContext {
		contextOrder = positioned
	}
	return stackIndex + contextZ*htmlStackZStride + contextOrder*htmlStackOrderStride + nestedZ + htmlStackOrder(box.Node)
}

func htmlStackOrder(node *dom.Node) int {
	if node == nil {
		return 0
	}
	return min(int(node.Order), htmlStackOrderLimit)
}

func htmlOverflowClipsStyle(computed *style.ComputedStyle) bool {
	if computed == nil {
		return false
	}
	return htmlOverflowClipsAxis(computed.OverflowX) || htmlOverflowClipsAxis(computed.OverflowY)
}

func htmlBoxCreatesStackContext(box *layout.Box) bool {
	if box == nil || box.Style == nil || box.Style.ZIndex.Kind != style.ValueNumber {
		return false
	}
	if box.Style.Position.Kind == style.ValueKeyword && box.Style.Position.Keyword != "static" {
		return true
	}
	if box.Parent != nil && box.Parent.Style != nil && box.Parent.Style.Display.Kind == style.ValueKeyword {
		switch box.Parent.Style.Display.Keyword {
		case "flex", "inline-flex":
			return true
		}
	}
	return false
}

func htmlOverflowClipsAxis(value style.Value) bool {
	if value.Kind != style.ValueKeyword {
		return false
	}
	switch value.Keyword {
	case "hidden", "clip", "scroll", "auto":
		return true
	default:
		return false
	}
}

func htmlOverflowScrollsAxis(value style.Value) bool {
	return value.Kind == style.ValueKeyword && (value.Keyword == "auto" || value.Keyword == "scroll")
}

type htmlOverflowExtent struct {
	width  float64
	height float64
}

// A clipped box keeps its internal overflow for its own scrolling, but only
// contributes its border box to an ancestor's scrollable area on clipped axes.
func htmlOverflowChildExtent(child *layout.Box, extent htmlOverflowExtent) htmlOverflowExtent {
	r := child.BorderBox()
	if child.Style != nil && htmlOverflowClipsAxis(child.Style.OverflowX) {
		extent.width = r.Width
	} else {
		extent.width = max(r.Width, child.Border.Left+extent.width)
	}
	if child.Style != nil && htmlOverflowClipsAxis(child.Style.OverflowY) {
		extent.height = r.Height
	} else {
		extent.height = max(r.Height, child.Border.Top+extent.height)
	}
	return extent
}

// htmlOverflowContentExtents computes every box's descendant extent in one
// post-order walk. Nested overflow containers then perform O(1) lookups rather
// than rescanning their complete subtree.
func htmlOverflowContentExtents(root *layout.Box, extents map[*layout.Box]htmlOverflowExtent) {
	if root == nil || extents == nil {
		return
	}
	var visit func(*layout.Box)
	visit = func(box *layout.Box) {
		if box == nil {
			return
		}
		r := box.PaddingBox()
		extent := htmlOverflowExtent{width: r.Width, height: r.Height}
		for _, child := range box.Children {
			visit(child)
			childExtent := htmlOverflowChildExtent(child, extents[child])
			childRect := child.BorderBox()
			extent.width = max(extent.width, childRect.X-r.X+childExtent.width)
			extent.height = max(extent.height, childRect.Y-r.Y+childExtent.height)
		}
		extents[box] = extent
	}
	visit(root)
}

type htmlOverflowClipBinding struct {
	node   *dom.Node
	entity ecs.Entity
	rect   layout.Rect
	parts  []ecs.Entity
}

// applyHTMLOverflowClips creates a lightweight parent for each CSS overflow
// box and reparents only the top-level generated visuals. This keeps the DOM
// layout coordinates intact while making ClipRect2D intersect through the
// normal UI transform hierarchy.
func applyHTMLOverflowClips(world *ecs.World, owner ecs.Entity, state *htmlRenderState, root *layout.Box, stackIndex int, visible bool) {
	if world == nil || state == nil || root == nil {
		return
	}
	if state.OverflowBindings == nil {
		state.OverflowBindings = make(map[*dom.Node]htmlOverflowClipBinding)
	}
	if state.OverflowEntities == nil {
		state.OverflowEntities = make(map[ecs.Entity]htmlOverflowClipBinding)
	}
	if state.OverflowRoutes == nil {
		state.OverflowRoutes = make(map[*dom.Node]htmlOverflowClipBinding)
	}
	if state.OverflowExtents == nil {
		state.OverflowExtents = make(map[*layout.Box]htmlOverflowExtent)
	}
	bindings := state.OverflowBindings
	entities := state.OverflowEntities
	routes := state.OverflowRoutes
	clear(bindings)
	clear(entities)
	clear(routes)
	clear(state.OverflowExtents)
	htmlOverflowContentExtents(root, state.OverflowExtents)
	state.Children = state.Children[:0]
	state.OwnerSpansValid = true
	var build func(*layout.Box, htmlOverflowClipBinding)
	build = func(box *layout.Box, parent htmlOverflowClipBinding) {
		if box == nil {
			return
		}
		rendered := state.BoxRenderings[box.Node]
		beginHTMLOwnerSpan(state, rendered)
		state.Children = append(state.Children, rendered.Entities...)
		if box.Node != nil && parent.entity != 0 {
			routes[box.Node] = parent
		}
		current := parent
		if htmlBoxNeedsOverflowClip(state, box) {
			state.EmittingBox = state.BoxRenderings[box.Node]
			binding := emitHTMLOverflowClip(world, owner, state, box, parent, stackIndex, visible)
			state.EmittingBox.Clip = binding.entity
			state.EmittingBox = nil
			state.Children = append(state.Children, binding.entity)
			state.Children = append(state.Children, binding.parts...)
			current = binding
		}
		if isHTMLControlNode(box.Node) {
			setHTMLOverflowRoute(state, box.Node, current)
		}
		for _, fragment := range box.TextFragments {
			if fragment.Node != nil && current.entity != 0 {
				routes[fragment.Node] = current
			}
		}
		for _, fragment := range box.ImageFragments {
			if fragment.Node != nil && current.entity != 0 {
				routes[fragment.Node] = current
			}
		}
		for _, fragment := range box.ControlFragments {
			if fragment.Node != nil && current.entity != 0 {
				routes[fragment.Node] = current
			}
		}
		for _, child := range box.Children {
			build(child, current)
		}
		rendered.OwnerEnd = len(state.Children)
	}
	build(root, htmlOverflowClipBinding{})
	if len(bindings) == 0 {
		return
	}
	routeHTMLOverflowEntities(world, owner, state, state.Children)
}

func emitHTMLOverflowClip(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, parent htmlOverflowClipBinding, stackIndex int, visible bool) htmlOverflowClipBinding {
	rect := box.PaddingBox()
	clipX, clipY := float32(0), float32(0)
	clipWidth, clipHeight := float32(rect.Width), float32(rect.Height)
	if !htmlOverflowClipsAxis(box.Style.OverflowX) {
		clipX = -htmlOverflowVisibleExtent
		clipWidth += htmlOverflowVisibleExtent * 2
	}
	if !htmlOverflowClipsAxis(box.Style.OverflowY) {
		clipY = -htmlOverflowVisibleExtent
		clipHeight += htmlOverflowVisibleExtent * 2
	}
	parentEntity := owner
	origin := layout.Rect{}
	if parent.entity != 0 {
		parentEntity, origin = parent.entity, parent.rect
	}
	bundle := beginHTMLEntity(world, owner, box.Node, htmlRoleOverflow, "", 0, 10)
	putHTML(&bundle, scene.UITransformFromXY(float32(rect.X-origin.X), float32(rect.Y-origin.Y)))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(htmlBoxStackIndex(box, stackIndex)-1))
	putHTML(&bundle, scene.Parent{Entity: parentEntity})
	showHorizontal, showVertical := false, false
	barSize := float32(0)
	if htmlOverflowScrollsAxis(box.Style.OverflowX) || htmlOverflowScrollsAxis(box.Style.OverflowY) {
		extent := state.OverflowExtents[box]
		contentWidth, contentHeight := max(rect.Width, extent.width), max(rect.Height, extent.height)
		barSize = htmlOverflowScrollbarSize(box.Style)
		// HTML uses overlay scrollbars: the scrollport and layout retain the
		// full padding box, with the bars painted inside its edges.
		showHorizontal, showVertical = htmlOverflowScrollbarVisibility(box.Style, rect.Width, rect.Height, contentWidth, contentHeight)
		if !htmlOverflowClipsAxis(box.Style.OverflowX) {
			clipX = -htmlOverflowVisibleExtent
			clipWidth += htmlOverflowVisibleExtent * 2
		}
		if !htmlOverflowClipsAxis(box.Style.OverflowY) {
			clipY = -htmlOverflowVisibleExtent
			clipHeight += htmlOverflowVisibleExtent * 2
		}
	}
	putHTML(&bundle, scene.Size2D{Width: float32(rect.Width), Height: float32(rect.Height)})
	putHTML(&bundle, scene.ClipRect2D{X: clipX, Y: clipY, Width: clipWidth, Height: clipHeight})
	if htmlOverflowClipsAxis(box.Style.OverflowX) && htmlOverflowClipsAxis(box.Style.OverflowY) {
		radius := htmlOverflowClipRadius(box.Style, rect.Width, rect.Height)
		if radius > 0 {
			maskColor := graphics.ColorRGBA{}
			if htmlStyleVisible(box.Style) {
				maskColor = htmlColor(box.Style.BackgroundColor)
			}
			putHTML(&bundle, scene.RoundedClip2D{X: clipX, Y: clipY, Width: clipWidth, Height: clipHeight, Radius: radius, MaskColor: maskColor})
		}
	}
	if htmlOverflowScrollsAxis(box.Style.OverflowX) || htmlOverflowScrollsAxis(box.Style.OverflowY) {
		extent := state.OverflowExtents[box]
		contentWidth, contentHeight := max(rect.Width, extent.width), max(rect.Height, extent.height)
		putHTML(&bundle, scene.UIScroll{
			ViewportSize:          mgl32.Vec2{clipWidth, clipHeight},
			ContentSize:           mgl32.Vec2{float32(contentWidth), float32(contentHeight)},
			WheelStep:             48,
			ScrollbarSize:         barSize,
			ScrollbarMinThumbSize: htmlOverflowScrollbarMinThumbSize(box.Style),
			ScrollbarThin:         box.Style.ScrollbarWidth.Kind == style.ValueKeyword && box.Style.ScrollbarWidth.Keyword == "thin",
			ScrollbarOverlay:      true,
			Horizontal:            showHorizontal,
			Vertical:              showVertical,
			ConsumeWheel:          true,
			DisableInput:          !htmlStyleVisible(box.Style),
		})
	}
	putHTML(&bundle, htmlOverflowClip{})
	putHTML(&bundle, htmlChild{Owner: owner, Node: box.Node})
	entity := bundle.finish()
	binding := htmlOverflowClipBinding{node: box.Node, entity: entity, rect: rect}
	if showHorizontal || showVertical {
		style := htmlOverflowScrollbarColors(box.Style)
		barStack := state.BoxRenderings[box.Node].PaintEnd - 1
		trackWidth, trackHeight := clipWidth, clipHeight
		if showVertical {
			trackWidth = max(trackWidth-barSize, 0)
		}
		if showHorizontal {
			trackHeight = max(trackHeight-barSize, 0)
		}
		if showHorizontal {
			binding.parts = appendHTMLScrollbarParts(binding.parts,
				emitHTMLScrollbarPart(world, owner, box.Node, entity, htmlRoleScrollbarTrack, ScrollbarHorizontal, ScrollbarTrack, 0, max(clipHeight-barSize, 0), trackWidth, barSize, style.TrackColor, barStack, visible),
				emitHTMLScrollbarPart(world, owner, box.Node, entity, htmlRoleScrollbarThumb, ScrollbarHorizontal, ScrollbarThumb, 0, max(clipHeight-barSize, 0), trackWidth, barSize, style.ThumbColor, barStack+1, visible),
			)
		}
		if showVertical {
			binding.parts = appendHTMLScrollbarParts(binding.parts,
				emitHTMLScrollbarPart(world, owner, box.Node, entity, htmlRoleScrollbarTrack, ScrollbarVertical, ScrollbarTrack, max(clipWidth-barSize, 0), 0, trackHeight, barSize, style.TrackColor, barStack, visible),
				emitHTMLScrollbarPart(world, owner, box.Node, entity, htmlRoleScrollbarThumb, ScrollbarVertical, ScrollbarThumb, max(clipWidth-barSize, 0), 0, trackHeight, barSize, style.ThumbColor, barStack+1, visible),
			)
		}
	}
	state.OverflowBindings[box.Node], state.OverflowEntities[entity] = binding, binding
	state.SyncStats.UpdatedClips++
	return binding
}

func appendHTMLScrollbarParts(parts []ecs.Entity, first, second ecs.Entity) []ecs.Entity {
	var entities [2]ecs.Entity
	count := 0
	if first != 0 {
		entities[count] = first
		count++
	}
	if second != 0 {
		entities[count] = second
		count++
	}
	return append(parts, entities[:count]...)
}

type htmlScrollbarColors struct {
	TrackColor graphics.ColorRGBA
	ThumbColor graphics.ColorRGBA
}

func htmlOverflowScrollbarSize(computed *style.ComputedStyle) float32 {
	if computed != nil {
		switch computed.ScrollbarWidth.Kind {
		case style.ValueLengthPx:
			if computed.ScrollbarWidth.LengthPx > 0 {
				return float32(computed.ScrollbarWidth.LengthPx)
			}
		case style.ValueKeyword:
			switch computed.ScrollbarWidth.Keyword {
			case "thin":
				return 10
			case "none":
				return 0
			}
		}
	}
	return 15
}

func htmlOverflowClipRadius(computed *style.ComputedStyle, width, height float64) float32 {
	if computed == nil || width <= 0 || height <= 0 {
		return 0
	}
	radius := max(
		style.ResolveLength(computed.BorderRadius.TopLeft.Horizontal, width),
		style.ResolveLength(computed.BorderRadius.TopRight.Horizontal, width),
		style.ResolveLength(computed.BorderRadius.BottomRight.Horizontal, width),
		style.ResolveLength(computed.BorderRadius.BottomLeft.Horizontal, width),
	)
	return float32(max(min(radius, min(width, height)*0.5), 0))
}

func htmlOverflowScrollbarMinThumbSize(computed *style.ComputedStyle) float32 {
	if computed != nil && computed.ScrollbarWidth.Kind == style.ValueKeyword && computed.ScrollbarWidth.Keyword == "thin" {
		return 18
	}
	return 20
}

// htmlOverflowScrollbarVisibility reports which axes are scrollable. It is
// independent of bar width: scrollbar-width:none keeps wheel scrolling while
// painting is skipped because the bar occupies zero width.
func htmlOverflowScrollbarVisibility(computed *style.ComputedStyle, width, height, contentWidth, contentHeight float64) (horizontal, vertical bool) {
	if computed == nil {
		return false, false
	}
	canHorizontal := htmlOverflowScrollsAxis(computed.OverflowX)
	canVertical := htmlOverflowScrollsAxis(computed.OverflowY)
	// Determine each axis from its authored scroll range. Explicit dual-axis
	// scrolling still reserves both bars; an auto axis does not appear solely
	// because the other bar reduced the viewport.
	horizontal = canHorizontal && (htmlOverflowAlwaysShowsAxis(computed.OverflowX) || contentWidth > width)
	vertical = canVertical && (htmlOverflowAlwaysShowsAxis(computed.OverflowY) || contentHeight > height)
	return horizontal, vertical
}

func htmlOverflowAlwaysShowsAxis(value style.Value) bool {
	return value.Kind == style.ValueKeyword && value.Keyword == "scroll"
}

func htmlOverflowScrollbarColors(computed *style.ComputedStyle) htmlScrollbarColors {
	colors := htmlScrollbarColors{
		TrackColor: graphics.ColorFromRGB(252, 252, 252),
		ThumbColor: graphics.ColorFromRGB(139, 139, 139),
	}
	if computed == nil {
		return colors
	}
	if computed.ScrollbarTrackColor.Kind == style.ValueColor {
		colors.TrackColor = htmlColor(computed.ScrollbarTrackColor)
	}
	if computed.ScrollbarThumbColor.Kind == style.ValueColor {
		colors.ThumbColor = htmlColor(computed.ScrollbarThumbColor)
	}
	return colors
}

func emitHTMLScrollbarPart(world *ecs.World, owner ecs.Entity, node *dom.Node, area ecs.Entity, role htmlRenderRole, axis, kind uint8, x, y, trackLength, barSize float32, color graphics.ColorRGBA, stackIndex int, visible bool) ecs.Entity {
	if trackLength <= 0 || barSize <= 0 || color.A <= 0 {
		return 0
	}
	sprite := scene.NewSprite2D()
	sprite.Shape = scene.RoundedRectShape2D(color, 0)
	sprite.Width, sprite.Height = barSize, trackLength
	if axis == ScrollbarHorizontal {
		sprite.Width, sprite.Height = trackLength, barSize
	}
	bundle := beginHTMLEntity(world, owner, node, role, htmlScrollbarVariant(axis, kind), area, 12)
	putHTML(&bundle, scene.UITransformFromXY(x, y))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: area})
	putHTML(&bundle, sprite)
	putHTML(&bundle, scene.Size2D{Width: sprite.Width, Height: sprite.Height})
	putHTML(&bundle, ScrollbarPart{Area: area, Axis: axis, Kind: kind})
	putHTML(&bundle, htmlChild{Owner: owner, Node: node})
	entity := bundle.finish()
	return entity
}

func htmlScrollbarVariant(axis, kind uint8) string {
	if axis == ScrollbarHorizontal {
		if kind == ScrollbarThumb {
			return "horizontal-thumb"
		}
		return "horizontal-track"
	}
	if kind == ScrollbarThumb {
		return "vertical-thumb"
	}
	return "vertical-track"
}

// SyncScrollbars updates only retained thumb geometry after UIScroll input
// changes. The caller runs it after UIScrollSystem and before transforms sync.
func SyncScrollbars(world *ecs.World) {
	if world == nil {
		return
	}
	for entity, access := range ecs.Query4[ScrollbarPart, scene.UITransform, scene.Size2D, scene.Sprite2D](world).All {
		if !func(_ ecs.Entity, partAccess ecs.ComponentAccess[ScrollbarPart], transformAccess ecs.ComponentAccess[scene.UITransform], sizeAccess ecs.ComponentAccess[scene.Size2D], spriteAccess ecs.ComponentAccess[scene.Sprite2D]) bool {
			part := partAccess.Ref()
			nextTransform, nextSize, nextSprite := transformAccess.Get(), sizeAccess.Get(), spriteAccess.Get()
			transform, size, sprite := &nextTransform, &nextSize, &nextSprite
			defer func() {
				if nextTransform != transformAccess.Get() {
					*transformAccess.Mut() = nextTransform
				}
				if nextSize != sizeAccess.Get() {
					*sizeAccess.Mut() = nextSize
				}
				if nextSprite != spriteAccess.Get() {
					*spriteAccess.Mut() = nextSprite
				}
			}()
			scroll, ok := ecs.Get[scene.UIScroll](world, part.Area)
			if !ok {
				return true
			}
			viewport := scroll.ViewportSize
			if viewport == (mgl32.Vec2{}) {
				if clip, exists := ecs.Get[scene.ClipRect2D](world, part.Area); exists {
					viewport = mgl32.Vec2{clip.Width, clip.Height}
				}
			}
			bar := scroll.ScrollbarSize
			if bar <= 0 {
				return true
			}
			axis := int(part.Axis)
			track := scene.UIScrollTrackSize(&scroll, viewport)
			barX, barY := viewport.X(), viewport.Y()
			if scroll.ScrollbarOverlay {
				barX, barY = max(barX-bar, 0), max(barY-bar, 0)
			}
			visible := (axis == int(ScrollbarHorizontal) && scroll.Horizontal) || (axis == int(ScrollbarVertical) && scroll.Vertical)
			length := track[axis]
			if length <= 0 || !visible {
				setHTMLScrollbarPartVisible(transform, size, sprite, false)
				return true
			}
			if part.Kind == ScrollbarTrack {
				trackWidth, trackHeight := track.X(), track.Y()
				if !scroll.ScrollbarOverlay && axis == int(ScrollbarHorizontal) && scroll.Vertical {
					trackWidth += bar
				}
				if !scroll.ScrollbarOverlay && axis == int(ScrollbarVertical) && scroll.Horizontal {
					trackHeight += bar
				}
				if axis == 0 {
					transform.Translation.X.Value, transform.Translation.Y.Value = 0, barY
					size.Width, size.Height = trackWidth, bar
				} else {
					transform.Translation.X.Value, transform.Translation.Y.Value = barX, 0
					size.Width, size.Height = bar, trackHeight
				}
				sprite.Width, sprite.Height = size.Width, size.Height
				setHTMLScrollbarPartVisible(transform, size, sprite, true)
				return true
			}
			usable := length
			thumb := scene.ScrollThumbSize(usable, viewport[axis], scroll.ContentSize[axis], scroll.ScrollbarMinThumbSize)
			thumb = min(max(thumb, 0), usable)
			maxScroll := max(scroll.ContentSize[axis]-viewport[axis], 0)
			scrollOffset := max(-scroll.Position[axis], 0)
			thumbOffset := float32(0)
			travel := max(usable-thumb, 0)
			if maxScroll > 0 && travel > 0 {
				thumbOffset = travel * min(max(scrollOffset/maxScroll, 0), 1)
			}
			inset := min(max(bar*0.2, 1), 3)
			if scene.UIScrollScrollbarActive(world, part.Area, part.Axis) {
				inset = 0
			}
			paintLength := max(thumb, 0)
			if axis == 0 {
				transform.Translation.X.Value = thumbOffset
				transform.Translation.Y.Value = barY + inset
				size.Width, size.Height = paintLength, max(bar-inset*2, 0)
			} else {
				transform.Translation.X.Value = barX + inset
				transform.Translation.Y.Value = thumbOffset
				size.Width, size.Height = max(bar-inset*2, 0), paintLength
			}
			sprite.Width, sprite.Height = size.Width, size.Height
			setHTMLScrollbarPartVisible(transform, size, sprite, size.Width > 0 && size.Height > 0)
			return true
		}(entity, access.A, access.B, access.C, access.D) {
			break
		}
	}
}

func setHTMLScrollbarPartVisible(transform *scene.UITransform, size *scene.Size2D, sprite *scene.Sprite2D, visible bool) {
	if visible {
		return
	}
	size.Width, size.Height = 0, 0
	sprite.Width, sprite.Height = 0, 0
	transform.Translation.X.Value, transform.Translation.Y.Value = 0, 0
}

func routeHTMLOverflowEntities(world *ecs.World, owner ecs.Entity, state *htmlRenderState, children []ecs.Entity) {
	for _, entity := range children {
		if ecs.Has[ScrollbarPart](world, entity) {
			continue
		}
		child, ok := ecs.Get[htmlChild](world, entity)
		if !ok || child.Node == nil {
			continue
		}
		parent := ecs.GetMut[scene.Parent](world, entity)
		transform := ecs.GetMut[scene.UITransform](world, entity)
		if (parent == nil) || (transform == nil) {
			continue
		}
		target, routed := state.OverflowRoutes[child.Node]
		_, isClip := state.OverflowEntities[entity]
		if isClip {
			// Control fragments route through their own clip. The clip itself
			// inherits scrolling from its enclosing viewport, never from itself.
			target, routed = state.OverflowEntities[parent.Entity]
		}
		if !routed {
			continue
		}
		oldOrigin := layout.Rect{}
		if old, exists := state.OverflowEntities[parent.Entity]; exists {
			oldOrigin = old.rect
		} else if parent.Entity != owner {
			continue
		}
		absoluteX := float64(transform.Translation.X.Value) + oldOrigin.X
		absoluteY := float64(transform.Translation.Y.Value) + oldOrigin.Y
		// A nested overflow clip is already parented to its enclosing viewport by
		// emitHTMLOverflowClip, so it only needs the scroll binding that carries
		// that viewport's offset down the transform hierarchy.
		if !isClip {
			transform.Translation.X.Value = float32(absoluteX - target.rect.X)
			transform.Translation.Y.Value = float32(absoluteY - target.rect.Y)
			parent.Entity = target.entity
			state.SyncStats.RoutedEntities++
		}
		if scroll, exists := ecs.Get[scene.UIScroll](world, target.entity); exists && (scroll.Horizontal || scroll.Vertical) {
			_ = ecs.Insert(world, entity, ScrollContent{
				Area: target.entity,
				X:    float32(absoluteX - target.rect.X),
				Y:    float32(absoluteY - target.rect.Y),
			})
		}
	}
}

const htmlOverflowVisibleExtent = float32(math.MaxFloat32 / 4)

func htmlNodeHasScrollBinding(state *htmlRenderState, node *dom.Node) bool {
	if state == nil || node == nil {
		return false
	}
	_, bound := state.NodeScrollBindings[node]
	return bound
}

func spawnHTMLBoxSurface(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, window htmlPaintWindow, stackIndex int, receiver ecs.Entity, passThrough, visible bool) (ecs.Entity, bool) {
	if box == nil || !window.contains(box.BorderBox()) {
		return 0, false
	}
	rect := box.BorderBox()
	shape := htmlShape(box.Style, rect.Width, rect.Height)
	if htmlPropagatesCanvasBackground(box) {
		shape.Fill = graphics.ColorRGBA{}
	}
	if htmlOverflowBackgroundCovered(box) {
		shape.Fill = graphics.ColorRGBA{}
	}
	if shape.Kind == 0 || rect.Width <= 0 || rect.Height <= 0 {
		return 0, false
	}
	rasterPaint := htmlNeedsRasterPaint(box.Style)
	if shape.Fill.A <= 0 && shape.Border.A <= 0 && !htmlHasGradientBackground(box.Style) && box.Style.AnimatedPaint&(style.AnimatedBackgroundColor|style.AnimatedBorders) == 0 && !(rasterPaint && htmlHasVisibleBorder(box.Style, rect.Width)) {
		return 0, false
	}
	entity := spawnHTMLSurface(world, owner, box.Node, htmlRoleSurface, float32(rect.X), float32(rect.Y), float32(rect.Width), float32(rect.Height), shape, stackIndex, visible)
	if rasterPaint {
		if texture := htmlPaintTextureFor(state, box.Node, box.Style, float32(rect.Width), float32(rect.Height)); texture != nil {
			if sprite := ecs.GetMut[scene.Sprite2D](world, entity); sprite != nil {
				sprite.Texture, sprite.Tint, sprite.Shape = texture, graphics.White, scene.Shape2D{}
				state.Textures.bind(entity, texture)
			}
		}
	} else {
		spawnHTMLBackgroundLayers(world, owner, box, entity, stackIndex, visible)
	}
	_ = ecs.Insert(world, entity, htmlElementSurface{})
	_ = ecs.Insert(world, entity, scene.InputTarget2D{Width: float32(rect.Width), Height: float32(rect.Height), Receiver: receiver, PassThrough: passThrough})
	_ = ecs.Insert(world, entity, scene.InputState2D{})
	applyHTMLAnimatedVisual(world, state, entity, box.Node)
	return entity, true
}

func htmlHasGradientBackground(value *style.ComputedStyle) bool {
	if value == nil {
		return false
	}
	if isHTMLLinearGradient(value.BackgroundImage) {
		return true
	}
	for _, layer := range value.BackgroundLayers {
		if isHTMLLinearGradient(layer.Image) {
			return true
		}
	}
	return false
}

func htmlHasVisibleBorder(value *style.ComputedStyle, width float64) bool {
	for _, edge := range [...]struct{ width, style, color style.Value }{
		{value.BorderWidth.Top, value.BorderStyle.Top, value.BorderColor.Top},
		{value.BorderWidth.Right, value.BorderStyle.Right, value.BorderColor.Right},
		{value.BorderWidth.Bottom, value.BorderStyle.Bottom, value.BorderColor.Bottom},
		{value.BorderWidth.Left, value.BorderStyle.Left, value.BorderColor.Left},
	} {
		if isHTMLSolidBorder(edge.style) && edge.color.Color.A > 0 && style.ResolveLength(edge.width, width) > 0 {
			return true
		}
	}
	return false
}

// CSS paints the first background image on top. The primary surface already
// owns that layer; the remaining parsed layers are attached as retained
// sprites on the same entity so reconciliation keeps them stable.
func spawnHTMLBackgroundLayers(world *ecs.World, owner ecs.Entity, box *layout.Box, entity ecs.Entity, stackIndex int, visible bool) {
	if box == nil || box.Style == nil || entity == 0 || len(box.Style.BackgroundLayers) < 2 || htmlNeedsRasterPaint(box.Style) {
		return
	}
	state := ecs.GetMut[htmlRenderState](world, owner)
	if state == nil {
		return
	}
	rect := box.BorderBox()
	layers := make([]scene.SpriteLayer2D, 0, len(box.Style.BackgroundLayers)-1)
	for index := len(box.Style.BackgroundLayers) - 1; index >= 1; index-- {
		layer := box.Style.BackgroundLayers[index]
		if !isHTMLLinearGradient(layer.Image) {
			continue
		}
		texture := htmlGradientTextureFor(state, layer.Image, float32(rect.Width), float32(rect.Height))
		if texture == nil {
			continue
		}
		sprite := scene.NewSprite2D()
		sprite.Texture = texture
		sprite.Width, sprite.Height = float32(rect.Width), float32(rect.Height)
		layers = append(layers, scene.SpriteLayer2D{
			ID:     fmt.Sprintf("background-%d", index),
			Order:  index - len(box.Style.BackgroundLayers),
			Sprite: sprite, Anchor: scene.AnchorTopLeft, Visible: visible,
		})
	}
	if len(layers) == 0 {
		return
	}
	if existing := ecs.GetMut[scene.SpriteLayers2D](world, entity); existing != nil {
		existing.Layers = append(existing.Layers[:0], layers...)
		existing.InvalidateMatrices()
		return
	}
	_ = ecs.Insert(world, entity, scene.SpriteLayers2D{Layers: layers})
}

func spawnHTMLBoxShadows(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, stackIndex int, visible bool) {
	if world == nil || state == nil || box == nil || box.Style == nil || len(box.Style.BoxShadow) == 0 {
		return
	}
	rect := box.BorderBox()
	for index, shadow := range box.Style.BoxShadow {
		if shadow.Inset || shadow.Color.A == 0 {
			continue
		}
		shadowRect, padding := htmlBoxShadowRect(rect, shadow)
		width, height := shadowRect.Width, shadowRect.Height
		if width <= 0 || height <= 0 {
			continue
		}
		texture := htmlBoxShadowTextureFor(state, box.Node, box.Style, index, float32(width), float32(height), float32(rect.Width), float32(rect.Height), float32(padding))
		if texture == nil {
			continue
		}
		entity := spawnHTMLSurfaceVariant(world, owner, box.Node, htmlRoleShadow, fmt.Sprintf("shadow-%d", index), float32(shadowRect.X), float32(shadowRect.Y), float32(width), float32(height), scene.Shape2D{}, stackIndex-index-1, visible)
		if entity == 0 {
			continue
		}
		if sprite := ecs.GetMut[scene.Sprite2D](world, entity); sprite != nil {
			sprite.Texture, sprite.Tint, sprite.Shape = texture, graphics.White, scene.Shape2D{}
			state.Textures.bind(entity, texture)
		}
		markHTMLChildNode(world, entity, box.Node)
		state.Children = append(state.Children, entity)
	}
}

func htmlUniformRadius(value *style.ComputedStyle, width, height float64) float32 {
	if value == nil {
		return 0
	}
	radius := max(style.ResolveLength(value.BorderRadius.TopLeft.Horizontal, width), style.ResolveLength(value.BorderRadius.TopRight.Horizontal, width))
	radius = max(radius, max(style.ResolveLength(value.BorderRadius.BottomRight.Horizontal, width), style.ResolveLength(value.BorderRadius.BottomLeft.Horizontal, width)))
	return float32(max(min(radius, min(width, height)*0.5), 0))
}

func htmlOverflowBackgroundCovered(box *layout.Box) bool {
	if box == nil || box.Style == nil || !htmlOverflowClipsAxis(box.Style.OverflowX) || !htmlOverflowClipsAxis(box.Style.OverflowY) {
		return false
	}
	clip := box.PaddingBox()
	for _, child := range box.Children {
		if child == nil || !htmlStyleVisible(child.Style) {
			continue
		}
		if child.Style.Opacity.Kind == style.ValueNumber && child.Style.Opacity.Number < 1 || child.Style.AnimatedOpacity || child.Style.AnimatedPaint&style.AnimatedBackgroundColor != 0 || len(child.Style.Animations) != 0 || child.Style.TransformTransition.Enabled {
			continue
		}
		background := child.Style.BackgroundColor
		if background.Kind != style.ValueColor || background.Color.A <= 0 {
			if !isHTMLLinearGradient(child.Style.BackgroundImage) {
				continue
			}
		}
		rect := child.BorderBox()
		if rect.X <= clip.X && rect.Y <= clip.Y && rect.X+rect.Width >= clip.X+clip.Width && rect.Y+rect.Height >= clip.Y+clip.Height {
			return true
		}
	}
	return false
}

// The CPU compositor handles static boxes and shaped text. Replaced elements,
// controls and clipped/transformed contexts need a renderer offscreen layer.
func htmlCompositePaintBounds(state *htmlRenderState, box *layout.Box, face *font.Face) (layout.Rect, bool) {
	if box == nil || box.Style == nil || (box.Style.Opacity.Number >= 1 && !box.Style.AnimatedOpacity && len(box.Style.Filters) == 0) {
		return layout.Rect{}, false
	}
	if htmlPropagatesCanvasBackground(box) {
		return layout.Rect{}, false
	}
	var visit func(*layout.Box) (layout.Rect, bool)
	visit = func(current *layout.Box) (layout.Rect, bool) {
		if current != nil {
			restyleHTMLVisualBox(state, current)
		}
		if current == nil || current.Style == nil || current.Style.AnimatedPaint != 0 || face == nil && len(current.TextFragments) != 0 || len(current.ImageFragments) != 0 || len(current.ControlFragments) != 0 || len(current.Style.BoxShadow) != 0 || htmlOverflowClipsStyle(current.Style) || isHTMLControlNode(current.Node) || current.Node != nil && current.Node.Data == "img" {
			return layout.Rect{}, false
		}
		if current.Style.BackgroundImage != "" && !isHTMLLinearGradient(current.Style.BackgroundImage) {
			return layout.Rect{}, false
		}
		for _, layer := range current.Style.BackgroundLayers {
			if layer.Image != "" && layer.Image != "none" && !isHTMLLinearGradient(layer.Image) {
				return layout.Rect{}, false
			}
		}
		for _, border := range [...]style.Value{current.Style.BorderStyle.Top, current.Style.BorderStyle.Right, current.Style.BorderStyle.Bottom, current.Style.BorderStyle.Left} {
			if border.Keyword != "none" && border.Keyword != "" && !isHTMLSolidBorder(border) {
				return layout.Rect{}, false
			}
		}
		if current != box && (len(current.Style.Animations) != 0 || current.Style.AnimatedOpacity || current.Style.TransformTransition.Duration+current.Style.TransformTransition.Delay > 0 || current.Style.TransformScaleX != 1 || current.Style.TransformScaleY != 1 || current.Style.TransformRotation != 0 || current.Style.TransformTranslateX != style.LengthPx(0) || current.Style.TransformTranslateY != style.LengthPx(0) || current.Style.ZIndex != style.Keyword("auto")) {
			return layout.Rect{}, false
		}
		bounds := htmlOwnVisiblePaintBounds(current)
		for _, child := range current.Children {
			childBounds, ok := visit(child)
			if !ok {
				return layout.Rect{}, false
			}
			bounds = htmlUnionPaintBounds(bounds, childBounds)
		}
		if bounds.Width <= 0 || bounds.Height <= 0 {
			return bounds, true
		}
		left, top, right, bottom := htmlFilterBounds(current.Style.Filters, bounds.X, bounds.Y, bounds.X+bounds.Width, bounds.Y+bounds.Height)
		return layout.Rect{X: left, Y: top, Width: right - left, Height: bottom - top}, true
	}
	return visit(box)
}

func spawnHTMLFlattenedOpacitySurface(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, stackIndex int, visible bool) ecs.Entity {
	if world == nil || owner == 0 || box == nil {
		return 0
	}
	texture := htmlOpacityTexture(state, box)
	if texture == nil {
		return 0
	}
	bounds := state.Textures.rasters[box.Node].rect
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return 0
	}
	sprite := scene.NewSprite2D()
	sprite.Texture = texture
	sprite.Width, sprite.Height = float32(bounds.Width), float32(bounds.Height)
	sprite.Tint, sprite.Alpha = graphics.White, 1
	bundle := beginHTMLEntity(world, owner, box.Node, htmlRoleSurface, "", 0, 10)
	putHTML(&bundle, scene.UITransformFromXY(float32(bounds.X), float32(bounds.Y)))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: owner})
	putHTML(&bundle, sprite)
	putHTML(&bundle, scene.Size2D{Width: sprite.Width, Height: sprite.Height})
	putHTML(&bundle, htmlOpacitySurface{})
	putHTML(&bundle, htmlChild{Owner: owner, Node: box.Node})
	return bundle.finish()
}

func htmlOpacityTexture(state *htmlRenderState, root *layout.Box) *graphics.Texture {
	if state == nil || root == nil {
		return nil
	}
	raster := htmlOpacityRasterFor(&state.Textures, root)
	key := htmlTextureKey{node: root.Node}
	entry := state.Textures.entries[key]
	w, h := raster.image.Rect.Dx(), raster.image.Rect.Dy()
	if entry != nil && entry.texture.Width == w && entry.texture.Height == h {
		if raster.uploaded != raster.version {
			entry.texture.UpdateRegions([]graphics.TextureRegionUpdate{{Width: w, Height: h, Pixels: raster.image.Pix}})
			raster.uploaded = raster.version
		}
		return entry.texture
	}
	texture := graphics.CreateTextureByImageNoMipmap(&raster.image)
	if texture == nil {
		return nil
	}
	state.Textures.add(key, texture)
	raster.uploaded = raster.version
	return texture
}

func htmlCompositeRGBA(dst, src *image.RGBA, offsetX, offsetY int) {
	if dst == nil || src == nil {
		return
	}
	db, sb := dst.Bounds(), src.Bounds()
	for sy := sb.Min.Y; sy < sb.Max.Y; sy++ {
		dy := sy + offsetY
		if dy < db.Min.Y || dy >= db.Max.Y {
			continue
		}
		for sx := sb.Min.X; sx < sb.Max.X; sx++ {
			dx := sx + offsetX
			if dx < db.Min.X || dx >= db.Max.X {
				continue
			}
			si := src.PixOffset(sx, sy)
			di := dst.PixOffset(dx, dy)
			sa := float64(src.Pix[si+3]) / 255
			if sa <= 0 {
				continue
			}
			da := float64(dst.Pix[di+3]) / 255
			outA := sa + da*(1-sa)
			if outA <= 0 {
				continue
			}
			for channel := 0; channel < 3; channel++ {
				value := (float64(src.Pix[si+channel])*sa + float64(dst.Pix[di+channel])*da*(1-sa)) / outA
				dst.Pix[di+channel] = uint8(math.Round(value))
			}
			dst.Pix[di+3] = uint8(math.Round(outA * 255))
		}
	}
}

func htmlRoundedPixel(computed *style.ComputedStyle, x, y, width, height float64) bool {
	if computed == nil {
		return false
	}
	return htmlRoundedRasterPixel(prepareHTMLRasterCorners(computed, width, height), x, y, width, height)
}

type htmlRasterCorner struct{ x, y float64 }
type htmlRasterCorners [4]htmlRasterCorner

func prepareHTMLRasterCorners(computed *style.ComputedStyle, width, height float64) htmlRasterCorners {
	return normalizeHTMLRasterCorners(htmlRasterCorners{
		{style.ResolveLength(computed.BorderRadius.TopLeft.Horizontal, width), style.ResolveLength(computed.BorderRadius.TopLeft.Vertical, height)},
		{style.ResolveLength(computed.BorderRadius.TopRight.Horizontal, width), style.ResolveLength(computed.BorderRadius.TopRight.Vertical, height)},
		{style.ResolveLength(computed.BorderRadius.BottomRight.Horizontal, width), style.ResolveLength(computed.BorderRadius.BottomRight.Vertical, height)},
		{style.ResolveLength(computed.BorderRadius.BottomLeft.Horizontal, width), style.ResolveLength(computed.BorderRadius.BottomLeft.Vertical, height)},
	}, width, height)
}

func normalizeHTMLRasterCorners(radii htmlRasterCorners, width, height float64) htmlRasterCorners {
	scale := 1.0
	for _, sum := range [...]float64{radii[0].x + radii[1].x, radii[3].x + radii[2].x} {
		if sum > width && sum > 0 {
			scale = min(scale, width/sum)
		}
	}
	for _, sum := range [...]float64{radii[0].y + radii[3].y, radii[1].y + radii[2].y} {
		if sum > height && sum > 0 {
			scale = min(scale, height/sum)
		}
	}
	for index := range radii {
		radii[index].x *= scale
		radii[index].y *= scale
	}
	return radii
}

func htmlRoundedRasterPixel(radii htmlRasterCorners, x, y, width, height float64) bool {
	if x < 0 || y < 0 || x >= width || y >= height || width <= 0 || height <= 0 {
		return false
	}
	type cornerCheck struct {
		active bool
		cx, cy float64
		r      htmlRasterCorner
	}
	checks := [...]cornerCheck{
		{x < radii[0].x && y < radii[0].y, radii[0].x, radii[0].y, radii[0]},
		{x > width-radii[1].x && y < radii[1].y, width - radii[1].x, radii[1].y, radii[1]},
		{x > width-radii[2].x && y > height-radii[2].y, width - radii[2].x, height - radii[2].y, radii[2]},
		{x < radii[3].x && y > height-radii[3].y, radii[3].x, height - radii[3].y, radii[3]},
	}
	for _, check := range checks {
		if !check.active || check.r.x <= 0 || check.r.y <= 0 {
			continue
		}
		dx, dy := (x-check.cx)/check.r.x, (y-check.cy)/check.r.y
		return dx*dx+dy*dy <= 1
	}
	return true
}

// HTML shapes use one uniform border in the GPU path. Mixed CSS borders are
// represented by a filled box plus one thin surface per painted edge so
// border-top can render without accidentally outlining the whole element.
func spawnHTMLBorderSurfaces(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, stackIndex int, visible bool) {
	if world == nil || state == nil || box == nil || box.Node == nil {
		return
	}
	if htmlNeedsRasterPaint(box.Style) {
		return
	}
	rect := box.BorderBox()
	if rect.Width <= 0 || rect.Height <= 0 {
		return
	}
	if _, _, uniform := htmlUniformBorder(box.Style, rect.Width); uniform {
		return
	}
	spawnHTMLBorderSurface(
		world, owner, state, rect.X, rect.Y, rect.Width, box.Border.Top, box.Border.Top,
		box.Style.BorderStyle.Top, box.Style.BorderColor.Top, stackIndex+1, box.Node, htmlRoleBorderTop, visible,
	)
	spawnHTMLBorderSurface(
		world, owner, state, rect.X+rect.Width-box.Border.Right, rect.Y, box.Border.Right, rect.Height, box.Border.Right,
		box.Style.BorderStyle.Right, box.Style.BorderColor.Right, stackIndex+1, box.Node, htmlRoleBorderRight, visible,
	)
	spawnHTMLBorderSurface(
		world, owner, state, rect.X, rect.Y+rect.Height-box.Border.Bottom, rect.Width, box.Border.Bottom, box.Border.Bottom,
		box.Style.BorderStyle.Bottom, box.Style.BorderColor.Bottom, stackIndex+1, box.Node, htmlRoleBorderBottom, visible,
	)
	spawnHTMLBorderSurface(
		world, owner, state, rect.X, rect.Y, box.Border.Left, rect.Height, box.Border.Left,
		box.Style.BorderStyle.Left, box.Style.BorderColor.Left, stackIndex+1, box.Node, htmlRoleBorderLeft, visible,
	)
}

func spawnHTMLBorderSurface(world *ecs.World, owner ecs.Entity, state *htmlRenderState, x, y, width, height, borderWidth float64, borderStyle, borderColor style.Value, stackIndex int, node *dom.Node, role htmlRenderRole, visible bool) {
	if width <= 0 || height <= 0 || borderWidth <= 0 || !isHTMLSolidBorder(borderStyle) {
		return
	}
	color := htmlColor(borderColor)
	if color.A == 0 && state.Styles[node].AnimatedPaint&style.AnimatedBorders == 0 {
		return
	}
	entity := spawnHTMLSurface(world, owner, node, role, float32(x), float32(y), float32(width), float32(height), scene.RoundedRectShape2D(color, 0), stackIndex, visible)
	markHTMLChildNode(world, entity, node)
	state.Children = append(state.Children, entity)
}

func spawnHTMLSurface(world *ecs.World, owner ecs.Entity, node *dom.Node, role htmlRenderRole, x, y, width, height float32, shape scene.Shape2D, stackIndex int, visible bool) ecs.Entity {
	return spawnHTMLSurfaceVariant(world, owner, node, role, "", x, y, width, height, shape, stackIndex, visible)
}

func spawnHTMLSurfaceVariant(world *ecs.World, owner ecs.Entity, node *dom.Node, role htmlRenderRole, variant string, x, y, width, height float32, shape scene.Shape2D, stackIndex int, visible bool) ecs.Entity {
	sprite := scene.NewSprite2D()
	sprite.Shape, sprite.Width, sprite.Height = shape, width, height
	if state := ecs.GetMut[htmlRenderState](world, owner); state != nil {
		if computed, ok := state.Styles.Get(node); ok {
			sprite.Alpha = float32(computed.Opacity.Number)
			if (role == htmlRoleSurface || role == htmlRoleCanvas) && computed.BackgroundImage != "" {
				texture := htmlGradientTextureFor(state, computed.BackgroundImage, width, height)
				if texture != nil {
					sprite.Texture = texture
					sprite.Tint = graphics.White
					sprite.Shape = scene.Shape2D{}
				}
			}
		}
	}
	bundle := beginHTMLEntity(world, owner, node, role, variant, 0, 10)
	putHTML(&bundle, scene.UITransformFromXY(x, y))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: owner})
	putHTML(&bundle, sprite)
	putHTML(&bundle, scene.Size2D{Width: width, Height: height})
	putHTML(&bundle, htmlChild{Owner: owner})
	return bundle.finish()
}

// htmlGradientTexture rasterizes the supported CSS linear-gradient image into
// a regular sprite texture. The retained CSS value remains the source of
// truth; this small CPU raster is generated only when an HTML surface is
// emitted, matching the existing image-provider path used by HTML sprites.
func htmlGradientTexture(source string, width, height float32) *graphics.Texture {
	if width <= 0 || height <= 0 || !isHTMLLinearGradient(source) {
		return nil
	}
	stops, direction, ok := parseHTMLLinearGradient(source)
	if !ok || len(stops) < 2 {
		return nil
	}
	w, h := max(1, int(math.Ceil(float64(width)))), max(1, int(math.Ceil(float64(height))))
	raster := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pixelX := float64(x) * float64(width) / float64(max(w-1, 1))
			pixelY := float64(y) * float64(height) / float64(max(h-1, 1))
			progress := gradientProgress(pixelX, pixelY, direction.X, direction.Y, float64(width), float64(height))
			progress = max(0, min(1, progress))
			segment := len(stops) - 2
			for index := 0; index+1 < len(stops); index++ {
				if progress <= stops[index+1].Position {
					segment = index
					break
				}
			}
			left, right := stops[segment], stops[segment+1]
			local := (progress - left.Position) / max(right.Position-left.Position, 1e-9)
			local = max(0, min(1, local))
			raster.SetRGBA(x, y, imageRGBAInterpolate(left.Color, right.Color, local))
		}
	}
	return graphics.CreateTextureByImageNoMipmap(raster)
}

func htmlGradientTextureFor(state *htmlRenderState, source string, width, height float32) *graphics.Texture {
	if state == nil {
		return htmlGradientTexture(source, width, height)
	}
	w, h := max(1, int(math.Ceil(float64(width)))), max(1, int(math.Ceil(float64(height))))
	key := htmlTextureKey{gradient: htmlGradientKey{Source: source, Width: w, Height: h}}
	if entry := state.Textures.entries[key]; entry != nil {
		return entry.texture
	}
	texture := htmlGradientTexture(source, width, height)
	if texture == nil {
		return nil
	}
	state.Textures.add(key, texture)
	return texture
}

func isHTMLLinearGradient(source string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(source))
	return strings.HasPrefix(trimmed, "linear-gradient(") && strings.HasSuffix(trimmed, ")")
}

type htmlGradientDirection struct{ X, Y float64 }
type htmlGradientStop struct {
	Color    color.RGBA
	Position float64
	Explicit bool
}

func parseHTMLLinearGradient(source string) ([]htmlGradientStop, htmlGradientDirection, bool) {
	trimmed := strings.TrimSpace(source)
	open := strings.IndexByte(trimmed, '(')
	if open < 0 || !strings.HasSuffix(trimmed, ")") {
		return nil, htmlGradientDirection{}, false
	}
	parts := splitHTMLGradientParts(trimmed[open+1 : len(trimmed)-1])
	if len(parts) < 2 {
		return nil, htmlGradientDirection{}, false
	}
	direction := htmlGradientDirection{Y: 1}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(parts[0])), "to ") {
		directionText := strings.ToLower(strings.TrimSpace(parts[0]))
		direction = htmlGradientDirection{}
		if strings.Contains(directionText, "right") {
			direction.X = 1
		}
		if strings.Contains(directionText, "left") {
			direction.X = -1
		}
		if strings.Contains(directionText, "top") {
			direction.Y = -1
		}
		if strings.Contains(directionText, "bottom") {
			direction.Y = 1
		}
		parts = parts[1:]
	} else if angle, parsed := parseHTMLGradientAngle(parts[0]); parsed {
		direction = htmlGradientDirection{X: math.Sin(angle), Y: -math.Cos(angle)}
		parts = parts[1:]
	}
	stops := make([]htmlGradientStop, 0, len(parts))
	for _, part := range parts {
		colorText, positionText := splitHTMLGradientStop(part)
		if colorText == "" {
			return nil, htmlGradientDirection{}, false
		}
		parsedColor, ok := parseHTMLGradientColor(colorText)
		if !ok {
			return nil, htmlGradientDirection{}, false
		}
		stop := htmlGradientStop{Color: parsedColor}
		if positionText != "" {
			positionFields := strings.Fields(positionText)
			position, parsed := parseHTMLGradientStopPosition(positionFields[0])
			if !parsed {
				return nil, htmlGradientDirection{}, false
			}
			stop.Position, stop.Explicit = position, true
		}
		stops = append(stops, stop)
	}
	resolveHTMLGradientStopPositions(stops)
	return stops, direction, true
}

func splitHTMLGradientStop(input string) (string, string) {
	trimmed := strings.TrimSpace(input)
	depth := 0
	for index, r := range trimmed {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && (r == ' ' || r == '\t' || r == '\n' || r == '\r') {
				return trimmed[:index], strings.TrimSpace(trimmed[index:])
			}
		}
	}
	return trimmed, ""
}

func parseHTMLGradientStopPosition(input string) (float64, bool) {
	value := strings.TrimSpace(strings.ToLower(input))
	if strings.HasSuffix(value, "%") {
		position, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
		return max(0, min(1, position/100)), err == nil
	}
	return 0, false
}

func resolveHTMLGradientStopPositions(stops []htmlGradientStop) {
	if len(stops) == 0 {
		return
	}
	if !stops[0].Explicit {
		stops[0].Position = 0
		stops[0].Explicit = true
	}
	if !stops[len(stops)-1].Explicit {
		stops[len(stops)-1].Position = 1
		stops[len(stops)-1].Explicit = true
	}
	for index := 1; index < len(stops); index++ {
		if stops[index].Explicit && stops[index].Position < stops[index-1].Position {
			stops[index].Position = stops[index-1].Position
		}
	}
	for index := 1; index < len(stops)-1; {
		if stops[index].Explicit {
			index++
			continue
		}
		start := index - 1
		for index < len(stops) && !stops[index].Explicit {
			index++
		}
		end := index
		left, right := stops[start].Position, stops[end].Position
		span := max(end-start, 1)
		for current := start + 1; current < end; current++ {
			stops[current].Position = left + (right-left)*float64(current-start)/float64(span)
		}
	}
}

func parseHTMLGradientAngle(input string) (float64, bool) {
	value := strings.TrimSpace(strings.ToLower(input))
	if !strings.HasSuffix(value, "deg") {
		return 0, false
	}
	degrees, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "deg")), 64)
	if err != nil {
		return 0, false
	}
	return degrees * math.Pi / 180, true
}

func gradientProgress(x, y, dx, dy, width, height float64) float64 {
	length := math.Hypot(dx, dy)
	if length <= 1e-9 {
		return y / max(height, 1e-9)
	}
	dx, dy = dx/length, dy/length
	minimum, maximum := math.Inf(1), math.Inf(-1)
	for _, point := range [4][2]float64{{0, 0}, {width, 0}, {0, height}, {width, height}} {
		projection := point[0]*dx + point[1]*dy
		minimum, maximum = min(minimum, projection), max(maximum, projection)
	}
	return (x*dx + y*dy - minimum) / max(maximum-minimum, 1e-9)
}

func splitHTMLGradientParts(input string) []string {
	parts := make([]string, 0, 4)
	start, depth := 0, 0
	for index, r := range input {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(input[start:index]))
				start = index + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(input[start:]))
	return parts
}

func parseHTMLGradientColor(input string) (color.RGBA, bool) {
	parsed, err := style.ParseColor(input)
	return color.RGBA(parsed), err == nil
}

func imageRGBAInterpolate(left, right color.RGBA, amount float64) color.RGBA {
	lerp := func(a, b uint8) uint8 { return uint8(math.Round(float64(a) + (float64(b)-float64(a))*amount)) }
	return color.RGBA{R: lerp(left.R, right.R), G: lerp(left.G, right.G), B: lerp(left.B, right.B), A: lerp(left.A, right.A)}
}

func spawnHTMLControl(world *ecs.World, owner ecs.Entity, node *dom.Node, rect layout.Rect, border layout.Edges, shape scene.Shape2D, stackIndex int, visible bool) ecs.Entity {
	if !isHTMLControlNode(node) || rect.Width <= 0 || rect.Height <= 0 {
		return 0
	}
	width, height := float32(rect.Width), float32(rect.Height)
	sprite := scene.NewSprite2D()
	sprite.Shape, sprite.Width, sprite.Height = shape, width, height
	control := htmlControlFromNode(owner, node)
	control.PaddingBox = scene.TextRangeRect2D{
		X: float32(border.Left), Y: float32(border.Top),
		Width: float32(max(rect.Width-border.Left-border.Right, 0)), Height: float32(max(rect.Height-border.Top-border.Bottom, 0)),
	}
	if state := ecs.GetMut[htmlRenderState](world, owner); state != nil {
		state.SyncStats.UpdatedControls++
		control.RenderRevision = state.RenderGeneration
		if computed, exists := state.Styles.Get(node); exists && node.Data == "textarea" {
			control.Wrap = htmlTextareaWrap(computed)
		}
	}
	focusable, hasFocusable := htmlFocusableFromNode(owner, node)
	if state, ok := ecs.Get[htmlRenderState](world, owner); ok && state.ControlSnapshots != nil {
		if snapshot, exists := state.ControlSnapshots[node]; exists {
			restoreHTMLControlSnapshot(&control, snapshot)
		}
	}
	bundle := beginHTMLEntity(world, owner, node, htmlRoleControl, control.Type, 0, 15)
	putHTML(&bundle, scene.UITransformFromXY(float32(rect.X), float32(rect.Y)))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: owner})
	putHTML(&bundle, sprite)
	putHTML(&bundle, scene.Size2D{Width: width, Height: height})
	putHTML(&bundle, scene.ClipRect2D{Width: width, Height: height})
	putHTML(&bundle, scene.InputTarget2D{Width: width, Height: height, Disabled: control.Disabled, AllowDisabledInput: true})
	putHTML(&bundle, scene.InputState2D{})
	putHTML(&bundle, control)
	if hasFocusable {
		putHTML(&bundle, focusable)
	}
	putHTML(&bundle, htmlControlNodeRef{Node: node})
	putHTML(&bundle, htmlChild{Owner: owner})
	return bundle.finish()
}

const (
	htmlSelectionLayerFloor = 16
	htmlSelectionLayerCeil  = 128
)

// htmlSelectionLayers sizes the indexed selection pool to the text rows a
// control can show at once, so a drag past the pool end keeps painting.
func htmlSelectionLayers(world *ecs.World, owner ecs.Entity, computed *style.ComputedStyle) int {
	height := float32(0)
	if control, ok := ecs.Get[Control](world, owner); ok {
		height = control.PaddingBox.Height
	}
	if height <= 0 {
		if size, ok := ecs.Get[scene.Size2D](world, owner); ok {
			height = size.Height
		}
	}
	var face *font.Face
	if binding, ok := ecs.Get[ControlTextFont](world, owner); ok && binding.Font != nil {
		face = binding.Font.Face
	}
	lineHeight := float32(layout.UsedLineHeight(computed, face))
	if height <= 0 || lineHeight <= 0 {
		return htmlSelectionLayerFloor
	}
	return min(max(int(height/lineHeight)+3, htmlSelectionLayerFloor), htmlSelectionLayerCeil)
}

func htmlSpawnControlVisuals(world *ecs.World, owner ecs.Entity, state *htmlRenderState, control ecs.Entity, node *dom.Node, stackIndex int) {
	if state == nil || control == 0 {
		return
	}
	computed := state.Styles[node]
	if htmlStyleVisible(computed) && htmlNeedsRasterPaint(computed) {
		if sprite := ecs.GetMut[scene.Sprite2D](world, control); sprite != nil {
			if texture := htmlPaintTextureFor(state, node, computed, sprite.Width, sprite.Height); texture != nil {
				sprite.Texture, sprite.Tint, sprite.Shape = texture, graphics.White, scene.Shape2D{}
				state.Textures.bind(control, texture)
			}
		}
	}
	if !isHTMLEditableControlNode(node) {
		return
	}
	selections := 1
	if node.Data == "textarea" {
		selections = htmlSelectionLayers(world, control, computed)
	}
	for index := range selections {
		entity := spawnHTMLControlVisual(world, owner, control, ControlVisual{Control: control, Selection: true, Index: index}, stackIndex+1)
		markHTMLChildNode(world, entity, node)
		state.Children = append(state.Children, entity)
	}
	entity := spawnHTMLControlVisual(world, owner, control, ControlVisual{Control: control}, stackIndex+3)
	markHTMLChildNode(world, entity, node)
	state.Children = append(state.Children, entity)
	if node.Data != "textarea" {
		return
	}
	for _, kind := range [...]ControlVisualKind{
		ControlVisualScrollTrackHorizontal,
		ControlVisualScrollTrackVertical,
		ControlVisualScrollThumbHorizontal,
		ControlVisualScrollThumbVertical,
	} {
		entity := spawnHTMLControlVisual(world, owner, control, ControlVisual{Control: control, Kind: kind}, stackIndex+3)
		markHTMLChildNode(world, entity, node)
		state.Children = append(state.Children, entity)
	}
}

func htmlSpawnEditableVisuals(world *ecs.World, owner ecs.Entity, state *htmlRenderState, editable ecs.Entity, node *dom.Node, stackIndex int) {
	if state == nil || editable == 0 || node == nil {
		return
	}
	for index := range htmlSelectionLayers(world, editable, state.Styles[node]) {
		entity := spawnHTMLEditableVisual(world, owner, editable, EditableVisual{Editable: editable, Selection: true, Index: index}, stackIndex+1)
		markHTMLChildNode(world, entity, node)
		state.Children = append(state.Children, entity)
	}
	entity := spawnHTMLEditableVisual(world, owner, editable, EditableVisual{Editable: editable}, stackIndex+3)
	markHTMLChildNode(world, entity, node)
	state.Children = append(state.Children, entity)
}

func spawnHTMLEditableVisual(world *ecs.World, owner, editable ecs.Entity, visual EditableVisual, stackIndex int) ecs.Entity {
	bundle := beginHTMLEntity(world, owner, htmlEntityNode(world, editable), htmlRoleEditableVisual, "", editable, 12)
	putHTML(&bundle, scene.NewUITransform())
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: editable})
	putHTML(&bundle, scene.Size2D{Width: 1, Height: 1})
	putHTML(&bundle, scene.NewUIBackground2D(scene.RoundedRectShape2D(graphics.ColorRGBA{}, 0)))
	putHTML(&bundle, scene.NewUIImage2D(nil))
	putHTML(&bundle, scene.NewUIBorder2D(scene.Shape2D{}))
	putHTML(&bundle, visual)
	putHTML(&bundle, htmlChild{Owner: owner})
	return bundle.finish()
}

func spawnHTMLControlVisual(world *ecs.World, owner, control ecs.Entity, visual ControlVisual, stackIndex int) ecs.Entity {
	bundle := beginHTMLEntity(world, owner, htmlEntityNode(world, control), htmlRoleControlVisual, "", control, 12)
	putHTML(&bundle, scene.NewUITransform())
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: control})
	putHTML(&bundle, scene.Size2D{Width: 1, Height: 1})
	putHTML(&bundle, scene.NewUIBackground2D(scene.RoundedRectShape2D(graphics.ColorRGBA{}, 0)))
	putHTML(&bundle, scene.NewUIImage2D(nil))
	putHTML(&bundle, scene.NewUIBorder2D(scene.Shape2D{}))
	putHTML(&bundle, visual)
	putHTML(&bundle, htmlChild{Owner: owner})
	return bundle.finish()
}

func isHTMLControlNode(node *dom.Node) bool {
	if node == nil || node.Type != dom.ElementNode {
		return false
	}
	switch node.Data {
	case "button", "input", "select", "textarea":
		return true
	default:
		return false
	}
}

func isHTMLEditableControlNode(node *dom.Node) bool {
	if node == nil || node.Type != dom.ElementNode {
		return false
	}
	if node.Data == "textarea" {
		return true
	}
	if node.Data != "input" {
		return false
	}
	typeValue, _ := node.Attr("type")
	switch strings.ToLower(strings.TrimSpace(typeValue)) {
	case "button", "checkbox", "color", "file", "hidden", "image", "radio", "range", "reset", "submit":
		return false
	default:
		return true
	}
}

func isHTMLCheckboxNode(node *dom.Node) bool {
	if node == nil || node.Type != dom.ElementNode || node.Data != "input" {
		return false
	}
	typeValue, _ := node.Attr("type")
	return strings.EqualFold(strings.TrimSpace(typeValue), "checkbox")
}

func htmlUsesNativeAppearance(computed *style.ComputedStyle) bool {
	return computed.Appearance.Kind != style.ValueKeyword || computed.Appearance.Keyword != "none"
}

func htmlTextareaWrap(computed *style.ComputedStyle) bool {
	return computed.WhiteSpace.Keyword != "pre" && computed.WhiteSpace.Keyword != "nowrap"
}

func htmlControlFromNode(owner ecs.Entity, node *dom.Node) Control {
	control := Control{Owner: owner, Tag: node.Data, Selected: -1, DefaultSelected: -1, SelectionAnchor: -1, SelectionFocus: -1}
	control.ID, _ = node.Attr("id")
	if form := dom.FormOwner(node); form != nil {
		control.FormID, _ = form.Attr("id")
	}
	control.Name, _ = node.Attr("name")
	control.Type, _ = node.Attr("type")
	control.Placeholder, _ = node.Attr("placeholder")
	control.Disabled = dom.IsDisabled(node)
	control.ReadOnly = htmlNodeHasAttr(node, "readonly")
	control.DefaultChecked = htmlNodeHasAttr(node, "checked")
	control.Checked = dom.IsChecked(node)
	control.DirtyChecked, _ = node.States.Get(dom.StateDirtyChecked)
	if value, ok := node.Attr("value"); ok {
		control.DefaultValue, control.Value = value, value
	} else if node.Data == "textarea" {
		control.DefaultValue = htmlNodeText(node)
		control.Value = control.DefaultValue
	} else if node.Data == "input" && (strings.EqualFold(strings.TrimSpace(control.Type), "checkbox") || strings.EqualFold(strings.TrimSpace(control.Type), "radio")) {
		control.Value = "on"
	}
	control.DirtyValue, _ = node.States.Get(dom.StateDirtyValue)
	if raw, ok := node.Attr("maxlength"); ok {
		control.MaxLength, _ = strconv.Atoi(strings.TrimSpace(raw))
	}
	if node.Data == "select" {
		control.Multiple = htmlNodeHasAttr(node, "multiple")
		appendHTMLSelectOptions(node, &control)
		syncHTMLSelectOptionStates(node, &control)
		control.DefaultSelected = dom.DefaultSelectedIndex(node)
	}
	return control
}

func htmlFocusableFromNode(owner ecs.Entity, node *dom.Node) (Focusable, bool) {
	if node == nil || node.Type != dom.ElementNode {
		return Focusable{}, false
	}
	tabIndex, explicitTabIndex := dom.TabIndex(node)
	control := isHTMLControlNode(node)
	editingHost := dom.IsEditingHost(node)
	if control && (dom.IsDisabled(node) || node.Data == "input" && strings.EqualFold(strings.TrimSpace(inputType(node)), "hidden")) {
		return Focusable{}, false
	}
	if !control && !explicitTabIndex && !editingHost {
		_, hasHref := node.Attr("href")
		if node.Data != "a" || !hasHref {
			return Focusable{}, false
		}
	}
	if !explicitTabIndex {
		tabIndex = 0
	}
	value := Focusable{
		Owner: owner, ID: attributeValue(node, "id"), Tag: node.Data, Type: inputType(node), TabIndex: tabIndex, Order: node.Order,
		Sequential: !explicitTabIndex || tabIndex >= 0, Pointer: true,
		AutoFocus: htmlNodeHasAttr(node, "autofocus"), FocusVisible: editingHost || isHTMLEditableControlNode(node),
	}
	return value, true
}

func inputType(node *dom.Node) string {
	value, _ := node.Attr("type")
	return value
}

// ControlByID returns the rendered form control for the first matching DOM
// id in this surface. It reuses the document and render indexes. Before a node
// is rendered, after replacing the source, or for a non-control match it returns
// false. Dirty document indexes are refreshed before resolving the id.
func ControlByID(world *ecs.World, owner ecs.Entity, id string) (ecs.Entity, bool) {
	if world == nil || owner == 0 || id == "" {
		return 0, false
	}
	state := ecs.GetRef[htmlRenderState](world, owner)
	source := ecs.GetRef[Surface](world, owner)
	if state == nil || source == nil || state.Root == nil || state.Source != source.Value {
		return 0, false
	}
	if state.IndexDirty || state.NodeByID == nil {
		state = ecs.GetMut[htmlRenderState](world, owner)
		syncHTMLDocumentIndex(state)
	}
	node := state.NodeByID[id]
	if !isHTMLControlNode(node) {
		return 0, false
	}
	group := state.EntityGroups[htmlRenderIdentity{Node: node, Role: htmlRoleControl, Variant: inputType(node)}]
	if group == nil {
		return 0, false
	}
	for _, binding := range group.Bindings {
		if control := ecs.GetRef[Control](world, binding.Entity); control != nil && control.Owner == owner {
			return binding.Entity, true
		}
	}
	return 0, false
}

func attributeValue(node *dom.Node, name string) string {
	value, _ := node.Attr(name)
	return value
}

func appendHTMLSelectOptions(node *dom.Node, control *Control) {
	appendHTMLSelectOptionsInto(node, control, false)
}

func appendHTMLSelectOptionsInto(node *dom.Node, control *Control, inheritedDisabled bool) {
	if node == nil || control == nil {
		return
	}
	for _, child := range node.Children {
		if child == nil || child.Type != dom.ElementNode {
			continue
		}
		if child.Data == "option" {
			label := strings.TrimSpace(htmlNodeText(child))
			optionValue, hasValue := child.Attr("value")
			if !hasValue {
				optionValue = label
			}
			dirty, _ := child.States.Get(dom.StateDirtySelected)
			option := SelectOption{
				Value: optionValue, Label: label, Disabled: inheritedDisabled || htmlNodeHasAttr(child, "disabled"),
				Selected: dom.IsChecked(child), DefaultSelected: htmlNodeHasAttr(child, "selected"), DirtySelected: dirty,
			}
			control.Options = append(control.Options, option)
			continue
		}
		childDisabled := inheritedDisabled || child.Data == "optgroup" && htmlNodeHasAttr(child, "disabled")
		appendHTMLSelectOptionsInto(child, control, childDisabled)
	}
}

func syncHTMLSelectOptionStates(node *dom.Node, control *Control) {
	if node == nil || control == nil || control.Tag != "select" {
		return
	}
	control.Selected = -1
	control.SelectedCount = 0
	control.DirtySelected = false
	index := 0
	syncHTMLSelectOptionStatesInto(node, control, &index)
	if control.Selected >= 0 && control.Selected < len(control.Options) {
		control.Value = control.Options[control.Selected].Value
	} else {
		control.Value = ""
	}
}

func syncHTMLSelectOptionStatesInto(node *dom.Node, control *Control, index *int) {
	for _, child := range node.Children {
		if child == nil || child.Type != dom.ElementNode {
			continue
		}
		if child.Data == "option" {
			if *index < len(control.Options) {
				option := &control.Options[*index]
				option.Selected = dom.IsChecked(child)
				option.DefaultSelected = htmlNodeHasAttr(child, "selected")
				option.DirtySelected, _ = child.States.Get(dom.StateDirtySelected)
				if option.Selected {
					if control.Selected < 0 {
						control.Selected = *index
					}
					control.SelectedCount++
				}
				control.DirtySelected = control.DirtySelected || option.DirtySelected
			}
			*index++
			continue
		}
		syncHTMLSelectOptionStatesInto(child, control, index)
	}
}

func htmlNodeHasAttr(node *dom.Node, name string) bool {
	_, ok := node.Attr(name)
	return ok
}

func htmlNodeText(node *dom.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == dom.TextNode {
		return node.Data
	}
	var builder strings.Builder
	for _, child := range node.Children {
		builder.WriteString(htmlNodeText(child))
	}
	return builder.String()
}

func setHTMLNodeText(node *dom.Node, value string) {
	if node == nil {
		return
	}
	if len(node.Children) == 1 && node.Children[0] != nil && node.Children[0].Type == dom.TextNode {
		node.Children[0].Data = value
		return
	}
	for _, child := range node.Children {
		if child != nil {
			child.Parent = nil
		}
	}
	if cap(node.Children) > 0 {
		node.Children = node.Children[:1]
		node.Children[0] = dom.NewText(value)
	} else {
		node.Children = []*dom.Node{dom.NewText(value)}
	}
	node.Children[0].Parent = node
}

// spawnHTMLEditableText flattens an editing host into one live text visual.
// Descendant boxes still retain their layout surfaces, while duplicate text
// fragments are skipped after the first call for the host.
func spawnHTMLEditableText(world *ecs.World, owner ecs.Entity, state *htmlRenderState, editable ecs.Entity, box *layout.Box, fontValue, boldFont *onixtext.Font, dpi float64, stackIndex int, passThrough, visible bool) bool {
	if editable == 0 || box == nil {
		return false
	}
	value, ok := ecs.Get[Editable](world, editable)
	ref := ecs.GetMut[htmlEditableNodeRef](world, editable)
	if (!ok) || (ref == nil) || (ref.Node == nil) {
		return false
	}
	if !htmlStyleVisible(box.Style) {
		return false
	}
	for _, fragment := range box.TextFragments {
		if !htmlStyleVisible(fragment.Style) {
			return false
		}
	}
	if ref.Text != 0 {
		return true
	}
	fragment := layout.TextFragment{Node: ref.Node, Rect: box.Content, Style: box.Style, Text: value.Value}
	entity := spawnHTMLText(world, owner, 0, 0, editable, layout.Rect{}, fragment, fontValue, boldFont, dpi, stackIndex, passThrough, visible)
	if entity != 0 {
		_ = ecs.Insert(world, entity, EditableText{Editable: editable})
		ref.Text = entity
		state.Children = append(state.Children, entity)
	}
	return true
}

func spawnHTMLImage(world *ecs.World, owner ecs.Entity, fragment layout.ImageFragment, stackIndex int, visible bool) (ecs.Entity, bool) {
	if !htmlStyleVisible(fragment.Style) {
		return 0, false
	}
	provider, ok := ecs.GetResource[ImageProvider](world)
	if surface := ecs.GetRef[Surface](world, owner); surface != nil && surface.Images != nil {
		provider, ok = surface.Images.Provider, surface.Images.Provider != nil
	}
	if !ok || provider == nil || fragment.Node == nil {
		return 0, false
	}
	source, ok := fragment.Node.Attr("src")
	if !ok {
		return 0, false
	}
	texture, ok := provider.ResolveHTMLImage(source)
	if !ok || texture == nil || !texture.Valid() {
		return 0, false
	}
	sprite := scene.NewSprite2D()
	sprite.Texture = texture
	sprite.Width, sprite.Height = float32(fragment.Rect.Width), float32(fragment.Rect.Height)
	width, height := sprite.Width, sprite.Height
	bundle := beginHTMLEntity(world, owner, fragment.Node, htmlRoleImage, "", 0, 12)
	putHTML(&bundle, scene.UITransformFromXY(float32(fragment.Rect.X), float32(fragment.Rect.Y)))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: owner})
	if fit := fragment.Style.ObjectFit.Keyword; fit != "" && fit != "fill" {
		naturalWidth, naturalHeight := float32(texture.LogicalWidth()), float32(texture.LogicalHeight())
		scale := float32(1)
		if naturalWidth > 0 && naturalHeight > 0 {
			switch fit {
			case "contain":
				scale = min(width/naturalWidth, height/naturalHeight)
			case "cover":
				scale = max(width/naturalWidth, height/naturalHeight)
			case "scale-down":
				scale = min(1, min(width/naturalWidth, height/naturalHeight))
			}
		}
		sprite.Width, sprite.Height = naturalWidth*scale, naturalHeight*scale
		// Fit the pixels within the content box without changing layout or hit bounds.
		putHTML(&bundle, scene.SpriteLayers2D{Layers: []scene.SpriteLayer2D{{
			Sprite: sprite, Anchor: scene.AnchorTopLeft, Visible: true,
			Transform: scene.SpriteLayerTransform2D{
				X: htmlObjectPosition(fragment.Style.ObjectPosX, fragment.Style.ObjectPosXOrigin, width-sprite.Width),
				Y: htmlObjectPosition(fragment.Style.ObjectPosY, fragment.Style.ObjectPosYOrigin, height-sprite.Height),
			},
		}}})
		putHTML(&bundle, scene.ClipRect2D{Width: width, Height: height})
	} else {
		putHTML(&bundle, sprite)
	}
	putHTML(&bundle, scene.Size2D{Width: width, Height: height})
	putHTML(&bundle, scene.InputTarget2D{Width: width, Height: height})
	putHTML(&bundle, scene.InputState2D{})
	putHTML(&bundle, htmlChild{Owner: owner})
	entity := bundle.finish()
	markHTMLChildNode(world, entity, fragment.Node)
	return entity, true
}

func htmlObjectPosition(value style.Value, origin string, free float32) float32 {
	offset := free * .5
	switch value.Kind {
	case style.ValueNumber:
		offset = free * float32(value.Number)
	case style.ValueLengthPx:
		offset = float32(value.LengthPx)
	}
	if origin == "right" || origin == "bottom" {
		return free - offset
	}
	return offset
}

// LCD coverage requires an opaque backdrop and no ancestor opacity group.
// Alpha applies the containing element's opacity to its text primitive;
// overlapping opacity groups still require an offscreen compositing layer.
func htmlTextPaint(styles style.Map, node *dom.Node) (bool, float32) {
	if node != nil && node.Type == dom.TextNode {
		node = node.Parent
	}
	if computed, ok := styles.Get(node); ok {
		return computed.TextOpaqueBackdrop && !computed.TextInOpacityGroup, float32(computed.Opacity.Number)
	}
	return false, 1
}

func spawnHTMLText(world *ecs.World, owner, control, receiver, focusOwner ecs.Entity, controlRect layout.Rect, fragment layout.TextFragment, fontValue, boldFont *onixtext.Font, dpi float64, stackIndex int, passThrough, visible bool) ecs.Entity {
	fontValue = htmlFontForControl(world, control, fontValue, boldFont, fragment.Style)
	_, editable := ecs.Get[Editable](world, focusOwner)
	if fontValue == nil || control == 0 && !editable && strings.TrimSpace(fragment.Text) == "" && !htmlPreservesWhitespace(fragment.Style) {
		return 0
	}
	text := scene.NewText2D(fragment.Text)
	if state, ok := ecs.Get[htmlRenderState](world, owner); ok {
		text.LCD, text.Alpha = htmlTextPaint(state.Styles, fragment.Node)
	}

	text.Font = fontValue
	text.FontSize = fragmentFontSize(fragment.Style)
	text.DPI = dpi
	if fontValue != nil {
		text.LineHeight = float32(layout.UsedLineHeight(fragment.Style, fontValue.Face))
	} else if fragment.Style.LineHeight.Kind == style.ValueLengthPx && fragment.Style.LineHeight.LengthPx > 0 {
		text.LineHeight = float32(fragment.Style.LineHeight.LengthPx)
	}
	text.WrapMode = scene.TextWrapNone
	text.MaxWidth = 0
	text.Color = htmlColor(fragment.Style.Color)
	text.TabWidth = htmlTextTabWidth(&text, fragment.Style)
	x, y := fragment.Rect.X, fragment.Rect.Y
	parent := owner
	capacity := 12
	if control != 0 {
		x, y = x-controlRect.X, y-controlRect.Y
		parent = control
	}
	var selectClip scene.ClipRect2D
	selectLabel := false
	if value := ecs.GetRef[Control](world, control); value != nil && value.Tag == "select" {
		selectLabel = true
		right := controlRect.Width - style.ResolveLength(fragment.Style.Padding.Right, controlRect.Width) - style.ResolveLength(fragment.Style.BorderWidth.Right, controlRect.Width)
		if htmlUsesNativeAppearance(fragment.Style) {
			right = min(right, controlRect.Width-float64(htmlSelectIndicatorSize(float32(controlRect.Height)))-12)
		}
		selectClip = scene.ClipRect2D{Y: -float32(y), Width: float32(max(min(fragment.Rect.Width, right-x), 0)), Height: float32(controlRect.Height)}
	}
	bundle := beginHTMLEntity(world, owner, fragment.Node, htmlRoleText, "", parent, capacity)
	putHTML(&bundle, scene.UITransformFromXY(float32(x), float32(y)))
	putHTML(&bundle, scene.UIGlobalTransform{})
	putHTML(&bundle, scene.Visibility{Visible: visible})
	putHTML(&bundle, scene.AnchorTopLeft)
	putHTML(&bundle, scene.UIStackIndex(stackIndex))
	putHTML(&bundle, scene.Parent{Entity: parent})
	putHTML(&bundle, text)
	if selectLabel {
		putHTML(&bundle, selectClip)
	}
	putHTML(&bundle, scene.Size2D{Width: float32(fragment.Rect.Width), Height: float32(fragment.Rect.Height)})
	if receiver != 0 {
		putHTML(&bundle, scene.InputBlocker2D{Width: float32(fragment.Rect.Width), Height: float32(fragment.Rect.Height), Receiver: receiver, PassThrough: passThrough})
	} else {
		putHTML(&bundle, scene.InputTarget2D{Width: float32(fragment.Rect.Width), Height: float32(fragment.Rect.Height), PassThrough: passThrough})
		putHTML(&bundle, scene.InputState2D{})
	}
	putHTML(&bundle, htmlChild{Owner: owner})
	if focusOwner != 0 {
		putHTML(&bundle, FocusOwner{Entity: focusOwner})
	}
	if control != 0 {
		putHTML(&bundle, ControlText{Control: control, Content: scene.TextRangeRect2D{
			X: float32(x), Y: float32(y), Width: float32(fragment.Rect.Width), Height: float32(fragment.Rect.Height),
		}})
		putHTML(&bundle, htmlControlTextNode{Node: fragment.Node})
	}
	entity := bundle.finish()
	markHTMLChildNode(world, entity, fragment.Node)
	return entity
}

func htmlTextTabWidth(text *scene.Text2D, computed *style.ComputedStyle) float32 {
	switch computed.TabSize.Kind {
	case style.ValueLengthPx, style.ValueLengthCalc:
		return float32(max(computed.TabSize.LengthPx, 0))
	case style.ValueNumber:
		if text.Font != nil && text.Font.Head.UnitsPerEm != 0 {
			face, id := text.Font.ResolveGlyph(' ')
			metrics, err := face.HorizontalMetrics(id)
			if err != nil {
				return 0
			}
			dpi := text.DPI
			if dpi <= 0 {
				dpi = 72
			}
			space := float64(metrics.AdvanceWidth) * text.FontSize * dpi / (72 * float64(face.Head.UnitsPerEm))
			return float32(max(computed.TabSize.Number, 0)) * float32(math.Round(space*64)/64)
		}
	}
	return 0
}

func spawnHTMLTextVisuals(world *ecs.World, owner ecs.Entity, state *htmlRenderState, control, receiver, focusOwner ecs.Entity, controlRect layout.Rect, fragment layout.TextFragment, fontValue, boldFont *onixtext.Font, dpi float64, stackIndex int, passThrough, visible bool) {
	if control != 0 && htmlStyleVisible(fragment.Style) && !htmlStyleVisible(state.Styles[htmlEntityNode(world, control)]) {
		control, receiver = 0, 0
	}
	if control == 0 && !htmlStyleVisible(fragment.Style) {
		return
	}
	text := spawnHTMLText(world, owner, control, receiver, focusOwner, controlRect, fragment, fontValue, boldFont, dpi, stackIndex, passThrough, visible)
	if text == 0 {
		return
	}
	state.Children = append(state.Children, text)
	if decoration := spawnHTMLTextDecoration(world, owner, control, controlRect, fragment, stackIndex, visible); decoration != 0 {
		state.Children = append(state.Children, decoration)
	}
}

func spawnHTMLTextDecoration(world *ecs.World, owner, control ecs.Entity, controlRect layout.Rect, fragment layout.TextFragment, stackIndex int, visible bool) ecs.Entity {
	decoration := fragment.Style.TextDecorationLine
	if decoration.Kind != style.ValueKeyword || decoration.Keyword != "line-through" || fragment.Rect.Width <= 0 || fragment.Rect.Height <= 0 {
		return 0
	}
	color := htmlColor(fragment.Style.Color)
	if color.A == 0 && fragment.Style.AnimatedPaint&style.AnimatedColor == 0 {
		return 0
	}
	thickness := min(max(fragmentFontSize(fragment.Style)/14, 1), fragment.Rect.Height)
	x := fragment.Rect.X
	y := fragment.Rect.Y + (fragment.Rect.Height-thickness)*0.5
	parent := owner
	if control != 0 {
		x -= controlRect.X
		y -= controlRect.Y
		parent = control
	}
	entity := spawnHTMLSurface(
		world, owner, fragment.Node, htmlRoleDecoration, float32(x), float32(y), float32(fragment.Rect.Width), float32(thickness),
		scene.RoundedRectShape2D(color, 0), stackIndex, visible,
	)
	_ = ecs.Insert(world, entity, htmlTextDecoration{})
	markHTMLChildNode(world, entity, fragment.Node)
	if parent != owner {
		if value := ecs.GetMut[scene.Parent](world, entity); value != nil {
			value.Entity = parent
		}
	}
	return entity
}

func htmlPreservesWhitespace(value *style.ComputedStyle) bool {
	if value.WhiteSpace.Kind != style.ValueKeyword {
		return false
	}
	switch value.WhiteSpace.Keyword {
	case "pre", "pre-wrap":
		return true
	default:
		return false
	}
}

func htmlFontForControl(world *ecs.World, control ecs.Entity, regular, bold *onixtext.Font, value *style.ComputedStyle) *onixtext.Font {
	if override := ecs.GetRef[ControlTextFont](world, control); override != nil && override.Font != nil {
		return override.Font
	}
	return htmlFontForStyle(regular, bold, value)
}

func htmlFontForStyle(regular, bold *onixtext.Font, value *style.ComputedStyle) *onixtext.Font {
	// The project ships Regular and Bold faces, so semi-bold weights use the
	// nearest available bold face instead of silently reusing Regular glyphs.
	if bold != nil && value.FontWeight.Kind == style.ValueNumber && value.FontWeight.Number >= 600 {
		return bold
	}
	return regular
}

func fragmentFontSize(value *style.ComputedStyle) float64 {
	if value.FontSize.Kind == style.ValueLengthPx && value.FontSize.LengthPx > 0 {
		return value.FontSize.LengthPx
	}
	return 16
}

func htmlColor(value style.Value) graphics.ColorRGBA {
	if value.Kind != style.ValueColor {
		return graphics.ColorRGBA{}
	}
	return graphics.ColorFromRGBA(value.Color.R, value.Color.G, value.Color.B, value.Color.A)
}

func htmlShape(value *style.ComputedStyle, width, height float64) scene.Shape2D {
	fill := htmlColor(value.BackgroundColor)
	border, borderWidth, uniformBorder := htmlUniformBorder(value, width)
	radius := max(
		style.ResolveLength(value.BorderRadius.TopLeft.Horizontal, width),
		style.ResolveLength(value.BorderRadius.TopRight.Horizontal, width),
		style.ResolveLength(value.BorderRadius.BottomRight.Horizontal, width),
		style.ResolveLength(value.BorderRadius.BottomLeft.Horizontal, width),
	)
	if width <= 0 || height <= 0 {
		return scene.Shape2D{}
	}
	// Transparent controls still supply their geometry to native popup surfaces.
	shape := scene.RoundedRectShape2D(fill, float32(max(min(radius, min(width, height)*0.5), 0)))
	if uniformBorder {
		shape.Border, shape.BorderWidth = border, float32(max(borderWidth, 0))
	}
	return shape
}

func htmlUniformBorder(value *style.ComputedStyle, width float64) (graphics.ColorRGBA, float64, bool) {
	if value.AnimatedPaint&style.AnimatedBorders != 0 {
		return graphics.ColorRGBA{}, 0, false
	}
	if !isHTMLSolidBorder(value.BorderStyle.Top) || !isHTMLSolidBorder(value.BorderStyle.Right) ||
		!isHTMLSolidBorder(value.BorderStyle.Bottom) || !isHTMLSolidBorder(value.BorderStyle.Left) {
		return graphics.ColorRGBA{}, 0, false
	}
	topWidth := style.ResolveLength(value.BorderWidth.Top, width)
	if topWidth != style.ResolveLength(value.BorderWidth.Right, width) ||
		topWidth != style.ResolveLength(value.BorderWidth.Bottom, width) ||
		topWidth != style.ResolveLength(value.BorderWidth.Left, width) {
		return graphics.ColorRGBA{}, 0, false
	}
	topColor := htmlColor(value.BorderColor.Top)
	if topColor != htmlColor(value.BorderColor.Right) || topColor != htmlColor(value.BorderColor.Bottom) || topColor != htmlColor(value.BorderColor.Left) {
		return graphics.ColorRGBA{}, 0, false
	}
	return topColor, topWidth, true
}

func isHTMLSolidBorder(value style.Value) bool {
	return value.Kind == style.ValueKeyword && value.Keyword == "solid"
}
