package scene

import (
	"image"
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/reuse"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

// Content snapshots belong to their entity, so extracting one changed entity
// cannot overwrite a snapshot still used by an unchanged entity.
type retainedEntity2D struct {
	snapshots       *renderSnapshots2D
	sprites         []Sprite2D
	nineSlices      []NineSlice2D
	previousSprites int
	first           int
	extra           []int
	count           int
	queued          bool
	dynamic         uint8
	sourceText      *Text2D
	text            *Text2D
	matrix          mgl32.Mat4
	clip            clipRect2D
	ui              bool
	properties      bool
}

func (node *retainedEntity2D) reset() {
	if node.snapshots != nil {
		node.snapshots.reset()
	}
	node.previousSprites = len(node.sprites)
	node.sprites = node.sprites[:0]
	node.nineSlices = node.nineSlices[:0]
	node.count = 0
	node.sourceText, node.text = nil, nil
}

func (node *retainedEntity2D) snapshot(item renderItem2D) renderItem2D {
	if item.sprite != nil {
		sprite := *item.sprite
		if sprite.Slice != nil {
			node.nineSlices = append(node.nineSlices, *sprite.Slice)
			sprite.Slice = &node.nineSlices[len(node.nineSlices)-1]
		}
		node.sprites = append(node.sprites, sprite)
		item.sprite = &node.sprites[len(node.sprites)-1]
	}
	if node.snapshots == nil && (item.text != nil || item.emitter != nil || item.tilemap != nil || item.spine != nil) {
		node.snapshots = &renderSnapshots2D{}
	}
	if item.text != nil {
		if node.sourceText != item.text {
			node.sourceText = item.text
			node.text = node.snapshots.addText(*item.text)
		}
		item.text = node.text
	}
	if item.emitter != nil {
		item.emitter = node.snapshots.addEmitter(*item.emitter)
	}
	if item.tilemap != nil {
		item.tilemap = node.snapshots.addTilemap(*item.tilemap)
	}
	if item.spine != nil {
		item.spine = node.snapshots.addSpine(*item.spine)
	}
	return item
}

// A single output needs no position allocation. Multi-part entities keep only
// the additional positions; paint records are contiguous in the render cache.
func (node *retainedEntity2D) positionCount() int {
	if node.first < 0 {
		return 0
	}
	return 1 + len(node.extra)
}

func (node *retainedEntity2D) positionAt(index int) int {
	if index == 0 {
		return node.first
	}
	return node.extra[index-1]
}

func (node *retainedEntity2D) positions(yield func(int) bool) {
	if node.first < 0 || !yield(node.first) {
		return
	}
	for _, position := range node.extra {
		if !yield(position) {
			return
		}
	}
}

func (node *retainedEntity2D) releaseUnused() {
	clearSnapshotTail(node.sprites, node.previousSprites)
	if node.snapshots != nil {
		node.snapshots.releaseUnused()
	}
}

func (cache *retainedRender2D) snapshot(node *retainedEntity2D, position int, item renderItem2D, device *render.Device) renderItem2D {
	item.device = device
	item = node.snapshot(item)
	cache.paints[position] = preparePaintItem2D(&item)
	item.paint = &cache.paints[position]
	return item
}

// Fixed-size blocks keep snapshot owners stable as the scene grows, without
// allocating one GC object for every entity.
func (cache *retainedRender2D) allocateNode() *retainedEntity2D {
	if len(cache.freeNodes) == 0 {
		block := make([]retainedEntity2D, 64)
		cache.nodeBlocks = append(cache.nodeBlocks, block)
		for i := range block {
			cache.freeNodes = append(cache.freeNodes, &block[i])
		}
	}
	last := len(cache.freeNodes) - 1
	node := cache.freeNodes[last]
	cache.freeNodes[last] = nil
	cache.freeNodes = cache.freeNodes[:last]
	node.first = -1
	return node
}

type renderInput2D struct {
	revision func(*ecs.World) (uint64, uint64)
	changed  func(*ecs.World, *ecs.SystemContext, *retainedRender2D)
	poll     func(*ecs.World, *retainedRender2D)
}

func retainedInput2D[T any]() renderInput2D {
	return renderInput2D{
		revision: (*ecs.World).ComponentRevision[T],
		changed: func(world *ecs.World, changes *ecs.SystemContext, cache *retainedRender2D) {
			for entity := range world.Query[T](changes.Changed[T]()).All {
				cache.mark(entity)
			}
		},
	}
}

func inheritedRetainedInput2D[T any]() renderInput2D {
	return renderInput2D{
		revision: (*ecs.World).ComponentValueRevision[T],
		changed: func(world *ecs.World, changes *ecs.SystemContext, cache *retainedRender2D) {
			hierarchy := HierarchyFor(world)
			cache.hierarchyQueue = cache.hierarchyQueue[:0]
			cache.hierarchyMarks = reuse.Resize(cache.hierarchyMarks, world.EntitySlots())
			cache.hierarchyGeneration++
			if cache.hierarchyGeneration == 0 {
				clear(cache.hierarchyMarks)
				cache.hierarchyGeneration++
			}
			for entity := range world.Query[T](ecs.Unscoped(), changes.Changed[T]()).All {
				cache.enqueueHierarchy(entity)
			}
			for i := 0; i < len(cache.hierarchyQueue); i++ {
				entity := cache.hierarchyQueue[i]
				if cache.nodes[entity] != nil {
					cache.mark(entity)
				}
				for child := hierarchy.FirstChild(entity); child != 0; child = hierarchy.NextSibling(child) {
					cache.enqueueHierarchy(child)
				}
			}
		},
	}
}

func (cache *retainedRender2D) enqueueHierarchy(entity ecs.Entity) {
	index, _ := entity.Index()
	if cache.hierarchyMarks[index] != cache.hierarchyGeneration {
		cache.hierarchyMarks[index] = cache.hierarchyGeneration
		cache.hierarchyQueue = append(cache.hierarchyQueue, entity)
	}
}

func (cache *retainedRender2D) mark(entity ecs.Entity) {
	if node := cache.nodes[entity]; node == nil {
		cache.missing = true
	} else if !node.queued {
		node.queued = true
		cache.dirty = append(cache.dirty, entity)
		if node.positionCount() != 0 {
			cache.dirtyNodes++
		}
	}
}

func dynamicRetainedInput2D[T any](kind renderCollector2DKind, drawable func(*T, *render.Device) bool) renderInput2D {
	input := retainedInput2D[T]()
	flag := uint8(1 << kind)
	input.poll = func(world *ecs.World, cache *retainedRender2D) {
		for entity, access := range world.Query2[T, Visibility]().All {
			node := cache.nodes[entity]
			visible := access.B.Ref().Visible && drawable(access.A.Ref(), graphics.DeviceFor(world)) && (world.Has[GlobalTransform](entity) || world.Has[UIGlobalTransform](entity))
			if visible || node != nil && node.dynamic&flag != 0 {
				// A dynamic candidate can emit nothing (for example, clipped tiles
				// or hidden sprite layers). Remember its empty result so polling it
				// does not rebuild unrelated static content on every frame.
				if node == nil {
					node = cache.allocateNode()
					cache.nodes[entity] = node
				}
				if visible {
					node.dynamic |= flag
				} else {
					node.dynamic &^= flag
				}
				cache.mark(entity)
			}
		}
	}
	return input
}

// Inherited metadata changes invalidate the shared property cache. Geometry
// and paint changes can instead extract only the affected entity set.
var retainedInputs2D = [...]renderInput2D{
	inheritedRetainedInput2D[PainterGroup2D](), inheritedRetainedInput2D[Parent](), inheritedRetainedInput2D[SceneLayer](),
	inheritedRetainedInput2D[RenderLayers](), inheritedRetainedInput2D[UITargetCamera](),
	inheritedRetainedInput2D[UIStackIndex](), inheritedRetainedInput2D[RenderOrder2D](),
	retainedInput2D[Sprite2D](), retainedInput2D[Text2D](),
	retainedInput2D[GlobalTransform](), retainedInput2D[UIGlobalTransform](),
	retainedInput2D[Visibility](), retainedInput2D[Anchor](),
	retainedInput2D[Size2D](), retainedInput2D[UIBackground2D](),
	retainedInput2D[UIImage2D](), retainedInput2D[UIBorder2D](),
	retainedInput2D[RenderOutput2D](), retainedInput2D[Shader[graphics.Material2DShader]](),
	dynamicRetainedInput2D(renderCollectorSpriteTrail2D, func(t *SpriteTrail2D, device *render.Device) bool { return len(t.samples) != 0 }),
	dynamicRetainedInput2D(renderCollectorSpriteLayers2D, func(l *SpriteLayers2D, device *render.Device) bool {
		for i := range l.Layers {
			layer := &l.Layers[i]
			width, height := layer.Sprite.Size()
			if layer.Visible && layer.Sprite.Alpha > 0 && layer.Sprite.Tint.A > 0 && layer.Sprite.Texture != nil && layer.Sprite.Texture.ValidFor(device) && width > 0 && height > 0 {
				return true
			}
		}
		return false
	}),
	dynamicRetainedInput2D(renderCollectorParticle2D, func(e *ParticleEmitter2D, device *render.Device) bool { return len(e.particles) != 0 }),
	dynamicRetainedInput2D(renderCollectorTilemap2D, func(t *Tilemap2D, device *render.Device) bool { return t.Alpha > 0 && t.ValidFor(device) }),
	dynamicRetainedInput2D(renderCollectorSpine2D, func(s *Spine2D, device *render.Device) bool {
		return s.Player != nil && s.Player.Resource != nil && s.Player.Alpha > 0
	}),
}

var dynamicCollectors2D = [...]renderCollector2DKind{renderCollectorSpriteTrail2D, renderCollectorSpriteLayers2D, renderCollectorParticle2D, renderCollectorTilemap2D, renderCollectorSpine2D}

const retainedRenderInputCount2D = 24
const inheritedRenderInputs2D = 7
const dynamicRenderInputs2D = 19

type retainedRender2D struct {
	nodes                map[ecs.Entity]*retainedEntity2D
	nodeBlocks           [][]retainedEntity2D
	freeNodes            []*retainedEntity2D
	paints               []paintItem2D
	items                []renderItem2D
	dirty                []ecs.Entity
	dirtyNodes           int
	damage               []renderDamage2D
	damageFlags          []bool
	damageValid          bool
	hierarchyQueue       []ecs.Entity
	hierarchyMarks       []uint64
	hierarchyGeneration  uint64
	scope                ecs.QueryScope
	changes              ecs.SystemContext
	versions, structures [retainedRenderInputCount2D]uint64
	lastRun              ecs.Tick
	width, height        float32
	rasterScale          float64
	device               *render.Device
	registry             uint64
	textureRevision      uint64
	textures             map[*graphics.Texture]*retainedTexture2D
	initialized          bool
	missing              bool
	direct               bool
	queryScope           *ecs.QueryScope
	scopeVersion         uint64
	outputRevision       uint64
	materialStructure    uint64
	// Changed is a generation, independent of ECS writes during drawing.
	generation uint64
	stats      RenderPreparation2D
}

type renderDamage2D struct {
	position int
	previous image.Rectangle
}

func (cache *retainedRender2D) recordDamage(node *retainedEntity2D) {
	if cache.damageValid {
		for position := range node.positions {
			cache.recordItemDamage(position)
		}
	}
}

func (cache *retainedRender2D) recordItemDamage(position int) {
	if cache.damageValid && !cache.damageFlags[position] {
		cache.damageFlags[position] = true
		cache.damage = append(cache.damage, renderDamage2D{position: position, previous: cache.paints[position].bounds})
	}
}

// RenderPreparation2D counts CPU preparation work for the latest extraction.
type RenderPreparation2D struct {
	Extracted         int
	Reused            int
	Sorted            bool
	PropertiesUpdated int
	PaintUpdated      int
}

func (node *retainedEntity2D) captureProperties(world *ecs.World, entity ecs.Entity, items []renderItem2D) {
	node.properties = node.positionCount() != 0 && !world.Has[RenderOutput2D](entity)
	if !node.properties {
		return
	}
	node.ui = items[node.first].ui
	if node.ui {
		value := world.GetRef[UIGlobalTransform](entity)
		if value == nil {
			node.properties = false
			return
		}
		node.matrix, node.clip = value.Matrix, value.clip
	} else {
		value := world.GetRef[GlobalTransform](entity)
		if value == nil {
			node.properties = false
			return
		}
		node.matrix, node.clip = value.Matrix, clipRect2D{}
	}
	for position := range node.positions {
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
		value := world.GetRef[UIGlobalTransform](entity)
		if value == nil {
			return false
		}
		matrix, clip = value.Matrix, value.clip
	} else {
		value := world.GetRef[GlobalTransform](entity)
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
		text := world.GetRef[Text2D](entity)
		if text == nil {
			return false
		}
		window := text.visibleGlyphWindow(matrix, anchorForEntity2D(world, entity), node.ui, clip, canvasHeight)
		if window.valid && (!text.glyphBand.valid || window.top < text.glyphBand.top || window.bottom > text.glyphBand.bottom) {
			return false
		}
	}
	for position := range node.positions {
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
	for _, change := range cache.damage {
		cache.damageFlags[change.position] = false
	}
	cache.damage, cache.damageValid = cache.damage[:0], false
	versions, structures := cache.revisions(world)
	width, height := c.Size()
	scope, scopeVersion := world.QueryScopeKey()
	full := !cache.initialized || cache.structures != structures || cache.registry != state.registry2D || cache.width != width || cache.height != height || cache.rasterScale != c.RasterScale() || cache.device != c.Device() || cache.queryScope != scope || cache.scopeVersion != scopeVersion || cache.outputRevision != renderOutputRevision2D
	materialStructure := uint64(0)
	for _, material := range state.materials2D {
		materialStructure += material.structure(world)
	}
	full = full || cache.materialStructure != materialStructure
	writes := uint64(0)
	propertiesOnly := true
	if cache.initialized && cache.structures == structures {
		for i := inheritedRenderInputs2D; i < len(versions); i++ {
			writes += versions[i] - cache.versions[i]
		}
	}
	for i := range versions {
		if i != 9 && i != 10 && versions[i] != cache.versions[i] {
			propertiesOnly = false
		}
	}
	if cache.initialized && !propertiesOnly && writes > uint64(max(16, len(cache.nodes)/4)) {
		// Content that changes densely keeps the sequential extraction path.
		// Do not copy it into per-entity retained storage or build paint hashes.
		return cache.extractDirect(c, world, state)
	}
	if cache.nodes == nil {
		cache.nodes = make(map[ecs.Entity]*retainedEntity2D)
	}
	if !full {
		cache.dirty = cache.dirty[:0]
		cache.dirtyNodes = 0
		cache.missing = false
		cache.damageValid = true
		if !cache.direct {
			cache.refreshTextures()
		}
		for i, kind := range dynamicCollectors2D {
			if state.collectors2D[kind] != nil {
				retainedInputs2D[dynamicRenderInputs2D+i].poll(world, cache)
			}
		}
		if cache.dirtyNodes > len(cache.nodes)/4 {
			return cache.extractDirect(c, world, state)
		}
		for _, material := range state.materials2D {
			material.poll(world, cache)
		}
		if len(cache.dirty) != 0 {
			propertiesOnly = false
		}
		if !cache.direct && !cache.missing && len(cache.dirty) == 0 && cache.versions == versions {
			cache.stats.Reused = len(cache.items)
			if cache.stats.PaintUpdated != 0 {
				cache.generation++
			}
			return cache.items
		}
		since := cache.lastRun
		if since == world.ChangeTick() {
			since--
		}
		cache.changes = world.ChangesSince(since)
		for i := range retainedInputs2D {
			if cache.versions[i] != versions[i] {
				retainedInputs2D[i].changed(world, &cache.changes, cache)
			}
		}
		// Dense changes use the sequential collector path; sparse changes reuse
		// the retained order and touch only matching rows in the ECS tables.
		full = cache.direct || cache.missing || !propertiesOnly && cache.dirtyNodes > len(cache.nodes)/4
		cache.damageValid = !full && len(cache.dirty)+cache.stats.PaintUpdated <= max(16, len(cache.nodes)/8)
		if !full && propertiesOnly {
			updated := 0
			for _, entity := range cache.dirty {
				node := cache.nodes[entity]
				cache.recordDamage(node)
				node.queued = false
				if !node.updateProperties(c, world, entity, cache.items) {
					full = true
					break
				}
				updated += node.positionCount()
			}
			if !full {
				cache.stats.Reused, cache.stats.PropertiesUpdated = len(cache.items), updated
				cache.capture(c, world, state)
				if updated != 0 || cache.stats.PaintUpdated != 0 {
					cache.generation++
				}
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
			cache.recordDamage(cache.nodes[entity])
			cache.nodes[entity].reset()
		}
		for _, item := range items {
			node := cache.nodes[item.entity]
			if node == nil || node.count >= node.positionCount() {
				full = true
				break
			}
			position := node.positionAt(node.count)
			// Sequence only breaks ties between parts emitted by the same entity.
			item.sequence = cache.items[position].sequence
			if item.renderSortKey() != cache.items[position].renderSortKey() {
				full = true
				break
			}
			previousTexture := cache.items[position].texture
			cache.items[position] = cache.snapshot(node, position, item, c.Device())
			cache.replaceTexture(position, previousTexture)
			node.count++
		}
		for _, entity := range cache.dirty {
			node := cache.nodes[entity]
			full = full || node.count != node.positionCount()
			node.queued = false
			node.releaseUnused()
			node.captureProperties(world, entity, cache.items)
		}
		if !full {
			cache.stats.Extracted, cache.stats.Reused = len(items), len(cache.items)-len(items)
		}
	}
	if full {
		cache.damageValid = false
		items := collectRenderItems2D(c, world, false)
		if len(items) < len(cache.items) {
			clear(cache.items[len(items):])
		}
		cache.items = append(cache.items[:0], items...)
		cache.paints = reuse.Resize(cache.paints, len(items))
		cache.damage = cache.damage[:0]
		cache.damageFlags = reuse.Resize(cache.damageFlags, len(items))
		clear(cache.damageFlags)
		for _, dependency := range cache.textures {
			dependency.positions = dependency.positions[:0]
		}
		for _, node := range cache.nodes {
			node.reset()
			// Rebuild membership from the current component queries, including
			// entities whose dynamic component or visibility was removed.
			node.dynamic = 0
			node.first = -1
			node.extra = node.extra[:0]
			node.queued = false
		}
		for i, item := range cache.items {
			node := cache.nodes[item.entity]
			if node == nil {
				node = cache.allocateNode()
				cache.nodes[item.entity] = node
			}
			cache.items[i] = cache.snapshot(node, i, item, c.Device())
			cache.trackTexture(i)
			if node.first < 0 {
				node.first = i
			} else {
				node.extra = append(node.extra, i)
			}
		}
		for texture, dependency := range cache.textures {
			if len(dependency.positions) == 0 {
				delete(cache.textures, texture)
			}
		}
		cache.dirty = cache.dirty[:0]
		cache.dirtyNodes = 0
		for i, kind := range dynamicCollectors2D {
			if state.collectors2D[kind] != nil {
				retainedInputs2D[dynamicRenderInputs2D+i].poll(world, cache)
			}
		}
		for entity, node := range cache.nodes {
			node.queued = false
			node.releaseUnused()
			node.captureProperties(world, entity, cache.items)
			if node.positionCount() == 0 && node.dynamic == 0 {
				delete(cache.nodes, entity)
				*node = retainedEntity2D{first: -1}
				cache.freeNodes = append(cache.freeNodes, node)
			}
		}
		cache.stats.Extracted, cache.stats.Sorted = len(items), true
	}
	cache.capture(c, world, state)
	if full || cache.stats.Extracted != 0 || cache.stats.PaintUpdated != 0 {
		cache.generation++
	}
	cache.direct = false
	return cache.items
}

func (cache *retainedRender2D) extractDirect(c *graphics.Canvas2D, world *ecs.World, state *RenderWorld) []renderItem2D {
	items := collectRenderItems2D(c, world, false)
	for _, entity := range cache.dirty {
		if node := cache.nodes[entity]; node != nil {
			node.queued = false
		}
	}
	cache.capture(c, world, state)
	cache.generation++
	cache.direct = true
	cache.stats = RenderPreparation2D{Extracted: len(items), Sorted: true}
	return items
}

func (cache *retainedRender2D) capture(c *graphics.Canvas2D, world *ecs.World, state *RenderWorld) {
	cache.versions, cache.structures = cache.revisions(world)
	cache.width, cache.height = c.Size()
	cache.rasterScale, cache.device = c.RasterScale(), c.Device()
	cache.registry, cache.textureRevision = state.registry2D, graphics.TextureRevision()
	cache.queryScope, cache.scopeVersion = world.QueryScopeKey()
	cache.outputRevision = renderOutputRevision2D
	cache.materialStructure = 0
	for _, material := range state.materials2D {
		cache.materialStructure += material.structure(world)
	}
	cache.lastRun, cache.initialized = world.ChangeTick(), true
}
