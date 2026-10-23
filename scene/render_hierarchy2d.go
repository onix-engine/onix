package scene

import "github.com/diiyw/onix/ecs"

type renderHierarchy2DInfo struct {
	scene    SceneLayer
	layers   RenderLayers
	target   ecs.Entity
	stack    int
	hasStack bool
}

type renderEntity2DInfo struct {
	scene     SceneLayer
	layers    RenderLayers
	target    ecs.Entity
	sortIndex int
}

type renderHierarchy2DCacheScratch struct{}

type renderHierarchy2DCache struct {
	values    map[ecs.Entity]renderHierarchy2DInfo
	visiting  map[ecs.Entity]bool
	versions  [6]uint64
	structure uint64
}

func frameRenderHierarchy2DCache(world *ecs.World) renderHierarchy2DCache {
	return *frameRenderHierarchy2DCachePtr(world)
}

// frameRenderHierarchy2DCachePtr retains inherited state until its inputs change.
func frameRenderHierarchy2DCachePtr(world *ecs.World) *renderHierarchy2DCache {
	cache := ecs.ScopeValue[renderHierarchy2DCacheScratch, renderHierarchy2DCache](world)
	versions := [6]uint64{
		ecs.ComponentVersion[Parent](world), ecs.ComponentVersion[SceneLayer](world),
		ecs.ComponentVersion[RenderLayers](world), ecs.ComponentVersion[UITargetCamera](world),
		ecs.ComponentVersion[UIStackIndex](world), ecs.ComponentVersion[RenderOrder2D](world),
	}
	if cache.values == nil {
		cache.values = make(map[ecs.Entity]renderHierarchy2DInfo)
		cache.visiting = make(map[ecs.Entity]bool)
	}
	if cache.versions != versions || cache.structure != world.StructureVersion() {
		clear(cache.values)
		clear(cache.visiting)
		cache.versions, cache.structure = versions, world.StructureVersion()
	}
	return cache
}

func renderEntityInfo2D(world *ecs.World, entity ecs.Entity, ui bool, fallback int, cache *renderHierarchy2DCache) renderEntity2DInfo {
	resolved := cache.resolve(world, entity)
	if !ui {
		layers, _ := ecs.Get[RenderLayers](world, entity)
		if order, ok := ecs.Get[RenderOrder2D](world, entity); ok {
			fallback = int(order)
		}
		return renderEntity2DInfo{scene: resolved.scene, layers: layers, sortIndex: fallback}
	}
	if resolved.hasStack {
		fallback = resolved.stack
	}
	return renderEntity2DInfo{scene: resolved.scene, layers: resolved.layers, target: resolved.target, sortIndex: fallback}
}

func (c *renderHierarchy2DCache) resolve(world *ecs.World, entity ecs.Entity) renderHierarchy2DInfo {
	if info, ok := c.values[entity]; ok {
		return info
	}
	if entity == 0 || c.visiting[entity] {
		return renderHierarchy2DInfo{layers: RenderLayers{}}
	}
	c.visiting[entity] = true
	info := renderHierarchy2DInfo{layers: RenderLayers{}}
	if parent, ok := ecs.Get[Parent](world, entity); ok {
		info = c.resolve(world, parent.Entity)
	}
	if layers, ok := ecs.Get[RenderLayers](world, entity); ok {
		info.layers = layers
	}
	if layer, ok := ecs.Get[SceneLayer](world, entity); ok {
		info.scene = layer
	}
	if target, ok := ecs.Get[UITargetCamera](world, entity); ok {
		info.target = target.Entity
	}
	if stack, ok := ecs.Get[UIStackIndex](world, entity); ok {
		info.stack, info.hasStack = int(stack), true
	}
	delete(c.visiting, entity)
	c.values[entity] = info
	return info
}
