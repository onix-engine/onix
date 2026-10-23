package project

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/go-gl/mathgl/mgl32"
)

type sceneSpawnPlan struct {
	Nodes []sceneSpawnNode
	Has3D bool
	HasUI bool
}

type sceneSpawnNode struct {
	Spec            SceneNodeSpec
	Has3DComponents bool
	HasSprite       bool
	HasAnimation    bool
	HasText         bool
	HasHTML         bool
	HasParticle     bool
	HasTilemap      bool
	HasSpine        bool
	HasModel        bool
	HasCamera       bool
	HasAmbientLight bool
	HasLight        bool
	HasPointLight   bool
	HasSpotLight    bool
	HasAreaLight    bool
	HasPhysics2D    bool
	HasPhysics3D    bool
	Parent          uint32
	TargetCamera    uint32
	LayoutOrder     int
	UI              bool
	Visible         bool
	Transform       scene.Transform
	UITransform     sceneUITransform
	// WidgetTarget stores a one-based plan node index for the authored Widget
	// target. The runtime component receives the actual Entity after spawning.
	WidgetTarget uint32
	TextStyle    sceneTextStyle
	Camera       sceneCamera
	AmbientLight render3d.AmbientLight
	Light        render3d.DirectionalLight
	PointLight   render3d.PointLight
	SpotLight    render3d.SpotLight
	AreaLight    render3d.AreaLight
	// Target arrays store one-based plan node indices, matching Parent and
	// TargetCamera references and avoiding path parsing at playback. Animation
	// clips themselves live in Bundle's immutable cache because spawn plans are
	// serialized by the preprocessor.
	AnimationKey string
	// LocalID is stable across prefab mounts and runtime clones. Binding paths
	// are authored with these hierarchy-local IDs, while Spec.ID may be
	// namespaced for a mounted or cloned instance.
	LocalID                 string
	AnimationFloatTargets   []uint32
	AnimationTextureTargets []uint32
	AnimationBoolTargets    []uint32
}

type sceneTextStyle struct {
	Color           graphics.ColorRGBA
	OutlineColor    graphics.ColorRGBA
	ShadowColor     graphics.ColorRGBA
	ShadowX         float32
	ShadowY         float32
	HasColor        bool
	HasOutlineColor bool
	HasShadowColor  bool
}

// Spawn plans store authored transform values; runtime composition matrices
// are transient scene state and must not enter the binary resource schema.
type sceneUITransform struct {
	Translation scene.Val2
	Scale       mgl32.Vec2
	Rotation    float32
	Skew        mgl32.Vec2
	Pivot       mgl32.Vec2
}

func newSceneUITransform(t scene.UITransform) sceneUITransform {
	return sceneUITransform{Translation: t.Translation, Scale: t.Scale, Rotation: t.Rotation, Skew: t.Skew, Pivot: t.Pivot}
}

func (t sceneUITransform) Component() scene.UITransform {
	return scene.UITransform{Translation: t.Translation, Scale: t.Scale, Rotation: t.Rotation, Skew: t.Skew, Pivot: t.Pivot}
}

type sceneCamera struct {
	Viewport   scene.CameraViewport
	Order      int
	Active     bool
	ClearColor graphics.ColorRGBA
	Projection scene.Projection
}

