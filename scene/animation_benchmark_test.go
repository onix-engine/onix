package scene

import (
	"github.com/diiyw/onix/ecs"
	"testing"
	"time"
)

func BenchmarkUpdateWorldAnimatorSystem(b *testing.B) {
	const animatorCount = 256
	for _, playing := range []bool{true, false} {
		name := "paused"
		if playing {
			name = "playing"
		}
		b.Run(name, func(b *testing.B) {
			world := animationBenchmarkWorld(animatorCount, playing)
			system := UpdateWorldAnimatorSystem("")
			if err := system.Run(world); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := system.Run(world); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*animatorCount), "ns/animator")
		})
	}
}

func animationBenchmarkWorld(count int, playing bool) *ecs.World {
	clip := animationBenchmarkClip()
	world := ecs.NewWorld()
	clock := NewTime()
	clock.Set(time.Second/60, 0, 0)
	world.InsertResource(clock)
	spawner := world.NewSpawner4[Animator, Transform, Sprite2D, Visibility]()
	for range count {
		animator := NewBoundAnimator(clip, true, ecs.Entity(0), nil, nil, nil)
		if !playing {
			animator.Pause()
		}
		spawner.Spawn(animator, NewTransform(), NewSprite2D(), Visibility{Visible: true})
	}
	return world
}

func animationBenchmarkClip() *AnimationClip {
	clip := &AnimationClip{Duration: .75}
	for _, target := range []AnimationTarget{
		AnimationTargetTransformPositionX,
		AnimationTargetTransformPositionY,
		AnimationTargetTransformScaleX,
	} {
		clip.FloatTracks = append(clip.FloatTracks, NewAnimationTrack(
			AnimationBinding{Target: target},
			animationBenchmarkKeys(1),
		))
	}
	return clip
}

func animationBenchmarkKeys(offset float32) []AnimationKeyframe[float32] {
	keys := make([]AnimationKeyframe[float32], 6)
	for index := range keys {
		keys[index] = AnimationKeyframe[float32]{
			Time:  float32(index) * .15,
			Value: offset + float32(index),
			Tween: AnimationLinear,
		}
	}
	return keys
}
