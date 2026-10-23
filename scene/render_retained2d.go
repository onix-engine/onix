package scene

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

// Content snapshots belong to their entity, so extracting one changed entity
// cannot overwrite a snapshot still used by an unchanged entity.
type retainedEntity2D struct {
	snapshots  renderSnapshots2D
	positions  []int
	paints     []paintItem2D
	count      int
	queued     bool
	sourceText *Text2D
	text       *Text2D
	matrix     mgl32.Mat4
	clip       clipRect2D
	ui         bool
	properties bool
}

func (node *retainedEntity2D) reset() {
	node.snapshots.reset()
	node.count = 0
	node.paints = node.paints[:0]
	node.sourceText, node.text = nil, nil
}

func (node *retainedEntity2D) snapshot(item renderItem2D) renderItem2D {
	if item.sprite != nil {
		item.sprite = node.snapshots.addSprite(*item.sprite)
	}
	if item.text != nil {
		if node.sourceText != item.text {
			node.sourceText = item.text
			node.text = node.snapshots.addText(*item.text)
		}
		item.text = node.text
	}
	node.paints = append(node.paints, preparePaintItem2D(&item))
	item.paint = &node.paints[len(node.paints)-1]
	return item
}

type renderInput2D struct {
	revision func(*ecs.World) (uint64, uint64)
	changed  func(*ecs.World, *ecs.SystemContext, *retainedRender2D)
}

func retainedInput2D[T any]() renderInput2D {
	return renderInput2D{
		revision: ecs.ComponentRevision[T],
		changed: func(world *ecs.World, changes *ecs.SystemContext, cache *retainedRender2D) {
			for entity := range ecs.Query[T](world, ecs.Changed[T](changes)).All {
				if cache.nodes[entity] == nil {
					cache.missing = true
				}
				if node := cache.nodes[entity]; node != nil && !node.queued {
					node.queued = true
					cache.dirty = append(cache.dirty, entity)
				}
			}
		},
	}
}

// Inherited metadata changes invalidate the shared property cache. Geometry
// and paint changes can instead extract only the affected entity set.
var retainedInputs2D = [...]renderInput2D{
	retainedInput2D[Parent](), retainedInput2D[SceneLayer](),
	retainedInput2D[RenderLayers](), retainedInput2D[UITargetCamera](),
	retainedInput2D[UIStackIndex](), retainedInput2D[RenderOrder2D](),
	retainedInput2D[Sprite2D](), retainedInput2D[Text2D](),
	retainedInput2D[GlobalTransform](), retainedInput2D[UIGlobalTransform](),
	retainedInput2D[Visibility](), retainedInput2D[Anchor](),
	retainedInput2D[Size2D](), retainedInput2D[UIBackground2D](),
	retainedInput2D[UIImage2D](), retainedInput2D[UIBorder2D](),
	retainedInput2D[RenderOutput2D](), retainedInput2D[Shader[graphics.Material2DShader]](),
	retainedInput2D[SpriteTrail2D](), retainedInput2D[SpriteLayers2D](),
	retainedInput2D[ParticleEmitter2D](), retainedInput2D[Tilemap2D](), retainedInput2D[Spine2D](),
}

const retainedRenderInputCount2D = 23
const inheritedRenderInputs2D = 6
const dynamicRenderInputs2D = 18

type retainedRender2D struct {
	nodes                map[ecs.Entity]*retainedEntity2D
	items                []renderItem2D
	dirty                []ecs.Entity
	scope                ecs.QueryScope
	changes              ecs.SystemContext
	versions, structures [retainedRenderInputCount2D]uint64
	lastRun              ecs.Tick
	width, height        float32
	rasterScale          float64
	device               *render.Device
	registry             uint64
	textureRevision      uint64
	initialized          bool
	missing              bool
	direct               bool
	queryScope           *ecs.QueryScope
	scopeVersion         uint64
	outputRevision       uint64
	// Changed is a generation, independent of ECS writes during drawing.
	generation uint64
	stats      RenderPreparation2D
}

// RenderPreparation2D counts CPU preparation work for the latest extraction.
type RenderPreparation2D struct {
	Extracted         int
	Reused            int
	Sorted            bool
	PropertiesUpdated int
}