func compileSceneSpawnPlan(specs []SceneNodeSpec) (sceneSpawnPlan, error) {
	plan := sceneSpawnPlan{Nodes: make([]sceneSpawnNode, 0, countSceneNodes(specs))}
	indices := make(map[string]uint32, cap(plan.Nodes))
	targets := make([]string, 0, cap(plan.Nodes))
	widgetTargets := make([]string, 0, cap(plan.Nodes))
	var appendSpec func(SceneNodeSpec, uint32, int) error
	appendSpec = func(spec SceneNodeSpec, structuralParent uint32, siblingIndex int) error {
		parent := structuralParent
		node, err := compileSceneSpawnNode(spec)
		if err != nil {
			return err
		}
		node.Parent = parent
		node.LayoutOrder = siblingIndex
		index := uint32(len(plan.Nodes))
		plan.Nodes = append(plan.Nodes, node)
		targets = append(targets, strings.TrimSpace(spec.TargetCamera))
		widgetTarget := ""
		if spec.Widget != nil {
			widgetTarget = strings.TrimSpace(spec.Widget.TargetID)
		}
		widgetTargets = append(widgetTargets, widgetTarget)
		indices[node.Spec.ID] = index + 1
		plan.Has3D = plan.Has3D || node.Has3DComponents || node.HasPhysics3D
		plan.HasUI = plan.HasUI || specUsesUI(&spec)
		for childIndex, child := range spec.Children {
			if err := appendSpec(child, index+1, childIndex); err != nil {
				return err
			}
		}
		return nil
	}
	for rootIndex, spec := range specs {
		if err := appendSpec(spec, 0, rootIndex); err != nil {
			return sceneSpawnPlan{}, err
		}
	}
	for index, target := range targets {
		if target == "" {
			continue
		}
		targetIndex, ok := indices[target]
		if !ok {
			return sceneSpawnPlan{}, fmt.Errorf("game object %q targetCamera %q does not exist", plan.Nodes[index].Spec.ID, target)
		}
		if !plan.Nodes[targetIndex-1].HasCamera {
			return sceneSpawnPlan{}, fmt.Errorf("game object %q targetCamera %q is not a camera", plan.Nodes[index].Spec.ID, target)
		}
		plan.Nodes[index].TargetCamera = targetIndex
	}
	for index, target := range widgetTargets {
		if target == "" {
			continue
		}
		targetIndex, ok := indices[target]
		if !ok {
			return sceneSpawnPlan{}, fmt.Errorf("game object %q widget target %q does not exist", plan.Nodes[index].Spec.ID, target)
		}
		if !plan.Nodes[targetIndex-1].UI {
			return sceneSpawnPlan{}, fmt.Errorf("game object %q widget target %q requires UI coordinate space", plan.Nodes[index].Spec.ID, target)
		}
		plan.Nodes[index].WidgetTarget = targetIndex
	}
	if err := compileSceneAnimationBindings(&plan); err != nil {
		return sceneSpawnPlan{}, err
	}
	return plan, nil
}

func compileSceneAnimationBindings(plan *sceneSpawnPlan) error {
	if plan == nil || len(plan.Nodes) == 0 {
		return nil
	}
	hasAnimation := false
	for index := range plan.Nodes {
		component := plan.Nodes[index].Spec.Components.Animation
		if component != nil && component.Clip.HasContent() {
			hasAnimation = true
			break
		}
	}
	if !hasAnimation {
		return nil
	}
	claimed := make(map[animationResolvedBinding]uint32, animationTrackCount(plan))
	children := sceneAnimationChildIndex(plan)
	for ownerIndex := range plan.Nodes {
		node := &plan.Nodes[ownerIndex]
		component := node.Spec.Components.Animation
		if component == nil {
			continue
		}
		if !component.Clip.HasContent() {
			continue
		}
		clip := component.Clip
		// compileSceneSpawnNode has already normalized and validated this clip.
		// This pass only resolves hierarchy-local binding paths.
		seen := make(map[animationResolvedBinding]struct{}, len(clip.FloatTracks)+len(clip.TextureTracks)+len(clip.BoolTracks))
		floatEnd := len(clip.FloatTracks)
		textureEnd := floatEnd + len(clip.TextureTracks)
		targetStorage := make([]uint32, textureEnd+len(clip.BoolTracks))
		var floatTargets, textureTargets, boolTargets []uint32
		if floatEnd > 0 {
			floatTargets = targetStorage[:floatEnd]
		}
		if textureEnd > floatEnd {
			textureTargets = targetStorage[floatEnd:textureEnd]
		}
		if len(clip.BoolTracks) > 0 {
			boolTargets = targetStorage[textureEnd:]
		}
		if err := resolveSceneAnimationTrackTargets(plan, children, ownerIndex, node.Spec.ID, "float", clip.FloatTracks, floatTargets, seen, claimed); err != nil {
			return err
		}
		if err := resolveSceneAnimationTrackTargets(plan, children, ownerIndex, node.Spec.ID, "texture", clip.TextureTracks, textureTargets, seen, claimed); err != nil {
			return err
		}
		if err := resolveSceneAnimationTrackTargets(plan, children, ownerIndex, node.Spec.ID, "bool", clip.BoolTracks, boolTargets, seen, claimed); err != nil {
			return err
		}
		node.AnimationFloatTargets = floatTargets
		node.AnimationTextureTargets = textureTargets
		node.AnimationBoolTargets = boolTargets
	}
	return nil
}

