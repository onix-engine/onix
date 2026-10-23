package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

// ControlState is the visual input state shared by interactive widgets.
type ControlState struct {
	Hovered  bool
	Pressed  bool
	Selected bool
	Focused  bool
	Disabled bool
}

// StateStyle resolves a value for an input state. Missing overrides fall
// back to Normal.
type StateStyle[T any] struct {
	Normal   T
	Hovered  *T
	Pressed  *T
	Selected *T
	Focused  *T
	Disabled *T
}

func NewStateStyle[T any](normal T) StateStyle[T] {
	return StateStyle[T]{Normal: normal}
}

func (style StateStyle[T]) WithHovered(value T) StateStyle[T] {
	style.Hovered = &value
	return style
}

func (style StateStyle[T]) WithPressed(value T) StateStyle[T] {
	style.Pressed = &value
	return style
}

func (style StateStyle[T]) WithSelected(value T) StateStyle[T] {
	style.Selected = &value
	return style
}

func (style StateStyle[T]) WithFocused(value T) StateStyle[T] {
	style.Focused = &value
	return style
}

func (style StateStyle[T]) WithDisabled(value T) StateStyle[T] {
	style.Disabled = &value
	return style
}

func (style StateStyle[T]) Resolve(state ControlState) T {
	switch {
	case state.Disabled:
		if style.Disabled != nil {
			return *style.Disabled
		}
	case state.Pressed:
		if style.Pressed != nil {
			return *style.Pressed
		}
	case state.Selected:
		if style.Selected != nil {
			return *style.Selected
		}
	case state.Hovered:
		if style.Hovered != nil {
			return *style.Hovered
		}
	case state.Focused:
		if style.Focused != nil {
			return *style.Focused
		}
	default:
	}
	return style.Normal
}

// ToggleStyle contains input styles for off/on or unselected/selected states.
type ToggleStyle[T any] struct {
	Off StateStyle[T]
	On  StateStyle[T]
}

func (style ToggleStyle[T]) Resolve(on bool, state ControlState) T {
	if on {
		return style.On.Resolve(state)
	}
	return style.Off.Resolve(state)
}

// StyleOverride stores a strongly typed per-entity style override.
type StyleOverride[T any] struct {
	Value T
}

// SurfaceStyle describes the independently rendered background, image and border layers.
type SurfaceStyle struct {
	Background scene.Shape2D
	Image      scene.UIImage2D
	Border     scene.Shape2D
}

// IconStyle renders Texture when present and otherwise renders Fallback text.
type IconStyle struct {
	Texture  *graphics.Texture
	Fallback string
	Width    float32
	Height   float32
}

func TextIcon(value string) IconStyle {
	return IconStyle{Fallback: value}
}

func TextureIcon(texture *graphics.Texture, width, height float32) IconStyle {
	return IconStyle{Texture: texture, Width: width, Height: height}
}

func (icon IconStyle) Empty() bool {
	return icon.Texture == nil && icon.Fallback == ""
}

func iconFallback(icon IconStyle) string {
	if icon.Texture != nil {
		return ""
	}
	return icon.Fallback
}

func iconWithFallback(icon IconStyle, fallback string) IconStyle {
	if icon.Fallback == "" {
		icon.Fallback = fallback
	}
	return icon
}

func measureIconSize(theme Theme, icon IconStyle) (float32, float32) {
	if icon.Texture != nil {
		return iconTextureSize(icon)
	}
	value := iconFallback(icon)
	if value == "" || theme.Font == nil {
		return 0, 0
	}
	face := theme.Font.FontFace(theme.FontSize, theme.DPI)
	measuredWidth, measuredHeight, _ := theme.Font.MeasureString(face, value)
	width, height := float32(measuredWidth), float32(measuredHeight)
	if icon.Width > 0 {
		width = icon.Width
	}
	if icon.Height > 0 {
		height = icon.Height
	}
	return width, height
}

func measureIconWidth(theme Theme, icon IconStyle) float32 {
	width, _ := measureIconSize(theme, icon)
	return width
}

