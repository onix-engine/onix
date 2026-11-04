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
	Keyword  string
	LengthPx float64
	Percent  float64
	Number   float64
	Color    Color
	Kind     ValueKind
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
	Shadow Shadow
	Matrix [20]float64
	Kind   FilterKind
	Amount float64
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
	Timing   TimingFunction
	Duration float64
	Delay    float64
	Enabled  bool
	Easing   TransformEasing
}

type BackgroundLayer struct {
	Image      string
	PosXOrigin string
	PosYOrigin string
	Origin     string
	Clip       string
	Attachment string
	SizeX      Value
	SizeY      Value
	PosX       Value
	PosY       Value
	RepeatX    Value
	RepeatY    Value
}

// styleCore holds the comparable part of a resolved style so instances
// can be interned by value; slice-valued properties live outside it.
type styleCore struct {
	BackgroundImage          string
	BackgroundAttachment     string
	BackgroundPosYOrigin     string
	ObjectPosYOrigin         string
	ObjectPosXOrigin         string
	ContainerName            string
	GridTemplateRows         string
	GridTemplateColumns      string
	BackgroundPosXOrigin     string
	BackgroundClip           string
	BackgroundOrigin         string
	BorderRadius             Corners
	BorderWidth              Edges
	Padding                  Edges
	Margin                   Edges
	BorderColor              Edges
	BorderStyle              Edges
	TextOverflow             Value
	JustifyItems             Value
	BackgroundSizeY          Value
	BackgroundPosX           Value
	BackgroundPosY           Value
	BackgroundRepeatX        Value
	BackgroundRepeatY        Value
	MaxHeight                Value
	MaxWidth                 Value
	MinHeight                Value
	MinWidth                 Value
	Height                   Value
	Width                    Value
	BoxSizing                Value
	Appearance               Value
	PointerEvents            Value
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
	Display                  Value
	TableLayout              Value
	ObjectPosY               Value
	FontFamily               Value
	FontWeight               Value
	ListStyleType            Value
	WhiteSpace               Value
	WordBreak                Value
	OverflowWrap             Value
	Visibility               Value
	FlexDirection            Value
	FlexWrap                 Value
	VerticalAlign            Value
	BorderSpacingY           Value
	GridColumnStart          Value
	GridColumnEnd            Value
	GridRowStart             Value
	GridRowEnd               Value
	JustifyContent           Value
	BackgroundSizeX          Value
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
	BorderSpacingX           Value
	Position                 Value
	Float                    Value
	Clear                    Value
	ZIndex                   Value
	Opacity                  Value
	ObjectPosX               Value
	TransformTranslateX      Value
	TransformTranslateY      Value
	ObjectFit                Value
	Left                     Value
	Bottom                   Value
	Right                    Value
	Top                      Value
	TransformOriginX         Value
	TransformOriginY         Value
	ScrollbarThumbHoverColor Value
	ScrollbarThumbColor      Value
	ScrollbarTrackColor      Value
	ScrollbarWidth           Value
	OverflowX                Value
	OverflowY                Value
	TransformRotation        float64
	TransformTranslateCrossY float64
	TransformTranslateCrossX float64
	TransformSkewX           float64
	TransformScaleY          float64
	TransformScaleX          float64
	AnimatedPaint            AnimatedProperties
	AnimatedOpacity          bool
	AnimationImportant       AnimatedProperties
	ColorInherited           bool
	LineHeightNormal         bool
	TextInOpacityGroup       bool
	// Derived paint context, propagated during the existing cascade traversal.
	TextOpaqueBackdrop bool
}

type ComputedStyle struct {
	motion           *motionStyle
	BackgroundLayers []BackgroundLayer
	BoxShadow        []Shadow
	Filters          []Filter
	styleCore
}

// Map stores one computed style per node. ComputedStyle is a large (~5.5KB)
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
