package ui

import (
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
)

type CornerRadius float32

// Circle uses the largest radius allowed by the surface dimensions.
const Circle CornerRadius = -1

// ButtonContentAlignment controls how a button arranges its icon and label.
type ButtonContentAlignment uint8

const (
	ButtonContentStart ButtonContentAlignment = iota
	ButtonContentCenter
)

func (radius CornerRadius) pixels(width, height float32) float32 {
	limit := max(min(width, height)*0.5, 0)
	if radius == Circle {
		return limit
	}
	return min(max(float32(radius), 0), limit)
}

// Control marks an input target as a styled UI control. Geometry, disabled
// state, ordering and input state remain in generic ECS components.
type Control struct {
	Selected bool
}

type controlView struct {
	Width, Height float32
	StackIndex    int
	Disabled      bool
	InputFilter   scene.InputFilter
	PassThrough   bool
	Hovered       bool
	Pressed       bool
	Selected      bool
}

func controlViewFor(world *ecs.World, entity ecs.Entity) (controlView, bool) {
	control := ecs.GetRef[Control](world, entity)
	target := ecs.GetRef[scene.InputTarget2D](world, entity)
	if (control == nil) || (target == nil) {
		return controlView{}, false
	}
	view := controlView{
		Width: target.Width, Height: target.Height, Disabled: target.Disabled,
		InputFilter: target.InputFilter,
		PassThrough: target.PassThrough,
	}
	if stackIndex, ok := ecs.Get[scene.UIStackIndex](world, entity); ok {
		view.StackIndex = int(stackIndex)
	}
	if pointer := ecs.GetRef[scene.InputState2D](world, entity); pointer != nil {
		view.Hovered, view.Pressed = pointer.Hovered, pointer.Pressed
	}
	view.Selected = control.Selected
	return view, true
}

func ControlBounds(world *ecs.World, entity ecs.Entity) (float32, float32, bool) {
	view, ok := controlViewFor(world, entity)
	return view.Width, view.Height, ok
}

func SetControlBounds(world *ecs.World, entity ecs.Entity, width, height float32) {
	if target, ok := ecs.Get[scene.InputTarget2D](world, entity); ok {
		if target.Width == width && target.Height == height {
			return
		}
		if target := ecs.GetMut[scene.InputTarget2D](world, entity); target != nil {
			target.Width, target.Height = width, height
		}
	}
}

func SetControlDisabled(world *ecs.World, entity ecs.Entity, disabled bool) {
	if target, ok := ecs.Get[scene.InputTarget2D](world, entity); ok {
		if target.Disabled == disabled {
			return
		}
		if target := ecs.GetMut[scene.InputTarget2D](world, entity); target != nil {
			target.Disabled = disabled
		}
	}
}

func SetControlSelected(world *ecs.World, entity ecs.Entity, selected bool) {
	if control := ecs.GetMut[Control](world, entity); control != nil {
		control.Selected = selected
	}
}

func SetControlPassThrough(world *ecs.World, entity ecs.Entity, passThrough bool) {
	if target, ok := ecs.Get[scene.InputTarget2D](world, entity); ok {
		if target.PassThrough == passThrough {
			return
		}
		if target := ecs.GetMut[scene.InputTarget2D](world, entity); target != nil {
			target.PassThrough = passThrough
		}
	}
}

func SetControlInputFilter(world *ecs.World, entity ecs.Entity, filter scene.InputFilter) {
	if target, ok := ecs.Get[scene.InputTarget2D](world, entity); ok {
		if target.InputFilter == filter {
			return
		}
		if target := ecs.GetMut[scene.InputTarget2D](world, entity); target != nil {
			target.InputFilter = filter
		}
	}
}

type Panel struct {
	Title string
}

// WindowTitleBarAction identifies a standard custom-window control.
type WindowTitleBarAction uint8

const (
	WindowTitleBarMinimize WindowTitleBarAction = iota
	WindowTitleBarMaximize
	WindowTitleBarClose
	WindowTitleBarFullscreen
)

// WindowTitleBar describes a reusable custom window title bar and its controls.
type WindowTitleBar struct {
	Buttons    []ecs.Entity
	Width      float32
	Height     float32
	StackIndex int
}

// WindowTitleBarButton binds a standard window action to a button entity.
type WindowTitleBarButton struct {
	Bar    ecs.Entity
	Action WindowTitleBarAction
}

// WindowTitleBarEvent is emitted when a title-bar button is activated.
type WindowTitleBarEvent struct {
	Bar    ecs.Entity
	Button ecs.Entity
	Action WindowTitleBarAction
}

