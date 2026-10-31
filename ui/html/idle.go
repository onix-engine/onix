package html

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

// NeedsFrame includes changes made after HTML synchronization. An app must
// flush them before suspending, even if it already consumed its render request.
func NeedsFrame(world *ecs.World) bool {
	for entity, access := range world.Query[htmlComponentInstance]().All {
		instance := access.Ref()
		if instance.Dirty && instance.Definition != nil && scene.IsActive(world, entity) {
			return true
		}
	}
	imageProviderChanged := htmlImageProviderChanged(world)
	if owners, ok := world.GetResource[*htmlSurfaceOwners](); ok {
		for entity := range owners.Children {
			surface := world.GetRef[Surface](entity)
			state := world.GetRef[htmlRenderState](entity)
			if surface == nil || state == nil {
				return true
			}
			if scene.IsActive(world, entity) && (!state.VisibilityKnown || state.Visible != htmlVisible(world, entity) ||
				!state.Rendered || state.pending() || state.Key != htmlSurfaceRenderKey(world, *surface, state) ||
				state.ImageProviderChanged != imageProviderChanged || htmlAnimating(state) || htmlPaintCullPending(world, state)) {
				return true
			}
		}
	}
	// Rendered owners above also include removed surfaces awaiting cleanup.
	// Only new surfaces need a separate query; existing ones are visited once.
	for entity := range world.Query[Surface](ecs.Without[htmlRenderState]()).All {
		if scene.IsActive(world, entity) {
			return true
		}
	}
	return false
}

// Animating reports whether an active CSS animation needs another frame.
func Animating(world *ecs.World) bool {
	for entity, access := range world.Query[htmlRenderState]().All {
		if scene.IsActive(world, entity) && htmlAnimating(access.Ref()) {
			return true
		}
	}
	return false
}

func htmlAnimating(state *htmlRenderState) bool {
	for _, node := range state.Animations {
		if node.Active != 0 {
			return true
		}
		for _, animation := range node.Tracks {
			definition := animation.Track.Definition
			if !definition.Paused && animation.Track.Properties != 0 && animation.Elapsed < definition.Delay+htmlAnimationActiveDuration(definition) {
				return true
			}
		}
	}
	return false
}
