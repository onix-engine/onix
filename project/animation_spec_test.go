package project

import (
	"encoding/json"
	"github.com/diiyw/onix/scene"
	"math"
	"testing"
)

func TestAnimationClipSpecValidatesTypedTracks(t *testing.T) {
	spec := AnimationClipSpec{
		Duration: 1, SampleRate: 12, Loop: true,
		TextureTracks: []AnimationTrackSpec[TextureSpec]{{
			Binding: scene.AnimationBinding{Target: scene.AnimationTargetSpriteTexture},
			Keyframes: []AnimationKeyframeSpec[TextureSpec]{
				{Time: 0, Value: TextureSpec{Color: "#ffffff"}, Tween: scene.AnimationStep},
				{Time: .5, Value: TextureSpec{Color: "#000000"}, Tween: scene.AnimationStep},
			},
		}},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	spec.TextureTracks[0].Keyframes[1].Time = 0
	if err := spec.Validate(); err == nil {
		t.Fatal("expected duplicate keyframe time to fail")
	}
}

func TestAnimationClipSpecRejectsLegacyTweenSpellings(t *testing.T) {
	spec := AnimationClipSpec{
		Duration: 1,
		FloatTracks: []AnimationTrackSpec[float32]{
			{
				Binding:   scene.AnimationBinding{Target: scene.AnimationTargetTransformPositionX},
				Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 0, Tween: "ease-in"}},
			},
		},
	}
	if err := spec.Validate(); err == nil {
		t.Fatal("legacy tween spelling was accepted")
	}
}

func TestAnimationClipSpecHasContent(t *testing.T) {
	if (AnimationClipSpec{}).HasContent() {
		t.Fatal("empty animation clip has content")
	}
	if !(AnimationClipSpec{Duration: 1}).HasContent() {
		t.Fatal("authored duration was not detected")
	}
}

func TestAnimationClipSpecCloneDeepCopiesTracksAndSlices(t *testing.T) {
	slice := &NineSlice{Left: 2, Right: 3, Top: 4, Bottom: 5}
	spec := AnimationClipSpec{
		Duration: 1,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding:   scene.AnimationBinding{Target: scene.AnimationTargetTransformPositionX},
			Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 1}},
		}},
		TextureTracks: []AnimationTrackSpec[TextureSpec]{{
			Binding:   scene.AnimationBinding{Target: scene.AnimationTargetSpriteTexture},
			Keyframes: []AnimationKeyframeSpec[TextureSpec]{{Time: 0, Value: TextureSpec{Image: "frame.png", Slice: slice}}},
		}},
		Events: []AnimationEventSpec{{Time: .5, Name: "hit"}},
	}

	clone := spec.Clone()
	clone.FloatTracks[0].Keyframes[0].Value = 2
	clone.TextureTracks[0].Keyframes[0].Value.Slice.Left = 9
	clone.Events[0].Name = "miss"

	if spec.FloatTracks[0].Keyframes[0].Value != 1 || spec.Events[0].Name != "hit" {
		t.Fatal("clone mutation changed the source clip")
	}
	if spec.TextureTracks[0].Keyframes[0].Value.Slice.Left != 2 {
		t.Fatal("clone mutation changed the source texture slice")
	}
	if &clone.FloatTracks[0].Keyframes[0] == &spec.FloatTracks[0].Keyframes[0] ||
		clone.TextureTracks[0].Keyframes[0].Value.Slice == spec.TextureTracks[0].Keyframes[0].Value.Slice {
		t.Fatal("clone retained source backing data")
	}
}

func TestAnimationTweenTakesPrecedenceAndRoundTripsJSON(t *testing.T) {
	spec := AnimationClipSpec{
		Duration: 1, SampleRate: 60,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding: scene.AnimationBinding{Target: scene.AnimationTargetTransformPositionX},
			Keyframes: []AnimationKeyframeSpec[float32]{
				{Time: 0, Value: 0, Tween: scene.AnimationEaseIn},
				{Time: 1, Value: 10},
			},
		}},
	}
	if got := EffectiveAnimationTween(spec.FloatTracks[0].Keyframes[0], scene.AnimationLinear); got != scene.AnimationEaseIn {
		t.Fatalf("effective tween = %q", got)
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var decoded AnimationClipSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	key := decoded.FloatTracks[0].Keyframes[0]
	if key.Tween != scene.AnimationEaseIn {
		t.Fatalf("decoded tween key = %+v", key)
	}
	clip, err := newResources().compileAnimationClipSpec(spec, "inline")
	if err != nil {
		t.Fatal(err)
	}
	value, _ := scene.SampleAnimationTrack(clip.FloatTracks[0], .5, func(a, b, progress float32) float32 { return a + (b-a)*progress })
	if value != 2.5 {
		t.Fatalf("sampled tween = %v, want 2.5", value)
	}
}

