package style

import (
	"math"
	"testing"

	"github.com/diiyw/onix/html/css"
	"github.com/diiyw/onix/html/dom"
)

func TestCSSAnimationCascadeAndPropertyKeyframes(t *testing.T) {
	root := dom.NewDocument()
	node := dom.NewElement("div", map[string]string{"id": "target"})
	root.AppendChild(node)
	sheet, err := css.Parse(`#target {
 animation: move 2s linear -500ms 2.5 alternate both, fade 4s ease-in infinite;
 animation-duration: 1s;
 transform: translateX(20px);
 opacity: .8 !important;
 }
 @keyframes move { 50% {transform:translateX(100%);opacity:0} 50% {transform:translateX(50%)} to {transform:translateX(100%)} }
 @keyframes fade {from{opacity:0}to{opacity:1}}
 `)
	if err != nil {
		t.Fatal(err)
	}
	computed, err := Compute(root, sheet)
	if err != nil {
		t.Fatal(err)
	}
	value := computed[node]
	if len(value.Animations()) != 2 || value.Animations()[0].Duration != 1 || value.Animations()[1].Duration != 1 || value.Animations()[0].Iterations != 2.5 || value.Animations()[0].Delay != -.5 {
		t.Fatalf("animations = %+v", value.Animations())
	}
	tracks := CompileAnimationTracks(node, value, sheet, &QueryEnvironment{Styles: computed})
	if len(tracks) != 2 || tracks[0].Properties != AnimatedTransform || tracks[1].Properties != 0 {
		t.Fatalf("important priority = %+v", tracks)
	}
	sampled := tracks[0].Sample(.25, BaseAnimationValue(value), false)
	if got := ResolveLength(sampled.TranslateX, 100); got != 35 || sampled.Opacity != .8 {
		t.Fatalf("missing endpoint / repeated offset: %+v", sampled)
	}
	if got := ResolveLength(tracks[0].Sample(.5, BaseAnimationValue(value), false).TranslateX, 100); got != 50 {
		t.Fatalf("last repeated keyframe = %v", got)
	}
}

func TestCSSAnimationTimeline(t *testing.T) {
	cases := []struct {
		name          string
		animation     CSSAnimation
		elapsed, want float64
		applies       bool
	}{
		{"delay", CSSAnimation{Duration: 1, Iterations: 1, Delay: .5}, .25, 0, false},
		{"reverse backwards", CSSAnimation{Duration: 1, Iterations: 1, Delay: .5, Direction: "reverse", Fill: "backwards"}, .25, 1, true},
		{"negative delay", CSSAnimation{Duration: 1, Iterations: 1, Delay: -.25}, 0, .25, true},
		{"iteration boundary", CSSAnimation{Duration: 1, Iterations: 2, Direction: "alternate"}, 1, 1, true},
		{"fractional end", CSSAnimation{Duration: 1, Iterations: 2.5, Direction: "alternate", Fill: "forwards"}, 2.5, .5, true},
		{"alternate final", CSSAnimation{Duration: 1, Iterations: 2, Direction: "alternate", Fill: "both"}, 2, 0, true},
		{"no fill", CSSAnimation{Duration: 1, Iterations: 1}, 1, 0, false},
		{"zero duration infinite", CSSAnimation{Duration: 0, Iterations: math.Inf(1)}, 0, 0, false},
		{"zero duration", CSSAnimation{Duration: 0, Iterations: 1, Fill: "forwards"}, 0, 1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, applies, _ := c.animation.Progress(c.elapsed)
			if applies != c.applies || applies && got != c.want {
				t.Fatalf("progress = %v,%v; want %v,%v", got, applies, c.want, c.applies)
			}
		})
	}
}