func iconTextureSize(icon IconStyle) (float32, float32) {
	if icon.Texture == nil {
		return 0, 0
	}
	width := float32(icon.Texture.LogicalWidth())
	height := float32(icon.Texture.LogicalHeight())
	if width <= 0 || height <= 0 {
		return max(icon.Width, 0), max(icon.Height, 0)
	}
	switch {
	case icon.Width > 0 && icon.Height > 0:
		scale := min(icon.Width/width, icon.Height/height)
		return width * scale, height * scale
	case icon.Width > 0:
		return icon.Width, height * icon.Width / width
	case icon.Height > 0:
		return width * icon.Height / height, icon.Height
	default:
		return width, height
	}
}

func fitIconSize(icon IconStyle, maxWidth, maxHeight float32) (float32, float32) {
	width, height := iconTextureSize(icon)
	maxWidth, maxHeight = max(maxWidth, 0), max(maxHeight, 0)
	if width <= 0 || height <= 0 || maxWidth == 0 || maxHeight == 0 {
		return 0, 0
	}
	scale := min(float32(1), min(maxWidth/width, maxHeight/height))
	return width * scale, height * scale
}

func ColorSurface(color graphics.ColorRGBA) SurfaceStyle {
	return SurfaceStyle{Background: scene.RoundedRectShape2D(color, 0)}
}

func TextureSurface(texture *graphics.Texture) SurfaceStyle {
	image := scene.NewUIImage2D(texture)
	image.Mode = scene.UIImageStretch
	return SurfaceStyle{Image: image}
}

func NineSliceSurface(texture *graphics.Texture, slice scene.NineSlice2D) SurfaceStyle {
	image := scene.NewUIImage2D(texture)
	image.Mode = scene.UIImageSliced
	image.Slice = &slice
	return SurfaceStyle{Image: image}
}

func (surface SurfaceStyle) WithBackground(shape scene.Shape2D) SurfaceStyle {
	surface.Background = shape
	return surface
}

func (surface SurfaceStyle) WithBorder(shape scene.Shape2D) SurfaceStyle {
	surface.Border = shape
	return surface
}

type PanelStyle struct {
	Body SurfaceStyle
}

type ButtonStyle struct {
	Body       StateStyle[SurfaceStyle]
	JoinedEdge StateStyle[SurfaceStyle]
}

type ButtonStyles struct {
	Primary ButtonStyle
	Surface ButtonStyle
	Danger  ButtonStyle
}

type ButtonGroupStyle struct {
	Button  ButtonStyle
	Frame   SurfaceStyle
	Divider SurfaceStyle
}

type TextFieldStyle struct {
	Body      StateStyle[SurfaceStyle]
	Caret     SurfaceStyle
	Selection SurfaceStyle
}

type NumberInputStyle struct {
	Field TextFieldStyle
	Step  StateStyle[SurfaceStyle]
}

type MenuStyle struct {
	Trigger   ButtonStyle
	Popup     SurfaceStyle
	Item      StateStyle[SurfaceStyle]
	Indicator IconStyle
}

type CheckboxStyle struct {
	Track ToggleStyle[SurfaceStyle]
	Mark  StateStyle[SurfaceStyle]
}

type SelectorStyle struct {
	Body  StateStyle[SurfaceStyle]
	Popup SurfaceStyle
	Item  StateStyle[SurfaceStyle]
}

type TabStyle struct {
	Frame SurfaceStyle
	Item  ToggleStyle[SurfaceStyle]
	Panel SurfaceStyle
}

type ProgressStyle struct {
	Track StateStyle[SurfaceStyle]
	Fill  SurfaceStyle
	Thumb StateStyle[SurfaceStyle]
}

// VirtualJoystickStyle controls the track and thumb of a virtual joystick.
// Both parts resolve against the root control's pointer state.
type VirtualJoystickStyle struct {
	Track StateStyle[SurfaceStyle]
	Knob  StateStyle[SurfaceStyle]
}

type ScrollStyle struct {
	Track SurfaceStyle
	Thumb StateStyle[SurfaceStyle]
}

