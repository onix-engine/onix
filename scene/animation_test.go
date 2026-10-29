package scene

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

func TestEqualAnimationSamplesPreserveComponentChanges(t *testing.T) {
	for _, ui := range []bool{false, true} {
		for _, bound := range []bool{false, true} {
			world := ecs.NewWorld()
			clip := &AnimationClip{
				Duration: 1,
				FloatTracks: []AnimationTrack[float32]{
					NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformPositionX}, []AnimationKeyframe[float32]{{Value: 5}}),
					NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformRotationZ}, []AnimationKeyframe[float32]{{Value: .3}}),
					NewAnimationTrack(AnimationBinding{Target: AnimationTargetSpriteAlpha}, []AnimationKeyframe[float32]{{Value: .5}}),
				},
				TextureTracks: []AnimationTrack[*graphics.Texture]{NewAnimationTrack(AnimationBinding{Target: AnimationTargetSpriteTexture}, []AnimationKeyframe[*graphics.Texture]{{Value: nil}})},
				BoolTracks:    []AnimationTrack[bool]{NewAnimationTrack(AnimationBinding{Target: AnimationTargetVisibility}, []AnimationKeyframe[bool]{{Value: false}})},
			}
			animator := NewAnimator(clip, false)
			bundle := ecs.NewBundle(5)
			bundle.Put(animator)
			bundle.Put(NewSprite2D())
			bundle.Put(Visibility{Visible: true})
			if ui {
				bundle.Put(NewUITransform())
			} else {
				bundle.Put(NewTransform())
			}
			e := world.Spawn(bundle)
			if bound {
				world.GetMut[Animator](e).BindTargets(e, nil, nil, nil)
			}
			system := UpdateWorldAnimatorSystem("")
			if ui {
				system = UpdateUIAnimatorSystem("")
			}
			if err := system.Run(world); err != nil {
				t.Fatal(err)
			}
			versions := func() [4]uint64 {
				return [4]uint64{world.ComponentVersion[Transform](), world.ComponentVersion[UITransform](), world.ComponentVersion[Sprite2D](), world.ComponentVersion[Visibility]()}
			}
			before := versions()
			apply := func() {
				world.GetMut[Animator](e).Seek(.5)
				if err := system.Run(world); err != nil {
					t.Fatal(err)
				}
			}
			if n := testing.AllocsPerRun(100, apply); n != 0 {
				t.Fatalf("UI=%t bound=%t repeat sample allocated %g objects", ui, bound, n)
			}
			if after := versions(); after != before {
				t.Fatalf("UI=%t bound=%t equal samples changed versions: %v -> %v", ui, bound, before, after)
			}
		}
	}
}

func TestSampleAnimationTrackSupportsStepAndBezier(t *testing.T) {
	step := NewAnimationTrack(AnimationBinding{Target: AnimationTargetSpriteTexture}, []AnimationKeyframe[int]{
		{Time: 0, Value: 1, Tween: AnimationStep},
		{Time: 1, Value: 2, Tween: AnimationStep},
	})
	if value, ok := step.Sample(.75, nil); !ok || value != 1 {
		t.Fatalf("step sample = %d, %v", value, ok)
	}
	bezier := NewAnimationTrack(AnimationBinding{Target: AnimationTargetSpriteAlpha}, []AnimationKeyframe[float32]{
		{Time: 0, Value: 0, Tween: AnimationBezier, Curve: AnimationCurve{X1: .42, X2: 1, Y2: 1}},
		{Time: 1, Value: 1},
	})
	value, ok := bezier.Sample(.5, func(a, b, progress float32) float32 { return a + (b-a)*progress })
	if !ok || value <= 0 || value >= .5 {
		t.Fatalf("ease-in sample = %f, %v", value, ok)
	}
}

func TestSampleAnimationTrackUsesCubicHermiteTangents(t *testing.T) {
	track := NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformPositionX}, []AnimationKeyframe[float32]{
		{Time: 0, Value: 0, OutTangent: 2, Tween: AnimationCubicSpline},
		{Time: 1, Value: 1},
	})
	value, ok := track.Sample(.5, func(a, b, progress float32) float32 { return a + (b-a)*progress })
	if !ok || math.Abs(float64(value-.75)) > 1e-6 {
		t.Fatalf("cubic sample = %g, %v", value, ok)
	}
	cursor := 0
	value, ok = sampleAnimationFloatTrackAt(&track, .5, &cursor)
	if !ok || math.Abs(float64(value-.75)) > 1e-6 {
		t.Fatalf("compiled cubic sample = %g, %v", value, ok)
	}
}