func (node *retainedEntity2D) captureProperties(world *ecs.World, entity ecs.Entity, items []renderItem2D) {
	node.properties = len(node.positions) != 0 && !ecs.Has[RenderOutput2D](world, entity)
	if !node.properties {
		return
	}
	node.ui = items[node.positions[0]].ui
	if node.ui {
		value := ecs.GetRef[UIGlobalTransform](world, entity)
		if value == nil {
			node.properties = false
			return
		}
		node.matrix, node.clip = value.Matrix, value.clip
	} else {
		value := ecs.GetRef[GlobalTransform](world, entity)
		if value == nil {
			node.properties = false
			return
		}
		node.matrix, node.clip = value.Matrix, clipRect2D{}
	}
	for _, position := range node.positions {
		item := &items[position]
		if item.ui != node.ui || item.clip != node.clip {
			node.properties = false
		}
	}
}

func (node *retainedEntity2D) updateProperties(c *graphics.Canvas2D, world *ecs.World, entity ecs.Entity, items []renderItem2D) bool {
	if !node.properties {
		return false
	}
	matrix, clip := node.matrix, node.clip
	if node.ui {
		value := ecs.GetRef[UIGlobalTransform](world, entity)
		if value == nil {
			return false
		}
		matrix, clip = value.Matrix, value.clip
	} else {
		value := ecs.GetRef[GlobalTransform](world, entity)
		if value == nil {
			return false
		}
		matrix = value.Matrix
	}
	linear := matrix
	linear[12], linear[13] = node.matrix[12], node.matrix[13]
	if linear != node.matrix {
		return false
	}
	dx, dy := matrix[12]-node.matrix[12], matrix[13]-node.matrix[13]
	if !finitePaintBounds2D(matrix[12], matrix[13], dx, dy) {
		return false
	}
	// Fractional movement must rebuild glyph raster phases and exact bounds.
	if dx != float32(math.Trunc(float64(dx))) || dy != float32(math.Trunc(float64(dy))) {
		return false
	}
	if node.text != nil {
		_, canvasHeight := c.Size()
		text := ecs.GetRef[Text2D](world, entity)
		if text == nil {
			return false
		}
		window := text.visibleGlyphWindow(matrix, anchorForEntity2D(world, entity), node.ui, clip, canvasHeight)
		if window.valid && (!text.glyphBand.valid || window.top < text.glyphBand.top || window.bottom > text.glyphBand.bottom) {
			return false
		}
	}
	for _, position := range node.positions {
		item := &items[position]
		item.transform[12] += dx
		item.transform[13] += dy
		item.clip = clip
		if item.paint.cacheable {
			item.paint.rawBounds[0] += dx
			item.paint.rawBounds[2] += dx
			item.paint.rawBounds[1] += dy
			item.paint.rawBounds[3] += dy
			item.paint.resolve(item)
		}
	}
	node.matrix, node.clip = matrix, clip
	return true
}

func (cache *retainedRender2D) revisions(world *ecs.World) (versions, structures [retainedRenderInputCount2D]uint64) {
	for i, input := range retainedInputs2D {
		versions[i], structures[i] = input.revision(world)
	}
	return
}