type TreeStyle struct {
	Background SurfaceStyle
	Row        ToggleStyle[SurfaceStyle]
	Collapsed  IconStyle
	Expanded   IconStyle
	Leaf       IconStyle
}

type ImageGridStyle struct {
	Cell   SurfaceStyle
	Remove StateStyle[SurfaceStyle]
}

// Skin contains the visual styles used by all built-in widgets.
type Skin struct {
	Panel           PanelStyle
	Button          ButtonStyles
	ButtonGroup     ButtonGroupStyle
	TextField       TextFieldStyle
	NumberInput     NumberInputStyle
	Menu            MenuStyle
	ContextMenu     MenuStyle
	Checkbox        CheckboxStyle
	Selector        SelectorStyle
	Tab             TabStyle
	Progress        ProgressStyle
	VirtualJoystick VirtualJoystickStyle
	Scroll          ScrollStyle
	Tree            TreeStyle
	ImageGrid       ImageGridStyle
}

type defaultSkinCache struct {
	Theme Theme
	Skin  Skin
}

func DefaultSkin(theme Theme) Skin {
	theme = themeWithDefaults(theme)
	primary := interactiveSurface(theme.Primary, theme.PrimaryHover, theme.PrimaryActive, theme.SurfaceDisabled)
	surface := interactiveSurface(theme.Surface, theme.SurfaceHover, theme.SurfaceActive, theme.SurfaceDisabled)
	danger := interactiveSurface(alphaColor(theme.Danger, 0.78), alphaColor(theme.Danger, 0.9), alphaColor(theme.Danger, 0.72), theme.SurfaceDisabled)
	transparent := interactiveSurface(graphics.ColorRGBA{}, theme.SurfaceHover, theme.SurfaceActive, theme.SurfaceDisabled)
	selectorItem := interactiveSurface(theme.Surface, theme.PrimaryHover, theme.PrimaryActive, theme.SurfaceDisabled)
	buttonPrimary := ButtonStyle{Body: primary, JoinedEdge: primary}
	buttonSurface := ButtonStyle{Body: surface, JoinedEdge: surface}
	buttonDanger := ButtonStyle{Body: danger, JoinedEdge: danger}
	textField := TextFieldStyle{
		Body:      surface,
		Caret:     ColorSurface(theme.Text),
		Selection: ColorSurface(theme.Selection),
	}
	checkboxOff := checkboxTrackStates(theme, false)
	checkboxOn := checkboxTrackStates(theme, true)
	checkboxMark := interactiveSurface(theme.Primary, theme.PrimaryHover, theme.PrimaryActive, alphaColor(theme.Text, 0.55))
	checkboxMark.Normal.Background = scene.CheckmarkShape2D(theme.Primary)
	checkboxMark.Hovered.Background = scene.CheckmarkShape2D(theme.PrimaryHover)
	checkboxMark.Pressed.Background = scene.CheckmarkShape2D(theme.PrimaryActive)
	checkboxMark.Disabled.Background = scene.CheckmarkShape2D(alphaColor(theme.Text, 0.55))
	tabOff := interactiveSurface(alphaColor(theme.Surface, 0.92), theme.SurfaceHover, theme.SurfaceActive, theme.SurfaceDisabled)
	tabOn := NewStateStyle(ColorSurface(theme.Accent)).WithDisabled(ColorSurface(theme.SurfaceDisabled))
	treeOff := interactiveSurface(graphics.ColorRGBA{}, alphaColor(theme.SurfaceHover, 0.7), alphaColor(theme.SurfaceActive, 0.85), alphaColor(theme.SurfaceDisabled, 0.85))
	treeOn := NewStateStyle(ColorSurface(alphaColor(theme.PrimaryHover, 0.75))).WithDisabled(ColorSurface(alphaColor(theme.SurfaceDisabled, 0.85)))
	return Skin{
		Panel:  PanelStyle{Body: ColorSurface(theme.Background)},
		Button: ButtonStyles{Primary: buttonPrimary, Surface: buttonSurface, Danger: buttonDanger},
		ButtonGroup: ButtonGroupStyle{
			Button:  buttonPrimary,
			Frame:   ColorSurface(graphics.ColorRGBA{}),
			Divider: ColorSurface(alphaColor(theme.Muted, 0.48)),
		},
		TextField:   textField,
		NumberInput: NumberInputStyle{Field: textField, Step: surface},
		Menu:        MenuStyle{Trigger: buttonPrimary, Popup: ColorSurface(theme.Surface), Item: surface, Indicator: TextIcon("v")},
		ContextMenu: MenuStyle{Trigger: buttonPrimary, Popup: ColorSurface(theme.Surface), Item: surface},
		Checkbox:    CheckboxStyle{Track: ToggleStyle[SurfaceStyle]{Off: checkboxOff, On: checkboxOn}, Mark: checkboxMark},
		Selector: SelectorStyle{
			Body: surface,
			Popup: ColorSurface(theme.Surface).WithBorder(
				scene.OutlinedRoundedRectShape2D(graphics.ColorRGBA{}, alphaColor(theme.Muted, 0.48), 0, 1),
			),
			Item: selectorItem,
		},
		Tab: TabStyle{
			Frame: ColorSurface(alphaColor(theme.Background, 0.98)),
			Item:  ToggleStyle[SurfaceStyle]{Off: tabOff, On: tabOn},
			Panel: ColorSurface(alphaColor(theme.Surface, 0.32)),
		},
		Progress: ProgressStyle{Track: surface, Fill: ColorSurface(theme.Accent), Thumb: NewStateStyle(ColorSurface(theme.Text))},
		VirtualJoystick: VirtualJoystickStyle{
			Track: interactiveSurface(alphaColor(theme.Surface, 0.68), alphaColor(theme.SurfaceHover, 0.78), alphaColor(theme.SurfaceActive, 0.84), alphaColor(theme.SurfaceDisabled, 0.68)),
			Knob:  interactiveSurface(alphaColor(theme.Primary, 0.9), theme.PrimaryHover, theme.PrimaryActive, alphaColor(theme.Muted, 0.55)),
		},
		Scroll: ScrollStyle{
			Track: ColorSurface(alphaColor(theme.SurfaceHover, 0.9)),
			Thumb: NewStateStyle(ColorSurface(alphaColor(theme.Muted, 0.48))).WithHovered(ColorSurface(alphaColor(theme.Muted, 0.62))),
		},
		Tree: TreeStyle{
			Background: ColorSurface(theme.Background),
			Row:        ToggleStyle[SurfaceStyle]{Off: treeOff, On: treeOn},
			Collapsed:  TextIcon(">"),
			Expanded:   TextIcon("v"),
			Leaf:       TextIcon("-"),
		},
		ImageGrid: ImageGridStyle{Cell: ColorSurface(theme.Surface), Remove: transparent},
	}
}

