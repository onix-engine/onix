package style

import (
	"fmt"
	"math"
	"math/bits"
	"slices"
	"strconv"
	"strings"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

const (
	animationName = iota
	animationDuration
	animationTiming
	animationDelay
	animationIterations
	animationDirection
	animationFill
	animationPlay
)

var animationProperties = [...]string{"animation-name", "animation-duration", "animation-timing-function", "animation-delay", "animation-iteration-count", "animation-direction", "animation-fill-mode", "animation-play-state"}
var animationInitial = [...]string{"none", "0s", "ease", "0s", "1", "normal", "none", "running"}

type CSSAnimation struct {
	Name                        string
	Duration, Delay, Iterations float64
	Timing                      TimingFunction
	Direction, Fill             string
	Paused                      bool
}

func parseAnimationField(index int, raw string, animation *CSSAnimation) error {
	value := strings.TrimSpace(raw)
	lower := strings.ToLower(value)
	switch index {
	case animationName:
		tokenizer := css.NewTokenizer(value)
		token, err := tokenizer.Next()
		if err != nil || (token.Type != css.TokenIdent && token.Type != css.TokenString) {
			return fmt.Errorf("invalid animation name %q", raw)
		}
		if end, _ := tokenizer.Next(); end.Type != css.TokenEOF {
			return fmt.Errorf("invalid animation name %q", raw)
		}
		if token.Type == css.TokenIdent {
			switch lower {
			case "initial", "inherit", "unset", "revert", "revert-layer":
				return fmt.Errorf("invalid animation name %q", raw)
			}
		}
		animation.Name = token.Value
		if lower == "none" && token.Type != css.TokenString {
			animation.Name = ""
		}
	case animationDuration, animationDelay:
		time, valid, err := parseCSSTime(value)
		if err != nil || !valid || (index == animationDuration && time < 0) {
			return fmt.Errorf("invalid animation time %q", raw)
		}
		if index == animationDuration {
			animation.Duration = time
		} else {
			animation.Delay = time
		}
	case animationTiming:
		timing, ok := ParseTimingFunction(value)
		if !ok {
			return fmt.Errorf("invalid animation timing %q", raw)
		}
		animation.Timing = timing
	case animationIterations:
		count := math.Inf(1)
		if lower != "infinite" {
			var err error
			count, err = strconv.ParseFloat(value, 64)
			if err != nil || math.IsNaN(count) || math.IsInf(count, 0) || count < 0 {
				return fmt.Errorf("invalid animation iterations %q", raw)
			}
		}
		animation.Iterations = count
	case animationDirection:
		switch lower {
		case "normal", "reverse", "alternate", "alternate-reverse":
			animation.Direction = lower
		default:
			return fmt.Errorf("invalid animation direction %q", raw)
		}
	case animationFill:
		switch lower {
		case "none", "forwards", "backwards", "both":
			animation.Fill = lower
		default:
			return fmt.Errorf("invalid animation fill %q", raw)
		}
	case animationPlay:
		switch lower {
		case "running":
			animation.Paused = false
		case "paused":
			animation.Paused = true
		default:
			return fmt.Errorf("invalid animation state %q", raw)
		}
	}
	return nil
}

func applyAnimationProperty(computed *ComputedStyle, property, raw string) error {
	if property != "animation" {
		index := slices.Index(animationProperties[:], property)
		if index < 0 {
			return nil
		}
		entries := splitCSSList(raw)
		if len(entries) == 0 {
			return fmt.Errorf("empty animation value")
		}
		for _, value := range entries {
			if err := parseAnimationField(index, value, &CSSAnimation{}); err != nil {
				return err
			}
		}
		computed.mutableMotion().animation[index] = raw
		return nil
	}
	var builders [8]strings.Builder
	entries := splitCSSList(raw)
	if len(entries) == 0 {
		return fmt.Errorf("empty animation")
	}
	for item, entry := range entries {
		values := animationInitial
		var assigned [8]bool
		for _, field := range splitCSSFields(entry) {
			lower := strings.ToLower(field)
			index := animationName
			if _, valid, err := parseCSSTime(field); valid || err != nil {
				index = animationDuration
				if assigned[index] {
					index = animationDelay
				}
			} else if _, valid := ParseTimingFunction(field); valid && !assigned[animationTiming] {
				index = animationTiming
			} else {
				switch lower {
				case "infinite":
					if !assigned[animationIterations] {
						index = animationIterations
					}
				case "normal", "reverse", "alternate", "alternate-reverse":
					if !assigned[animationDirection] {
						index = animationDirection
					}
				case "forwards", "backwards", "both":
					if !assigned[animationFill] {
						index = animationFill
					}
				case "none":
					if !assigned[animationFill] {
						index = animationFill
					}
				case "running", "paused":
					if !assigned[animationPlay] {
						index = animationPlay
					}
				default:
					if number, err := strconv.ParseFloat(field, 64); err == nil && number >= 0 && !assigned[animationIterations] {
						index = animationIterations
					}
				}
			}
			if assigned[index] {
				return fmt.Errorf("duplicate animation value %q", field)
			}
			if err := parseAnimationField(index, field, &CSSAnimation{}); err != nil {
				return err
			}
			values[index], assigned[index] = field, true
		}
		for i, value := range values {
			if item > 0 {
				builders[i].WriteByte(',')
			}
			builders[i].WriteString(value)
		}
	}
	motion := computed.mutableMotion()
	for i := range builders {
		motion.animation[i] = builders[i].String()
	}
	return nil
}

func resolveAnimations(values [8]string) []CSSAnimation {
	if values == ([8]string{}) {
		return nil
	}
	var lists [8][]string
	for i, raw := range values {
		if raw == "" {
			raw = animationInitial[i]
		}
		lists[i] = splitCSSList(raw)
	}
	animations := make([]CSSAnimation, len(lists[animationName]))
	for i := range animations {
		for field := range lists {
			_ = parseAnimationField(field, lists[field][i%len(lists[field])], &animations[i])
		}
	}
	return animations
}

type AnimatedProperties uint8

const (
	AnimatedTransform AnimatedProperties = 1 << iota
	AnimatedOpacity
	AnimatedColor
	AnimatedBackgroundColor
	AnimatedBorderTopColor
	AnimatedBorderRightColor
	AnimatedBorderBottomColor
	AnimatedBorderLeftColor
)

type AnimationValue struct {
	Rotation                                float64
	TranslateX, TranslateY                  Value
	ScaleX, ScaleY                          float64
	SkewX, TranslateCrossX, TranslateCrossY float64
	Opacity                                 float64
	Colors                                  [AnimatedColorCount]AnimationColor
}

func BaseAnimationValue(computed *ComputedStyle) AnimationValue {
	value := AnimationValue{Rotation: computed.TransformRotation, TranslateX: computed.TransformTranslateX, TranslateY: computed.TransformTranslateY, ScaleX: computed.TransformScaleX, ScaleY: computed.TransformScaleY, Opacity: computed.Opacity.Number, SkewX: computed.TransformSkewX, TranslateCrossX: computed.TransformTranslateCrossX, TranslateCrossY: computed.TransformTranslateCrossY}
	for i, color := range paintColorFields(computed) {
		value.Colors[i] = ColorForAnimation(*color)
	}
	if computed.ColorInherited {
		value.Colors[AnimationForeground] = AnimationColor{Current: 1}
	}
	return value
}

type AnimationPoint struct {
	Offset     float64
	Value      AnimationValue
	Properties AnimatedProperties
	Timing     TimingFunction
	HasTiming  bool
}

type AnimationTrack struct {
	Definition CSSAnimation
	Points     []AnimationPoint
	Properties AnimatedProperties
}

// CompileAnimationTracks resolves lengths once, outside the per-frame path.
// Tracks contain separate endpoints per property; omitted endpoints use the underlying style.
func CompileAnimationTracks(node *dom.Node, computed *ComputedStyle, stylesheet *css.Stylesheet, queries *QueryEnvironment) []AnimationTrack {
	if stylesheet == nil || len(computed.Animations()) == 0 {
		return nil
	}
	compiled := compiledMatcher(stylesheet)
	ctx := parseContext{Viewport: queries.Viewport, FontSize: computed.FontSize.LengthPx, ParentFontSize: computed.FontSize.LengthPx, RootFontSize: 16}
	if queries.Styles != nil {
		root := node
		for root.Parent != nil {
			root = root.Parent
		}
		if root.Type == dom.DocumentNode {
			for _, child := range root.Children {
				if child.Type == dom.ElementNode {
					root = child
					break
				}
			}
		}
		if rootStyle := queries.Styles[root]; rootStyle != nil {
			ctx.RootFontSize = rootStyle.FontSize.LengthPx
		}
	}
	var tracks []AnimationTrack
	for _, definition := range computed.Animations() {
		if definition.Name == "" {
			continue
		}
		var keyframes *css.Keyframes
		definitions := compiled.keyframes[definition.Name].definitions
		for index := len(definitions) - 1; index >= 0; index-- {
			candidate := &definitions[index]
			if compiledConditionsMatch(candidate.conditions, node, queries) {
				keyframes = candidate.frames
				break
			}
		}
		if keyframes == nil {
			continue
		}
		track := AnimationTrack{Definition: definition}
		for _, frame := range keyframes.Frames {
			point := AnimationPoint{}
			for _, declaration := range frame.Declarations {
				if declaration.Important {
					continue
				}
				switch declaration.Property {
				case "transform":
					transform, err := parseTransform(declaration.Value, ctx)
					if err != nil {
						continue
					}
					point.Value.Rotation, point.Value.TranslateX, point.Value.TranslateY = transform.Rotation, transform.TranslateX, transform.TranslateY
					point.Value.ScaleX, point.Value.ScaleY = transform.ScaleX, transform.ScaleY
					point.Value.SkewX, point.Value.TranslateCrossX, point.Value.TranslateCrossY = transform.SkewX, transform.TranslateCrossX, transform.TranslateCrossY
					point.Properties |= AnimatedTransform
				case "opacity":
					value := defaultStyle(nil)
					if err := applyProperty(&value, "opacity", declaration.Value, nil, ctx); err != nil {
						continue
					}
					point.Value.Opacity = value.Opacity.Number
					point.Properties |= AnimatedOpacity
				case "color", "background", "background-color", "border", "border-color", "border-top", "border-right", "border-bottom", "border-left", "border-top-color", "border-right-color", "border-bottom-color", "border-left-color":
					value := defaultStyle(nil)
					if err := applyProperty(&value, declaration.Property, declaration.Value, nil, ctx); err != nil {
						continue
					}
					properties := AnimatedProperty(declaration.Property)
					CopyAnimatedProperties(&point.Value, BaseAnimationValue(&value), properties)
					point.Properties |= properties
				case "animation-timing-function":
					if timing, ok := ParseTimingFunction(declaration.Value); ok {
						point.Timing, point.HasTiming = timing, true
					}
				}
			}
			for _, offset := range frame.Offsets {
				point.Offset = offset
				track.Points = append(track.Points, point)
			}
		}
		slices.SortStableFunc(track.Points, func(a, b AnimationPoint) int {
			if a.Offset < b.Offset {
				return -1
			}
			if a.Offset > b.Offset {
				return 1
			}
			return 0
		})
		// Repeated offsets cascade by source order, independently for each property.
		points := track.Points[:0]
		for _, point := range track.Points {
			if len(points) > 0 && points[len(points)-1].Offset == point.Offset {
				previous := &points[len(points)-1]
				CopyAnimatedProperties(&previous.Value, point.Value, point.Properties)
				if point.HasTiming {
					previous.Timing, previous.HasTiming = point.Timing, true
				}
				previous.Properties |= point.Properties
			} else {
				points = append(points, point)
			}
			track.Properties |= point.Properties
		}
		track.Points = points
		track.Properties &^= computed.AnimationImportant
		tracks = append(tracks, track)
	}
	return tracks
}

// Progress handles delay, fractional iteration counts, direction and fill at exact boundaries.
func (a CSSAnimation) Progress(elapsed float64) (progress float64, applies, before bool) {
	activeTime := elapsed - a.Delay
	if activeTime < 0 {
		if a.Fill != "backwards" && a.Fill != "both" {
			return 0, false, false
		}
		return a.direct(0, 0), true, !a.reverse(0)
	}
	total := a.Duration * a.Iterations
	if a.Duration == 0 {
		total = 0
	}
	if activeTime >= total {
		if a.Fill != "forwards" && a.Fill != "both" {
			return 0, false, false
		}
		fraction := a.Iterations - math.Floor(a.Iterations)
		iteration := math.Floor(a.Iterations)
		if math.IsInf(a.Iterations, 1) {
			fraction, iteration = 1, 0
		}
		if fraction == 0 && a.Iterations > 0 {
			fraction, iteration = 1, iteration-1
		}
		return a.direct(fraction, iteration), true, a.reverse(iteration)
	}
	iteration := math.Floor(activeTime / a.Duration)
	return a.direct(activeTime/a.Duration-iteration, iteration), true, false
}

func (a CSSAnimation) reverse(iteration float64) bool {
	reverse := a.Direction == "reverse" || a.Direction == "alternate-reverse"
	if (a.Direction == "alternate" || a.Direction == "alternate-reverse") && math.Mod(iteration, 2) == 1 {
		reverse = !reverse
	}
	return reverse
}

func (a CSSAnimation) direct(progress, iteration float64) float64 {
	if a.reverse(iteration) {
		return 1 - progress
	}
	return progress
}

func (track *AnimationTrack) Sample(progress float64, base AnimationValue, before bool) AnimationValue {
	result := base
	start := AnimationPoint{Value: base, Timing: track.Definition.Timing, HasTiming: true}
	end := AnimationPoint{Offset: 1, Value: base}
	var left, right [AnimatedPropertyCount]*AnimationPoint
	for i := range left {
		left[i], right[i] = &start, &end
	}
	var foundRight AnimatedProperties
	// Find all property intervals in a single traversal of the keyframes.
	for i := range track.Points {
		point := &track.Points[i]
		properties := point.Properties & track.Properties &^ foundRight
		for properties != 0 {
			index := bits.TrailingZeros8(uint8(properties))
			property := AnimatedProperties(1) << index
			if point.Offset <= progress && point.Offset < 1 && (point.Offset < progress || !before || point.Offset == 0) {
				left[index] = point
			} else {
				right[index] = point
				foundRight |= property
			}
			properties &^= property
		}
	}
	properties := track.Properties
	for properties != 0 {
		index := bits.TrailingZeros8(uint8(properties))
		property := AnimatedProperties(1) << index
		from, to := left[index], right[index]
		timing := from.Timing
		if !from.HasTiming {
			timing = track.Definition.Timing
		}
		factor := float64(0)
		if to.Offset > from.Offset {
			factor = timing.Sample((progress-from.Offset)/(to.Offset-from.Offset), before)
		}
		InterpolateAnimatedProperty(&result, from.Value, to.Value, property, factor)
		properties &^= property
	}
	return result
}

func interpolate(a, b, t float64) float64 { return a + (b-a)*t }
func interpolateLength(a, b Value, t float64) Value {
	pixels := func(v Value) float64 {
		if v.Kind == ValueLengthPx || v.Kind == ValueLengthCalc {
			return v.LengthPx
		}
		return 0
	}
	percent := func(v Value) float64 {
		if v.Kind == ValueLengthPercent || v.Kind == ValueLengthCalc {
			return v.Percent
		}
		return 0
	}
	return LengthCalc(interpolate(pixels(a), pixels(b), t), interpolate(percent(a), percent(b), t))
}

func ResolveAnimatedTranslation(value AnimationValue, width, height float64) (float64, float64) {
	return ResolveLength(value.TranslateX, width) + value.TranslateCrossX*height, ResolveLength(value.TranslateY, height) + value.TranslateCrossY*width
}