func TestSampleAnimationTrackIgnoresStaleInverseSpan(t *testing.T) {
	track := AnimationTrack[float32]{Keyframes: []AnimationKeyframe[float32]{
		{Time: 0, Value: 0, Tween: AnimationLinear, InverseSpan: 100},
		{Time: 1, Value: 10},
	}}
	value, ok := track.Sample(.5, func(a, b, progress float32) float32 {
		return a + (b-a)*progress
	})
	if !ok || value != 5 {
		t.Fatalf("sample with stale inverse span = %v, %v; want 5, true", value, ok)
	}
}

func TestAnimatorOwnerOnlyBindingIsBound(t *testing.T) {
	animator := NewBoundAnimator(nil, false, ecs.Entity(7), nil, nil, nil)
	if !animator.IsBound() {
		t.Fatal("owner-only bound animator was reported as unbound")
	}
}

func TestBindTargetsReusesTargetStorage(t *testing.T) {
	animator := NewAnimator(nil, false)
	animator.BindTargets(ecs.Entity(1), []ecs.Entity{ecs.Entity(2), ecs.Entity(3)}, nil, nil)
	address := &animator.boundTargets[0]
	source := []ecs.Entity{ecs.Entity(4), ecs.Entity(5)}
	if allocations := testing.AllocsPerRun(100, func() {
		animator.BindTargets(ecs.Entity(1), source, nil, nil)
	}); allocations != 0 {
		t.Fatalf("rebind allocated %.0f objects, want 0", allocations)
	}
	floatTargets, _, _ := animator.boundTargetSlices()
	if &animator.boundTargets[0] != address || floatTargets[0] != ecs.Entity(4) {
		t.Fatalf("rebind did not reuse target storage: %v", floatTargets)
	}
	animator.BindTargets(ecs.Entity(1), nil, nil, nil)
	if cap(animator.boundTargets) == 0 {
		t.Fatal("clearing bound targets discarded reusable storage")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		animator.BindTargets(ecs.Entity(1), source, nil, nil)
		animator.BindTargets(ecs.Entity(1), nil, nil, nil)
	}); allocations != 0 {
		t.Fatalf("bind/clear cycle allocated %.0f objects, want 0", allocations)
	}
	animator.BindTargets(0, nil, nil, nil)
	if animator.IsBound() {
		t.Fatal("clearing owner and targets left animator bound")
	}
}

func TestBoundAnimatorOwnerFallbackOnlyAppliesRootTracks(t *testing.T) {
	world := ecs.NewWorld()
	clip := &AnimationClip{Duration: 1, FloatTracks: []AnimationTrack[float32]{NewAnimationTrack[float32](
		AnimationBinding{Path: "child", Target: AnimationTargetTransformPositionX},
		[]AnimationKeyframe[float32]{{Time: 0, Value: 1}, {Time: 1, Value: 3}},
	)}}
	spawner := world.NewSpawner2[Animator, Transform]()
	root := spawner.Spawn(NewBoundAnimator(clip, false, ecs.Entity(0), nil, nil, nil), NewTransform())
	animator := world.GetMut[Animator](root)
	if animator == nil {
		t.Fatal("bound animator was not spawned")
	}
	animator.BindTargets(root, nil, nil, nil)
	animator.Seek(.5)
	animator.Pause()
	if err := UpdateWorldAnimatorSystem("").Run(world); err == nil || !strings.Contains(err.Error(), "has no bound target") {
		t.Fatalf("missing descendant target error = %v", err)
	}
	transform, _ := world.Get[Transform](root)
	if transform.Translation.X() != 0 {
		t.Fatalf("owner fallback animated a descendant track: x=%v", transform.Translation.X())
	}
}