func interactiveSurface(normal, hovered, pressed, disabled graphics.ColorRGBA) StateStyle[SurfaceStyle] {
	return NewStateStyle(ColorSurface(normal)).
		WithHovered(ColorSurface(hovered)).
		WithPressed(ColorSurface(pressed)).
		WithSelected(ColorSurface(pressed)).
		WithDisabled(ColorSurface(disabled))
}

func checkboxTrackStates(theme Theme, checked bool) StateStyle[SurfaceStyle] {
	normalFill, normalBorder := alphaColor(theme.Background, 0.28), alphaColor(theme.Muted, 0.92)
	hoverFill, hoverBorder := alphaColor(theme.SurfaceHover, 0.9), alphaColor(theme.Text, 0.88)
	pressedFill, pressedBorder := alphaColor(theme.SurfaceActive, 0.92), alphaColor(theme.Text, 0.92)
	disabledFill, disabledBorder := alphaColor(theme.SurfaceDisabled, 0.75), alphaColor(theme.Muted, 0.45)
	if checked {
		normalFill, normalBorder = alphaColor(theme.Primary, 0.18), theme.Primary
		hoverFill, hoverBorder = normalFill, theme.PrimaryHover
		pressedFill, pressedBorder = normalFill, theme.PrimaryActive
		disabledFill, disabledBorder = alphaColor(theme.Primary, 0.35), alphaColor(theme.Primary, 0.55)
	}
	return NewStateStyle(checkboxSurface(normalFill, normalBorder)).
		WithHovered(checkboxSurface(hoverFill, hoverBorder)).
		WithPressed(checkboxSurface(pressedFill, pressedBorder)).
		WithDisabled(checkboxSurface(disabledFill, disabledBorder))
}

