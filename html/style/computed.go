package style

import "github.com/diiyw/onix/html/dom"

type ValueKind uint8

const (
	ValueKeyword ValueKind = iota
	ValueLengthPx
	ValueLengthPercent
	ValueColor
	ValueNumber
	ValueAuto
	ValueLengthCalc
)

type Color struct {
	R uint8
	G uint8
	B uint8
	A uint8
}

type Value struct {
	Kind     ValueKind
	Color    Color
	Keyword  string
	LengthPx float64
	Percent  float64
	Number   float64
}

type Viewport struct {
	Width  float64
	Height float64
}

type Edges struct {
	Top    Value
	Right  Value
	Bottom Value
	Left   Value
}

type Radius struct {
	Horizontal Value
	Vertical   Value
}

type Corners struct {
	TopLeft     Radius
	TopRight    Radius
	BottomRight Radius
	BottomLeft  Radius
}

type Shadow struct {
	OffsetX      Value
	OffsetY      Value
	BlurRadius   Value
	SpreadRadius Value
	Color        Color
	Inset        bool
}

type FilterKind int

const (
	FilterBlur FilterKind = iota
	FilterBrightness
	FilterOpacity
	FilterContrast
	FilterGrayscale
	FilterSaturate
	FilterSepia
	FilterInvert
	FilterHueRotate
	FilterColorMatrix
	FilterDropShadow
)

type Filter struct {
	Kind   FilterKind
	Amount float64
	Matrix [20]float64
	Shadow Shadow
}

type TransformEasing uint8

const (
	TransformEase TransformEasing = iota
	TransformLinear
	TransformEaseIn
	TransformEaseOut
	TransformEaseInOut
)

type TransformTransition struct {
	Enabled  bool
	Duration float64
	Delay    float64
	Easing   TransformEasing
	Timing   TimingFunction
}

type BackgroundLayer struct {
	Image      string
	SizeX      Value
	SizeY      Value
	PosX       Value
	PosY       Value
	PosXOrigin string
	PosYOrigin string
	RepeatX    Value
	RepeatY    Value
	Origin     string
	Clip       string
	Attachment string
}

// styleCore holds the comparable part of a resolved style so instances
// can be interned by value; slice-valued properties live outside it.
type styleCore struct {
	// Derived paint context, propagated during the existing cascade traversal.
	TextOpaqueBackdrop       bool
	TextInOpacityGroup       bool
	Display                  Value
	TableLayout              Value
	BorderSpacingX           Value
	BorderSpacingY           Value
	VerticalAlign            Value
	Visibility               Value
	PointerEvents            Value
	Appearance               Value
	BoxSizing                Value
	Width                    Value
	Height                   Value
	MinWidth                 Value
	MinHeight                Value
	MaxWidth                 Value
	MaxHeight                Value
	BackgroundImage          string
	BackgroundSizeX          Value
	BackgroundSizeY          Value
	BackgroundPosX           Value
	BackgroundPosY           Value
	BackgroundPosXOrigin     string
	BackgroundPosYOrigin     string
	BackgroundRepeatX        Value
	BackgroundRepeatY        Value
	BackgroundOrigin         string
	BackgroundClip           string
	BackgroundAttachment     string
	Margin                   Edges
	Padding                  Edges
	BorderWidth              Edges
	BorderColor              Edges
	BorderStyle              Edges
	BorderRadius             Corners
	BackgroundColor          Value
	Color                    Value
	PlaceholderColor         Value
	TextAlign                Value
	TextBoxTrim              Value
	TextBoxEdge              Value
	TextDecorationLine       Value
	TextIndent               Value
	LetterSpacing            Value
	TabSize                  Value
	FontSize                 Value
	LineHeight               Value
	LineHeightNormal         bool
	FontFamily               Value
	FontWeight               Value
	ListStyleType            Value
	WhiteSpace               Value
	WordBreak                Value
	OverflowWrap             Value
	TextOverflow             Value
	FlexDirection            Value
	FlexWrap                 Value
	GridTemplateColumns      string
	GridTemplateRows         string
	GridColumnStart          Value
	GridColumnEnd            Value
	GridRowStart             Value
	GridRowEnd               Value
	JustifyContent           Value
	JustifyItems             Value
	JustifySelf              Value
	AlignItems               Value
	AlignSelf                Value
	AlignContent             Value
	FlexGrow                 Value
	FlexShrink               Value
	FlexBasis                Value
	Order                    Value
	RowGap                   Value
	ColumnGap                Value
	ContainerType            Value
	ContainerName            string
	Position                 Value
	Float                    Value
	Clear                    Value
	ZIndex                   Value
	Opacity                  Value
	TransformRotation        float64
	TransformTranslateX      Value
	TransformTranslateY      Value
	TransformScaleX          float64
	TransformScaleY          float64
	TransformSkewX           float64
	TransformTranslateCrossX float64
	TransformTranslateCrossY float64
	TransformOriginX         Value
	TransformOriginY         Value
	TransformTransition      TransformTransition
	OpacityTransition        TransformTransition
	ColorTransitions         [AnimatedColorCount]TransformTransition
	TransitionRaw            [4]string
	AnimationRaw             [8]string
	AnimationImportant       AnimatedProperties
	AnimatedOpacity          bool
	AnimatedPaint            AnimatedProperties
	ColorInherited           bool
	OverflowX                Value
	OverflowY                Value
	ScrollbarWidth           Value
	ScrollbarTrackColor      Value
	ScrollbarThumbColor      Value
	ScrollbarThumbHoverColor Value
	Top                      Value
	Right                    Value
	Bottom                   Value
	Left                     Value
	ObjectFit                Value
	ObjectPosX               Value
	ObjectPosY               Value
	ObjectPosXOrigin         string
	ObjectPosYOrigin         string
}

