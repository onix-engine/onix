package scene

import (
	"image"
	"math"
	"math/bits"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/internal/reuse"
	"github.com/onix-engine/onix/render"
)

// Content snapshots belong to their entity, so extracting one changed entity
// cannot overwrite a snapshot still used by an unchanged entity.
type retainedEntity2D struct {
	text                  *textPaint2D
	snapshots             *renderSnapshots2D
	sourceText            *textPaint2D
	extra                 []int
	nineSlices            []NineSlice2D
	sprites               []Sprite2D
	texts                 []textPaint2D
	renderLayers          RenderLayers
	renderScene           SceneLayer
	first                 int
	previousSprites       int
	renderParent          ecs.Entity
	renderFirstChild      ecs.Entity
	renderNextSibling     ecs.Entity
	renderPreviousSibling ecs.Entity
	count                 int
	id                    uint64
	renderTarget          ecs.Entity
	matrix                mgl32.Mat4
	clip                  clipRect2D
	mixedCamera           bool
	dynamic               uint8
	dirty                 renderNodeDirty2D
	allocated             bool
	properties            bool
	ui                    bool
}

type renderNodeDirty2D uint8

const (
	renderNodeDirtyGeometry2D renderNodeDirty2D = 1 << iota
	renderNodeDirtyContent2D
	renderNodeDirtyVisibility2D
	renderNodeDirtyOrder2D
)

func (tree *renderTree2D) allocateNode() *retainedEntity2D {
	if len(tree.freeNodes) == 0 {
		block := make([]retainedEntity2D, 64)
		tree.nodeBlocks = append(tree.nodeBlocks, block)
		for i := range block {
			tree.freeNodes = append(tree.freeNodes, &block[i])
		}
	}
	last := len(tree.freeNodes) - 1
	node := tree.freeNodes[last]
	tree.freeNodes[last] = nil
	tree.freeNodes = tree.freeNodes[:last]
	tree.nextID++
	if tree.nextID == 0 {
		tree.nextID++
	}
	*node = retainedEntity2D{id: tree.nextID, first: -1, allocated: true}
	return node
}

func (tree *renderTree2D) clearLinks() {
	for _, node := range tree.nodes {
		node.renderParent = 0
		node.renderFirstChild = 0
		node.renderNextSibling = 0
		node.renderPreviousSibling = 0
	}
}

func (tree *renderTree2D) unlink(entity ecs.Entity) {
	node := tree.nodes[entity]
	if node == nil || node.renderParent == 0 {
		return
	}
	parent := tree.nodes[node.renderParent]
	previous := tree.nodes[node.renderPreviousSibling]
	next := tree.nodes[node.renderNextSibling]
	if previous != nil {
		previous.renderNextSibling = node.renderNextSibling
	} else if parent != nil {
		parent.renderFirstChild = node.renderNextSibling
	}
	if next != nil {
		next.renderPreviousSibling = node.renderPreviousSibling
	}
	node.renderParent = 0
	node.renderNextSibling = 0
	node.renderPreviousSibling = 0
}

func (tree *renderTree2D) reparent(entity ecs.Entity, hierarchy *Hierarchy) {
	node := tree.nodes[entity]
	if node == nil || node.first < 0 {
		return
	}
	if hierarchy.Depth(entity) < 0 {
		tree.unlink(entity)
		return
	}
	for remaining, parent := len(hierarchy.Entities()), hierarchy.Parent(entity); parent != 0 && remaining > 0; remaining, parent = remaining-1, hierarchy.Parent(parent) {
		ancestor := tree.nodes[parent]
		if ancestor == nil || ancestor.first < 0 {
			continue
		}
		if node.renderParent == parent {
			return
		}
		tree.unlink(entity)
		node.renderParent = parent
		node.renderNextSibling = ancestor.renderFirstChild
		if next := tree.nodes[node.renderNextSibling]; next != nil {
			next.renderPreviousSibling = entity
		}
		ancestor.renderFirstChild = entity
		return
	}
	tree.unlink(entity)
}

