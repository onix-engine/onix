package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	textIconX              float32 = 12
	textIconGap            float32 = 8
	defaultMenuPad         float32 = 6
	menuTextPaddingX       float32 = 12
	selectorPopupStackBase         = math.MaxInt - 5
)

type PanelOptions struct {
	X, Y          float32
	Width, Height float32
	Title         string
	Radius        CornerRadius
	Border        graphics.ColorRGBA
	BorderWidth   float32
	PassThrough   bool
	Theme         Theme
	Style         *PanelStyle
}

const WindowTitleBarButtonWidth float32 = 46

// WindowTitleBarItem configures one standard title-bar control.
type WindowTitleBarItem struct {
	Action WindowTitleBarAction
	Icon   IconStyle
	Width  float32
	Style  *ButtonStyle
}

type WindowTitleBarOptions struct {
	X, Y        float32
	Width       float32
	Height      float32
	Items       []WindowTitleBarItem
	Radius      CornerRadius
	StackIndex  int
	PassThrough bool
	Theme       Theme
}

type DialogOptions = PanelOptions

type LabelOptions struct {
	X, Y        float32
	Value       string
	Color       graphics.ColorRGBA
	FontSize    float64
	DPI         float64
	PassThrough bool
	Theme       Theme
}

type ButtonOptions struct {
	X, Y          float32
	Width, Height float32
	Label         string
	Icon          IconStyle
	ContentAlign  ButtonContentAlignment
	Variant       ButtonVariant
	Radius        CornerRadius
	JoinLeft      bool
	JoinRight     bool
	Disabled      bool
	PassThrough   bool
	FontSize      float64
	DPI           float64
	Theme         Theme
	Style         *ButtonStyle
}

type ButtonGroupItem[T any] struct {
	Value    T
	Width    float32
	Label    string
	Icon     IconStyle
	Disabled bool
}

type ButtonGroupDividerOptions struct {
	Width float32
	Inset float32
	Color graphics.ColorRGBA
}

type ButtonGroupOptions[T any] struct {
	X, Y        float32
	Height      float32
	Items       []ButtonGroupItem[T]
	Radius      CornerRadius
	Divider     *ButtonGroupDividerOptions
	PassThrough bool
	FontSize    float64
	DPI         float64
	Theme       Theme
	Style       *ButtonGroupStyle
}

type TextOptions struct {
	X, Y          float32
	Width, Height float32
	Value         string
	Placeholder   string
	Password      bool
	Wrap          bool
	MaxRunes      int
	Radius        CornerRadius
	JoinLeft      bool
	JoinRight     bool
	Disabled      bool
	PassThrough   bool
	FontSize      float64
	DPI           float64
	Theme         Theme
	Style         *TextFieldStyle
}

type NumberInputOptions struct {
	X, Y            float32
	Width, Height   float32
	Value           string
	Placeholder     string
	MaxRunes        int
	Disabled        bool
	PassThrough     bool
	Step            float64
	Integer         bool
	StepButtonWidth float32
	UpIcon          IconStyle
	DownIcon        IconStyle
	Radius          CornerRadius
	FontSize        float64
	DPI             float64
	Theme           Theme
	Style           *NumberInputStyle
}

type MenuOptions struct {
	X, Y          float32
	Width, Height float32
	Label         string
	Items         []string
	Padding       float32
	Radius        CornerRadius
	PassThrough   bool
	Theme         Theme
	Style         *MenuStyle
}

type ContextMenuOptions struct {
	X, Y        float32
	Width       float32
	ItemHeight  float32
	Items       []string
	Open        bool
	Padding     float32
	Radius      CornerRadius
	PassThrough bool
	Theme       Theme
	Style       *MenuStyle
}

type CheckboxOptions struct {
	X, Y          float32
	Width, Height float32
	Label         string
	Checked       bool
	Radius        CornerRadius
	Disabled      bool
	PassThrough   bool
	FontSize      float64
	DPI           float64
	Theme         Theme
	Style         *CheckboxStyle
}

type SelectorOptions struct {
	X, Y          float32
	Width, Height float32
	Label         string
	Options       []string
	Selected      int
	Disabled      bool
	PassThrough   bool
	Icon          IconStyle
	ItemHeight    float32
	ListHeight    float32
	ScrollY       float32
	Upward        bool
	Radius        CornerRadius
	JoinLeft      bool
	JoinRight     bool
	Theme         Theme
	Style         *SelectorStyle
}

type TabItemSpec struct {
	Label    string
	Icon     IconStyle
	Disabled bool
}

type TabOptions struct {
	X, Y          float32
	Width, Height float32
	HeaderHeight  float32
	Items         []TabItemSpec
	Selected      int
	Highlight     graphics.ColorRGBA
	Radius        CornerRadius
	PassThrough   bool
	ItemBorders   bool
	Theme         Theme
	Style         *TabStyle
}

type TabPanelOptions struct {
	Index       int
	Radius      CornerRadius
	PassThrough bool
	Theme       Theme
	Style       *TabStyle
}

type ProgressOptions struct {
	X, Y            float32
	Width, Height   float32
	Min, Max, Value float32
	Indeterminate   bool
	Slider          bool
	ThumbSize       float32
	Radius          CornerRadius
	PassThrough     bool
	Theme           Theme
	Style           *ProgressStyle
}

type ScrollAreaOptions struct {
	ReserveScrollbarSpace bool
	X, Y                  float32
	Width, Height         float32
	ContentWidth          float32
	ContentHeight         float32
	Controlled            bool
	DisableDrag           bool
	WheelStep             float32
	ScrollX               float32
	ScrollY               float32
	BarSize               float32
	MinThumbSize          float32
	ThumbBarSize          float32
	HoverBarSize          float32
	ScrollBarStackIndex   int
	Radius                CornerRadius
	PassThrough           bool
	Theme                 Theme
	Style                 *ScrollStyle
}

func SpawnPanel(world *ecs.World, opts PanelOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	bundle := newSurfaceBundle(scene.UITransformFromXY(opts.X, opts.Y), opts.Width, opts.Height, scene.RoundedRectShape2D(theme.Background, 0), theme.StackIndex, 4)
	bundle.Put(opts.Radius)
	bundle.Put(Panel{Title: opts.Title})
	bundle.Put(scene.InputBlocker2D{Width: opts.Width, Height: opts.Height, PassThrough: opts.PassThrough})
	style := skinFor(world, theme).Panel
	if opts.Style != nil {
		style = *opts.Style
	}
	if opts.BorderWidth > 0 {
		style.Body.Border = scene.OutlinedRoundedRectShape2D(graphics.ColorRGBA{}, opts.Border, 0, opts.BorderWidth)
	}
	if opts.Style != nil || opts.BorderWidth > 0 {
		bundle.Put(StyleOverride[PanelStyle]{Value: style})
	}
	entity := world.Spawn(bundle)
	applySurface(world, entity, opts.Width, opts.Height, style.Body)
	if opts.Title != "" {
		spawnText[TitlePart](world, entity, 12, 10, opts.Title, theme)
	}
	return entity
}