type ComputedStyle struct {
	styleCore
	BackgroundLayers []BackgroundLayer
	BoxShadow        []Shadow
	Filters          []Filter
	Animations       []CSSAnimation
}

// Map stores one computed style per node. ComputedStyle is a large (~6KB)
// struct, so it is shared by pointer: layout boxes and paint fragments
// reference the same instance instead of copying it per node. The cascade
// interns equal instances, so many nodes point at one allocation.
type Map map[*dom.Node]*ComputedStyle

type ChangeImpact uint8

const (
	ChangeNone ChangeImpact = iota
	ChangePaint
	// ChangeVisual changes generated paint primitives without changing geometry.
	ChangeVisual
	ChangeRebuild
)

func (m Map) Get(node *dom.Node) (*ComputedStyle, bool) {
	style, ok := m[node]
	return style, ok
}

// Exclusive detaches a node from its interned style so a caller can change it
// without reaching the sibling nodes which share the same instance.
func (m Map) Exclusive(node *dom.Node) *ComputedStyle {
	computed := *m[node]
	m[node] = &computed
	return &computed
}

func Keyword(value string) Value {
	return Value{Kind: ValueKeyword, Keyword: value}
}

func LengthPx(value float64) Value {
	return Value{Kind: ValueLengthPx, LengthPx: value}
}

func LengthPercent(value float64) Value {
	return Value{Kind: ValueLengthPercent, Percent: value}
}

func LengthCalc(pixels, percent float64) Value {
	return Value{Kind: ValueLengthCalc, LengthPx: pixels, Percent: percent}
}

func ColorValue(value Color) Value {
	return Value{Kind: ValueColor, Color: value}
}

func Number(value float64) Value {
	return Value{Kind: ValueNumber, Number: value}
}

func Auto() Value {
	return Value{Kind: ValueAuto}
}

func uniformEdges(value Value) Edges {
	return Edges{Top: value, Right: value, Bottom: value, Left: value}
}

func uniformRadius(value Value) Radius {
	return Radius{Horizontal: value, Vertical: value}
}

func uniformCorners(value Value) Corners {
	radius := uniformRadius(value)
	return Corners{TopLeft: radius, TopRight: radius, BottomRight: radius, BottomLeft: radius}
}