func TestBoundAnimatorKeepsTypedTargetSlicesContiguous(t *testing.T) {
	world := ecs.NewWorld()
	transformSpawner := world.NewSpawner1[Transform]()
	floatEntities := []ecs.Entity{transformSpawner.Spawn(NewTransform()), transformSpawner.Spawn(NewTransform())}
	textureSpawner := world.NewSpawner1[Sprite2D]()
	textureEntities := []ecs.Entity{textureSpawner.Spawn(NewSprite2D()), textureSpawner.Spawn(NewSprite2D())}
	visibilitySpawner := world.NewSpawner1[Visibility]()
	visibilityEntities := []ecs.Entity{visibilitySpawner.Spawn(Visibility{}), visibilitySpawner.Spawn(Visibility{})}
	first := graphics.CreateTextureByColor(graphics.White)
	second := graphics.CreateTextureByColor(graphics.Black)
	t.Cleanup(func() {
		_ = first.Dispose()
		_ = second.Dispose()
	})
	clip := &AnimationClip{
		Duration: 1,
		FloatTracks: []AnimationTrack[float32]{
			NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformPositionX}, []AnimationKeyframe[float32]{{Time: 0, Value: 1}}),
			NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformPositionX}, []AnimationKeyframe[float32]{{Time: 0, Value: 2}}),
		},
		TextureTracks: []AnimationTrack[*graphics.Texture]{
			NewAnimationTrack(AnimationBinding{Target: AnimationTargetSpriteTexture}, []AnimationKeyframe[*graphics.Texture]{{Time: 0, Value: first}}),
			NewAnimationTrack(AnimationBinding{Target: AnimationTargetSpriteTexture}, []AnimationKeyframe[*graphics.Texture]{{Time: 0, Value: second}}),
		},
		BoolTracks: []AnimationTrack[bool]{
			NewAnimationTrack(AnimationBinding{Target: AnimationTargetVisibility}, []AnimationKeyframe[bool]{{Time: 0, Value: true}}),
			NewAnimationTrack(AnimationBinding{Target: AnimationTargetVisibility}, []AnimationKeyframe[bool]{{Time: 0, Value: false}}),
		},
	}
	animator := NewBoundAnimator(clip, false, ecs.Entity(0), floatEntities, textureEntities, visibilityEntities)
	animator.Pause()
	animatorSpawner := world.NewSpawner1[Animator]()
	animatorEntity := animatorSpawner.Spawn(animator)
	if err := UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	for index, entity := range floatEntities {
		transform, _ := world.Get[Transform](entity)
		if got := transform.Translation.X(); got != float32(index+1) {
			t.Fatalf("float target %d = %v, want %d", index, got, index+1)
		}
	}
	for index, entity := range textureEntities {
		sprite, _ := world.Get[Sprite2D](entity)
		want := first
		if index == 1 {
			want = second
		}
		if sprite.Texture != want {
			t.Fatalf("texture target %d = %p, want %p", index, sprite.Texture, want)
		}
	}
	for index, entity := range visibilityEntities {
		visibility, _ := world.Get[Visibility](entity)
		want := index == 0
		if visibility.Visible != want {
			t.Fatalf("visibility target %d = %v, want %v", index, visibility.Visible, want)
		}
	}
	if current, ok := world.Get[Animator](animatorEntity); !ok || len(current.boundTargets) != 6 {
		t.Fatalf("bound target storage = %#v, want six entities", current)
	}
}

func TestAnimationClipPrepareOnlyAllocatesRootIndexesWhenNeeded(t *testing.T) {
	descendant := &AnimationClip{FloatTracks: []AnimationTrack[float32]{NewAnimationTrack[float32](
		AnimationBinding{Path: "child", Target: AnimationTargetTransformPositionX}, nil,
	)}}
	if err := descendant.Prepare(); err != nil {
		t.Fatal(err)
	}
	if descendant.rootFloatTracks != nil || descendant.rootRequirements != 0 {
		t.Fatalf("descendant-only root metadata = indexes:%v requirements:%d", descendant.rootFloatTracks, descendant.rootRequirements)
	}
	if descendant.FloatTracks[0].root {
		t.Fatal("descendant track was marked as a root track")
	}
	root := &AnimationClip{FloatTracks: []AnimationTrack[float32]{
		NewAnimationTrack[float32](AnimationBinding{Target: AnimationTargetTransformPositionX}, nil),
		NewAnimationTrack[float32](AnimationBinding{Path: "child", Target: AnimationTargetTransformPositionY}, nil),
	}}
	if err := root.Prepare(); err != nil {
		t.Fatal(err)
	}
	if len(root.rootFloatTracks) != 1 || cap(root.rootFloatTracks) != 1 || root.rootFloatTracks[0] != 0 {
		t.Fatalf("root metadata = indexes:%v cap:%d", root.rootFloatTracks, cap(root.rootFloatTracks))
	}
	if !root.FloatTracks[0].root || root.FloatTracks[1].root {
		t.Fatalf("track root flags = %v/%v, want true/false", root.FloatTracks[0].root, root.FloatTracks[1].root)
	}
}