func (tree *renderTree2D) rebuildLinks(hierarchy *Hierarchy) {
	tree.clearLinks()
	for entity := range tree.nodes {
		tree.reparent(entity, hierarchy)
	}
}

// A part identifies one output of a render node. Parts are assigned by node
// when the tree rebuilds. They retain their slots through sparse updates.
type renderPart2D uint32

func (tree *renderTree2D) itemForPart(part renderPart2D) *renderItem2D {
	return &tree.items[tree.partPositions[part]]
}

func (node *retainedEntity2D) captureCamera(items []renderItem2D) {
	node.mixedCamera = false
	if node.first < 0 {
		node.renderLayers = RenderLayers{}
		return
	}
	first := &items[node.first]
	node.renderLayers, node.renderScene, node.renderTarget = first.layers, first.scene, first.target
	for _, position := range node.extra {
		item := &items[position]
		if !sameRenderCamera2D(item, first) {
			node.mixedCamera = true
			return
		}
	}
}

func sameRenderCamera2D(a, b *renderItem2D) bool {
	return a.ui == b.ui && a.target == b.target && a.scene.ID == b.scene.ID &&
		a.layers.empty == b.layers.empty && slices.Equal(a.layers.blocks, b.layers.blocks)
}

type cameraRenderPositions2DScratch struct{}

func (tree *renderTree2D) visiblePositions(world *ecs.World, camera ExtractedCamera, defaultUICamera ecs.Entity) []uint32 {
	visible := world.FrameScratch[visibleRenderItems2DScratch, uint32]()
	visible.Values = tree.appendVisiblePositions(world, camera, defaultUICamera, visible.Values)
	visible.ReleaseUnused()
	return visible.Values
}

func (tree *renderTree2D) appendVisiblePositions(world *ecs.World, camera ExtractedCamera, defaultUICamera ecs.Entity, dst []uint32) []uint32 {
	// Nodes can emit parts that interleave with other nodes in painter order.
	// Resolve routing per node, then use a bitset to emit in that order.
	mask := world.FrameScratch[cameraRenderPositions2DScratch, uint64]()
	mask.Values = reuse.Resize(mask.Values, (len(tree.items)+63)/64)
	clear(mask.Values)
	for _, node := range tree.nodes {
		if node.first < 0 {
			continue
		}
		if !node.mixedCamera && !matchesRenderCamera2D(node.renderScene.ID, node.renderLayers, tree.items[node.first].ui, node.renderTarget, camera, defaultUICamera) {
			continue
		}
		for position := range node.positions {
			if !node.mixedCamera || renderItemForCamera2D(&tree.items[position], camera, defaultUICamera) {
				mask.Values[position/64] |= uint64(1) << (position % 64)
			}
		}
	}
	for index, word := range mask.Values {
		for word != 0 {
			position := index*64 + bits.TrailingZeros64(word)
			dst = append(dst, uint32(position))
			word &= word - 1
		}
	}
	return dst
}

func (node *retainedEntity2D) reset() {
	if node.snapshots != nil {
		node.snapshots.reset()
	}
	node.previousSprites = len(node.sprites)
	clear(node.texts)
	node.texts = node.texts[:0]
	node.sprites = node.sprites[:0]
	node.nineSlices = node.nineSlices[:0]
	node.count = 0
	node.sourceText, node.text = nil, nil
	node.dirty = 0
}

