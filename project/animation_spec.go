package project

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

type AnimationKeyframeSpec[T any] struct {
	Time       float32              `json:"time"`
	Value      T                    `json:"value"`
	InTangent  T                    `json:"inTangent,omitzero"`
	OutTangent T                    `json:"outTangent,omitzero"`
	Tween      scene.AnimationTween `json:"tween,omitempty"`
	Curve      scene.AnimationCurve `json:"curve,omitzero"`
}

// EffectiveAnimationTween returns the curve used for the segment starting at
// key, defaulting to the track's type-specific curve.
func EffectiveAnimationTween[T any](key AnimationKeyframeSpec[T], defaultTween scene.AnimationTween) scene.AnimationTween {
	tween := key.Tween
	if tween == "" {
		tween = defaultTween
	}
	return tween
}

type AnimationTrackSpec[T any] struct {
	Binding   scene.AnimationBinding     `json:"binding"`
	Keyframes []AnimationKeyframeSpec[T] `json:"keyframes"`
	TimeTween scene.AnimationTween       `json:"timeTween,omitempty"`
	TimeCurve scene.AnimationCurve       `json:"timeCurve,omitzero"`
}

type AnimationEventSpec struct {
	Time  float32 `json:"time"`
	Name  string  `json:"name"`
	Value string  `json:"value,omitempty"`
}

type AnimationClipSpec struct {
	Duration      float32                           `json:"duration"`
	SampleRate    float32                           `json:"sampleRate"`
	Loop          bool                              `json:"loop"`
	FloatTracks   []AnimationTrackSpec[float32]     `json:"floatTracks,omitempty"`
	TextureTracks []AnimationTrackSpec[TextureSpec] `json:"textureTracks,omitempty"`
	BoolTracks    []AnimationTrackSpec[bool]        `json:"boolTracks,omitempty"`
	Events        []AnimationEventSpec              `json:"events,omitempty"`
}

// HasContent reports whether the clip contains authored animation data.
func (s AnimationClipSpec) HasContent() bool {
	return s.Duration != 0 || s.SampleRate != 0 || s.Loop ||
		len(s.FloatTracks) != 0 || len(s.TextureTracks) != 0 ||
		len(s.BoolTracks) != 0 || len(s.Events) != 0
}

// Clone returns an independent authoring copy, including texture slice data.
func (s AnimationClipSpec) Clone() AnimationClipSpec {
	clone := s
	clone.FloatTracks = cloneAnimationTrackSpecs(s.FloatTracks)
	clone.TextureTracks = cloneAnimationTrackSpecs(s.TextureTracks)
	clone.BoolTracks = cloneAnimationTrackSpecs(s.BoolTracks)
	clone.Events = slices.Clone(s.Events)
	for track := range clone.TextureTracks {
		for key := range clone.TextureTracks[track].Keyframes {
			if s.TextureTracks[track].Keyframes[key].Value.Slice != nil {
				value := *s.TextureTracks[track].Keyframes[key].Value.Slice
				clone.TextureTracks[track].Keyframes[key].Value.Slice = &value
			}
		}
	}
	return clone
}

func cloneAnimationTrackSpecs[T any](tracks []AnimationTrackSpec[T]) []AnimationTrackSpec[T] {
	clone := slices.Clone(tracks)
	for index := range clone {
		clone[index].Keyframes = slices.Clone(tracks[index].Keyframes)
	}
	return clone
}