// SpawnWindowTitleBar creates a reusable custom-window title-bar control group.
// The returned entity is an invisible layout root; its children are the
// interactive buttons and carry WindowTitleBarButton actions.
func SpawnWindowTitleBar(world *ecs.World, opts WindowTitleBarOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	root := spawnAnchor(world, opts.X, opts.Y)
	setUIStackIndex(world, root, opts.StackIndex)
	_ = world.Insert(root, scene.Size2D{Width: opts.Width, Height: opts.Height})
	_ = world.Insert(root, WindowTitleBar{Width: opts.Width, Height: opts.Height, StackIndex: opts.StackIndex})
	buttons := make([]ecs.Entity, 0, len(opts.Items))
	width := float32(0)
	for _, item := range opts.Items {
		buttonWidth := item.Width
		if buttonWidth <= 0 {
			buttonWidth = WindowTitleBarButtonWidth
		}
		button := SpawnButton(world, ButtonOptions{
			X: width, Y: 0, Width: buttonWidth, Height: opts.Height,
			Icon: item.Icon, Radius: opts.Radius, PassThrough: opts.PassThrough,
			Theme: theme, Style: item.Style,
		})
		_ = world.Insert(button, scene.Parent{Entity: root})
		_ = world.Insert(button, WindowTitleBarButton{Bar: root, Action: item.Action})
		buttons = append(buttons, button)
		width += buttonWidth
	}
	if titleBar := world.GetMut[WindowTitleBar](root); titleBar != nil {
		titleBar.Buttons = buttons
		titleBar.Width = opts.Width
		titleBar.Height = opts.Height
	}
	return root
}

// UpdateWindowTitleBar changes the root bounds and keeps its controls aligned
// to the right edge of the title bar.
func UpdateWindowTitleBar(world *ecs.World, entity ecs.Entity, opts WindowTitleBarOptions) bool {
	titleBar := world.GetMut[WindowTitleBar](entity)
	if titleBar == nil {
		return false
	}
	titleBar.Width, titleBar.Height, titleBar.StackIndex = opts.Width, opts.Height, opts.StackIndex
	if size := world.GetMut[scene.Size2D](entity); size != nil {
		size.Width, size.Height = opts.Width, opts.Height
	}
	setUIStackIndex(world, entity, opts.StackIndex)
	width := float32(0)
	for _, item := range opts.Items {
		if item.Width > 0 {
			width += item.Width
		} else {
			width += WindowTitleBarButtonWidth
		}
	}
	x := max(opts.Width-width, 0)
	for index, button := range titleBar.Buttons {
		if index >= len(opts.Items) {
			setVisible(world, button, false)
			continue
		}
		item := opts.Items[index]
		buttonWidth := item.Width
		if buttonWidth <= 0 {
			buttonWidth = WindowTitleBarButtonWidth
		}
		if transform := world.GetMut[scene.UITransform](button); transform != nil {
			transform.Translation.X.Value = x
			transform.Translation.Y.Value = 0
		}
		if target := world.GetMut[scene.InputTarget2D](button); target != nil {
			target.Width, target.Height = buttonWidth, opts.Height
		}
		if size := world.GetMut[scene.Size2D](button); size != nil {
			size.Width, size.Height = buttonWidth, opts.Height
		}
		setUIStackIndex(world, button, opts.StackIndex)
		setVisible(world, button, true)
		x += buttonWidth
	}
	return true
}

func SpawnDialog(world *ecs.World, opts DialogOptions) ecs.Entity {
	entity := SpawnPanel(world, opts)
	_ = world.Insert(entity, Dialog{})
	return entity
}

func SpawnLabel(world *ecs.World, opts LabelOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	text := scene.NewText2D(opts.Value)
	text.Font = theme.Font
	text.FontSize = sizeOr(opts.FontSize, theme.FontSize)
	text.DPI = sizeOr(opts.DPI, theme.DPI)
	text.Color = colorOr(opts.Color, theme.Text)
	width, height, _ := scene.MeasureTextValue2D(&text, text.Value)
	bundle := newUIBundle(scene.UITransformFromXY(opts.X, opts.Y), theme.StackIndex+2, 10)
	bundle.Put(text)
	bundle.Put(Label{Value: opts.Value, Color: opts.Color, FontSize: opts.FontSize, DPI: opts.DPI})
	bundle.Put(scene.InputBlocker2D{Width: width, Height: height, PassThrough: opts.PassThrough})
	bundle.PutEntity(func(entity ecs.Entity) Relation[scene.Text2D] {
		return Relation[scene.Text2D]{Owner: entity}
	})
	bundle.Put(Part[LabelPart]{})
	entity := world.Spawn(bundle)
	return entity
}

func SpawnButton(world *ecs.World, opts ButtonOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	textTheme := theme
	textTheme.FontSize = sizeOr(opts.FontSize, theme.FontSize)
	textTheme.DPI = sizeOr(opts.DPI, theme.DPI)
	width := opts.Width
	if width <= 0 {
		width = measuredButtonWidth(textTheme, opts.Label, opts.Icon)
	}
	color := theme.Primary
	if opts.Variant == ButtonSurface {
		color = theme.Surface
	}
	bundle := newInteractiveBundle(ControlOptions{X: opts.X, Y: opts.Y, Width: width, Height: opts.Height, StackIndex: theme.StackIndex, Disabled: opts.Disabled, PassThrough: opts.PassThrough}, 8)
	bundle.Put(opts.Radius)
	bundle.Put(Button{})
	bundle.Put(Content[string]{Value: opts.Label})
	bundle.Put(IconContent{Value: opts.Icon})
	bundle.Put(opts.ContentAlign)
	bundle.Put(Typography{FontSize: opts.FontSize, DPI: opts.DPI})
	bundle.Put(opts.Variant)
	style := buttonStyleFor(skinFor(world, theme), opts.Variant, false)
	if opts.Style != nil {
		style = *opts.Style
		bundle.Put(StyleOverride[ButtonStyle]{Value: style})
	}
	entity := world.Spawn(bundle)
	body := spawnControlBody(world, entity, width, opts.Height, color, theme.StackIndex, opts.Radius)
	applySurface(world, body, width, opts.Height, style.Body.Resolve(controlStateForValues(false, false, false, opts.Disabled)))
	spawnJoinedEdges(world, entity, opts.JoinLeft, opts.JoinRight, theme)
	iconText := spawnText[IconPart](world, entity, textIconX, 0, iconFallback(opts.Icon), textTheme)
	centerTextVertically(world, iconText, opts.Height)
	spawnSurfacePart[IconPart](world, entity, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex+2)
	iconWidth := measureIconWidth(textTheme, opts.Icon)
	if opts.Label != "" {
		x := float32(14)
		if iconWidth > 0 {
			x = buttonLabelX(width, iconWidth, measuredButtonLabelWidth(textTheme, opts.Label), opts.ContentAlign)
		} else {
			x = buttonLabelX(width, 0, measuredButtonLabelWidth(textTheme, opts.Label), opts.ContentAlign)
		}
		text := spawnText[LabelPart](world, entity, x, 0, opts.Label, textTheme)
		centerTextVertically(world, text, opts.Height)
	}
	return entity
}