// Dialog marks a panel as a transient dialog surface.
type Dialog struct{}

type Label struct {
	Value    string
	Color    graphics.ColorRGBA
	FontSize float64
	DPI      float64
}

// Button marks controls whose reusable behavior is button styling. Activation
// remains the original scene.InputEventTap and is handled by bound game logic.
type Button struct{}

type Content[T any] struct {
	Value T
}

type Action[T any] struct {
	Value T
}

type IconContent struct {
	Value IconStyle
}

type Typography struct {
	FontSize float64
	DPI      float64
	Color    graphics.ColorRGBA
}

type ButtonVariant uint8

const (
	ButtonPrimary ButtonVariant = iota
	ButtonSurface
)

type ButtonGroup struct {
	Buttons    []ecs.Entity
	Width      float32
	Height     float32
	StackIndex int
}

type ButtonGroupValue[T any] struct {
	Group ecs.Entity
	Index int
	Value T
}

type buttonGroupItem struct {
	Group ecs.Entity
	Index int
}

type buttonGroupDivider struct {
	Group       ecs.Entity
	Index       int
	Width       float32
	Inset       float32
	Color       graphics.ColorRGBA
	CustomColor bool
}

type Danger struct{}

type TextField struct {
	// Buffer optionally binds native input to a document-owned piece tree.
	Buffer      *input.TextBuffer
	Value       string
	Placeholder string
	Password    bool
	Composition string
	// ScrollX is the signed horizontal text offset. Positive values move the
	// text right; negative values reveal the caret in overflowing values.
	ScrollX   float32
	Caret     int
	Anchor    int
	Selecting bool
	Focused   bool
	MaxRunes  int
	Submitted bool
	FontSize  float64
	DPI       float64

	lastScrollValue       string
	lastScrollComposition string
	lastScrollCaret       int
	lastScrollFocused     bool
	lastScrollViewport    float32
	scrollInitialized     bool
}

// TextFieldKeyHandler intercepts keyboard editing before native text input and
// focus traversal. Returning true lets editor widgets own Enter, Tab, arrows
// and Escape without also inserting text or moving the document caret.
type TextFieldKeyHandler struct {
	Handle func(*input.State) bool
}

type Input struct{}

type NumberInput struct {
	Changed         bool
	Step            float64
	Integer         bool
	StepButtonWidth float32
	UpIcon          IconStyle
	DownIcon        IconStyle
}

type NumberInputStep struct {
	Input     ecs.Entity
	Direction int
}

// Textarea stores multiline text behavior, content padding, and the current content offset.
// Scroll offsets use positive coordinates from the content's top-left corner.
type Textarea struct {
	Wrap    bool
	Padding float32
	// WrapTokens preserves words and identifiers, allowing long tokens to overflow.
	WrapTokens bool
	// SelectTokens selects a code word, operator or quoted contents on a mouse double-click.
	SelectTokens bool
	// WrapWidthPercent scales the available content width; zero uses the full width.
	WrapWidthPercent float32
	// FollowTail reveals the end when the value or viewport changes, even without focus.
	FollowTail bool
	// CaptureTab lets a code editor handle Tab without moving focus.
	CaptureTab bool
	// BarSize is the scrollbar width; zero uses the default and a negative value hides the bar while keeping scrolling.
	BarSize float32
	// WheelStep is the distance per wheel unit; zero uses the UI default.
	WheelStep               float32
	DisableHorizontalScroll bool
	ScrollX                 float32
	ScrollY                 float32

	scrollDragging    bool
	scrollDragAxis    uint8
	scrollDragPointer float32
	scrollDragOffset  float32
	preserveScroll    bool

	lastValue            string
	lastCaret            int
	lastFocused          bool
	lastViewportWidth    float32
	lastViewportHeight   float32
	lastWrapWidthPercent float32
	lastWrapTokens       bool
	lastWrap             bool
	lastFollowTail       bool
	scrollInitialized    bool
}

// ResetScroll returns the viewport to its origin and releases scrollbar dragging.
func (textarea *Textarea) ResetScroll() {
	textarea.ScrollX, textarea.ScrollY = 0, 0
	textarea.scrollDragging = false
	textarea.scrollDragAxis = 0
	textarea.scrollDragPointer, textarea.scrollDragOffset = 0, 0
	textarea.scrollInitialized = false
}