type animationResolvedBinding struct {
	node   uint32
	target scene.AnimationTarget
}

type sceneAnimationChildKey struct {
	parent uint32
	id     string
}

func sceneAnimationChildIndex(plan *sceneSpawnPlan) map[sceneAnimationChildKey]int {
	if plan == nil || len(plan.Nodes) == 0 {
		return nil
	}
	children := make(map[sceneAnimationChildKey]int, len(plan.Nodes))
	for index, node := range plan.Nodes {
		key := sceneAnimationChildKey{parent: node.Parent, id: node.Spec.ID}
		if _, exists := children[key]; exists {
			children[key] = -1
			continue
		}
		children[key] = index
	}
	return children
}

func resolveSceneAnimationTrackTargets[T any](plan *sceneSpawnPlan, children map[sceneAnimationChildKey]int, ownerIndex int, ownerID, kind string, tracks []AnimationTrackSpec[T], targets []uint32, seen map[animationResolvedBinding]struct{}, claimed map[animationResolvedBinding]uint32) error {
	if len(targets) != len(tracks) {
		return fmt.Errorf("animation %s target storage length %d does not match track count %d", kind, len(targets), len(tracks))
	}
	for index, track := range tracks {
		target, err := resolveSceneAnimationPathIndexed(*plan, children, ownerIndex, track.Binding.Path)
		if err != nil {
			return fmt.Errorf("game object %q animation %s track %d: %w", ownerID, kind, index, err)
		}
		if err := validateSceneAnimationTarget(*plan, ownerIndex, target, track.Binding.Target); err != nil {
			return fmt.Errorf("game object %q animation %s track %d: %w", ownerID, kind, index, err)
		}
		key := animationResolvedBinding{node: uint32(target + 1), target: track.Binding.Target}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("game object %q animation contains duplicate target path %q target %q", ownerID, track.Binding.Path, track.Binding.Target)
		}
		seen[key] = struct{}{}
		if err := claimSceneAnimationTarget(plan, claimed, ownerIndex, target, track.Binding.Target); err != nil {
			return fmt.Errorf("game object %q animation %s track %d: %w", ownerID, kind, index, err)
		}
		targets[index] = uint32(target + 1)
	}
	return nil
}

func animationTrackCount(plan *sceneSpawnPlan) int {
	if plan == nil {
		return 0
	}
	count := 0
	for _, node := range plan.Nodes {
		if animation := node.Spec.Components.Animation; animation != nil {
			count += len(animation.Clip.FloatTracks) + len(animation.Clip.TextureTracks) + len(animation.Clip.BoolTracks)
		}
	}
	return count
}

// DefaultAnimationClip creates the initial clip for an animation component
// that has no authored clip yet.  Keeping this construction in the project
// package makes scene loading, prefab instantiation, and the editor agree on
// the same first keyframe.
func DefaultAnimationClip(spec SceneNodeSpec) AnimationClipSpec {
	return AnimationClipSpec{
		Duration:   1,
		SampleRate: 60,
		FloatTracks: []AnimationTrackSpec[float32]{{
			Binding: scene.AnimationBinding{Target: scene.AnimationTargetTransformPositionX},
			Keyframes: []AnimationKeyframeSpec[float32]{{
				Time: 0, Value: spec.X, Tween: scene.AnimationLinear,
			}},
		}},
	}
}

func resolveSceneAnimationPathIndexed(plan sceneSpawnPlan, children map[sceneAnimationChildKey]int, ownerIndex int, path string) (int, error) {
	current := ownerIndex
	if path == "" {
		return current, nil
	}
	for remaining := path; remaining != ""; {
		segment, next, foundSeparator := strings.Cut(remaining, "/")
		if !foundSeparator {
			next = ""
		}
		index, exists := children[sceneAnimationChildKey{parent: uint32(current + 1), id: segment}]
		if !exists {
			return -1, fmt.Errorf("path %q does not name a descendant", path)
		}
		if index < 0 {
			return -1, fmt.Errorf("path %q is ambiguous at %q", path, segment)
		}
		current = index
		remaining = next
	}
	return current, nil
}