func (cache *retainedRender2D) extract(c *graphics.Canvas2D, world *ecs.World, state *RenderWorld) []renderItem2D {
	cache.stats = RenderPreparation2D{}
	versions, structures := cache.revisions(world)
	width, height := c.Size()
	scope, scopeVersion := world.QueryScopeKey()
	full := !cache.initialized || cache.structures != structures || cache.registry != state.registry2D || cache.width != width || cache.height != height || cache.rasterScale != c.RasterScale() || cache.device != c.Device() || cache.textureRevision != graphics.TextureRevision() || cache.queryScope != scope || cache.scopeVersion != scopeVersion || cache.outputRevision != renderOutputRevision2D
	for i := range inheritedRenderInputs2D {
		full = full || cache.versions[i] != versions[i]
	}
	dynamic := len(state.materials2D) != 0
	for i := dynamicRenderInputs2D; i < len(versions); i++ {
		dynamic = dynamic || versions[i] != 0
	}
	full = full || dynamic
	writes := uint64(0)
	propertiesOnly := true
	if cache.initialized && cache.structures == structures {
		for i := inheritedRenderInputs2D; i < len(versions); i++ {
			writes += versions[i] - cache.versions[i]
		}
	}
	for i := range versions {
		if i != 8 && i != 9 && versions[i] != cache.versions[i] {
			propertiesOnly = false
		}
	}
	if dynamic || cache.initialized && !propertiesOnly && writes > uint64(max(16, len(cache.nodes)/4)) {
		// Content that changes densely keeps the sequential extraction path.
		// Do not copy it into per-entity retained storage or build paint hashes.
		items := collectRenderItems2D(c, world, false)
		cache.capture(c, world, state)
		cache.direct = true
		cache.stats = RenderPreparation2D{Extracted: len(items), Sorted: true}
		return items
	}
	if !full && !cache.direct && cache.versions == versions {
		cache.stats.Reused = len(cache.items)
		return cache.items
	}
	full = full || cache.direct
	if cache.nodes == nil {
		cache.nodes = make(map[ecs.Entity]*retainedEntity2D)
	}
	if !full {
		cache.dirty = cache.dirty[:0]
		cache.missing = false
		since := cache.lastRun
		if since == world.ChangeTick() {
			since--
		}
		cache.changes = world.ChangesSince(since)
		for i := inheritedRenderInputs2D; i < dynamicRenderInputs2D; i++ {
			if cache.versions[i] != versions[i] {
				retainedInputs2D[i].changed(world, &cache.changes, cache)
			}
		}
		// Dense changes use the sequential collector path; sparse changes reuse
		// the retained order and touch only matching rows in the ECS tables.
		full = cache.missing || !propertiesOnly && len(cache.dirty) > len(cache.nodes)/4
		if !full && propertiesOnly {
			updated := 0
			for _, entity := range cache.dirty {
				node := cache.nodes[entity]
				node.queued = false
				if !node.updateProperties(c, world, entity, cache.items) {
					full = true
					break
				}
				updated += len(node.positions)
			}
			if !full {
				cache.stats = RenderPreparation2D{Reused: len(cache.items), PropertiesUpdated: updated}
				cache.capture(c, world, state)
				return cache.items
			}
		}
	}
	if !full {
		cache.scope.Reset(cache.dirty)
		previous := world.SetQueryScope(&cache.scope)
		items := collectRenderItems2D(c, world, false)
		world.SetQueryScope(previous)
		for _, entity := range cache.dirty {
			cache.nodes[entity].reset()
		}
		for _, item := range items {
			node := cache.nodes[item.entity]
			if node == nil || node.count >= len(node.positions) {
				full = true
				break
			}
			position := node.positions[node.count]
			// Sequence only breaks ties between parts emitted by the same entity.
			item.sequence = cache.items[position].sequence
			if item.renderSortKey() != cache.items[position].renderSortKey() {
				full = true
				break
			}
			cache.items[position] = node.snapshot(item)
			node.count++
		}
		for _, entity := range cache.dirty {
			node := cache.nodes[entity]
			full = full || node.count != len(node.positions)
			node.queued = false
			node.snapshots.releaseUnused()
			node.captureProperties(world, entity, cache.items)
		}
		if !full {
			cache.stats.Extracted, cache.stats.Reused = len(items), len(cache.items)-len(items)
		}
	}
	if full {
		items := collectRenderItems2D(c, world, false)
		if len(items) < len(cache.items) {
			clear(cache.items[len(items):])
		}
		cache.items = append(cache.items[:0], items...)
		for _, node := range cache.nodes {
			node.reset()
			node.positions = node.positions[:0]
			node.queued = false
		}
		for i, item := range cache.items {
			node := cache.nodes[item.entity]
			if node == nil {
				node = &retainedEntity2D{}
				cache.nodes[item.entity] = node
			}
			cache.items[i] = node.snapshot(item)
			node.positions = append(node.positions, i)
		}
		for entity, node := range cache.nodes {
			node.snapshots.releaseUnused()
			node.captureProperties(world, entity, cache.items)
			if len(node.positions) == 0 {
				delete(cache.nodes, entity)
			}
		}
		cache.stats.Extracted, cache.stats.Sorted = len(items), true
	}
	cache.capture(c, world, state)
	cache.direct = false
	return cache.items
}

func (cache *retainedRender2D) capture(c *graphics.Canvas2D, world *ecs.World, state *RenderWorld) {
	cache.versions, cache.structures = cache.revisions(world)
	cache.width, cache.height = c.Size()
	cache.rasterScale, cache.device = c.RasterScale(), c.Device()
	cache.registry, cache.textureRevision = state.registry2D, graphics.TextureRevision()
	cache.queryScope, cache.scopeVersion = world.QueryScopeKey()
	cache.outputRevision = renderOutputRevision2D
	cache.lastRun, cache.initialized = world.ChangeTick(), true
	cache.generation++
}