// RestoreScroll restores a saved viewport without revealing the caret on the
// next layout pass. Layout still clamps the offset to the current content.
func (textarea *Textarea) RestoreScroll(x, y float32) {
	textarea.ResetScroll()
	textarea.ScrollX, textarea.ScrollY = x, y
	textarea.preserveScroll = true
}

type TextControlMode interface {
	Input | Textarea
}

// TextControl configures explicit text-field behavior for an existing control
// root. T selects single-line Input or multiline Textarea behavior in code.
type TextControl[T TextControlMode] struct {
	Value       string
	Placeholder string
	Password    bool
	Wrap        bool
	Padding     float32
	MaxRunes    int
	FontSize    float64
	DPI         float64
}

type Menu struct {
	Label      string
	Open       bool
	Selected   int
	Width      float32
	ItemHeight float32
	Padding    float32
	StackIndex int
}

type Item[Kind any] struct {
	Owner    ecs.Entity
	Index    int
	Label    string
	Disabled bool
}

type MenuItem = Item[Menu]

type ContextMenu struct {
	Open       bool
	Selected   int
	Width      float32
	ItemHeight float32
	Padding    float32
	StackIndex int
}

type ContextMenuItem = Item[ContextMenu]

type Toggle[T comparable] struct {
	Value   T
	Off     T
	On      T
	Changed bool
}

type Selector struct {
	Label      string
	Open       bool
	Selected   int
	Icon       IconStyle
	ItemHeight float32
	ListHeight float32
	Upward     bool
	StackIndex int
}

type SelectorList struct {
	Selector ecs.Entity
}

type SelectorItem = Item[Selector]

type Tab struct {
	Selected     int
	Width        float32
	Height       float32
	HeaderHeight float32
	Highlight    graphics.ColorRGBA
	ItemBorders  bool
}

type TabItemBorderSide uint8

const (
	TabItemBorderTop TabItemBorderSide = iota
	TabItemBorderRight
	TabItemBorderBottom
	TabItemBorderLeft
)

type TabItemBorder struct {
	Side TabItemBorderSide
}

type TabItem struct {
	Tabs     ecs.Entity
	Index    int
	Width    float32
	Label    string
	Icon     IconStyle
	Disabled bool
}

type joinedEdgeSide uint8

const (
	joinedEdgeLeft joinedEdgeSide = iota
	joinedEdgeRight
	joinedEdgeTop
	joinedEdgeBottom
)

type joinedEdge struct {
	Owner ecs.Entity
	Side  joinedEdgeSide
}

type TabPanel struct {
	Tabs  ecs.Entity
	Index int
}

type Progress struct {
	Min           float32
	Max           float32
	Value         float32
	Indeterminate bool
	Slider        bool
	Changed       bool
	Fill          ecs.Entity
	Thumb         ecs.Entity
}

// ProgressThumb links an authored thumb control to its progress source.
type ProgressThumb struct {
	Progress ecs.Entity
}

type ScrollArea struct {
	Width              float32
	Height             float32
	ContentWidth       float32
	ContentHeight      float32
	Controlled         bool
	DisableDrag        bool
	WheelStep          float32
	BarSize            float32
	MinThumbSize       float32
	ThumbBarSize       float32
	HoverBarSize       float32
	HoverHorizontal    bool
	HoverVertical      bool
	DraggingHorizontal bool
	DraggingVertical   bool
	StackIndex         int
	// ScrollBarStackIndex overrides the visual layer of scrollbar parts; zero follows StackIndex.
	ScrollBarStackIndex int
}

type ScrollContent struct {
	Area ecs.Entity
	X    float32
	Y    float32
}

type ChangedEvent[T any] struct {
	Entity ecs.Entity
	Value  T
}

type InputEvent[T any] struct {
	Entity ecs.Entity
	Value  T
}

type SubmittedEvent[T any] struct {
	Entity ecs.Entity
	Value  T
}

type SelectedEvent[T any] struct {
	Entity ecs.Entity
	Index  int
	Value  T
}

type FocusChangedEvent struct {
	Entity       ecs.Entity
	Focused      bool
	FocusVisible bool
}

type LinkActivatedEvent struct {
	Entity ecs.Entity
	Target string
}

type uiEventState struct {
	Focused          ecs.Entity
	FocusVisible     bool
	ActivatedOnPress ecs.Entity
	LastInputClick   ecs.Entity
	LastInputClickAt time.Time
	LastInputClickX  float32
	LastInputClickY  float32
	InputEvents      ecs.MessageReader[scene.InputEvent]
}

type caretBlinkState struct {
	phase uint64
}