func validateSceneAnimationTarget(plan sceneSpawnPlan, ownerIndex, targetIndex int, target scene.AnimationTarget) error {
	if targetIndex < 0 || targetIndex >= len(plan.Nodes) {
		return fmt.Errorf("target node index %d is invalid", targetIndex)
	}
	node := plan.Nodes[targetIndex]
	switch target {
	case scene.AnimationTargetSpriteTexture, scene.AnimationTargetSpriteAlpha:
		// A root inline clip can materialize the host Sprite2D from its
		// animation tracks. Descendant targets still require an authored sprite
		// on that child because the animator must not invent unrelated hierarchy
		// components.
		if !node.HasSprite && targetIndex != ownerIndex {
			return fmt.Errorf("target path resolves to node %q without a sprite component", node.Spec.ID)
		}
	case scene.AnimationTargetTransformRotationX, scene.AnimationTargetTransformRotationY:
		if node.UI {
			return fmt.Errorf("X/Y rotation is not available on UI node %q", node.Spec.ID)
		}
	case scene.AnimationTargetTransformPositionZ, scene.AnimationTargetTransformScaleZ:
		if node.UI {
			return fmt.Errorf("Z transform animation is not available on UI node %q", node.Spec.ID)
		}
	case scene.AnimationTargetTransformPositionX, scene.AnimationTargetTransformPositionY,
		scene.AnimationTargetTransformScaleX, scene.AnimationTargetTransformScaleY,
		scene.AnimationTargetTransformRotationZ, scene.AnimationTargetVisibility:
		// Every compiled scene node owns the corresponding transform and
		// visibility component.
	default:
		return fmt.Errorf("unsupported animation target %q", target)
	}
	return nil
}

func claimSceneAnimationTarget(plan *sceneSpawnPlan, claimed map[animationResolvedBinding]uint32, ownerIndex, targetIndex int, target scene.AnimationTarget) error {
	ownerRef, targetRef := uint32(ownerIndex+1), uint32(targetIndex+1)
	key := animationResolvedBinding{node: targetRef, target: animationOwnershipTarget(target)}
	if previous, exists := claimed[key]; exists && previous != ownerRef {
		return fmt.Errorf("target node %q property %q is already animated by node %q", plan.Nodes[targetIndex].Spec.ID, target, plan.Nodes[previous-1].Spec.ID)
	}
	claimed[key] = ownerRef
	return nil
}

func animationOwnershipTarget(target scene.AnimationTarget) scene.AnimationTarget {
	switch target {
	case scene.AnimationTargetTransformRotationX,
		scene.AnimationTargetTransformRotationY,
		scene.AnimationTargetTransformRotationZ:
		// Transform stores all axes in one quaternion, so separate animators
		// cannot own individual Euler axes without overwriting each other.
		return scene.AnimationTargetTransformRotationZ
	default:
		return target
	}
}

