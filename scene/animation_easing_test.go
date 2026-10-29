package scene

import (
	"math"
	"testing"
)

func TestAnimationEasingFunctionsAndSamplers(t *testing.T) {
	families := [...]struct {
		name   string
		tweens [3]AnimationTween
		values [3]float32 // In/Out at 1/2, In-out at 1/4.
	}{
		{"Sine", [3]AnimationTween{AnimationEaseInSine, AnimationEaseOutSine, AnimationEaseInOutSine}, [3]float32{.29289322, .70710678, .14644661}},
		{"Quad", [3]AnimationTween{AnimationEaseIn, AnimationEaseOut, AnimationEaseInOut}, [3]float32{.25, .75, .125}},
		{"Cubic", [3]AnimationTween{AnimationEaseInCubic, AnimationEaseOutCubic, AnimationEaseInOutCubic}, [3]float32{.125, .875, .0625}},
		{"Quart", [3]AnimationTween{AnimationEaseInQuart, AnimationEaseOutQuart, AnimationEaseInOutQuart}, [3]float32{.0625, .9375, .03125}},
		{"Quint", [3]AnimationTween{AnimationEaseInQuint, AnimationEaseOutQuint, AnimationEaseInOutQuint}, [3]float32{.03125, .96875, .015625}},
		{"Expo", [3]AnimationTween{AnimationEaseInExpo, AnimationEaseOutExpo, AnimationEaseInOutExpo}, [3]float32{.03125, .96875, .015625}},
		{"Circ", [3]AnimationTween{AnimationEaseInCirc, AnimationEaseOutCirc, AnimationEaseInOutCirc}, [3]float32{.13397460, .86602540, .06698730}},
		{"Back", [3]AnimationTween{AnimationEaseInBack, AnimationEaseOutBack, AnimationEaseInOutBack}, [3]float32{-.0876975, 1.0876975, -.09968184}},
		{"Elastic", [3]AnimationTween{AnimationEaseInElastic, AnimationEaseOutElastic, AnimationEaseInOutElastic}, [3]float32{-.015625, 1.015625, .01196944}},
		{"Bounce", [3]AnimationTween{AnimationEaseInBounce, AnimationEaseOutBounce, AnimationEaseInOutBounce}, [3]float32{.234375, .765625, .1171875}},
	}
	for _, family := range families {
		t.Run(family.name, func(t *testing.T) {
			for direction, tween := range family.tweens {
				if !tween.Valid() {
					t.Fatalf("%s is not accepted by the runtime", tween)
				}
				progress := float32(.5)
				if direction == 2 {
					progress = .25
				}
				want := family.values[direction]
				if got := SampleAnimationProgress(tween, AnimationCurve{}, progress); absEasing(got-want) > 1e-6 {
					t.Fatalf("%s(%g)=%g, want %g", tween, progress, got, want)
				}
				track := AnimationTrack[float32]{Keyframes: []AnimationKeyframe[float32]{{Time: 0, Value: 10, Tween: tween, InverseSpan: .5}, {Time: 2, Value: 30}}}
				interpolate := func(left, right, value float32) float32 { return left + (right-left)*value }
				cursor := 0
				generic, genericOK := track.Sample(progress*2, interpolate)
				compiled, compiledOK := sampleAnimationFloatTrackAt(&track, progress*2, &cursor)
				if !genericOK || !compiledOK || absEasing(generic-(10+20*want)) > 2e-5 || generic != compiled {
					t.Fatalf("%s samplers: generic=%g compiled=%g want=%g", tween, generic, compiled, 10+20*want)
				}
				for _, edge := range [...]float32{-1, 0, 1, 2} {
					if got := SampleAnimationProgress(tween, AnimationCurve{}, edge); got != min(max(edge, 0), 1) {
						t.Fatalf("%s endpoint %g=%g", tween, edge, got)
					}
				}
				for i := 0; i <= 100; i++ {
					value := SampleAnimationProgress(tween, AnimationCurve{}, float32(i)/100)
					if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
						t.Fatalf("%s produced a non-finite value", tween)
					}
				}
				if allocations := testing.AllocsPerRun(100, func() {
					SampleAnimationProgress(tween, AnimationCurve{}, progress)
					track.Sample(progress*2, interpolate)
					sampleAnimationFloatTrackAt(&track, progress*2, &cursor)
				}); allocations != 0 {
					t.Fatalf("%s sampling allocated %g", tween, allocations)
				}
			}
		})
	}
}

func TestAnimationStepProgressStaysAtStartUntilEnd(t *testing.T) {
	for _, progress := range [...]float32{0, .5, .999, 1} {
		want := float32(0)
		if progress == 1 {
			want = 1
		}
		if got := SampleAnimationProgress(AnimationStep, AnimationCurve{}, progress); got != want {
			t.Fatalf("Step(%g)=%g, want %g", progress, got, want)
		}
	}
}

func absEasing(value float32) float32 { return float32(math.Abs(float64(value))) }