func measuredButtonLabelWidth(theme Theme, label string) float32 {
	measured := float32(len([]rune(label))) * float32(theme.FontSize) * 0.58
	if theme.Font != nil {
		face := theme.Font.FontFace(theme.FontSize, theme.DPI)
		width, _, _ := theme.Font.MeasureString(face, label)
		measured = float32(width)
	}
	return measured
}

func buttonContentX(width, iconWidth, labelWidth float32, alignment ButtonContentAlignment) float32 {
	if alignment != ButtonContentCenter || iconWidth <= 0 {
		return textIconX
	}
	contentWidth := iconWidth
	if labelWidth > 0 {
		contentWidth += textIconGap + labelWidth
	}
	return (width - contentWidth) * 0.5
}

func buttonLabelX(width, iconWidth, labelWidth float32, alignment ButtonContentAlignment) float32 {
	if iconWidth <= 0 {
		return (width - labelWidth) * 0.5
	}
	return buttonContentX(width, iconWidth, labelWidth, alignment) + iconWidth + textIconGap
}

func SpawnButtonGroup[T any](world *ecs.World, opts ButtonGroupOptions[T]) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	style := skinFor(world, theme).ButtonGroup
	if opts.Style != nil {
		style = *opts.Style
	}
	transform := scene.NewUITransform()
	transform.Translation.X.Value = opts.X
	transform.Translation.Y.Value = opts.Y
	group := world.Spawn(newSurfaceBundle(transform, 0, opts.Height, scene.RoundedRectShape2D(graphics.ColorRGBA{}, 0), theme.StackIndex, 0))
	setCornerRadius(world, group, opts.Radius)
	buttons := make([]ecs.Entity, 0, len(opts.Items))
	x := float32(0)
	for index, item := range opts.Items {
		button := SpawnButton(world, ButtonOptions{
			X: x, Width: item.Width, Height: opts.Height,
			Label: item.Label, Icon: item.Icon,
			Radius: opts.Radius, Disabled: item.Disabled, PassThrough: opts.PassThrough,
			FontSize: opts.FontSize, DPI: opts.DPI, Theme: theme, Style: &style.Button,
		})
		world.Remove[StyleOverride[ButtonStyle]](button)
		_ = world.Insert(button, scene.Parent{Entity: group})
		_ = world.Insert(button, buttonGroupItem{Group: group, Index: index})
		_ = world.Insert(button, ButtonGroupValue[T]{Group: group, Index: index, Value: item.Value})
		width, _, _ := ControlBounds(world, button)
		x += width
		buttons = append(buttons, button)
		if opts.Radius > 0 || opts.Radius == Circle {
			if index > 0 {
				spawnJoinedEdge(world, button, joinedEdgeLeft, theme)
			}
			if index < len(opts.Items)-1 {
				spawnJoinedEdge(world, button, joinedEdgeRight, theme)
			}
		}
		if opts.Divider != nil && index < len(opts.Items)-1 {
			spawnButtonGroupDivider(world, group, index, x, opts.Height, *opts.Divider, theme)
		}
	}
	_ = world.Insert(group, ButtonGroup{Buttons: buttons, Width: x, Height: opts.Height, StackIndex: theme.StackIndex})
	putStyleOverride(world, group, opts.Style)
	frame := style.Frame
	frame.Border = scene.Shape2D{}
	applySurface(world, group, x, opts.Height, frame)
	border := spawnSurfacePart[FrameBorderPart](world, group, 0, 0, x, opts.Height, graphics.ColorRGBA{}, theme.StackIndex+3)
	setCornerRadius(world, border, opts.Radius)
	applySurface(world, border, x, opts.Height, SurfaceStyle{
		Background: scene.RoundedRectShape2D(graphics.ColorRGBA{}, 0),
		Border:     style.Frame.Border,
	})
	return group
}

func spawnButtonGroupDivider(world *ecs.World, group ecs.Entity, index int, x, height float32, opts ButtonGroupDividerOptions, theme Theme) {
	width := opts.Width
	if width <= 0 {
		width = 1
	}
	inset := min(max(opts.Inset, 0), height*0.5)
	color := colorOr(opts.Color, alphaColor(theme.Muted, 0.48))
	divider := spawnBox(world, x-width*0.5, inset, width, height-inset*2, theme.StackIndex+2, color)
	_ = world.Insert(divider, scene.Parent{Entity: group})
	_ = world.Insert(divider, buttonGroupDivider{Group: group, Index: index, Width: width, Inset: inset, Color: color, CustomColor: opts.Color != (graphics.ColorRGBA{})})
}

func measuredButtonWidth(theme Theme, label string, icon IconStyle) float32 {
	labelWidth := float64(len([]rune(label))) * theme.FontSize * 0.58
	if theme.Font != nil {
		face := theme.Font.FontFace(theme.FontSize, theme.DPI)
		measured, _, _ := theme.Font.MeasureString(face, label)
		labelWidth = float64(measured)
	}
	iconWidth := float64(measureIconWidth(theme, icon))
	switch {
	case !icon.Empty() && label != "":
		return textIconX + float32(iconWidth) + textIconGap + float32(labelWidth) + 14
	case !icon.Empty():
		return textIconX + float32(iconWidth) + 12
	default:
		return float32(labelWidth) + 28
	}
}

