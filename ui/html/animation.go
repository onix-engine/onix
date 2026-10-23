package html

import (
	"math"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/internal/css"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

type htmlCSSAnimation struct {
	Track     style.AnimationTrack
	Elapsed   float64
	nextMatch int
}

type htmlCSSTransition struct {
	Config          style.TransformTransition
	Elapsed, Factor float64
}

type htmlNodeAnimation struct {
	Computed    *style.ComputedStyle
	Stylesheet  *css.Stylesheet
	Viewport    style.Viewport
	Current     style.AnimationValue
	Foreground  style.AnimationColor
	PaintSource *style.ComputedStyle
	PaintStyle  *style.ComputedStyle
	*htmlNodeMotion
}

// Static elements retain their last value for transitions introduced later,
// without owning transition workspaces or keyframe tracks.
type htmlNodeMotion struct {
	Tracks                     []htmlCSSAnimation
	TrackScratch               []htmlCSSAnimation
	From, Target, ReverseStart style.AnimationValue
	Transitions                [style.AnimatedPropertyCount]htmlCSSTransition
	Active                     style.AnimatedProperties
}

func prepareHTMLNodeAnimation(state *htmlRenderState, node *dom.Node, computed *style.ComputedStyle) *htmlNodeAnimation {
	if state.Animations == nil {
		state.Animations = make(map[*dom.Node]*htmlNodeAnimation)
	}
	animation := state.Animations[node]
	if animation != nil && animation.Computed == computed && animation.Stylesheet == state.Stylesheet && animation.Viewport == state.Viewport {
		return animation
	}
	value := style.BaseAnimationValue(computed)
	target := value
	target.Colors[style.AnimationForeground] = style.ColorForAnimation(computed.Color)
	if animation == nil {
		animation = &htmlNodeAnimation{Current: target, Foreground: target.Colors[style.AnimationForeground]}
		state.Animations[node] = animation
	}
	needsMotion := len(computed.Animations) != 0
	for i := range style.AnimatedPropertyList {
		needsMotion = needsMotion || computed.TransitionFor(i).Enabled
	}
	if !needsMotion {
		animation.Current = value
		animation.htmlNodeMotion = nil
		animation.Computed, animation.Stylesheet, animation.Viewport = computed, state.Stylesheet, state.Viewport
		return animation
	}
	if animation.htmlNodeMotion == nil {
		previous := target
		if animation.Computed != nil {
			previous = style.BaseAnimationValue(animation.Computed)
			previous.Colors[style.AnimationForeground] = style.ColorForAnimation(animation.Computed.Color)
		}
		animation.htmlNodeMotion = &htmlNodeMotion{Target: previous}
	}
	if animation.Computed != nil {
		for i, property := range style.AnimatedPropertyList {
			config := computed.TransitionFor(i)
			if !config.Enabled {
				animation.Active &^= property
				continue
			}
			if style.SameAnimatedProperty(animation.Target, target, property) {
				continue
			}
			from := animation.Current
			if property == style.AnimatedColor && from.Colors[style.AnimationForeground].Current != 0 {
				from.Colors[style.AnimationForeground] = animation.Foreground
			}
			transition := &animation.Transitions[i]
			factor := float64(1)
			if animation.Active&property != 0 && style.SameAnimatedProperty(target, animation.ReverseStart, property) {
				progress := max(transition.Elapsed-transition.Config.Delay, 0) / transition.Config.Duration
				if transition.Config.Duration == 0 {
					progress = 1
				}
				eased := transition.Config.Timing.Sample(progress, false)
				factor = min(math.Abs(eased*transition.Factor+1-transition.Factor), 1)
				style.CopyAnimatedProperties(&animation.ReverseStart, animation.Target, property)
			} else {
				style.CopyAnimatedProperties(&animation.ReverseStart, from, property)
			}
			style.CopyAnimatedProperties(&animation.From, from, property)
			config.Duration *= factor
			if config.Delay < 0 {
				config.Delay *= factor
			}
			*transition = htmlCSSTransition{Config: config, Factor: factor}
			if config.Duration+config.Delay > 0 && !style.SameAnimatedProperty(animation.From, target, property) {
				animation.Active |= property
			} else {
				animation.Active &^= property
			}
		}
	}
	animation.Target = target
	tracks := style.CompileAnimationTracks(node, computed, state.Stylesheet, &style.QueryEnvironment{Viewport: state.Viewport, Styles: state.Styles, ContainerMetrics: state.StyleCache.ContainerMetrics})
	syncHTMLAnimationTracks(state, animation, tracks)
	animation.Computed, animation.Stylesheet, animation.Viewport = computed, state.Stylesheet, state.Viewport
	sampleHTMLNodeAnimation(animation, 0)
	return animation
}

func syncHTMLAnimationTracks(state *htmlRenderState, animation *htmlNodeAnimation, tracks []style.AnimationTrack) {
	if state.AnimationTrackHeads == nil {
		state.AnimationTrackHeads = make(map[string]int)
	}
	heads := state.AnimationTrackHeads
	defer clear(heads)
	for i := range animation.Tracks {
		old := &animation.Tracks[i]
		name := old.Track.Definition.Name
		old.nextMatch = heads[name]
		heads[name] = i + 1
	}
	next := slices.Grow(animation.TrackScratch[:0], len(tracks))[:len(tracks)]
	// Match names from the end, as CSS Animations specifies, retaining local time.
	for i := len(tracks) - 1; i >= 0; i-- {
		next[i] = htmlCSSAnimation{Track: tracks[i]}
		name := tracks[i].Definition.Name
		if head := heads[name]; head != 0 {
			old := &animation.Tracks[head-1]
			next[i].Elapsed = old.Elapsed
			heads[name] = old.nextMatch
		}
	}
	clear(animation.Tracks)
	animation.Tracks, animation.TrackScratch = next, animation.Tracks[:0]
}

func syncHTMLAnimations(state *htmlRenderState) {
	defer state.NodeFlags.clear()
	hiddenByAncestor := func(node *dom.Node, hidden bool) bool {
		value := state.Styles[node]
		return hidden || value != nil && value.Display == style.Keyword("none")
	}
	for node, computed := range state.Styles {
		if node.Type != dom.ElementNode {
			continue
		}
		if state.NodeFlags.resolve(node, htmlNodeParent, hiddenByAncestor) {
			delete(state.Animations, node)
			continue
		}
		prepareHTMLNodeAnimation(state, node, computed)
	}
	for node := range state.Animations {
		if state.Styles[node] == nil {
			delete(state.Animations, node)
		}
	}
}

func sampleHTMLNodeAnimation(animation *htmlNodeAnimation, delta float64) {
	value := animation.Target
	if animation.Computed.ColorInherited {
		value.Colors[style.AnimationForeground] = style.AnimationColor{Current: 1}
	}
	for i, property := range style.AnimatedPropertyList {
		if animation.Active&property == 0 {
			continue
		}
		transition := &animation.Transitions[i]
		transition.Elapsed += delta
		elapsed := transition.Elapsed - transition.Config.Delay
		if elapsed < 0 {
			style.CopyAnimatedProperties(&value, animation.From, property)
		} else if elapsed >= transition.Config.Duration {
			animation.Active &^= property
		} else {
			factor := transition.Config.Timing.Sample(elapsed/transition.Config.Duration, false)
			style.InterpolateAnimatedProperty(&value, animation.From, animation.Target, property, factor)
		}
	}
	for i := range animation.Tracks {
		active := &animation.Tracks[i]
		if !active.Track.Definition.Paused {
			active.Elapsed += delta
		}
		if progress, applies, before := active.Track.Definition.Progress(active.Elapsed); applies {
			sampled := active.Track.Sample(progress, value, before)
			style.CopyAnimatedProperties(&value, sampled, active.Track.Properties)
		}
	}
	animation.Current = value
}

func advanceHTMLAnimations(world *ecs.World, state *htmlRenderState, delta float64) {
	if delta <= 0 {
		return
	}
	changed := false
	for _, animation := range state.Animations {
		if animation.htmlNodeMotion == nil || animation.Active == 0 && len(animation.Tracks) == 0 {
			continue
		}
		previous := animation.Current
		sampleHTMLNodeAnimation(animation, delta)
		changed = changed || previous != animation.Current
	}
	if !changed {
		return
	}
	for node, rendered := range state.RenderNodes {
		for _, entity := range rendered.Entities {
			applyHTMLAnimatedVisual(world, state, entity, node)
		}
	}
}

func syncHTMLNodeTransform(world *ecs.World, state *htmlRenderState, entity ecs.Entity, node *dom.Node, computed *style.ComputedStyle) {
	if state == nil || computed == nil || node == nil {
		return
	}
	for current := node; current != nil; current = current.Parent {
		if value := state.Styles[current]; value != nil && current.Type == dom.ElementNode {
			prepareHTMLNodeAnimation(state, current, value)
		}
	}
	applyHTMLAnimatedVisual(world, state, entity, node)
}

func applyHTMLAnimatedVisual(world *ecs.World, state *htmlRenderState, entity ecs.Entity, node *dom.Node) {
	child := ecs.GetRef[htmlChild](world, entity)
	ok := child != nil
	if !ok {
		return
	}
	transform, ok := ecs.Get[scene.UITransform](world, entity)
	if !ok {
		return
	}
	// A generated control/clip container already carries the DOM transforms
	// above its node; only compose the portion below that ECS parent.
	var stop *dom.Node
	originOffset := child.scrollOrigin
	if parent, ok := ecs.Get[scene.Parent](world, entity); ok {
		if ancestor, ok := ecs.Get[htmlChild](world, parent.Entity); ok && ancestor.Owner == child.Owner {
			stop = ancestor.Node
			// Reparented visuals use coordinates relative to their generated
			// container, while CSS transform origins remain in page coordinates.
			if clip, ok := state.OverflowEntities[parent.Entity]; ok {
				originOffset = mgl32.Vec2{float32(clip.rect.X), float32(clip.rect.Y)}
			} else if box := state.BoxesByNode[stop]; box != nil {
				rect := box.BorderBox()
				originOffset = mgl32.Vec2{float32(rect.X), float32(rect.Y)}
			}
		}
	}
	translation := transform.Translation.Resolve(scene.UIResolveContext{}).Sub(child.transformOffset)
	matrix := mgl32.Ident4()
	ownNode := node
	if ownNode.Type == dom.TextNode {
		ownNode = ownNode.Parent
	}
	ownOnly := true
	opacity := float64(1)
	foreground := style.AnimationColor{Current: 1}
	stopped := false
	for current := node; current != nil; current = current.Parent {
		computed := state.Styles[current]
		if computed == nil || current.Type != dom.ElementNode {
			continue
		}
		var value style.AnimationValue
		if animation := state.Animations[current]; animation != nil {
			value = animation.Current
		} else {
			value = style.BaseAnimationValue(computed)
		}
		if foreground.Current != 0 {
			color := value.Colors[style.AnimationForeground]
			weight := foreground.Current
			foreground.R += weight * color.R
			foreground.G += weight * color.G
			foreground.B += weight * color.B
			foreground.A += weight * color.A
			foreground.Current *= color.Current
		}
		// Opacity is composited into flattened sprites. Their root opacity is
		// deliberately left out of the raster when an animation controls it.
		if current == node && ecs.Has[htmlOpacitySurface](world, entity) {
			if computed.AnimatedOpacity {
				opacity *= value.Opacity
			}
		} else {
			opacity *= value.Opacity
		}
		if current == stop {
			stopped = true
		}
		if stopped {
			continue
		}
		box := state.BoxesByNode[current]
		if box == nil {
			continue
		}
		rect := box.BorderBox()
		tx, ty := style.ResolveAnimatedTranslation(value, rect.Width, rect.Height)
		if value.Rotation == 0 && value.SkewX == 0 && value.ScaleX == 1 && value.ScaleY == 1 && tx == 0 && ty == 0 {
			continue
		}
		if current != ownNode {
			ownOnly = false
		}
		origin := mgl32.Vec2{
			float32(rect.X+style.ResolveLength(computed.TransformOriginX, rect.Width)) - originOffset.X() + child.offset.X() - translation.X(),
			float32(rect.Y+style.ResolveLength(computed.TransformOriginY, rect.Height)) - originOffset.Y() + child.offset.Y() - translation.Y(),
		}
		local := mgl32.Translate3D(origin.X()+float32(tx), origin.Y()+float32(ty), 0).
			Mul4(mgl32.HomogRotate3DZ(float32(value.Rotation * math.Pi / 180))).
			Mul4(mgl32.Mat4{1, 0, 0, 0, float32(math.Tan(value.SkewX * math.Pi / 180)), 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}).
			Mul4(mgl32.Scale3D(float32(value.ScaleX), float32(value.ScaleY), 1)).
			Mul4(mgl32.Translate3D(-origin.X(), -origin.Y(), 0))
		matrix = local.Mul4(matrix)
	}
	// Keep single-node transforms in their authored representation, preserving
	// complete turns for spinning animations and the inspectable CSS pivot.
	rotation, scale, pivot, skew := float32(0), mgl32.Vec2{1, 1}, mgl32.Vec2{}, mgl32.Vec2{}
	offset := mgl32.Vec2{matrix[12], matrix[13]}
	linear := mgl32.Mat4{}
	if node != stop {
		current := node
		if current.Type == dom.TextNode {
			current = current.Parent
		}
		value := style.AnimationValue{ScaleX: 1, ScaleY: 1}
		if animation := state.Animations[current]; animation != nil {
			value = animation.Current
		}
		if box := state.BoxesByNode[current]; ownOnly && current != stop && box != nil {
			rect := box.BorderBox()
			computed := state.Styles[current]
			rotation, scale = float32(value.Rotation*math.Pi/180), mgl32.Vec2{float32(value.ScaleX), float32(value.ScaleY)}
			skew[0] = float32(value.SkewX * math.Pi / 180)
			animation := state.Animations[current]
			if rotation != 0 || skew[0] != 0 || scale != (mgl32.Vec2{1, 1}) || animation != nil && animation.htmlNodeMotion != nil && animation.Active&style.AnimatedTransform != 0 {
				pivot = mgl32.Vec2{float32(rect.X+style.ResolveLength(computed.TransformOriginX, rect.Width)) - originOffset.X() + child.offset.X() - translation.X(), float32(rect.Y+style.ResolveLength(computed.TransformOriginY, rect.Height)) - originOffset.Y() + child.offset.Y() - translation.Y()}
			}
			tx, ty := style.ResolveAnimatedTranslation(value, rect.Width, rect.Height)
			offset = mgl32.Vec2{float32(tx), float32(ty)}
		} else {
			linear = matrix
			linear[12], linear[13] = 0, 0
		}
	}
	if transform.Rotation != rotation || transform.Scale != scale || transform.Pivot != pivot || transform.Skew != skew || transform.LinearMatrix != linear || child.transformOffset != offset {
		updated := ecs.GetMut[scene.UITransform](world, entity)
		updated.Rotation, updated.Scale, updated.Pivot, updated.Skew, updated.LinearMatrix = rotation, scale, pivot, skew, linear
		updated.Translation.X.Value += offset.X() - child.transformOffset.X()
		updated.Translation.Y.Value += offset.Y() - child.transformOffset.Y()
		mutable := ecs.GetMut[htmlChild](world, entity)
		mutable.transformOffset = offset
	}
	if sprite := ecs.GetRef[scene.Sprite2D](world, entity); (sprite != nil) && sprite.Alpha != float32(opacity) {
		updated := ecs.GetMut[scene.Sprite2D](world, entity)
		updated.Alpha = float32(opacity)
	}
	if text := ecs.GetRef[scene.Text2D](world, entity); text != nil {
		lcd := false
		if computed := state.Styles[ownNode]; computed != nil {
			lcd = computed.TextOpaqueBackdrop && !computed.TextInOpacityGroup && opacity == 1
		}
		if text.Alpha != float32(opacity) || text.LCD != lcd {
			updated := ecs.GetMut[scene.Text2D](world, entity)
			updated.Alpha, updated.LCD = float32(opacity), lcd
		}
	}
	foreground.A += foreground.Current
	foreground.Current = 0
	if animation := state.Animations[ownNode]; animation != nil {
		animation.Foreground = foreground
	}
	applyHTMLAnimatedPaint(world, state, entity, ownNode, child, foreground)
}

func htmlAnimationActiveDuration(definition style.CSSAnimation) float64 {
	if definition.Duration == 0 {
		return 0
	}
	return definition.Duration * definition.Iterations
}

func syncHTMLAnimationChanges(state *htmlRenderState) {
	for _, change := range state.StyleChanges {
		if change.Tree {
			syncHTMLAnimations(state)
			return
		}
		if computed := state.Styles[change.Node]; computed != nil && change.Node.Type == dom.ElementNode {
			prepareHTMLNodeAnimation(state, change.Node, computed)
		}
	}
}