func compileSceneSpawnNode(spec SceneNodeSpec) (sceneSpawnNode, error) {
	spec.Components = spec.Components.Clone()
	spec.Scripts = cloneScriptComponents(spec.Scripts)
	spec.Control = cloneOptional(spec.Control)
	spec.Layout = cloneOptional(spec.Layout)
	spec.Widget = cloneOptional(spec.Widget)
	if spec.Widget != nil {
		// Scene manifests store stable node IDs; runtime Entity handles belong
		// to the world that created them and must never cross scene instantiation.
		spec.Widget.Target = 0
	}
	var err error
	spec.ID = strings.TrimSpace(spec.ID)
	if spec.ScaleX == 0 {
		spec.ScaleX = 1
	}
	if spec.ScaleY == 0 {
		spec.ScaleY = 1
	}
	if spec.ScaleZ == 0 {
		spec.ScaleZ = 1
	}
	spec.Alpha = alphaOrDefault(spec.Alpha)
	if err := validateSceneComponents("scene", spec.ID, spec.Components); err != nil {
		return sceneSpawnNode{}, err
	}
	if spec.Components.Animation != nil {
		if spec.Components.Animation.Speed <= 0 {
			spec.Components.Animation.Speed = 1
		}
		if !spec.Components.Animation.Clip.HasContent() {
			spec.Components.Animation.Clip = DefaultAnimationClip(spec)
		}
		if spec.Components.Animation.Clip.SampleRate <= 0 {
			spec.Components.Animation.Clip.SampleRate = 60
		}
		if err := spec.Components.Animation.Clip.Validate(); err != nil {
			return sceneSpawnNode{}, fmt.Errorf("game object %q animation: %w", spec.ID, err)
		}
	}
	if spec.Components.Text != nil {
		spec.Components.Text.Font = strings.TrimSpace(spec.Components.Text.Font)
	}
	if spec.Components.Spine != nil {
		spec.Components.Spine.Resource = strings.TrimSpace(spec.Components.Spine.Resource)
		spec.Components.Spine.Animation = strings.TrimSpace(spec.Components.Spine.Animation)
		spec.Components.Spine.Skin = strings.TrimSpace(spec.Components.Spine.Skin)
		if spec.Components.Spine.Speed <= 0 {
			spec.Components.Spine.Speed = 1
		}
	}
	spec.RenderLayers = slices.Compact(slices.Sorted(slices.Values(spec.RenderLayers)))
	node := sceneSpawnNode{
		Spec:            spec,
		LocalID:         spec.ID,
		Has3DComponents: spec.Components.Has3D(),
		HasSprite:       spec.Components.Sprite != nil,
		HasAnimation:    spec.Components.Animation != nil,
		HasText:         spec.Components.Text != nil,
		HasHTML:         spec.Components.HTML != nil,
		HasParticle:     spec.Components.Particle != nil,
		HasTilemap:      spec.Components.Tilemap != nil,
		HasSpine:        spec.Components.Spine != nil,
		HasModel:        spec.Components.Model != nil,
		HasCamera:       spec.Components.Camera != nil,
		HasAmbientLight: spec.Components.AmbientLight != nil,
		HasLight:        spec.Components.DirectionalLight != nil,
		HasPointLight:   spec.Components.PointLight != nil,
		HasSpotLight:    spec.Components.SpotLight != nil,
		HasAreaLight:    spec.Components.AreaLight != nil,
		HasPhysics2D:    spec.Components.HasPhysics2D(),
		HasPhysics3D:    spec.Components.HasPhysics3D(),
		UI:              specUsesUI(&spec),
		Visible:         spec.Visible == nil || *spec.Visible,
	}
	if node.UI {
		node.UITransform = newSceneUITransform(uiTransformForSpec(spec))
	} else if node.Has3DComponents || node.HasPhysics3D {
		node.Transform = transform3DForSpec(spec)
	} else {
		node.Transform = transform2DForSpec(spec)
	}
	if node.HasText {
		node.TextStyle, err = compileSceneTextStyle(spec)
		if err != nil {
			return sceneSpawnNode{}, err
		}
	}
	if node.HasCamera {
		node.Camera, err = compileSceneCamera(spec)
	}
	if err == nil && node.HasAmbientLight {
		node.AmbientLight, err = compileSceneAmbientLight(spec)
	}
	if err == nil && node.HasLight {
		node.Light, err = compileSceneDirectionalLight(spec)
	}
	if err == nil && node.HasPointLight {
		node.PointLight, err = compileScenePointLight(spec)
	}
	if err == nil && node.HasSpotLight {
		node.SpotLight, err = compileSceneSpotLight(spec)
	}
	if err == nil && node.HasAreaLight {
		node.AreaLight, err = compileSceneAreaLight(spec)
	}
	if err != nil {
		return sceneSpawnNode{}, err
	}
	node.Spec.Children = nil
	node.Spec.TargetCamera = ""
	node.Spec.Visible = nil
	// Camera and light values are kept in Spec as component data.  This lets a
	// composed node retain all of its components when it is reapplied or cloned.
	if node.Spec.Components.Text != nil {
		node.Spec.Components.Text.Color = ""
		node.Spec.Components.Text.OutlineColor = ""
		node.Spec.Components.Text.Shadow = nil
	}
	node.Spec.CoordinateSpace = ""
	node.Spec.X, node.Spec.Y, node.Spec.Z = 0, 0, 0
	node.Spec.ScaleX, node.Spec.ScaleY, node.Spec.ScaleZ = 0, 0, 0
	node.Spec.Rotation, node.Spec.RotationX, node.Spec.RotationY, node.Spec.RotationZ = 0, 0, 0, 0
	return node, nil
}