func TestAnimationClipTracksRootRotationMetadata(t *testing.T) {
	clip := &AnimationClip{FloatTracks: []AnimationTrack[float32]{
		NewAnimationTrack[float32](AnimationBinding{Target: AnimationTargetTransformPositionX}, nil),
		NewAnimationTrack[float32](AnimationBinding{Path: "child", Target: AnimationTargetTransformRotationX}, nil),
		NewAnimationTrack[float32](AnimationBinding{Target: AnimationTargetTransformRotationZ}, nil),
	}}
	if err := clip.Prepare(); err != nil {
		t.Fatal(err)
	}
	if !clip.hasRotationTracks || !clip.hasRootRotationTracks {
		t.Fatalf("rotation metadata = all:%v root:%v, want both true", clip.hasRotationTracks, clip.hasRootRotationTracks)
	}

	positionOnly := &AnimationClip{FloatTracks: []AnimationTrack[float32]{NewAnimationTrack[float32](
		AnimationBinding{Target: AnimationTargetTransformPositionX}, nil,
	)}}
	if err := positionOnly.Prepare(); err != nil {
		t.Fatal(err)
	}
	if positionOnly.hasRotationTracks || positionOnly.hasRootRotationTracks {
		t.Fatalf("position-only rotation metadata = all:%v root:%v", positionOnly.hasRotationTracks, positionOnly.hasRootRotationTracks)
	}
}

func TestAnimatorBindingTargetsChildNode(t *testing.T) {
	world := ecs.NewWorld()
	clip := &AnimationClip{
		Duration: 1,
		FloatTracks: []AnimationTrack[float32]{NewAnimationTrack(AnimationBinding{Path: "child", Target: AnimationTargetTransformPositionX}, []AnimationKeyframe[float32]{
			{Time: 0, Value: 10, Tween: AnimationLinear}, {Time: 1, Value: 30},
		})},
	}
	childSpawner := world.NewSpawner1[Transform]()
	child := childSpawner.Spawn(NewTransform())
	rootSpawner := world.NewSpawner2[Animator, Transform]()
	root := rootSpawner.Spawn(NewBoundAnimator(clip, false, ecs.Entity(0), []ecs.Entity{child}, nil, nil), NewTransform())
	animator := world.GetMut[Animator](root)
	animator.Seek(.5)
	animator.Pause()
	if err := UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatalf("update: %v", err)
	}
	transform, _ := world.Get[Transform](child)
	if transform.Translation.X() != 20 {
		t.Fatalf("child position x = %v, want 20", transform.Translation.X())
	}
}

func TestAnimatorAppliesXYZRotationTracksToChild(t *testing.T) {
	clip := &AnimationClip{
		Duration: 1,
		FloatTracks: []AnimationTrack[float32]{
			NewAnimationTrack(AnimationBinding{Path: "card", Target: AnimationTargetTransformRotationX}, []AnimationKeyframe[float32]{{Time: 0, Value: .1}, {Time: 1, Value: .3}}),
			NewAnimationTrack(AnimationBinding{Path: "card", Target: AnimationTargetTransformRotationY}, []AnimationKeyframe[float32]{{Time: 0, Value: .2}, {Time: 1, Value: .4}}),
			NewAnimationTrack(AnimationBinding{Path: "card", Target: AnimationTargetTransformRotationZ}, []AnimationKeyframe[float32]{{Time: 0, Value: .3}, {Time: 1, Value: .5}}),
		},
	}
	world := ecs.NewWorld()
	childSpawner := world.NewSpawner1[Transform]()
	child := childSpawner.Spawn(NewTransform())
	animator := NewBoundAnimator(clip, false, ecs.Entity(0), []ecs.Entity{child, child, child}, nil, nil)
	animator.Seek(.5)
	animator.Pause()
	rootSpawner := world.NewSpawner1[Animator]()
	root := rootSpawner.Spawn(animator)
	if err := UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	rotation, _ := world.Get[Transform](child)
	want := mgl32.QuatRotate(.4, mgl32.Vec3{0, 0, 1}).Mul(mgl32.QuatRotate(.3, mgl32.Vec3{0, 1, 0})).Mul(mgl32.QuatRotate(.2, mgl32.Vec3{1, 0, 0})).Normalize()
	got := rotation.Rotation.Mat4()
	wantMatrix := want.Mat4()
	for index := range got {
		if math.Abs(float64(got[index]-wantMatrix[index])) > 1e-5 {
			t.Fatalf("sampled child rotation = %v, want %v", rotation.Rotation, want)
		}
	}
	_ = root
}

func TestBoundRotationUpdateDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	targetSpawner := world.NewSpawner1[Transform]()
	targets := make([]ecs.Entity, 8)
	for index := range targets {
		targets[index] = targetSpawner.Spawn(NewTransform())
	}
	rotationTargets := [...]AnimationTarget{
		AnimationTargetTransformRotationX,
		AnimationTargetTransformRotationY,
		AnimationTargetTransformRotationZ,
	}
	tracks := make([]AnimationTrack[float32], 0, len(targets)*len(rotationTargets))
	boundTargets := make([]ecs.Entity, 0, cap(tracks))
	for _, targetEntity := range targets {
		for _, target := range rotationTargets {
			tracks = append(tracks, NewAnimationTrack(
				AnimationBinding{Path: "target", Target: target},
				[]AnimationKeyframe[float32]{{Time: 0, Value: .25}},
			))
			boundTargets = append(boundTargets, targetEntity)
		}
	}
	clip := &AnimationClip{Duration: 1, FloatTracks: tracks}
	if err := clip.Prepare(); err != nil {
		t.Fatal(err)
	}
	animatorSpawner := world.NewSpawner1[Animator]()
	animatorEntity := animatorSpawner.Spawn(NewBoundAnimator(clip, true, ecs.Entity(0), boundTargets, nil, nil))
	animator := world.GetMut[Animator](animatorEntity)
	if animator == nil {
		t.Fatal("bound animator was not spawned")
	}
	if len(animator.rotationBaselines) != len(tracks) || len(animator.rotationGroupFirst) != len(tracks) || len(animator.rotationGroupNext) != len(tracks) {
		t.Fatalf("prepared rotation storage = %d/%d/%d, want %d", len(animator.rotationBaselines), len(animator.rotationGroupFirst), len(animator.rotationGroupNext), len(tracks))
	}
	clock := NewTime()
	clock.Set(16*time.Millisecond, 16*time.Millisecond, 1)
	world.InsertResource(clock)
	system := UpdateWorldAnimatorSystem("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if len(animator.rotationGroupFirst) != len(tracks) || len(animator.rotationGroupNext) != len(tracks) {
		t.Fatalf("rotation groups = %d/%d, want %d", len(animator.rotationGroupFirst), len(animator.rotationGroupNext), len(tracks))
	}
	for index := range targets {
		first := index * len(rotationTargets)
		if animator.rotationGroupFirst[first] != first || animator.rotationGroupNext[first+2] != -1 {
			t.Fatalf("rotation group %d = first %d next %d, want head %d and tail -1", index, animator.rotationGroupFirst[first], animator.rotationGroupNext[first+2], first)
		}
		if animator.rotationGroupNext[first] != first+1 || animator.rotationGroupNext[first+1] != first+2 {
			t.Fatalf("rotation group %d chain = %v, want [%d %d -1]", index, animator.rotationGroupNext[first:first+3], first+1, first+2)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		animator.Seek(.25)
		clock.Set(16*time.Millisecond, 16*time.Millisecond, 1)
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("bound rotation update allocated %.0f objects, want 0", allocations)
	}
}