func spawnTextControl[T Input | Textarea](world *ecs.World, opts TextOptions, component T) (ecs.Entity, Theme) {
	theme := themeForWorld(world, opts.Theme)
	bundle := newInteractiveBundle(ControlOptions{X: opts.X, Y: opts.Y, Width: opts.Width, Height: opts.Height, StackIndex: theme.StackIndex, Disabled: opts.Disabled, PassThrough: opts.PassThrough}, 6)
	bundle.Put(opts.Radius)
	bundle.Put(component)
	bundle.Put(newTextField(opts.Value, opts.Placeholder, opts.Password, opts.MaxRunes, opts.FontSize, opts.DPI))
	bundle.Put(scene.ClipRect2D{Width: opts.Width, Height: opts.Height})
	if opts.Password {
		bundle.Put(textFieldMaskCache{})
	}
	if opts.Style != nil {
		bundle.Put(StyleOverride[TextFieldStyle]{Value: *opts.Style})
	}
	entity := world.Spawn(bundle)
	spawnControlBody(world, entity, opts.Width, opts.Height, theme.Surface, theme.StackIndex, opts.Radius)
	spawnJoinedEdges(world, entity, opts.JoinLeft, opts.JoinRight, theme)
	return entity, theme
}

func SpawnInput(world *ecs.World, opts TextOptions) ecs.Entity {
	entity, theme := spawnTextControl(world, opts, Input{})
	valueText := spawnText[ValuePart](world, entity, 12, textBaseline(opts.Height), inputDisplay(opts.Value, opts.Placeholder, opts.Password, false, "", 0), theme)
	centerTextVertically(world, valueText, opts.Height)
	spawnTextFieldElements(world, entity, theme)
	return entity
}

func SpawnNumberInput(world *ecs.World, opts NumberInputOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	stepButtonWidth := numberInputStepButtonWidth(opts.StepButtonWidth)
	upIcon := iconWithFallback(opts.UpIcon, "^")
	downIcon := iconWithFallback(opts.DownIcon, "v")
	entity := spawnInteractiveBox(world, opts.X, opts.Y, opts.Width, opts.Height, theme.StackIndex, theme.Surface, opts.Disabled, opts.PassThrough)
	setCornerRadius(world, entity, opts.Radius)
	step := opts.Step
	if step == 0 {
		step = 1
	}
	_ = world.Insert(entity, NumberInput{
		Step:            step,
		Integer:         opts.Integer,
		StepButtonWidth: stepButtonWidth,
		UpIcon:          upIcon,
		DownIcon:        downIcon,
	})
	putStyleOverride(world, entity, opts.Style)
	_ = world.Insert(entity, newTextField(opts.Value, opts.Placeholder, false, opts.MaxRunes, opts.FontSize, opts.DPI))
	_ = world.Insert(entity, scene.ClipRect2D{Width: opts.Width, Height: opts.Height})
	valueText := spawnText[ValuePart](world, entity, 12, textBaseline(opts.Height), inputDisplay(opts.Value, opts.Placeholder, false, false, "", 0), theme)
	centerTextVertically(world, valueText, opts.Height)
	spawnTextFieldElements(world, entity, theme)
	up := spawnInteractiveBox(world, opts.Width-stepButtonWidth, 0, stepButtonWidth, opts.Height*0.5, theme.StackIndex+1, theme.Surface, opts.Disabled, opts.PassThrough)
	setCornerRadius(world, up, opts.Radius)
	_ = world.Insert(up, scene.Parent{Entity: entity})
	_ = world.Insert(up, NumberInputStep{Input: entity, Direction: 1})
	spawnJoinedEdge(world, up, joinedEdgeBottom, theme)
	spawnText[IconPart](world, up, stepButtonTextX(stepButtonWidth), stepButtonTextY(opts.Height*0.5), iconFallback(upIcon), theme)
	spawnSurfacePart[NumberStepIconPart](world, up, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex+3)
	down := spawnInteractiveBox(world, opts.Width-stepButtonWidth, opts.Height*0.5, stepButtonWidth, opts.Height*0.5, theme.StackIndex+1, theme.Surface, opts.Disabled, opts.PassThrough)
	setCornerRadius(world, down, opts.Radius)
	_ = world.Insert(down, scene.Parent{Entity: entity})
	_ = world.Insert(down, NumberInputStep{Input: entity, Direction: -1})
	spawnJoinedEdge(world, down, joinedEdgeTop, theme)
	spawnText[IconPart](world, down, stepButtonTextX(stepButtonWidth), stepButtonTextY(opts.Height*0.5), iconFallback(downIcon), theme)
	spawnSurfacePart[NumberStepIconPart](world, down, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex+3)
	border := spawnSurfacePart[NumberInputBorderPart](world, entity, 0, 0, opts.Width, opts.Height, graphics.ColorRGBA{}, theme.StackIndex+4)
	setCornerRadius(world, border, opts.Radius)
	return entity
}

func SpawnTextarea(world *ecs.World, opts TextOptions) ecs.Entity {
	opts.Password = false
	entity, theme := spawnTextControl(world, opts, Textarea{Wrap: opts.Wrap})
	spawnText[ValuePart](world, entity, textareaTextPaddingX, textareaTextPaddingY, inputDisplay(opts.Value, opts.Placeholder, false, false, "", 0), theme)
	spawnTextFieldElements(world, entity, theme)
	for _, child := range []ecs.Entity{
		spawnSurfacePart[ScrollTrackHorizontalPart](world, entity, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex),
		spawnSurfacePart[ScrollTrackVerticalPart](world, entity, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex),
		spawnSurfacePart[ScrollThumbHorizontalPart](world, entity, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex+1),
		spawnSurfacePart[ScrollThumbVerticalPart](world, entity, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex+1),
	} {
		_ = world.Insert(child, scene.ClipExempt2D{})
	}
	return entity
}

func newTextField(value, placeholder string, password bool, maxRunes int, fontSize, dpi float64) TextField {
	return TextField{
		Value:       value,
		Placeholder: placeholder,
		Password:    password,
		Caret:       runeCount(value),
		MaxRunes:    maxRunes,
		FontSize:    fontSize,
		DPI:         dpi,
	}
}