func TestInlineCubicSplineTangentsRoundTripAndSample(t *testing.T) {
	spec := AnimationClipSpec{Duration: 1, FloatTracks: []AnimationTrackSpec[float32]{{
		Binding: scene.AnimationBinding{Target: scene.AnimationTargetTransformPositionX},
		Keyframes: []AnimationKeyframeSpec[float32]{
			{Time: 0, Value: 0, OutTangent: 2, Tween: scene.AnimationCubicSpline},
			{Time: 1, Value: 1},
		},
	}}}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var decoded AnimationClipSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	clip, err := newResources().compileAnimationClipSpec(decoded, "cubic")
	if err != nil {
		t.Fatal(err)
	}
	value, ok := scene.SampleAnimationTrack(clip.FloatTracks[0], .5, func(a, b, progress float32) float32 { return a + (b-a)*progress })
	if !ok || math.Abs(float64(value-.75)) > 1e-6 {
		t.Fatalf("cubic sample = %g, %v", value, ok)
	}
}

func TestAnimationEasingPresetsValidateRoundTripAndCompile(t *testing.T) {
	tweens := [...]scene.AnimationTween{
		scene.AnimationEaseInSine, scene.AnimationEaseOutSine, scene.AnimationEaseInOutSine,
		scene.AnimationEaseInCubic, scene.AnimationEaseOutCubic, scene.AnimationEaseInOutCubic,
		scene.AnimationEaseInQuart, scene.AnimationEaseOutQuart, scene.AnimationEaseInOutQuart,
		scene.AnimationEaseInQuint, scene.AnimationEaseOutQuint, scene.AnimationEaseInOutQuint,
		scene.AnimationEaseInExpo, scene.AnimationEaseOutExpo, scene.AnimationEaseInOutExpo,
		scene.AnimationEaseInCirc, scene.AnimationEaseOutCirc, scene.AnimationEaseInOutCirc,
		scene.AnimationEaseInBack, scene.AnimationEaseOutBack, scene.AnimationEaseInOutBack,
		scene.AnimationEaseInElastic, scene.AnimationEaseOutElastic, scene.AnimationEaseInOutElastic,
		scene.AnimationEaseInBounce, scene.AnimationEaseOutBounce, scene.AnimationEaseInOutBounce,
	}
	for _, tween := range tweens {
		t.Run(string(tween), func(t *testing.T) {
			spec := AnimationClipSpec{Duration: 1, FloatTracks: []AnimationTrackSpec[float32]{{
				Binding:   scene.AnimationBinding{Target: scene.AnimationTargetTransformPositionX},
				Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 10, Tween: tween}, {Time: 1, Value: 30}},
			}}}
			if err := spec.Validate(); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(spec)
			if err != nil {
				t.Fatal(err)
			}
			var decoded AnimationClipSpec
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			clip, err := newResources().compileAnimationClipSpec(decoded, "easing")
			if err != nil {
				t.Fatal(err)
			}
			if clip.FloatTracks[0].Keyframes[0].Tween != tween {
				t.Fatal("compilation lost the easing preset")
			}
			got, ok := scene.SampleAnimationTrack(clip.FloatTracks[0], .25, func(a, b, progress float32) float32 { return a + (b-a)*progress })
			want := 10 + 20*scene.SampleAnimationProgress(tween, scene.AnimationCurve{}, .25)
			if !ok || math.Abs(float64(got-want)) > 1e-6 {
				t.Fatalf("compiled sample=%g, want %g", got, want)
			}
		})
	}
}