func cloneScriptComponents(value []ScriptComponentSpec) []ScriptComponentSpec {
	if len(value) == 0 {
		return nil
	}
	clone := make([]ScriptComponentSpec, len(value))
	for index, script := range value {
		clone[index] = ScriptComponentSpec{Name: script.Name}
		if len(script.Values) == 0 {
			continue
		}
		clone[index].Values = make(map[string]json.RawMessage, len(script.Values))
		for name, raw := range script.Values {
			clone[index].Values[name] = slices.Clone(raw)
		}
	}
	return clone
}

func compileSceneTextStyle(spec SceneNodeSpec) (sceneTextStyle, error) {
	text := spec.Components.Text
	style := sceneTextStyle{}
	if text.Color != "" {
		color, err := parseColorValue(text.Color, 1)
		if err != nil {
			return sceneTextStyle{}, fmt.Errorf("game object %q color: %w", spec.ID, err)
		}
		style.Color, style.HasColor = color, true
	}
	if text.OutlineWidth < 0 || math.IsNaN(float64(text.OutlineWidth)) || math.IsInf(float64(text.OutlineWidth), 0) {
		return sceneTextStyle{}, fmt.Errorf("game object %q outline width must be finite and non-negative", spec.ID)
	}
	if text.OutlineColor != "" {
		color, err := parseColorValue(text.OutlineColor, 1)
		if err != nil {
			return sceneTextStyle{}, fmt.Errorf("game object %q outline color: %w", spec.ID, err)
		}
		style.OutlineColor, style.HasOutlineColor = color, true
	}
	if text.Shadow != nil {
		color, err := parseColorValue(text.Shadow.Color, 1)
		if err != nil {
			return sceneTextStyle{}, fmt.Errorf("game object %q shadow color: %w", spec.ID, err)
		}
		style.ShadowColor, style.ShadowX, style.ShadowY, style.HasShadowColor = color, text.Shadow.OffsetX, text.Shadow.OffsetY, true
	}
	return style, nil
}

func compileSceneCamera(spec SceneNodeSpec) (sceneCamera, error) {
	camera := spec.Components.Camera
	clearColor, err := parseColorValue(camera.ClearColor, 1)
	if err != nil {
		return sceneCamera{}, fmt.Errorf("game object %q camera clearColor: %w", spec.ID, err)
	}
	return sceneCamera{
		Viewport: scene.CameraViewport{
			X: camera.Viewport.X, Y: camera.Viewport.Y,
			Width: camera.Viewport.Width, Height: camera.Viewport.Height,
		},
		Order: camera.Order, Active: spec.Visible == nil || *spec.Visible, ClearColor: clearColor,
		Projection: scene.Projection{
			Kind:        scene.ProjectionPerspective,
			Perspective: scene.PerspectiveProjection{Fov: camera.Fov, Near: camera.Near},
		},
	}, nil
}

func compileSceneDirectionalLight(spec SceneNodeSpec) (render3d.DirectionalLight, error) {
	light := spec.Components.DirectionalLight
	color, err := parseColorValue(light.Color, 1)
	if err != nil {
		return render3d.DirectionalLight{}, fmt.Errorf("game object %q directionalLight color: %w", spec.ID, err)
	}
	value := render3d.DirectionalLight{Color: color.Linear(), Illuminance: light.Illuminance}
	if s := light.Shadows; s != nil {
		value.Shadows = s.Settings()
	}
	return value, value.Validate()
}

func compileSceneAmbientLight(spec SceneNodeSpec) (render3d.AmbientLight, error) {
	light := spec.Components.AmbientLight
	color, err := parseColorValue(light.Color, 1)
	if err != nil {
		return render3d.AmbientLight{}, fmt.Errorf("game object %q ambientLight color: %w", spec.ID, err)
	}
	return render3d.AmbientLight{Color: color.Linear(), Brightness: light.Brightness}, nil
}

func countSceneNodes(specs []SceneNodeSpec) int {
	count := len(specs)
	for _, spec := range specs {
		count += countSceneNodes(spec.Children)
	}
	return count
}