func checkboxSurface(fill, border graphics.ColorRGBA) SurfaceStyle {
	return SurfaceStyle{Background: scene.RoundedRectShape2D(fill, 0), Border: scene.OutlinedRoundedRectShape2D(graphics.ColorRGBA{}, border, 0, 2)}
}

func skinFor(world *ecs.World, theme Theme) Skin {
	if skin, ok := ecs.GetResource[Skin](world); ok {
		return skin
	}
	theme = themeWithDefaults(theme)
	if cache, ok := ecs.GetResource[*defaultSkinCache](world); ok && cache != nil && cache.Theme == theme {
		return cache.Skin
	}
	skin := DefaultSkin(theme)
	if cache, ok := ecs.GetResource[*defaultSkinCache](world); ok && cache != nil {
		cache.Theme = theme
		cache.Skin = skin
	} else {
		ecs.InsertResource(world, &defaultSkinCache{Theme: theme, Skin: skin})
	}
	return skin
}

func styleFor[T any](world *ecs.World, entity ecs.Entity, fallback T) T {
	if override, ok := ecs.Get[StyleOverride[T]](world, entity); ok {
		return override.Value
	}
	return fallback
}

func putStyleOverride[T any](world *ecs.World, entity ecs.Entity, style *T) {
	if style != nil {
		_ = ecs.Insert(world, entity, StyleOverride[T]{Value: *style})
	}
}

func controlState(control *controlView, focused bool) ControlState {
	if control == nil {
		return ControlState{Focused: focused}
	}
	return ControlState{Hovered: control.Hovered, Pressed: control.Pressed, Selected: control.Selected, Focused: focused, Disabled: control.Disabled}
}

func controlStateForValues(hovered, pressed, focused, disabled bool) ControlState {
	return ControlState{Hovered: hovered, Pressed: pressed, Focused: focused, Disabled: disabled}
}

func buttonStyleFor(skin Skin, variant ButtonVariant, danger bool) ButtonStyle {
	if danger {
		return skin.Button.Danger
	}
	if variant == ButtonSurface {
		return skin.Button.Surface
	}
	return skin.Button.Primary
}

func textFieldStyleFor(world *ecs.World, entity ecs.Entity, skin Skin) TextFieldStyle {
	if _, ok := ecs.Get[NumberInput](world, entity); ok {
		return styleFor(world, entity, skin.NumberInput).Field
	}
	return styleFor(world, entity, skin.TextField)
}

func applySurface(world *ecs.World, entity ecs.Entity, width, height float32, surface SurfaceStyle) {
	ecs.SetIfChanged(world, entity, scene.Size2D{Width: width, Height: height})
	radius := cornerRadiusPixels(world, entity, width, height)
	if background, ok := ecs.Get[scene.UIBackground2D](world, entity); ok {
		background.Shape, background.Alpha = withSurfaceRadius(surface.Background, radius), 1
		ecs.SetIfChanged(world, entity, background)
	}
	ecs.SetIfChanged(world, entity, surface.Image)
	if border, ok := ecs.Get[scene.UIBorder2D](world, entity); ok {
		border.Shape, border.Alpha = withSurfaceRadius(surface.Border, radius), 1
		ecs.SetIfChanged(world, entity, border)
	}
}

func withSurfaceRadius(shape scene.Shape2D, radius float32) scene.Shape2D {
	if shape.Kind != 0 {
		shape.Radius = radius
	}
	return shape
}