// Validate validates an inline animation clip owned by a scene or prefab
// node.
func (s AnimationClipSpec) Validate() error {
	id := "inline"
	if s.Duration <= 0 || !finiteAnimationSpecValue(s.Duration) {
		return fmt.Errorf("animation %q duration must be positive", id)
	}
	if !finiteAnimationSpecValue(s.SampleRate) || s.SampleRate < 0 {
		return fmt.Errorf("animation %q sampleRate must be non-negative", id)
	}
	if len(s.FloatTracks)+len(s.TextureTracks)+len(s.BoolTracks) == 0 {
		return fmt.Errorf("animation %q requires at least one track", id)
	}
	seen := make(map[scene.AnimationBinding]struct{}, len(s.FloatTracks)+len(s.TextureTracks)+len(s.BoolTracks))
	if err := validateAnimationTracks(id, s.FloatTracks, s.Duration, scene.AnimationLinear, seen, func(index int, track AnimationTrackSpec[float32]) error {
		if !isFloatAnimationTarget(track.Binding.Target) {
			return fmt.Errorf("animation %q float track %d has unsupported target %q", id, index, track.Binding.Target)
		}
		return nil
	}, func(_ int, track AnimationTrackSpec[float32]) error {
		for keyIndex, key := range track.Keyframes {
			if !finiteAnimationSpecValue(key.Value) {
				return fmt.Errorf("animation %q binding %q target %q keyframe %d value must be finite", id, track.Binding.Path, track.Binding.Target, keyIndex)
			}
			if !finiteAnimationSpecValue(key.InTangent) || !finiteAnimationSpecValue(key.OutTangent) {
				return fmt.Errorf("animation %q binding %q target %q keyframe %d tangents must be finite", id, track.Binding.Path, track.Binding.Target, keyIndex)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := validateAnimationTracks(id, s.TextureTracks, s.Duration, scene.AnimationStep, seen, func(index int, track AnimationTrackSpec[TextureSpec]) error {
		if track.Binding.Target != scene.AnimationTargetSpriteTexture {
			return fmt.Errorf("animation %q texture track %d has unsupported target %q", id, index, track.Binding.Target)
		}
		return nil
	}, func(_ int, track AnimationTrackSpec[TextureSpec]) error {
		for keyIndex := range track.Keyframes {
			if err := track.Keyframes[keyIndex].Value.validate(false); err != nil {
				return fmt.Errorf("animation %q binding %q target %q keyframe %d texture %w", id, track.Binding.Path, track.Binding.Target, keyIndex, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := validateAnimationTracks(id, s.BoolTracks, s.Duration, scene.AnimationStep, seen, func(index int, track AnimationTrackSpec[bool]) error {
		if track.Binding.Target != scene.AnimationTargetVisibility {
			return fmt.Errorf("animation %q bool track %d has unsupported target %q", id, index, track.Binding.Target)
		}
		return nil
	}, nil); err != nil {
		return err
	}
	previousEventTime := float32(-1)
	for index, event := range s.Events {
		if strings.TrimSpace(event.Name) == "" {
			return fmt.Errorf("animation %q event %d name is required", id, index)
		}
		if !finiteAnimationSpecValue(event.Time) || event.Time < 0 || event.Time > s.Duration {
			return fmt.Errorf("animation %q event %d time must be between 0 and duration", id, index)
		}
		if event.Time < previousEventTime {
			return fmt.Errorf("animation %q event times must be sorted", id)
		}
		previousEventTime = event.Time
	}
	return nil
}

func isFloatAnimationTarget(target scene.AnimationTarget) bool {
	switch target {
	case scene.AnimationTargetSpriteAlpha,
		scene.AnimationTargetTransformPositionX,
		scene.AnimationTargetTransformPositionY,
		scene.AnimationTargetTransformPositionZ,
		scene.AnimationTargetTransformScaleX,
		scene.AnimationTargetTransformScaleY,
		scene.AnimationTargetTransformScaleZ,
		scene.AnimationTargetTransformRotationX,
		scene.AnimationTargetTransformRotationY,
		scene.AnimationTargetTransformRotationZ:
		return true
	default:
		return false
	}
}

func finiteAnimationSpecValue(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func claimAnimationBinding(id string, binding scene.AnimationBinding, seen map[scene.AnimationBinding]struct{}) error {
	if _, exists := seen[binding]; exists {
		return fmt.Errorf("animation %q contains duplicate binding %q target %q", id, binding.Path, binding.Target)
	}
	seen[binding] = struct{}{}
	return nil
}

func validateAnimationTracks[T any](id string, tracks []AnimationTrackSpec[T], duration float32, defaultTween scene.AnimationTween, seen map[scene.AnimationBinding]struct{}, validateTarget, validateValues func(int, AnimationTrackSpec[T]) error) error {
	for index, track := range tracks {
		if err := scene.ValidateAnimationTimeCurve(track.TimeTween, track.TimeCurve); err != nil {
			return fmt.Errorf("animation %q track %d target %q: %w", id, index, track.Binding.Target, err)
		}
		if validateTarget != nil {
			if err := validateTarget(index, track); err != nil {
				return err
			}
		}
		if err := validateAnimationTrack(id, track.Binding, track.Keyframes, duration, defaultTween); err != nil {
			return err
		}
		if validateValues != nil {
			if err := validateValues(index, track); err != nil {
				return err
			}
		}
		if err := claimAnimationBinding(id, track.Binding, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateAnimationTrack[T any](id string, binding scene.AnimationBinding, keys []AnimationKeyframeSpec[T], duration float32, defaultTween scene.AnimationTween) error {
	if strings.TrimSpace(string(binding.Target)) == "" {
		return fmt.Errorf("animation %q contains a track without a target", id)
	}
	if err := validateAnimationBindingPath(binding.Path); err != nil {
		return fmt.Errorf("animation %q target %q: %w", id, binding.Target, err)
	}
	if len(keys) == 0 {
		return fmt.Errorf("animation %q binding %q target %q requires at least one keyframe", id, binding.Path, binding.Target)
	}
	previous := float32(-1)
	for index, key := range keys {
		if !finiteAnimationSpecValue(key.Time) || key.Time < 0 || key.Time > duration {
			return fmt.Errorf("animation %q binding %q target %q keyframe %d time must be between 0 and duration", id, binding.Path, binding.Target, index)
		}
		if index > 0 && key.Time <= previous {
			return fmt.Errorf("animation %q binding %q target %q keyframe times must be strictly increasing", id, binding.Path, binding.Target)
		}
		tween := key.Tween
		if tween == "" {
			tween = defaultTween
		}
		if !tween.Valid() {
			return fmt.Errorf("animation %q binding %q target %q keyframe %d has unsupported tween %q", id, binding.Path, binding.Target, index, tween)
		}
		if defaultTween == scene.AnimationStep && tween != scene.AnimationStep {
			return fmt.Errorf("animation %q binding %q target %q only supports step tween", id, binding.Path, binding.Target)
		}
		if tween == scene.AnimationBezier {
			if err := key.Curve.Validate(); err != nil {
				return fmt.Errorf("animation %q binding %q target %q keyframe %d: %w", id, binding.Path, binding.Target, index, err)
			}
		}
		previous = key.Time
	}
	return nil
}

func validateAnimationBindingPath(path string) error {
	if path == "" {
		return nil
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.TrimSpace(segment) != segment {
			return fmt.Errorf("animation binding path %q is invalid", path)
		}
	}
	return nil
}

// compileAnimationClipSpec builds the immutable runtime clip embedded in a
// scene or prefab animation component. Every non-empty binding path is
// resolved by the scene spawn plan to a real descendant entity.
func (r *Resources) compileAnimationClipSpec(spec AnimationClipSpec, id string) (*scene.AnimationClip, error) {
	if r == nil {
		return nil, fmt.Errorf("resources are nil")
	}
	if strings.TrimSpace(id) == "" {
		id = "inline"
	}
	if spec.SampleRate <= 0 {
		spec.SampleRate = 60
	}
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("animation %q: %w", id, err)
	}
	clip := &scene.AnimationClip{Duration: spec.Duration}
	clip.Events = make([]scene.AnimationEvent, len(spec.Events))
	for index, event := range spec.Events {
		clip.Events[index] = scene.AnimationEvent{Time: event.Time, Name: event.Name, Value: event.Value}
	}
	clip.FloatTracks = compileAnimationTracks(spec.FloatTracks, scene.AnimationLinear, func(value float32) float32 { return value })
	clip.BoolTracks = compileAnimationTracks(spec.BoolTracks, scene.AnimationStep, func(value bool) bool { return value })
	clip.TextureTracks = make([]scene.AnimationTrack[*graphics.Texture], len(spec.TextureTracks))
	for trackIndex, track := range spec.TextureTracks {
		keys := make([]scene.AnimationKeyframe[*graphics.Texture], len(track.Keyframes))
		for keyIndex, key := range track.Keyframes {
			texture, err := r.texture(&key.Value)
			if err != nil {
				return nil, fmt.Errorf("load animation %q binding %q target %q keyframe %d: %w", id, track.Binding.Path, track.Binding.Target, keyIndex, err)
			}
			keys[keyIndex] = compileAnimationKeyframe(key, texture, scene.AnimationStep)
		}
		// The compiler owns this freshly allocated keyframe slice, so avoid the
		// defensive copy performed by the public constructor.
		clip.TextureTracks[trackIndex] = scene.AnimationTrack[*graphics.Texture]{Binding: track.Binding, Keyframes: keys, TimeTween: track.TimeTween, TimeCurve: track.TimeCurve}
	}
	if err := clip.Prepare(); err != nil {
		return nil, fmt.Errorf("animation %q: %w", id, err)
	}
	return clip, nil
}

func compileAnimationTracks[S, T any](tracks []AnimationTrackSpec[S], defaultTween scene.AnimationTween, convert func(S) T) []scene.AnimationTrack[T] {
	result := make([]scene.AnimationTrack[T], len(tracks))
	for trackIndex, track := range tracks {
		keys := make([]scene.AnimationKeyframe[T], len(track.Keyframes))
		for keyIndex, key := range track.Keyframes {
			value := convert(key.Value)
			keys[keyIndex] = compileAnimationKeyframe(key, value, defaultTween)
			keys[keyIndex].InTangent = convert(key.InTangent)
			keys[keyIndex].OutTangent = convert(key.OutTangent)
		}
		// The compiler owns this freshly allocated keyframe slice, so avoid the
		// defensive copy performed by the public constructor.
		result[trackIndex] = scene.AnimationTrack[T]{Binding: track.Binding, Keyframes: keys, TimeTween: track.TimeTween, TimeCurve: track.TimeCurve}
	}
	return result
}

func compileAnimationKeyframe[S, T any](key AnimationKeyframeSpec[S], value T, defaultTween scene.AnimationTween) scene.AnimationKeyframe[T] {
	return scene.AnimationKeyframe[T]{
		Time: key.Time, Value: value,
		Tween: EffectiveAnimationTween(key, defaultTween),
		Curve: key.Curve,
	}
}