func TestAnimatorComposesXYZRotationTracksOnTransform(t *testing.T) {
	clip := &AnimationClip{Duration: 1, FloatTracks: []AnimationTrack[float32]{
		NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformRotationX}, []AnimationKeyframe[float32]{{Time: 0, Value: .2}}),
		NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformRotationY}, []AnimationKeyframe[float32]{{Time: 0, Value: .3}}),
		NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformRotationZ}, []AnimationKeyframe[float32]{{Time: 0, Value: .4}}),
	}}
	world := ecs.NewWorld()
	spawner := world.NewSpawner2[Animator, Transform]()
	entity := spawner.Spawn(NewAnimator(clip, false), NewTransform())
	if err := UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	transform, _ := world.Get[Transform](entity)
	want := mgl32.QuatRotate(.4, mgl32.Vec3{0, 0, 1}).
		Mul(mgl32.QuatRotate(.3, mgl32.Vec3{0, 1, 0})).
		Mul(mgl32.QuatRotate(.2, mgl32.Vec3{1, 0, 0})).Normalize().Mat4()
	got := transform.Rotation.Mat4()
	for index := range got {
		if math.Abs(float64(got[index]-want[index])) > 1e-5 {
			t.Fatalf("rotation matrix[%d] = %v, want %v", index, got[index], want[index])
		}
	}
}

func TestAnimatorEmitsEventsAcrossLoopBoundary(t *testing.T) {
	world := ecs.NewWorld()
	clip := &AnimationClip{Duration: 1, Events: []AnimationEvent{{Time: 0, Name: "start"}, {Time: .75, Name: "hit", Value: "heavy"}}}
	spawner := world.NewSpawner1[Animator]()
	entity := spawner.Spawn(NewAnimator(clip, true))
	clock := NewTime()
	clock.Set(800*time.Millisecond, 800*time.Millisecond, 1)
	world.InsertResource(clock)
	if err := UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatalf("first update: %v", err)
	}
	reader := &ecs.MessageReader[AnimationEventMessage]{}
	first := reader.Read(world)
	if len(first) != 2 || first[0].Name != "start" || first[1].Name != "hit" || first[1].Value != "heavy" {
		t.Fatalf("first events = %+v", first)
	}
	clock.Set(400*time.Millisecond, 1200*time.Millisecond, 2)
	if err := UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatalf("wrapped update: %v", err)
	}
	wrapped := reader.Read(world)
	if len(wrapped) != 1 || wrapped[0].Entity != entity || wrapped[0].Name != "start" {
		t.Fatalf("wrapped events = %+v", wrapped)
	}
}

func TestUpdateAnimatorAppliesTypedTracks(t *testing.T) {
	world := ecs.NewWorld()
	first := graphics.CreateTextureByColor(graphics.White)
	second := graphics.CreateTextureByColor(graphics.Black)
	t.Cleanup(func() { _ = first.Dispose() })
	t.Cleanup(func() { _ = second.Dispose() })
	clip := &AnimationClip{
		Duration: 1,
		TextureTracks: []AnimationTrack[*graphics.Texture]{NewAnimationTrack(AnimationBinding{Target: AnimationTargetSpriteTexture}, []AnimationKeyframe[*graphics.Texture]{
			{Time: 0, Value: first, Tween: AnimationStep},
			{Time: .5, Value: second, Tween: AnimationStep},
		})},
		FloatTracks: []AnimationTrack[float32]{NewAnimationTrack(AnimationBinding{Target: AnimationTargetSpriteAlpha}, []AnimationKeyframe[float32]{
			{Time: 0, Value: 0, Tween: AnimationLinear},
			{Time: 1, Value: 1},
		})},
	}
	spawner := world.NewSpawner3[Animator, Sprite2D, Transform]()
	slice := NineSlice2D{Left: 2, Right: 2, Top: 2, Bottom: 2}
	entity := spawner.Spawn(
		NewAnimator(clip, false), Sprite2D{Slice: &slice}, NewTransform(),
	)
	animator := world.GetMut[Animator](entity)
	animator.Seek(.75)
	animator.Pause()
	if err := UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatalf("update: %v", err)
	}
	sprite, _ := world.Get[Sprite2D](entity)
	if sprite.Texture != second || sprite.Alpha != .75 || sprite.Slice != nil {
		t.Fatalf("sprite was not sampled: texture=%p alpha=%f slice=%+v", sprite.Texture, sprite.Alpha, sprite.Slice)
	}
}