func (node *retainedEntity2D) snapshot(item renderItem2D) renderItem2D {
	if item.snapshot[Sprite2D]() != nil {
		sprite := *item.snapshot[Sprite2D]()
		if sprite.Slice != nil {
			node.nineSlices = append(node.nineSlices, *sprite.Slice)
			sprite.Slice = &node.nineSlices[len(node.nineSlices)-1]
		}
		node.sprites = append(node.sprites, sprite)
		item.content = &node.sprites[len(node.sprites)-1]
	}
	if node.snapshots == nil && (item.material2D != nil || item.snapshot[ParticleEmitter2D]() != nil || item.snapshot[Tilemap2D]() != nil || item.snapshot[Spine2D]() != nil) {
		node.snapshots = &renderSnapshots2D{}
	}
	if item.snapshot[textPaint2D]() != nil {
		if node.sourceText != item.snapshot[textPaint2D]() {
			node.sourceText = item.snapshot[textPaint2D]()
			node.texts = append(node.texts, *item.snapshot[textPaint2D]())
			node.text = &node.texts[len(node.texts)-1]
		}
		item.content = node.text
	}
	if item.snapshot[ParticleEmitter2D]() != nil {
		item.content = node.snapshots.addEmitter(*item.snapshot[ParticleEmitter2D]())
	}
	if item.snapshot[Tilemap2D]() != nil {
		item.content = node.snapshots.addTilemap(*item.snapshot[Tilemap2D]())
	}
	if item.snapshot[Spine2D]() != nil {
		item.content = node.snapshots.addSpine(*item.snapshot[Spine2D]())
	}
	if item.material2D != nil {
		item.material2D = node.snapshots.addMaterial(*item.material2D)
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
	node.sourceText = nil
	clearSnapshotTail(node.sprites, node.previousSprites)
	if node.snapshots != nil {
		node.snapshots.releaseUnused()
	}
}

func (cache *renderTree2D) snapshot(node *retainedEntity2D, position int, item renderItem2D, device *render.Device) renderItem2D {
	item = node.snapshot(item)
	cache.paints[position] = preparePaintItem2D(&item, device)
	item.paint = &cache.paints[position]
	return item
}

// Fixed-size blocks avoid allocating one GC object for every entity. Owners
// stay stable during growth; maintenance can relocate them to compact sparse blocks.
type renderInput2D struct {
	revision func(*ecs.World) (uint64, uint64)
	changed  func(*ecs.World, *ecs.SystemContext, *renderTree2D)
	poll     func(*ecs.World, *renderTree2D) int
	dirty    renderNodeDirty2D
}

func retainedInput2D[T any](flags renderNodeDirty2D) renderInput2D {
	input := renderInput2D{
		revision: (*ecs.World).ComponentValueRevision[T],
		dirty:    flags,
		changed: func(world *ecs.World, changes *ecs.SystemContext, cache *renderTree2D) {
			for entity := range world.Query[T](changes.Changed[T]()).All {
				cache.mark(entity, flags)
			}
		},
	}
	return input
}

// Local ordering cannot make an entity drawable. Ignore writes to ancestors
// and other entities that have no retained output.
func localRetainedInput2D[T any]() renderInput2D {
	input := retainedInput2D[T](renderNodeDirtyOrder2D)
	input.changed = func(world *ecs.World, changes *ecs.SystemContext, cache *renderTree2D) {
		for entity := range world.Query[T](changes.Changed[T]()).All {
			if cache.nodes[entity] != nil {
				cache.mark(entity, renderNodeDirtyOrder2D)
			}
		}
	}
	return input
}

func inheritedRetainedInput2D[T any]() renderInput2D {
	input := renderInput2D{
		revision: (*ecs.World).ComponentValueRevision[T],
		dirty:    renderNodeDirtyGeometry2D | renderNodeDirtyVisibility2D | renderNodeDirtyOrder2D,
		changed: func(world *ecs.World, changes *ecs.SystemContext, cache *renderTree2D) {
			for entity := range world.Query[T](ecs.Unscoped(), changes.Changed[T]()).All {
				cache.enqueueHierarchy(entity)
			}
		},
	}
	return input
}

func (cache *renderTree2D) beginHierarchyChanges(world *ecs.World) {
	cache.hierarchyQueue = cache.hierarchyQueue[:0]
	cache.hierarchyMarks = reuse.Resize(cache.hierarchyMarks, world.EntitySlots())
	cache.hierarchyRetention[1].Record(len(cache.hierarchyMarks))
	cache.hierarchyGeneration++
	if cache.hierarchyGeneration == 0 {
		clear(cache.hierarchyMarks)
		cache.hierarchyGeneration++
	}
}

func (cache *renderTree2D) markHierarchyChanges(world *ecs.World) {
	if len(cache.hierarchyQueue) == 0 {
		return
	}
	hierarchy := HierarchyFor(world)
	for i := 0; i < len(cache.hierarchyQueue); i++ {
		entity := cache.hierarchyQueue[i]
		node := cache.nodes[entity]
		if node != nil {
			cache.mark(entity, renderNodeDirtyGeometry2D|renderNodeDirtyVisibility2D|renderNodeDirtyOrder2D)
		}
		if node != nil && node.first >= 0 {
			cache.reparent(entity, hierarchy)
			for child := node.renderFirstChild; child != 0; {
				next := cache.nodes[child].renderNextSibling
				cache.enqueueHierarchy(child)
				child = next
			}
			continue
		}
		for child := hierarchy.FirstChild(entity); child != 0; child = hierarchy.NextSibling(child) {
			cache.enqueueHierarchy(child)
		}
	}
	cache.hierarchyRetention[0].Record(len(cache.hierarchyQueue))
}

func (cache *renderTree2D) enqueueHierarchy(entity ecs.Entity) {
	index, _ := entity.Index()
	if cache.hierarchyMarks[index] != cache.hierarchyGeneration {
		cache.hierarchyMarks[index] = cache.hierarchyGeneration
		cache.hierarchyQueue = append(cache.hierarchyQueue, entity)
	}
}

func (cache *renderTree2D) mark(entity ecs.Entity, flags renderNodeDirty2D) {
	node := cache.nodes[entity]
	if node == nil {
		cache.missing = true
		return
	}
	if node.dirty == 0 {
		cache.dirty = append(cache.dirty, entity)
		if node.positionCount() != 0 {
			cache.dirtyNodes++
		}
	}
	node.dirty |= flags
}

func dynamicRetainedInput2D[T any](kind renderCollector2DKind, drawable func(*T, *render.Device) bool) renderInput2D {
	input := retainedInput2D[T](renderNodeDirtyContent2D)
	flag := uint8(1 << kind)
	input.poll = func(world *ecs.World, cache *renderTree2D) int {
		active := 0
		for entity, access := range world.Query2[T, Visibility]().All {
			node := cache.nodes[entity]
			visible := access.B.Ref().Visible && drawable(access.A.Ref(), graphics.DeviceFor(world)) && (world.Has[GlobalTransform](entity) || world.Has[UIGlobalTransform](entity))
			if visible {
				active++
			}
			if cache.nodes == nil {
				continue
			}
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
				cache.mark(entity, renderNodeDirtyContent2D)
			}
		}
		return active
	}
	return input
}