func SpawnMenu(world *ecs.World, opts MenuOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	style := skinFor(world, theme).Menu
	if opts.Style != nil {
		style = *opts.Style
	}
	padding := menuPadding(opts.Padding)
	height := opts.Height
	if height <= 0 {
		height = 32
	}
	width := measuredMenuWidth(theme, opts.Width, padding, append([]string{opts.Label}, opts.Items...))
	entity := SpawnButton(world, ButtonOptions{
		X: opts.X, Y: opts.Y, Width: width, Height: height,
		Label: opts.Label, Icon: style.Indicator, Radius: opts.Radius, PassThrough: opts.PassThrough, Theme: theme, Style: &style.Trigger,
	})
	world.Remove[StyleOverride[ButtonStyle]](entity)
	putStyleOverride(world, entity, opts.Style)
	_ = world.Insert(entity, Menu{Label: opts.Label, Selected: -1, Width: opts.Width, ItemHeight: height, Padding: padding, StackIndex: theme.StackIndex})
	panel := spawnSurfacePart[MenuPanelPart](world, entity, 0, height, width, padding*2+height*float32(len(opts.Items)), theme.Surface, theme.StackIndex+1)
	setCornerRadius(world, panel, opts.Radius)
	_ = world.Insert(panel, scene.InputBlocker2D{Width: width, Height: padding*2 + height*float32(len(opts.Items)), Receiver: entity, PassThrough: opts.PassThrough})
	setVisible(world, panel, false)
	for i, label := range opts.Items {
		item := spawnInteractiveBox(world, padding, height+padding+float32(i)*height, width-padding*2, height, theme.StackIndex+2, theme.Surface, false, opts.PassThrough)
		setCornerRadius(world, item, opts.Radius)
		_ = world.Insert(item, scene.Parent{Entity: entity})
		_ = world.Insert(item, MenuItem{Owner: entity, Index: i, Label: label})
		text := spawnText[ItemPart](world, item, menuTextPaddingX, 0, label, theme)
		centerTextVertically(world, text, height)
		setVisible(world, item, false)
	}
	return entity
}

func SpawnContextMenu(world *ecs.World, opts ContextMenuOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	itemHeight := opts.ItemHeight
	if itemHeight == 0 {
		itemHeight = 32
	}
	padding := menuPadding(opts.Padding)
	width := measuredMenuWidth(theme, opts.Width, padding, opts.Items)
	entity := spawnAnchor(world, opts.X, opts.Y)
	_ = world.Insert(entity, scene.InputBlocker2D{Width: width, Height: padding*2 + itemHeight*float32(len(opts.Items)), PassThrough: opts.PassThrough})
	setUIStackIndex(world, entity, theme.StackIndex+1)
	_ = world.Insert(entity, ContextMenu{
		Open:       opts.Open,
		Selected:   -1,
		Width:      opts.Width,
		ItemHeight: itemHeight,
		Padding:    padding,
		StackIndex: theme.StackIndex,
	})
	putStyleOverride(world, entity, opts.Style)
	panel := spawnSurfacePart[ContextMenuPanelPart](world, entity, 0, 0, width, padding*2+itemHeight*float32(len(opts.Items)), theme.Surface, theme.StackIndex+1)
	setCornerRadius(world, panel, opts.Radius)
	for i, label := range opts.Items {
		item := spawnInteractiveBox(world, padding, padding+itemHeight*float32(i), width-padding*2, itemHeight, theme.StackIndex+2, theme.Surface, false, opts.PassThrough)
		setCornerRadius(world, item, opts.Radius)
		_ = world.Insert(item, scene.Parent{Entity: entity})
		_ = world.Insert(item, ContextMenuItem{Owner: entity, Index: i, Label: label})
		text := spawnText[ItemPart](world, item, menuTextPaddingX, 0, label, theme)
		centerTextVertically(world, text, itemHeight)
		setVisible(world, item, opts.Open)
	}
	setVisible(world, entity, opts.Open)
	return entity
}

func SpawnCheckbox(world *ecs.World, opts CheckboxOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	entity := spawnInteractiveAnchor(world, opts.X, opts.Y, opts.Width, opts.Height, theme.StackIndex, opts.Disabled, opts.PassThrough)
	_ = world.Insert(entity, Toggle[bool]{Value: opts.Checked, On: true})
	_ = world.Insert(entity, Content[string]{Value: opts.Label})
	_ = world.Insert(entity, Typography{FontSize: opts.FontSize, DPI: opts.DPI})
	putStyleOverride(world, entity, opts.Style)
	boxSize := checkboxBoxSize(opts.Height)
	boxY := (opts.Height - boxSize) * 0.5
	track := spawnSurfacePart[TrackPart](world, entity, 0, boxY, boxSize, boxSize, theme.Surface, theme.StackIndex)
	setCornerRadius(world, track, opts.Radius)
	spawnSurfacePart[KnobPart](world, entity, 0, boxY, boxSize, boxSize, theme.Primary, theme.StackIndex+1)
	spawnText[LabelPart](world, entity, boxSize+12, textBaseline(opts.Height), opts.Label, theme)
	return entity
}

func SpawnSelector(world *ecs.World, opts SelectorOptions) ecs.Entity {
	entity := spawnAnchor(world, opts.X, opts.Y)
	_ = BindSelector(world, entity, opts)
	return entity
}

// BindSelector builds a data-driven selector on an existing UI entity.
// Its list and option entities are runtime-owned and hidden until opened.
func BindSelector(world *ecs.World, entity ecs.Entity, opts SelectorOptions) error {
	if world == nil || entity == 0 {
		return fmt.Errorf("UI selector target is invalid")
	}
	if world.Has[Selector](entity) {
		return fmt.Errorf("UI selector target %d is already bound", entity)
	}
	theme := themeForWorld(world, opts.Theme)
	icon := iconWithFallback(opts.Icon, "v")
	itemHeight := opts.ItemHeight
	if itemHeight <= 0 {
		itemHeight = opts.Height
	}
	selected := opts.Selected
	if selected < 0 || selected >= len(opts.Options) {
		selected = -1
	}
	if err := BindControl(world, entity, ControlOptions{
		Width: opts.Width, Height: opts.Height, StackIndex: theme.StackIndex,
		Disabled: opts.Disabled, PassThrough: opts.PassThrough,
	}, Selector{
		Label: opts.Label, Selected: selected, Icon: icon, ItemHeight: itemHeight,
		ListHeight: opts.ListHeight, Upward: opts.Upward, StackIndex: theme.StackIndex,
	}); err != nil {
		return err
	}
	spawnSurfacePart[BodyPart](world, entity, 0, 0, opts.Width, opts.Height, theme.Surface, theme.StackIndex)
	setCornerRadius(world, entity, opts.Radius)
	spawnJoinedEdges(world, entity, opts.JoinLeft, opts.JoinRight, theme)
	putStyleOverride(world, entity, opts.Style)
	spawnText[LabelPart](world, entity, 12, textBaseline(opts.Height), selectorLabel(opts.Label, opts.Options, selected), theme)
	spawnText[IconPart](world, entity, opts.Width-26, textBaseline(opts.Height), iconFallback(icon), theme)
	spawnSurfacePart[SelectorIconPart](world, entity, opts.Width-26, (opts.Height-12)*0.5, 12, 12, graphics.ColorRGBA{}, theme.StackIndex+2)
	list := spawnAnchor(world, 0, opts.Height)
	_ = world.Insert(list, scene.Parent{Entity: entity})
	setUIStackIndex(world, list, selectorPopupStackBase)
	_ = world.Insert(list, SelectorList{Selector: entity})
	_ = world.Insert(list, scene.ClipRect2D{Width: opts.Width, Height: opts.ListHeight})
	_ = world.Insert(list, scene.Size2D{Width: opts.Width, Height: opts.ListHeight})
	_ = world.Insert(list, scene.UIScroll{Position: mgl32.Vec2{0, -opts.ScrollY}, WheelStep: itemHeight, ConsumeWheel: true, Vertical: true})
	panel := spawnSurfacePart[SelectorPanelPart](world, list, 0, 0, opts.Width, opts.ListHeight, theme.Surface, selectorPopupStackBase+1)
	setCornerRadius(world, panel, opts.Radius)
	track := spawnSurfacePart[SelectorScrollTrackPart](world, list, 0, 0, 0, 0, graphics.ColorRGBA{}, selectorPopupStackBase+3)
	thumb := spawnSurfacePart[SelectorScrollThumbPart](world, list, 0, 0, 0, 0, graphics.ColorRGBA{}, selectorPopupStackBase+4)
	border := spawnSurfacePart[SelectorPanelBorderPart](world, list, 0, 0, opts.Width, opts.ListHeight, graphics.ColorRGBA{}, selectorPopupStackBase+4)
	setCornerRadius(world, track, opts.Radius)
	setCornerRadius(world, thumb, opts.Radius)
	setCornerRadius(world, border, opts.Radius)
	for i, label := range opts.Options {
		item := spawnInteractiveBox(world, 0, float32(i)*itemHeight, opts.Width, itemHeight, selectorPopupStackBase+2, theme.Surface, false, opts.PassThrough)
		setCornerRadius(world, item, opts.Radius)
		_ = world.Insert(item, scene.Parent{Entity: list})
		_ = world.Insert(item, SelectorItem{Owner: entity, Index: i, Label: label})
		spawnText[ItemPart](world, item, 12, textBaseline(itemHeight), label, theme)
		setVisible(world, item, false)
	}
	setVisible(world, list, false)
	return nil
}