func TestUpdateAnimatorAppliesTransformTracksToUITransform(t *testing.T) {
	world := ecs.NewWorld()
	clip := &AnimationClip{
		Duration: 1,
		FloatTracks: []AnimationTrack[float32]{
			NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformPositionX}, []AnimationKeyframe[float32]{{Time: 0, Value: 10}, {Time: 1, Value: 30}}),
			NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformRotationZ}, []AnimationKeyframe[float32]{{Time: 0, Value: 0}, {Time: 1, Value: 1}}),
		},
	}
	animator := NewAnimator(clip, false)
	animator.Seek(.5)
	animator.Pause()
	spawner := world.NewSpawner2[Animator, UITransform]()
	entity := spawner.Spawn(animator, NewUITransform())
	if err := UpdateUIAnimatorSystem("").Run(world); err != nil {
		t.Fatalf("update: %v", err)
	}
	transform, _ := world.Get[UITransform](entity)
	if transform.Translation.X.Value != 20 || transform.Rotation != .5 {
		t.Fatalf("UI transform was not sampled: %+v", transform)
	}
}

func TestPausedAnimatorOnlyReappliesAfterSeek(t *testing.T) {
	world := ecs.NewWorld()
	clip := &AnimationClip{Duration: 1, FloatTracks: []AnimationTrack[float32]{
		NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformPositionX}, []AnimationKeyframe[float32]{
			{Time: 0, Value: 0, Tween: AnimationLinear}, {Time: 1, Value: 10},
		}),
	}}
	animator := NewAnimator(clip, false)
	animator.Seek(.5)
	animator.Pause()
	spawner := world.NewSpawner2[Animator, Transform]()
	entity := spawner.Spawn(animator, NewTransform())
	system := UpdateWorldAnimatorSystem("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	transform := world.GetMut[Transform](entity)
	transform.Translation[0] = 99
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if transform.Translation[0] != 99 {
		t.Fatalf("paused animator reapplied unchanged track: %v", transform.Translation[0])
	}
	current := world.GetMut[Animator](entity)
	current.Seek(.75)
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if transform.Translation[0] != 7.5 {
		t.Fatalf("seeked animator value = %v, want 7.5", transform.Translation[0])
	}
}

func TestUpdateWorldAnimatorSystemDoesNotAllocateAfterWarmup(t *testing.T) {
	for _, playing := range []bool{true, false} {
		name := "paused"
		if playing {
			name = "playing"
		}
		t.Run(name, func(t *testing.T) {
			world := animationBenchmarkWorld(16, playing)
			system := UpdateWorldAnimatorSystem("")
			if err := system.Run(world); err != nil {
				t.Fatal(err)
			}

			if allocations := testing.AllocsPerRun(100, func() {
				if err := system.Run(world); err != nil {
					t.Fatal(err)
				}
			}); allocations != 0 {
				t.Fatalf("animator update allocated %.0f objects, want 0", allocations)
			}
		})
	}
}

func TestBoundAnimatorUpdateDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	clip := &AnimationClip{Duration: 1, FloatTracks: []AnimationTrack[float32]{
		NewAnimationTrack(AnimationBinding{Path: "child", Target: AnimationTargetTransformPositionX}, []AnimationKeyframe[float32]{
			{Time: 0, Value: 0, Tween: AnimationLinear}, {Time: 1, Value: 10},
		}),
	}}
	spawner := world.NewSpawner2[Transform, Animator]()
	root := spawner.Spawn(NewTransform(), NewBoundAnimator(clip, true, ecs.Entity(0), nil, nil, nil))
	childSpawner := world.NewSpawner1[Transform]()
	child := childSpawner.Spawn(NewTransform())
	animator := world.GetMut[Animator](root)
	if animator == nil {
		t.Fatal("bound animator was not spawned")
	}
	animator.BindTargets(root, []ecs.Entity{child}, nil, nil)
	clock := NewTime()
	world.InsertResource(clock)
	clock.Set(16*time.Millisecond, 16*time.Millisecond, 1)
	system := UpdateWorldAnimatorSystem("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if animator.rotationBaselines != nil || animator.rotationGroupFirst != nil || animator.rotationGroupNext != nil {
		t.Fatal("non-rotation clip allocated rotation baselines")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		clock.Set(16*time.Millisecond, 16*time.Millisecond, 1)
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("bound animator update allocated %.0f objects, want 0", allocations)
	}
}