// Inherited metadata changes invalidate the shared property cache. Geometry
// and paint changes can instead extract only the affected entity set.
var retainedInputs2D = [...]renderInput2D{
	inheritedRetainedInput2D[PainterGroup2D](), inheritedRetainedInput2D[Parent](), inheritedRetainedInput2D[SceneLayer](),
	inheritedRetainedInput2D[RenderLayers](), inheritedRetainedInput2D[UITargetCamera](),
	inheritedRetainedInput2D[UIStackIndex](), localRetainedInput2D[RenderOrder2D](),
	retainedInput2D[Sprite2D](renderNodeDirtyContent2D), retainedInput2D[Text2D](renderNodeDirtyContent2D),
	retainedInput2D[GlobalTransform](renderNodeDirtyGeometry2D), retainedInput2D[UIGlobalTransform](renderNodeDirtyGeometry2D),
	retainedInput2D[Visibility](renderNodeDirtyVisibility2D), retainedInput2D[Anchor](renderNodeDirtyContent2D),
	retainedInput2D[Size2D](renderNodeDirtyContent2D), retainedInput2D[UIBackground2D](renderNodeDirtyContent2D),
	retainedInput2D[UIImage2D](renderNodeDirtyContent2D), retainedInput2D[UIBorder2D](renderNodeDirtyContent2D),
	retainedInput2D[RenderOutput2D](renderNodeDirtyContent2D), retainedInput2D[Shader[graphics.Material2DShader]](renderNodeDirtyContent2D),
	dynamicRetainedInput2D(renderCollectorSpriteTrail2D, func(t *SpriteTrail2D, device *render.Device) bool { return len(t.samples) != 0 }),
	dynamicRetainedInput2D(renderCollectorSpriteLayers2D, func(l *SpriteLayers2D, device *render.Device) bool {
		for i := range l.Layers {
			layer := &l.Layers[i]
			width, height := layer.Sprite.Size()
			if layer.Visible && spriteDrawable2D(&layer.Sprite, device) && width > 0 && height > 0 {
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
const inheritedRenderInputs2D = 6
const dynamicRenderInputs2D = 19

// renderTree2D owns the render topology, snapshots in paint order, and the
// changes consumed by each camera's compositor.
type renderTree2D struct {
	device              *render.Device
	textureRetention    map[*graphics.Texture]reuse.Retention
	nodes               map[ecs.Entity]*retainedEntity2D
	textures            map[*graphics.Texture]*retainedTexture2D
	queryScope          *ecs.QueryScope
	nestedRetention     map[ecs.Entity]reuse.Retention
	changes             ecs.SystemContext
	paints              []paintItem2D
	damageFlags         []bool
	dirty               []ecs.Entity
	freeNodes           []*retainedEntity2D
	partPositions       []uint32
	hierarchyMarks      []uint64
	hierarchyQueue      []ecs.Entity
	damage              []renderDamage2D
	items               []renderItem2D
	nodeBlocks          [][]retainedEntity2D
	scope               ecs.QueryScope
	versions            [retainedRenderInputCount2D]uint64
	structures          [retainedRenderInputCount2D]uint64
	hierarchyRetention  [2]reuse.Retention
	stats               RenderPreparation2D
	textureMapRetention reuse.MapRetention
	nodeMapRetention    reuse.MapRetention
	partRetention       reuse.Retention
	flagRetention       reuse.Retention
	nodeRetention       reuse.Retention
	damageRetention     reuse.Retention
	dirtyRetention      reuse.Retention
	paintRetention      reuse.Retention
	itemRetention       reuse.Retention
	rasterScale         float64
	dirtyNodes          int
	retiredNodes        int
	outputRevision      uint64
	nextID              uint64
	hierarchyGeneration uint64
	registry            uint64
	textureRevision     uint64
	// Generation advances when a new set of changes is published.
	generation        uint64
	routingRevision   uint64
	materialStructure uint64
	scopeVersion      uint64
	height            float32
	width             float32
	lastRun           ecs.Tick
	direct            bool
	missing           bool
	initialized       bool
	damageValid       bool
	directFrames      uint8
}

type renderDamage2D struct {
	previous image.Rectangle
	part     renderPart2D
}

func (cache *renderTree2D) recordDamage(node *retainedEntity2D) {
	if cache.damageValid {
		for position := range node.positions {
			cache.recordItemDamage(position)
		}
	}
}

func (cache *renderTree2D) recordItemDamage(position int) {
	item := &cache.items[position]
	if cache.damageValid && !cache.damageFlags[item.part] {
		cache.damageFlags[item.part] = true
		cache.damage = append(cache.damage, renderDamage2D{part: item.part, previous: item.paint.boundsRect(item)})
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
		}
	}
	node.matrix, node.clip = matrix, clip
	node.dirty = 0
	return true
}

func (cache *renderTree2D) revisions(world *ecs.World) (versions, structures [retainedRenderInputCount2D]uint64) {
	for i, input := range retainedInputs2D {
		versions[i], structures[i] = input.revision(world)
	}
	return
}

func (cache *renderTree2D) extract(c *graphics.Canvas2D, world *ecs.World, state *RenderWorld) []renderItem2D {
	cache.stats = RenderPreparation2D{}
	for _, change := range cache.damage {
		cache.damageFlags[change.part] = false
	}
	cache.damageRetention.Record(len(cache.damage))
	cache.damage, cache.damageValid = cache.damage[:0], false
	cache.trim(world)
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
		if retainedInputs2D[i].dirty != renderNodeDirtyGeometry2D && versions[i] != cache.versions[i] {
			propertiesOnly = false
		}
	}
	nodeCount := len(cache.nodes)
	if cache.direct && cache.nodes == nil {
		nodeCount = cache.retiredNodes
	}
	if cache.initialized && !propertiesOnly && writes > uint64(max(16, nodeCount/4)) {
		// Content that changes densely keeps the sequential extraction path.
		// Do not copy it into per-entity retained storage or build paint hashes.
		return cache.extractDirect(c, world, state)
	}
	if cache.direct && cache.nodes == nil && !full {
		active := 0
		for i, kind := range dynamicCollectors2D {
			if state.collectors2D[kind] != nil {
				active += retainedInputs2D[dynamicRenderInputs2D+i].poll(world, cache)
			}
		}
		if active > max(16, cache.retiredNodes/4) {
			return cache.extractDirect(c, world, state)
		}
		full = true
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
		hierarchyChanged := false
		for i := range retainedInputs2D {
			if cache.versions[i] != versions[i] {
				if i < inheritedRenderInputs2D && !hierarchyChanged {
					cache.beginHierarchyChanges(world)
					hierarchyChanged = true
				}
				retainedInputs2D[i].changed(world, &cache.changes, cache)
			}
		}
		if hierarchyChanged {
			cache.markHierarchyChanges(world)
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
	routingChanged := false
	if !full {
		cache.scope.Reset(cache.dirty)
		previous := world.SetQueryScope(&cache.scope)
		items := collectRenderSnapshots2D(c, world, false, true)
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
			item.part = cache.items[position].part
			if item.renderSortKey() != cache.items[position].renderSortKey() {
				full = true
				break
			}
			routingChanged = routingChanged || !sameRenderCamera2D(&item, &cache.items[position])
			previousTexture := cache.items[position].texture
			cache.items[position] = cache.snapshot(node, position, item, c.Device())
			cache.replaceTexture(position, previousTexture)
			node.count++
		}
		for _, entity := range cache.dirty {
			node := cache.nodes[entity]
			full = full || node.count != node.positionCount()
			node.dirty = 0
			cache.releaseNode(entity, node)
			node.captureProperties(world, entity, cache.items)
			node.captureCamera(cache.items)
		}
		if !full {
			cache.stats.Extracted, cache.stats.Reused = len(items), len(cache.items)-len(items)
		}
		clear(items) // Do not retain borrowed component pointers in frame scratch.
	}
	if full {
		cache.damageValid = false
		previousLen := len(cache.items)
		cache.items = collectRenderSnapshotsInto2D(c, world, cache.items[:0], false, true)
		clearSnapshotTail(cache.items, previousLen)
		items := cache.items
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
			} else {
				cache.retainTexture(texture, dependency)
			}
		}
		cache.dirty = cache.dirty[:0]
		cache.dirtyNodes = 0
		for i, kind := range dynamicCollectors2D {
			if state.collectors2D[kind] != nil {
				retainedInputs2D[dynamicRenderInputs2D+i].poll(world, cache)
			}
		}
		cache.partPositions = reuse.Resize(cache.partPositions, len(items))
		part := renderPart2D(0)
		for entity, node := range cache.nodes {
			node.dirty = 0
			cache.releaseNode(entity, node)
			node.captureProperties(world, entity, cache.items)
			node.captureCamera(cache.items)
			for position := range node.positions {
				cache.items[position].part = part
				cache.partPositions[part] = uint32(position)
				part++
			}
			if node.positionCount() == 0 && node.dynamic == 0 {
				delete(cache.nodes, entity)
				*node = retainedEntity2D{first: -1}
				cache.freeNodes = append(cache.freeNodes, node)
			}
		}
		cache.rebuildLinks(HierarchyFor(world))
		cache.stats.Extracted, cache.stats.Sorted = len(items), true
	}
	cache.capture(c, world, state)
	if full || routingChanged {
		cache.routingRevision++
	}
	if full || cache.stats.Extracted != 0 || cache.stats.PaintUpdated != 0 {
		cache.generation++
	}
	cache.direct = false
	cache.directFrames = 0
	cache.retiredNodes = 0
	return cache.items
}

func (cache *renderTree2D) extractDirect(c *graphics.Canvas2D, world *ecs.World, state *RenderWorld) []renderItem2D {
	items := collectRenderItems2D(c, world, false)
	for _, entity := range cache.dirty {
		if node := cache.nodes[entity]; node != nil {
			node.dirty = 0
		}
	}
	cache.capture(c, world, state)
	cache.generation++
	cache.direct = true
	if cache.directFrames < reuse.RetentionFrames {
		cache.directFrames++
	}
	if cache.directFrames == reuse.RetentionFrames && cache.nodes != nil {
		// Direct extraction owns its frame snapshots. Retire the old retained
		// scene once sustained animation no longer uses it.
		cache.retiredNodes = len(cache.nodes)
		cache.nodes, cache.textures = nil, nil
		cache.nestedRetention, cache.textureRetention = nil, nil
		cache.nodeBlocks, cache.freeNodes = nil, nil
		cache.items, cache.paints, cache.partPositions, cache.damageFlags = nil, nil, nil, nil
		cache.dirty, cache.damage = nil, nil
		cache.scope.Reset(nil)
	}
	cache.stats = RenderPreparation2D{Extracted: len(items), Sorted: true}
	return items
}

// Observe once per extraction, including clean frames where no collector runs.
func (cache *renderTree2D) trim(world *ecs.World) {
	snapshots := world.FrameValue[renderSnapshots2DScratch, renderSnapshots2D]()
	if world.FrameValue[renderSnapshots2DScratch, reuse.Retention]().TrimCapacity(snapshots.capacity(), 0, 64) != 0 {
		// These are collector intermediates. Retained owners have copied their
		// contents, and direct items keep their old backing arrays alive until
		// the next extraction replaces them.
		*snapshots = renderSnapshots2D{}
	}
	world.FrameValue[spineRenderBatch2DScratch, spineRenderScratch2D]().trim()
	cache.trimNested()
	cache.hierarchyQueue = cache.hierarchyRetention[0].Trim(cache.hierarchyQueue[:0], 64)
	cache.hierarchyMarks = cache.hierarchyRetention[1].Trim(cache.hierarchyMarks[:0], 64)
	cache.nodes = cache.nodeMapRetention.Trim(cache.nodes, 64)
	cache.textures = cache.textureMapRetention.Trim(cache.textures, 16)
	cache.items = cache.itemRetention.Trim(cache.items, 64)
	cache.partPositions = cache.partRetention.Trim(cache.partPositions, 64)
	previous := cap(cache.paints)
	cache.paints = cache.paintRetention.Trim(cache.paints, 64)
	if cap(cache.paints) != previous {
		for i := range cache.items {
			cache.items[i].paint = &cache.paints[i]
		}
	}
	cache.damageFlags = cache.flagRetention.Trim(cache.damageFlags, 64)
	cache.damage = cache.damageRetention.Trim(cache.damage, 16)
	cache.dirtyRetention.Record(len(cache.dirty))
	previous = cap(cache.dirty)
	cache.dirty = cache.dirtyRetention.Trim(cache.dirty[:0], 16)
	if cap(cache.dirty) != previous {
		cache.scope.Reset(nil)
	}
	target := cache.nodeRetention.TrimCapacity(len(cache.nodeBlocks)*64, len(cache.nodes), 64)
	if target == 0 {
		return
	}
	cache.compactNodes(target)
}

func (cache *renderTree2D) capture(c *graphics.Canvas2D, world *ecs.World, state *RenderWorld) {
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
	cache.nodeMapRetention.Record(len(cache.nodes))
	cache.textureMapRetention.Record(len(cache.textures))
	cache.lastRun, cache.initialized = world.ChangeTick(), true
}