func TestAnimationSequenceTimeCurveRoundTripsAndCompiles(t *testing.T) {
	for _, tween := range []scene.AnimationTween{scene.AnimationEaseIn, scene.AnimationEaseOutBounce, scene.AnimationBezier} {
		spec := AnimationClipSpec{Duration: 1, TextureTracks: []AnimationTrackSpec[TextureSpec]{{
			Binding: scene.AnimationBinding{Target: scene.AnimationTargetSpriteTexture}, TimeTween: tween, TimeCurve: scene.EaseInAnimationCurve(),
			Keyframes: []AnimationKeyframeSpec[TextureSpec]{
				{Time: 0, Value: TextureSpec{Color: "#ffffff"}, Tween: scene.AnimationStep},
				{Time: .25, Value: TextureSpec{Color: "#ff0000"}, Tween: scene.AnimationStep},
				{Time: .5, Value: TextureSpec{Color: "#00ff00"}, Tween: scene.AnimationStep},
				{Time: .75, Value: TextureSpec{Color: "#0000ff"}, Tween: scene.AnimationStep},
				{Time: 1, Value: TextureSpec{Color: "#000000"}, Tween: scene.AnimationStep},
			},
		}}}
		data, err := json.Marshal(spec)
		if err != nil {
			t.Fatal(err)
		}
		var decoded AnimationClipSpec
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		clip, err := newResources().compileAnimationClipSpec(decoded, "sequence")
		if err != nil {
			t.Fatal(err)
		}
		track := clip.TextureTracks[0]
		if track.TimeTween != tween || track.TimeCurve != spec.TextureTracks[0].TimeCurve {
			t.Fatal("compilation lost the playback curve")
		}
		progress := min(max(scene.SampleAnimationProgress(tween, track.TimeCurve, .5), 0), 1)
		index := int(progress * 4)
		value, ok := scene.SampleAnimationTrack(track, .5, nil)
		if !ok || value != track.Keyframes[index].Value {
			t.Fatal("compiled frame sequence ignored the time curve")
		}
		if track.Keyframes[0].Tween != scene.AnimationStep {
			t.Fatal("time remapping changed discrete frame selection")
		}
	}
}

func TestAnimationSequenceRejectsInvalidTimeCurves(t *testing.T) {
	for _, test := range []struct {
		tween scene.AnimationTween
		curve scene.AnimationCurve
	}{
		{scene.AnimationCubicSpline, scene.AnimationCurve{}}, {"unknown", scene.AnimationCurve{}},
		{scene.AnimationBezier, scene.AnimationCurve{X1: 2}},
		{scene.AnimationBezier, scene.AnimationCurve{Y1: float32(math.NaN())}},
	} {
		spec := AnimationClipSpec{Duration: 1, TextureTracks: []AnimationTrackSpec[TextureSpec]{{
			Binding: scene.AnimationBinding{Target: scene.AnimationTargetSpriteTexture}, TimeTween: test.tween, TimeCurve: test.curve,
			Keyframes: []AnimationKeyframeSpec[TextureSpec]{{Time: 0, Value: TextureSpec{Color: "#ffffff"}}},
		}}}
		if err := spec.Validate(); err == nil {
			t.Fatalf("accepted invalid time curve: %+v", test)
		}
	}
}

func TestInlineAnimationSampleRateMayBeOmittedButNotNegative(t *testing.T) {
	spec := AnimationClipSpec{
		Duration: 1,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding:   scene.AnimationBinding{Target: scene.AnimationTargetTransformPositionX},
			Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 0}},
		}},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("omitted inline sample rate should be accepted: %v", err)
	}
	spec.SampleRate = -1
	if err := spec.Validate(); err == nil {
		t.Fatal("negative inline sample rate should be rejected")
	}
}

func TestAnimationClipSpecValidatesBindingPathsAndEvents(t *testing.T) {
	spec := AnimationClipSpec{
		Duration: 1, SampleRate: 60,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding:   scene.AnimationBinding{Path: "hand", Target: scene.AnimationTargetTransformPositionX},
			Keyframes: []AnimationKeyframeSpec[float32]{{Time: 0, Value: 1}},
		}},
		Events: []AnimationEventSpec{{Time: .5, Name: "hit"}},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	spec.FloatTracks[0].Binding.Path = "hand/"
	if err := spec.Validate(); err == nil {
		t.Fatal("expected invalid binding path to fail")
	}
	spec.FloatTracks[0].Binding.Path = "hand"
	spec.Events[0].Name = ""
	if err := spec.Validate(); err == nil {
		t.Fatal("expected empty event name to fail")
	}
}