func TestAnimatorReusesCursorStorageWhenClipChanges(t *testing.T) {
	clipA := &AnimationClip{Duration: 1, FloatTracks: []AnimationTrack[float32]{NewAnimationTrack(
		AnimationBinding{Target: AnimationTargetTransformPositionX},
		[]AnimationKeyframe[float32]{{Time: 0, Value: 0}, {Time: 1, Value: 1}},
	)}}
	clipB := &AnimationClip{Duration: 1, FloatTracks: []AnimationTrack[float32]{NewAnimationTrack(
		AnimationBinding{Target: AnimationTargetTransformPositionY},
		[]AnimationKeyframe[float32]{{Time: 0, Value: 0}, {Time: 1, Value: 1}},
	)}}
	if err := clipA.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := clipB.Prepare(); err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	spawner := world.NewSpawner2[Animator, Transform]()
	entity := spawner.Spawn(NewAnimator(clipA, false), NewTransform())
	system := UpdateWorldAnimatorSystem("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	animator := world.GetMut[Animator](entity)
	floatCursors, _, _ := animator.cursorSlices()
	if animator == nil || len(floatCursors) != 1 {
		t.Fatal("animator cursors were not initialized")
	}
	storage := &animator.cursors[0]
	animator.Clip = clipB
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if &animator.cursors[0] != storage {
		t.Fatal("changing clips discarded reusable cursor storage")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		animator.Clip = clipA
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
		animator.Clip = clipB
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("switching prepared clips allocated %.0f objects, want 0", allocations)
	}
}

func TestNewAnimatorPreallocatesPreparedClipCursors(t *testing.T) {
	clip := &AnimationClip{Duration: 1, FloatTracks: []AnimationTrack[float32]{NewAnimationTrack(
		AnimationBinding{Target: AnimationTargetTransformPositionX},
		[]AnimationKeyframe[float32]{{Time: 0, Value: 0}, {Time: 1, Value: 1}},
	)}}
	if err := clip.Prepare(); err != nil {
		t.Fatal(err)
	}
	animator := NewAnimator(clip, false)
	if animator.cursorClip != clip || len(animator.cursors) != 1 {
		t.Fatalf("prepared animator cursor storage = clip:%p len:%d", animator.cursorClip, len(animator.cursors))
	}
}

func TestAnimatorTrackCursorHandlesLoopWrap(t *testing.T) {
	world := ecs.NewWorld()
	clip := &AnimationClip{Duration: 1, FloatTracks: []AnimationTrack[float32]{
		NewAnimationTrack(AnimationBinding{Target: AnimationTargetTransformPositionX}, []AnimationKeyframe[float32]{
			{Time: 0, Value: 0, Tween: AnimationLinear},
			{Time: .25, Value: 1, Tween: AnimationLinear},
			{Time: .5, Value: 2, Tween: AnimationLinear},
			{Time: .75, Value: 3, Tween: AnimationLinear},
			{Time: 1, Value: 4},
		}),
	}}
	spawner := world.NewSpawner2[Animator, Transform]()
	entity := spawner.Spawn(NewAnimator(clip, true), NewTransform())
	clock := NewTime()
	world.InsertResource(clock)
	clock.Set(800*time.Millisecond, 800*time.Millisecond, 1)
	if err := UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	clock.Set(400*time.Millisecond, 1200*time.Millisecond, 2)
	if err := UpdateWorldAnimatorSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	transform, _ := world.Get[Transform](entity)
	if math.Abs(float64(transform.Translation[0]-.8)) > 1e-5 {
		t.Fatalf("wrapped cursor value = %v, want .8", transform.Translation[0])
	}
}

func TestAnimatorAdvanceWrapsMultipleLoops(t *testing.T) {
	animator := NewAnimator(&AnimationClip{Duration: .75}, true)
	animator.Advance(2)
	if animator.loops != 2 || math.Abs(float64(animator.Time()-.5)) > 1e-6 {
		t.Fatalf("wrapped animator = loops:%d time:%v, want loops:2 time:.5", animator.loops, animator.Time())
	}
}