func SpawnTab(world *ecs.World, opts TabOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	headerHeight := opts.HeaderHeight
	if headerHeight == 0 {
		headerHeight = 36
	}
	selected := opts.Selected
	if len(opts.Items) == 0 {
		selected = -1
	} else if selected < 0 || selected >= len(opts.Items) {
		selected = 0
	}
	entity := spawnBox(world, opts.X, opts.Y, opts.Width, opts.Height, theme.StackIndex, theme.Background)
	setCornerRadius(world, entity, opts.Radius)
	_ = world.Insert(entity, scene.InputBlocker2D{Width: opts.Width, Height: opts.Height, PassThrough: opts.PassThrough})
	_ = world.Insert(entity, Tab{
		Selected:     selected,
		Width:        opts.Width,
		Height:       opts.Height,
		HeaderHeight: headerHeight,
		Highlight:    colorOr(opts.Highlight, theme.Accent),
		ItemBorders:  opts.ItemBorders,
	})
	putStyleOverride(world, entity, opts.Style)
	border := spawnSurfacePart[FrameBorderPart](world, entity, 0, 0, opts.Width, headerHeight, graphics.ColorRGBA{}, theme.StackIndex+3)
	setCornerRadius(world, border, opts.Radius)
	if len(opts.Items) == 0 {
		return entity
	}
	itemWidth := opts.Width / float32(len(opts.Items))
	for i, spec := range opts.Items {
		item := spawnInteractiveBox(world, float32(i)*itemWidth, 0, itemWidth, headerHeight, theme.StackIndex+1, theme.Surface, false, opts.PassThrough)
		setCornerRadius(world, item, opts.Radius)
		_ = world.Insert(item, scene.Parent{Entity: entity})
		_ = world.Insert(item, TabItem{Tabs: entity, Index: i, Label: spec.Label, Icon: spec.Icon, Disabled: spec.Disabled})
		if opts.ItemBorders {
			for side := TabItemBorderTop; side <= TabItemBorderLeft; side++ {
				border := spawnSurfacePart[TabItemBorderPart](world, item, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex+3)
				_ = world.Insert(border, TabItemBorder{Side: side})
			}
		}
		SetControlDisabled(world, item, spec.Disabled)
		if opts.Radius > 0 || opts.Radius == Circle {
			if i > 0 {
				spawnJoinedEdge(world, item, joinedEdgeLeft, theme)
			}
			if i < len(opts.Items)-1 {
				spawnJoinedEdge(world, item, joinedEdgeRight, theme)
			}
		}
		labelX := textIconX
		iconText := spawnText[IconPart](world, item, textIconX, 0, iconFallback(spec.Icon), theme)
		centerTextVertically(world, iconText, headerHeight)
		if iconWidth := measureIconWidth(theme, spec.Icon); iconWidth > 0 {
			labelX += iconWidth + textIconGap
		}
		spawnSurfacePart[IconPart](world, item, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex+2)
		labelText := spawnText[ItemPart](world, item, labelX, 0, spec.Label, theme)
		centerTextVertically(world, labelText, headerHeight)
	}
	return entity
}

func spawnJoinedEdge(world *ecs.World, owner ecs.Entity, side joinedEdgeSide, theme Theme) {
	edge := spawnBox(world, 0, 0, 0, 0, theme.StackIndex+1, theme.Surface)
	_ = world.Insert(edge, scene.Parent{Entity: owner})
	_ = world.Insert(edge, joinedEdge{Owner: owner, Side: side})
}

func spawnJoinedEdges(world *ecs.World, owner ecs.Entity, left, right bool, theme Theme) {
	if left {
		spawnJoinedEdge(world, owner, joinedEdgeLeft, theme)
	}
	if right {
		spawnJoinedEdge(world, owner, joinedEdgeRight, theme)
	}
}

func SpawnTabPanel(world *ecs.World, tabs ecs.Entity, opts TabPanelOptions) ecs.Entity {
	tab := world.GetRef[Tab](tabs)
	if tab == nil {
		return 0
	}
	theme := themeForWorld(world, opts.Theme)
	height := tab.Height - tab.HeaderHeight
	if height < 0 {
		height = 0
	}
	entity := spawnBox(world, 0, tab.HeaderHeight, tab.Width, height, theme.StackIndex, alphaColor(theme.Surface, 0.32))
	setCornerRadius(world, entity, opts.Radius)
	_ = world.Insert(entity, scene.InputBlocker2D{Width: tab.Width, Height: height, PassThrough: opts.PassThrough})
	_ = world.Insert(entity, scene.Parent{Entity: tabs})
	_ = world.Insert(entity, TabPanel{Tabs: tabs, Index: opts.Index})
	putStyleOverride(world, entity, opts.Style)
	setVisible(world, entity, tab.Selected == opts.Index)
	return entity
}

