package project

import (
	"fmt"
	"strings"

	"github.com/diiyw/onix/internal/typedbin"
)

// ModelAnimationSource is a named collection of vector/quaternion keyframe
// tracks. Paths identify imported nodes; unique node names are also accepted.
type ModelAnimationSource struct {
	BlendMode int                         `json:"blendMode,omitempty"`
	Name      string                      `json:"name"`
	Duration  float32                     `json:"duration"`
	Tracks    []ModelAnimationTrackSource `json:"tracks"`
}

type ModelAnimationTrackSource struct {
	Name   string    `json:"name"`
	Type   string    `json:"type"`
	Times  []float32 `json:"times"`
	Values []float32 `json:"values"`
}

func (r *Resources) loadModelAnimations(path string, nodes []ModelNode) ([]compiledModelAnimation, error) {
	resolved, err := ResolveResourcePath(r.baseDir, path)
	if err != nil {
		return nil, err
	}
	blob, err := openProjectBlob(r.source, resolved)
	if err != nil {
		return nil, err
	}
	defer blob.Close()
	data, err := blob.Bytes()
	if err != nil {
		return nil, err
	}
	var clips []ModelAnimationSource
	if typedbin.IsEncoded(data) {
		clips, err = typedbin.Unmarshal[[]ModelAnimationSource](data)
	} else {
		clips, err = decodeJSON[[]ModelAnimationSource](data, path)
	}
	if err != nil {
		return nil, err
	}
	return compileAnimationSources(clips, nodes)
}

func compileAnimationSources(clips []ModelAnimationSource, nodes []ModelNode) ([]compiledModelAnimation, error) {
	paths := make(map[string]string, len(nodes))
	names := make(map[string]string, len(nodes))
	for _, node := range nodes {
		paths[node.Path] = node.Path
		if _, exists := names[node.Name]; exists {
			names[node.Name] = ""
		} else {
			names[node.Name] = node.Path
		}
	}
	result := make([]compiledModelAnimation, len(clips))
	clipNames := make(map[string]bool, len(clips))
	for i, clip := range clips {
		if clip.Name == "" || clipNames[clip.Name] || !finiteFloat32(clip.Duration) || clip.Duration < 0 || clip.BlendMode != 0 && clip.BlendMode != 2500 {
			return nil, fmt.Errorf("invalid model animation %q", clip.Name)
		}
		clipNames[clip.Name] = true
		result[i] = compiledModelAnimation{Name: clip.Name, Duration: clip.Duration}
		bindings := make(map[string]bool, len(clip.Tracks))
		for _, source := range clip.Tracks {
			end := strings.LastIndexByte(source.Name, '.')
			if end < 0 {
				return nil, fmt.Errorf("animation %q: invalid track %q", clip.Name, source.Name)
			}
			path, ok := paths[source.Name[:end]]
			if !ok {
				path, ok = names[source.Name[:end]]
			}
			if !ok || path == "" {
				return nil, fmt.Errorf("animation %q: missing or ambiguous node %q", clip.Name, source.Name[:end])
			}
			track := compiledModelAnimationTrack{Path: path, Interpolation: "LINEAR", Components: 3}
			switch source.Name[end+1:] {
			case "position":
				track.Target = "transform.translation"
			case "scale":
				track.Target = "transform.scale"
			case "quaternion":
				track.Target, track.Components = "transform.rotation", 4
			default:
				return nil, fmt.Errorf("animation %q: unsupported track %q", clip.Name, source.Name)
			}
			if source.Type != "vector" && track.Components == 3 || source.Type != "quaternion" && track.Components == 4 {
				return nil, fmt.Errorf("animation track %q has mismatched type", source.Name)
			}
			key := path + track.Target
			if bindings[key] {
				return nil, fmt.Errorf("duplicate animation track %q", source.Name)
			}
			bindings[key] = true
			if len(source.Times) == 0 || len(source.Values) != len(source.Times)*int(track.Components) {
				return nil, fmt.Errorf("animation track %q has invalid keyframe lengths", source.Name)
			}
			track.Keyframes = make([]compiledModelAnimationKeyframe, len(source.Times))
			for k, time := range source.Times {
				if !finiteFloat32(time) || time < 0 || time > clip.Duration+1e-5 || k > 0 && time <= source.Times[k-1] {
					return nil, fmt.Errorf("animation track %q has invalid keyframe time", source.Name)
				}
				frame := &track.Keyframes[k]
				frame.Time = time
				copy(frame.Value[:], source.Values[k*int(track.Components):(k+1)*int(track.Components)])
				length := float32(0)
				for _, value := range frame.Value {
					if !finiteFloat32(value) {
						return nil, fmt.Errorf("animation track %q has non-finite value", source.Name)
					}
					length += value * value
				}
				if track.Components == 4 && length < 1e-12 {
					return nil, fmt.Errorf("animation track %q has zero quaternion", source.Name)
				}
			}
			result[i].Tracks = append(result[i].Tracks, track)
		}
	}
	return result, nil
}
