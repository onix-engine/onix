package scene

import "github.com/diiyw/onix/ecs"

var inputPickInputs2D = [...]func(*ecs.World) uint64{
	(*ecs.World).ComponentVersion[Camera], (*ecs.World).ComponentVersion[Projection], (*ecs.World).ComponentVersion[Camera2D],
	(*ecs.World).ComponentVersion[GlobalTransform], (*ecs.World).ComponentVersion[UIGlobalTransform],
	(*ecs.World).ComponentVersion[InputTarget2D], (*ecs.World).ComponentVersion[InputBlocker2D],
	(*ecs.World).ComponentVersion[Visibility], (*ecs.World).ComponentVersion[Activity], (*ecs.World).ComponentVersion[Parent],
	(*ecs.World).ComponentVersion[RenderLayers], (*ecs.World).ComponentVersion[UITargetCamera],
	(*ecs.World).ComponentVersion[PainterGroup2D], (*ecs.World).ComponentVersion[UIStackIndex], (*ecs.World).ComponentVersion[RenderOrder2D], (*ecs.World).ComponentVersion[SiblingOrder],
	(*ecs.World).ComponentVersion[UITransform], (*ecs.World).ComponentVersion[Size2D], (*ecs.World).ComponentVersion[Anchor],
	(*ecs.World).ComponentVersion[ClipRect2D], (*ecs.World).ComponentVersion[RoundedClip2D], (*ecs.World).ComponentVersion[ClipExempt2D],
	(*ecs.World).ComponentVersion[Sprite2D], (*ecs.World).ComponentVersion[SpriteLayers2D], (*ecs.World).ComponentVersion[Text2D],
}

// Each execution scope owns its pick snapshot. Pointer motion only reads it;
// component revisions also detect writes/removals within the same ECS tick.
type inputPickCache2D struct {
	cameras          []inputCamera2D
	targets          []inputPickTarget2D
	versions         [len(inputPickInputs2D)]uint64
	lookup           inputCaptureLookup2D
	defaultUICamera  ecs.Entity
	scopeVersion     uint64
	registryRevision uint64
	pipelineRevision uint64
	renderState      *RenderWorld
	initialized      bool
	generation       uint64
}

func (cache *inputPickCache2D) refresh(world *ecs.World, entity, camera ecs.Entity) bool {
	var versions [len(inputPickInputs2D)]uint64
	for i, revision := range inputPickInputs2D {
		versions[i] = revision(world)
	}
	_, scopeVersion := world.QueryScopeKey()
	state, _ := world.GetResource[*RenderWorld]()
	registryRevision, pipelineRevision := uint64(0), uint64(0)
	if state != nil {
		registryRevision = state.registry2D
		for _, source := range state.cameraSettings {
			if source.revision != nil {
				pipelineRevision += source.revision(world)
			}
		}
	}
	changed := !cache.initialized || cache.versions != versions || cache.scopeVersion != scopeVersion || cache.renderState != state || cache.registryRevision != registryRevision || cache.pipelineRevision != pipelineRevision
	lookup := inputCaptureLookup2D{entity: entity, cameraEntity: camera}
	if changed {
		oldCameras, oldTargets := cache.cameras, cache.targets
		cache.cameras, cache.defaultUICamera = collectInputCamerasInto2D(world, cache.cameras, &lookup)
		cache.targets = collectInputTargetsInto2D(world, cache.targets, &lookup)
		if len(oldCameras) > len(cache.cameras) {
			clear(oldCameras[len(cache.cameras):])
		}
		if len(oldTargets) > len(cache.targets) {
			clear(oldTargets[len(cache.targets):])
		}
		cache.versions, cache.scopeVersion = versions, scopeVersion
		cache.renderState, cache.pipelineRevision = state, pipelineRevision
		cache.registryRevision = registryRevision
		cache.initialized = true
		cache.generation++
	} else if cache.lookup.entity == entity && cache.lookup.cameraEntity == camera {
		return false
	} else {
		lookup.defaultUICamera = cache.defaultUICamera
		for _, candidate := range cache.cameras {
			if candidate.entity == camera {
				lookup.camera, lookup.cameraFound = candidate, true
				break
			}
		}
		if lookup.cameraFound && entity != 0 {
			for i := range cache.targets {
				lookup.consider(&cache.targets[i])
			}
		}
	}
	cache.lookup = lookup
	return changed
}
