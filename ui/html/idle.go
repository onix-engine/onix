package html

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

// NeedsFrame includes changes made after HTML synchronization. An app must
// flush them before suspending, even if it already consumed its render request.
func NeedsFrame(world *ecs.World) bool {
	queue := world.FrameValue[htmlComponentDirtyTag, htmlComponentPending]()
	for _, entity := range queue.Entities {
		instance := world.GetRef[htmlComponentInstance](entity)
		if instance != nil && instance.Dirty && instance.Definition != nil && world.InQueryScope(entity) && scene.IsActive(world, entity) {
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
	return len(state.ActiveAnimations) != 0
}