func TestCSSAnimationTimingFunctions(t *testing.T) {
	cases := []struct {
		input    string
		at, want float64
	}{
		{"linear", .3, .3}, {"cubic-bezier(0, 0, 1, 1)", .3, .3},
		{"steps(4, jump-end)", .49, .25}, {"steps(4, jump-start)", 0, .25},
		{"steps(4, jump-none)", .5, 2.0 / 3}, {"steps(4, jump-both)", 0, .2},
		{"step-end", 1, 1}, {"step-start", 0, 1},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			timing, ok := ParseTimingFunction(c.input)
			if !ok || math.Abs(timing.Sample(c.at, false)-c.want) > 1e-7 {
				t.Fatalf("timing = %+v, %v", timing, timing.Sample(c.at, false))
			}
		})
	}
	for _, input := range []string{"steps(1, jump-none)", "steps(0)", "cubic-bezier(-1, 0, 1, 1)", "cubic-bezier(0, NaN, 1, 1)"} {
		if _, ok := ParseTimingFunction(input); ok {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestCSSAnimationLonghandsAndTransitionLists(t *testing.T) {
	value := defaultStyle(nil)
	ctx := parseContext{FontSize: 16}
	for _, d := range []css.Declaration{
		{Property: "transition", Value: "opacity 2s linear, transform 1s steps(4, end)"},
		{Property: "transition-delay", Value: "-250ms"},
		{Property: "transition-duration", Value: "4s, 3s"},
		{Property: "animation", Value: `"spin name" 2s linear paused`},
		{Property: "animation-play-state", Value: "running"},
	} {
		if err := applyProperty(&value, d.Property, d.Value, nil, ctx); err != nil {
			t.Fatal(err)
		}
	}
	resolveMotion(value.mutableMotion())
	if value.TransitionFor(1).Duration != 4 || value.TransitionFor(0).Duration != 3 || value.TransitionFor(0).Delay != -.25 || value.TransitionFor(0).Timing.Kind != TransformSteps {
		t.Fatalf("transition lists = %+v/%+v", value.TransitionFor(1), value.TransitionFor(0))
	}
	if len(value.Animations()) != 1 || value.Animations()[0].Name != "spin name" || value.Animations()[0].Paused {
		t.Fatalf("quoted name / play state = %+v", value.Animations())
	}
}

func TestCSSAnimationColorInterpolationAndCascade(t *testing.T) {
	root := dom.NewDocument()
	node := dom.NewElement("div", map[string]string{"id": "target"})
	root.AppendChild(node)
	sheet, err := css.Parse(`#target {color:red;background:transparent;border:2px solid;animation:paint 1s linear both;border-left-color:green !important}
	@keyframes paint {
	 from {color:red;background:transparent;border-color:transparent}
	 50% {background-color:blue} 50% {color:blue;transform:translateX(10px)}
	 to {color:blue;background:red;border-color:red;opacity:0 !important}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	styles, err := Compute(root, sheet)
	if err != nil {
		t.Fatal(err)
	}
	computed := styles[node]
	tracks := CompileAnimationTracks(node, computed, sheet, &QueryEnvironment{Styles: styles})
	track := &tracks[0]
	if track.Properties&AnimatedBorderLeftColor != 0 || track.Properties&AnimatedOpacity != 0 {
		t.Fatalf("important priority: %08b", track.Properties)
	}
	base := BaseAnimationValue(computed)
	value := track.Sample(.25, base, false)
	r, g, b, a := value.Colors[AnimationBackground].RGBA()
	if r != 0 || g != 0 || b != 1 || a != .5 {
		t.Fatalf("transparent-to-blue must retain blue chroma: %v %v %v %v", r, g, b, a)
	}
	value = track.Sample(.5, base, false)
	if value.Colors[AnimationForeground].Color() != (Color{B: 255, A: 255}) || value.Colors[AnimationBackground].Color() != (Color{B: 255, A: 255}) || ResolveLength(value.TranslateX, 100) != 10 {
		t.Fatalf("repeated offsets lost properties: %+v", value)
	}
	if computed.BorderColor.Top.Color != computed.Color.Color || computed.BorderColor.Top.Keyword != "currentcolor" {
		t.Fatalf("default border color: %+v", computed.BorderColor)
	}
	if allocations := testing.AllocsPerRun(100, func() { track.Sample(.37, base, false) }); allocations != 0 {
		t.Fatalf("color sampling allocated %v objects", allocations)
	}
}

func TestCSSAnimationCurrentColorAndInheritance(t *testing.T) {
	root := dom.NewDocument()
	parent := dom.NewElement("div", map[string]string{"style": "color:red"})
	root.AppendChild(parent)
	for _, color := range []string{"", "inherit", "unset", "currentColor", "red", "initial"} {
		node := dom.NewElement("span", map[string]string{"id": color, "style": "background:currentColor;border:1px solid currentColor"})
		if color != "" {
			node.Attributes["style"] = "color:" + color + ";background:currentColor;border:1px solid currentColor"
		}
		parent.AppendChild(node)
	}
	sheet, _ := css.Parse("")
	styles, err := Compute(root, sheet)
	if err != nil {
		t.Fatal(err)
	}
	for i, child := range parent.Children {
		computed := styles[child]
		if computed.ColorInherited != (i < 4) || computed.BackgroundColor.Color != computed.Color.Color || computed.BorderColor.Left.Color != computed.Color.Color {
			t.Fatalf("inheritance/currentcolor for child %d: %+v", i, computed)
		}
	}
}

func TestCSSAnimationStepsPhaseBoundaries(t *testing.T) {
	timing, _ := ParseTimingFunction("steps(4, jump-start)")
	track := AnimationTrack{Definition: CSSAnimation{Duration: 1, Delay: 1, Iterations: 1, Fill: "both", Timing: timing}, Properties: AnimatedOpacity,
		Points: []AnimationPoint{{Offset: 0, Properties: AnimatedOpacity}, {Offset: 1, Properties: AnimatedOpacity, Value: AnimationValue{Opacity: 1}}}}
	for _, c := range []struct {
		direction     string
		elapsed, want float64
	}{
		{"normal", .5, 0}, {"normal", 1, .25}, {"normal", 1.25, .5}, {"normal", 2, 1},
		{"reverse", .5, 1}, {"reverse", 1, 1}, {"reverse", 1.75, .5}, {"reverse", 2, 0},
	} {
		track.Definition.Direction = c.direction
		progress, applies, before := track.Definition.Progress(c.elapsed)
		if !applies || track.Sample(progress, AnimationValue{}, before).Opacity != c.want {
			t.Fatalf("%s at %v: progress=%v, before=%v, value=%v; want %v", c.direction, c.elapsed, progress, before, track.Sample(progress, AnimationValue{}, before).Opacity, c.want)
		}
	}
	end, _ := ParseTimingFunction("steps(4, jump-end)")
	track.Definition.Timing, track.Definition.Delay, track.Definition.Direction = end, 0, "normal"
	track.Points = []AnimationPoint{{Offset: 0, Properties: AnimatedOpacity}, {Offset: .5, Properties: AnimatedOpacity, Value: AnimationValue{Opacity: .4}}, {Offset: 1, Properties: AnimatedOpacity, Value: AnimationValue{Opacity: 1}}}
	if got := track.Sample(1, AnimationValue{}, true).Opacity; math.Abs(got-.85) > 1e-8 {
		t.Fatalf("before-flag at last segment endpoint: %v", got)
	}
}

func TestCSSAnimationColorTransitionShorthandMatching(t *testing.T) {
	computed := defaultStyle(nil)
	if err := applyTransitionProperty(&computed, "transition", "all 1s linear, border 2s ease-in, border-left-color 3s steps(2), background 4s"); err != nil {
		t.Fatal(err)
	}
	resolveMotion(computed.mutableMotion())
	for i, want := range [...]float64{1, 4, 2, 2, 2, 3} {
		if transition := computed.TransitionFor(i + 2); !transition.Enabled || transition.Duration != want {
			t.Fatalf("transition %s = %+v; want duration %v", animatedColorNames[i], transition, want)
		}
	}
	if err := applyTransitionProperty(&computed, "transition-property", "none"); err != nil {
		t.Fatal(err)
	}
	resolveMotion(computed.mutableMotion())
	for i := range AnimatedPropertyList {
		if computed.TransitionFor(i).Enabled {
			t.Fatalf("transition-property:none still matches %d", i)
		}
	}
}