func SpawnProgress(world *ecs.World, opts ProgressOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	entity := spawnInteractiveBox(world, opts.X, opts.Y, opts.Width, opts.Height, theme.StackIndex, theme.Surface, false, opts.PassThrough)
	setCornerRadius(world, entity, opts.Radius)
	fill := spawnBox(world, 0, 0, 0, opts.Height, theme.StackIndex+1, theme.Accent)
	setCornerRadius(world, fill, opts.Radius)
	_ = world.Insert(fill, scene.Parent{Entity: entity})
	_ = world.Insert(fill, Relation[Progress]{Owner: entity})
	_ = world.Insert(fill, Part[FillPart]{})
	thumbSize := opts.ThumbSize
	if thumbSize <= 0 {
		thumbSize = max(opts.Height+6, 14)
	}
	thumb := spawnInteractiveBox(world, -thumbSize*.5, (opts.Height-thumbSize)*.5, thumbSize, thumbSize, theme.StackIndex+2, theme.Text, false, opts.PassThrough)
	setCornerRadius(world, thumb, Circle)
	_ = world.Insert(thumb, scene.Parent{Entity: entity})
	_ = world.Insert(thumb, ProgressThumb{Progress: entity})
	setVisible(world, thumb, opts.Slider && !opts.Indeterminate)
	_ = world.Insert(entity, Progress{
		Min: opts.Min, Max: opts.Max, Value: opts.Value, Indeterminate: opts.Indeterminate,
		Slider: opts.Slider, Fill: fill, Thumb: thumb,
	})
	putStyleOverride(world, entity, opts.Style)
	return entity
}

func SpawnScrollArea(world *ecs.World, opts ScrollAreaOptions) ecs.Entity {
	theme := themeForWorld(world, opts.Theme)
	entity := spawnInteractiveAnchor(world, opts.X, opts.Y, opts.Width, opts.Height, theme.StackIndex, false, opts.PassThrough)
	area := ScrollArea{
		ReserveScrollbarSpace: opts.ReserveScrollbarSpace,
		Width:                 opts.Width,
		Height:                opts.Height,
		ContentWidth:          opts.ContentWidth,
		ContentHeight:         opts.ContentHeight,
		Controlled:            opts.Controlled,
		DisableDrag:           opts.DisableDrag,
		WheelStep:             opts.WheelStep,
		BarSize:               opts.BarSize,
		MinThumbSize:          opts.MinThumbSize,
		ThumbBarSize:          opts.ThumbBarSize,
		HoverBarSize:          opts.HoverBarSize,
		StackIndex:            theme.StackIndex,
		ScrollBarStackIndex:   opts.ScrollBarStackIndex,
	}
	_ = world.Insert(entity, area)
	_ = installScrollAreaGeometry(world, entity, &area)
	SetScrollAreaOffset(world, entity, mgl32.Vec2{opts.ScrollX, opts.ScrollY})
	putStyleOverride(world, entity, opts.Style)
	for _, child := range []ecs.Entity{
		spawnSurfacePart[ScrollTrackHorizontalPart](world, entity, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex),
		spawnSurfacePart[ScrollTrackVerticalPart](world, entity, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex),
		spawnSurfacePart[ScrollThumbHorizontalPart](world, entity, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex+1),
		spawnSurfacePart[ScrollThumbVerticalPart](world, entity, 0, 0, 0, 0, graphics.ColorRGBA{}, theme.StackIndex+1),
	} {
		setCornerRadius(world, child, opts.Radius)
		_ = world.Insert(child, scene.ClipExempt2D{ParentOnly: true})
	}
	return entity
}

func SpawnScrollContent(world *ecs.World, area ecs.Entity, x, y float32) ecs.Entity {
	entity := spawnAnchor(world, x, y)
	_ = world.Insert(entity, scene.Parent{Entity: area})
	_ = world.Insert(entity, ScrollContent{Area: area, X: x, Y: y})
	return entity
}

func newUIBundle(transform scene.UITransform, stackIndex, capacity int) ecs.Bundle {
	bundle := ecs.NewBundle(capacity)
	bundle.Put(transform)
	bundle.Put(scene.UIGlobalTransform{})
	bundle.Put(scene.Visibility{Visible: true})
	bundle.Put(scene.AnchorTopLeft)
	bundle.Put(scene.UIStackIndex(stackIndex))
	return bundle
}

func newInteractiveBundle(opts ControlOptions, additional int) ecs.Bundle {
	bundle := newUIBundle(scene.UITransformFromXY(opts.X, opts.Y), opts.StackIndex, 9+additional)
	bundle.Put(scene.Size2D{Width: opts.Width, Height: opts.Height})
	bundle.Put(Control{})
	bundle.Put(scene.InputTarget2D{Width: opts.Width, Height: opts.Height, PassThrough: opts.PassThrough, Disabled: opts.Disabled})
	bundle.Put(scene.InputState2D{})
	return bundle
}

func spawnInteractiveAnchor(world *ecs.World, x, y, width, height float32, stackIndex int, disabled, passThrough bool) ecs.Entity {
	return world.Spawn(newInteractiveBundle(ControlOptions{X: x, Y: y, Width: width, Height: height, StackIndex: stackIndex, Disabled: disabled, PassThrough: passThrough}, 0))
}

func spawnInteractiveBox(world *ecs.World, x, y, width, height float32, stackIndex int, color graphics.ColorRGBA, disabled, passThrough bool) ecs.Entity {
	entity := spawnInteractiveAnchor(world, x, y, width, height, stackIndex, disabled, passThrough)
	spawnSurfacePart[BodyPart](world, entity, 0, 0, width, height, color, stackIndex)
	return entity
}

func spawnAnchor(world *ecs.World, x, y float32) ecs.Entity {
	transform := scene.NewUITransform()
	transform.Translation.X.Value = x
	transform.Translation.Y.Value = y
	spawner := world.NewSpawner4[scene.UITransform, scene.UIGlobalTransform, scene.Visibility, scene.Anchor]()
	return spawner.Spawn(transform, scene.UIGlobalTransform{}, scene.Visibility{Visible: true}, scene.AnchorTopLeft)
}

func spawnBox(world *ecs.World, x, y, width, height float32, stackIndex int, color graphics.ColorRGBA) ecs.Entity {
	return world.Spawn(newSurfaceBundle(scene.UITransformFromXY(x, y), width, height, scene.RoundedRectShape2D(color, 0), stackIndex, 0))
}

