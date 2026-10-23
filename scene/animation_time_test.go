package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
)

func TestAnimationTrackTimeCurveChangesSequencePace(t *testing.T) {
	track := AnimationTrack[int]{Keyframes: []AnimationKeyframe[int]{
		{Time: 2, Value: 0, Tween: AnimationStep}, {Time: 3, Value: 1, Tween: AnimationStep},
		{Time: 4, Value: 2, Tween: AnimationStep}, {Time: 5, Value: 3, Tween: AnimationStep}, {Time: 6, Value: 4, Tween: AnimationStep},
	}}
	for _, test := range []struct {
		name  string
		tween AnimationTween
		curve AnimationCurve
		want  int
	}{
		{"constant", AnimationLinear, AnimationCurve{}, 2},
		{"accelerate", AnimationEaseIn, AnimationCurve{}, 1},
		{"decelerate", AnimationEaseOut, AnimationCurve{}, 3},
		{"custom", AnimationBezier, EaseInAnimationCurve(), 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			track.TimeTween, track.TimeCurve = test.tween, test.curve
			cursor := 0
			generic, genericOK := SampleAnimationTrack(track, 4, nil)
			compiled, compiledOK := sampleAnimationDiscreteTrackAt(&track, 4, &cursor)
			if !genericOK || !compiledOK || generic != test.want || compiled != test.want {
				t.Fatalf("halfway frame: generic=%d compiled=%d, want %d", generic, compiled, test.want)
			}
			for _, edge := range []struct {
				time float32
				want int
			}{{0, 0}, {2, 0}, {6, 4}, {10, 4}} {
				if value, _ := sampleAnimationDiscreteTrackAt(&track, edge.time, &cursor); value != edge.want {
					t.Fatalf("frame at %g=%d, want %d", edge.time, value, edge.want)
				}
			}
			if allocations := testing.AllocsPerRun(100, func() {
				SampleAnimationTrack(track, 4, nil)
				sampleAnimationDiscreteTrackAt(&track, 4, &cursor)
			}); allocations != 0 {
				t.Fatalf("time remapping allocated %g", allocations)
			}
		})
	}
	track.TimeTween = AnimationEaseOutElastic
	cursor := 0
	for i := 0; i <= 100; i++ {
		time := 2 + float32(i)/25
		generic, _ := SampleAnimationTrack(track, time, nil)
		compiled, _ := sampleAnimationDiscreteTrackAt(&track, time, &cursor)
		if generic != compiled || compiled < 0 || compiled > 4 {
			t.Fatalf("non-monotonic curve lost its cursor at %g: %d != %d", time, generic, compiled)
		}
	}
}

func TestAnimationTimeCurveRunsForOwnerAndBoundSequenceTracks(t *testing.T) {
	for _, bound := range []bool{false, true} {
		world := ecs.NewWorld()
		var frames [5]graphics.Texture
		track := AnimationTrack[*graphics.Texture]{Binding: AnimationBinding{Target: AnimationTargetSpriteTexture}, TimeTween: AnimationEaseIn}
		for i := range frames {
			track.Keyframes = append(track.Keyframes, AnimationKeyframe[*graphics.Texture]{Time: float32(i) / 4, Value: &frames[i], Tween: AnimationStep})
		}
		clip := &AnimationClip{Duration: 1, TextureTracks: []AnimationTrack[*graphics.Texture]{track}}
		if bound {
			clip.TextureTracks[0].Binding.Path = "frames"
		}
		spawner := ecs.NewSpawner3[Animator, Sprite2D, Transform](world)
		owner := spawner.Spawn(NewAnimator(clip, false), Sprite2D{}, NewTransform())
		target := owner
		if bound {
			target = spawner.Spawn(Animator{}, Sprite2D{}, NewTransform())
		}
		animator := ecs.GetMut[Animator](world, owner)
		if bound {
			animator.BindTargets(owner, nil, []ecs.Entity{target}, nil)
		}
		animator.Pause()
		for _, test := range []struct {
			time  float32
			frame int
		}{{.5, 1}, {.9, 3}, {.25, 0}, {1, 4}, {0, 0}} {
			animator.Seek(test.time)
			if err := UpdateWorldAnimatorSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if sprite := ecs.GetRef[Sprite2D](world, target); sprite.Texture != &frames[test.frame] {
				t.Fatalf("bound=%v frame at %g does not follow the speed curve", bound, test.time)
			}
		}
	}
}

func TestAnimationTimeCurveAlsoRemapsFloatSampling(t *testing.T) {
	track := AnimationTrack[float32]{TimeTween: AnimationEaseIn, Keyframes: []AnimationKeyframe[float32]{{Time: 2, Value: 0}, {Time: 6, Value: 100}}}
	interpolate := func(a, b, t float32) float32 { return a + (b-a)*t }
	cursor := 0
	generic, _ := SampleAnimationTrack(track, 4, interpolate)
	compiled, _ := sampleAnimationFloatTrackAt(&track, 4, &cursor)
	if generic != 25 || compiled != 25 {
		t.Fatalf("remapped float samplers: %g, %g, want 25", generic, compiled)
	}
}