func newSurfaceBundle(transform scene.UITransform, width, height float32, shape scene.Shape2D, stackIndex, additional int) ecs.Bundle {
	bundle := newUIBundle(transform, stackIndex, 9+additional)
	bundle.Put(scene.Size2D{Width: width, Height: height})
	bundle.Put(scene.NewUIBackground2D(shape))
	bundle.Put(scene.NewUIImage2D(nil))
	bundle.Put(scene.NewUIBorder2D(scene.Shape2D{}))
	return bundle
}

func setCornerRadius(world *ecs.World, entity ecs.Entity, radius CornerRadius) {
	_ = world.Insert(entity, radius)
	children := make([]ecs.Entity, 0, 1)
	for child, access := range world.Query[Relation[SurfaceStyle]]().All {
		element := access.Mut()

		if element.Owner == entity && world.Has[Part[BodyPart]](child) {
			children = append(children, child)
		}
	}
	for _, child := range children {
		_ = world.Insert(child, radius)
	}
	size, _ := world.Get[scene.Size2D](entity)
	pixels := radius.pixels(size.Width, size.Height)
	if background := world.GetMut[scene.UIBackground2D](entity); background != nil && background.Shape.Kind == render.ShapeRoundedRect {
		background.Shape.Radius = pixels
	}
	if border := world.GetMut[scene.UIBorder2D](entity); border != nil && border.Shape.Kind == render.ShapeRoundedRect {
		border.Shape.Radius = pixels
	}
}

func newSurfacePartBundle[P any](parent ecs.Entity, x, y, width, height float32, shape scene.Shape2D, stackIndex, additional int) ecs.Bundle {
	bundle := newSurfaceBundle(scene.UITransformFromXY(x, y), width, height, shape, stackIndex, 3+additional)
	bundle.Put(scene.Parent{Entity: parent})
	bundle.Put(Relation[SurfaceStyle]{Owner: parent})
	bundle.Put(Part[P]{})
	return bundle
}

func spawnSurfacePart[P any](world *ecs.World, parent ecs.Entity, x, y, width, height float32, color graphics.ColorRGBA, stackIndex int) ecs.Entity {
	return world.Spawn(newSurfacePartBundle[P](parent, x, y, width, height, scene.RoundedRectShape2D(color, 0), stackIndex, 0))
}

func spawnControlBody(world *ecs.World, owner ecs.Entity, width, height float32, color graphics.ColorRGBA, stackIndex int, radius CornerRadius) ecs.Entity {
	bundle := newSurfacePartBundle[BodyPart](owner, 0, 0, width, height, scene.RoundedRectShape2D(color, radius.pixels(width, height)), stackIndex, 1)
	bundle.Put(radius)
	return world.Spawn(bundle)
}

func spawnTextFieldElements(world *ecs.World, parent ecs.Entity, theme Theme) {
	for index := range 16 {
		entity := spawnSurfacePart[SelectionPart](world, parent, 0, 0, 1, 1, theme.Primary, theme.StackIndex+1)
		_ = world.Insert(entity, Indexed[SelectionPart]{Value: index})
		setVisible(world, entity, false)
	}
	entity := spawnSurfacePart[CaretPart](world, parent, 0, 0, 1, 1, theme.Text, theme.StackIndex+3)
	setVisible(world, entity, false)
}

func spawnText[P any](world *ecs.World, parent ecs.Entity, x, y float32, value string, theme Theme) ecs.Entity {
	text := scene.NewText2D(value)
	text.Font = theme.Font
	text.FontSize = theme.FontSize
	text.DPI = theme.DPI
	text.Color = theme.Text
	bundle := newUIBundle(scene.UITransformFromXY(x, y), theme.StackIndex+2, 9)
	bundle.Put(scene.Parent{Entity: parent})
	bundle.Put(text)
	bundle.Put(Relation[scene.Text2D]{Owner: parent})
	bundle.Put(Part[P]{})
	return world.Spawn(bundle)
}

func setUIStackIndex(world *ecs.World, entity ecs.Entity, stackIndex int) {
	if world.Has[scene.UIStackIndex](entity) {
		world.SetIfChanged(entity, scene.UIStackIndex(stackIndex))
		return
	}
	_ = world.Insert(entity, scene.UIStackIndex(stackIndex))
}

func setVisible(world *ecs.World, entity ecs.Entity, visible bool) {
	world.SetIfChanged(entity, scene.Visibility{Visible: visible})
}

func centerTextVertically(world *ecs.World, entity ecs.Entity, height float32) {
	text := world.GetRef[scene.Text2D](entity)
	transform := world.GetRef[scene.UITransform](entity)
	if (text == nil) || (transform == nil) {
		return
	}
	_, lineHeight, ok := scene.MeasureTextValue2D(text, text.Value)
	if ok {
		next := *transform
		next.Translation.Y.Value = (height - lineHeight) * 0.5
		world.SetIfChanged(entity, next)
	}
}

func menuPadding(value float32) float32 {
	if value <= 0 {
		return defaultMenuPad
	}
	return value
}

func measuredMenuWidth(theme Theme, minWidth, padding float32, labels []string) float32 {
	width := float32(0)
	for _, label := range labels {
		labelWidth := float64(len([]rune(label))) * theme.FontSize * 0.58
		if theme.Font != nil {
			face := theme.Font.FontFace(theme.FontSize, theme.DPI)
			measured, _, _ := theme.Font.MeasureString(face, label)
			labelWidth = float64(measured)
		}
		width = max(width, float32(labelWidth))
	}
	return max(minWidth, width+menuTextPaddingX*2+padding*2)
}

func textBaseline(height float32) float32 {
	if height <= 24 {
		return 4
	}
	return (height - 20) * 0.5
}

func inputDisplay(value, placeholder string, password, focused bool, composition string, caret int) string {
	display := value
	if composition != "" {
		byteIndex := runeByteIndex(value, max(caret, 0))
		display = value[:byteIndex] + composition + value[byteIndex:]
	}
	if display == "" && !focused {
		return placeholder
	}
	if password {
		return strings.Repeat("*", runeCount(display))
	}
	return display
}

func numberInputStepButtonWidth(value float32) float32 {
	if value <= 0 {
		return 18
	}
	return value
}

func stepButtonTextX(width float32) float32 {
	if width <= 12 {
		return 3
	}
	return width*0.5 - 4
}

func stepButtonTextY(height float32) float32 {
	if height <= 18 {
		return 0
	}
	return (height - 16) * 0.5
}

func checkboxBoxSize(height float32) float32 {
	if height <= 0 {
		return 20
	}
	boxSize := height * 0.56
	if boxSize < 18 {
		return 18
	}
	if boxSize > 22 {
		return 22
	}
	return boxSize
}

func selectorLabel(label string, options []string, selected int) string {
	if selected >= 0 && selected < len(options) {
		return options[selected]
	}
	return label
}

func sizeOr(value, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}
