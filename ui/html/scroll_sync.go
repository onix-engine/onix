package html

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

// Group retained consumers by every scroll container they depend on. Scopes
// keep independent indexes; unchanged groups never traverse their consumers.
type htmlScrollIndex[T any, S comparable] struct {
	groups       map[ecs.Entity]*htmlScrollGroup[S]
	seen         map[*htmlStickyBinding]bool
	context      ecs.SystemContext
	entries      []htmlScrollEntry[T]
	pending      []ecs.Entity
	versions     [7]uint64
	outputs      [5]uint64
	scopeVersion uint64
	revision     uint64
	generation   uint64
	lastRun      ecs.Tick
	initialized  bool
}

type htmlScrollEntry[T any] struct {
	value   T
	entity  ecs.Entity
	applied uint64
}

type htmlScrollGroup[S comparable] struct {
	snapshot S
	indices  []int
	dirty    bool
}

func (index *htmlScrollIndex[T, S]) rebuild(world *ecs.World, versions [7]uint64, revision uint64) bool {
	_, scopeVersion := world.QueryScopeKey()
	if index.initialized && index.versions == versions && index.scopeVersion == scopeVersion && index.revision == revision {
		return false
	}
	index.versions, index.scopeVersion, index.revision, index.initialized = versions, scopeVersion, revision, true
	clear(index.entries)
	index.entries = index.entries[:0]
	if index.groups == nil {
		index.groups = make(map[ecs.Entity]*htmlScrollGroup[S])
	}
	for _, group := range index.groups {
		group.indices, group.dirty = group.indices[:0], true
	}
	return true
}

func (index *htmlScrollIndex[T, S]) add(area ecs.Entity, entry int) {
	group := index.groups[area]
	if group == nil {
		group = &htmlScrollGroup[S]{dirty: true}
		index.groups[area] = group
	}
	// One consumer may reach a container through several sticky ancestors.
	if len(group.indices) == 0 || group.indices[len(group.indices)-1] != entry {
		group.indices = append(group.indices, entry)
	}
}

func (index *htmlScrollIndex[T, S]) trim() {
	for area, group := range index.groups {
		if len(group.indices) == 0 {
			delete(index.groups, area)
		}
	}
}

// Reconciliation and embedding code can rewrite geometry without changing
// scroll inputs. Replay just the containers with externally written outputs.
func (index *htmlScrollIndex[T, S]) dirtyOutput[Output any](world *ecs.World, version uint64, slot int, areaOf func(T) ecs.Entity) {
	if index.outputs[slot] == version {
		return
	}
	for _, access := range world.Query2[T, Output](index.context.Changed[Output]()).All {
		if group := index.groups[areaOf(access.A.Get())]; group != nil {
			group.dirty = true
		}
	}
}

func htmlScrollContentArea(content ScrollContent) ecs.Entity { return content.Area }
func htmlScrollbarArea(part ScrollbarPart) ecs.Entity        { return part.Area }

func htmlScrollbarOutputVersions(world *ecs.World) [5]uint64 {
	transforms, _ := world.ComponentValueRevision[scene.UITransform]()
	sizes, _ := world.ComponentValueRevision[scene.Size2D]()
	sprites, _ := world.ComponentValueRevision[scene.Sprite2D]()
	clips, _ := world.ComponentValueRevision[scene.ClipRect2D]()
	rounded, _ := world.ComponentValueRevision[scene.RoundedClip2D]()
	return [5]uint64{transforms, sizes, sprites, clips, rounded}
}

type htmlScrollContentTag struct{}
type htmlScrollbarSyncTag struct{}
type htmlStickyWorkTag struct{}

type htmlStickyWork struct{ revision, generation uint64 }

type htmlScrollPosition struct {
	position, contentSize mgl32.Vec2
	exists                bool
}

func (index *htmlScrollIndex[T, S]) addSticky(binding *htmlStickyBinding, entry int) {
	if binding == nil || index.seen[binding] {
		return
	}
	if index.seen == nil {
		index.seen = make(map[*htmlStickyBinding]bool)
	}
	index.seen[binding] = true
	index.add(binding.area, entry)
	index.addSticky(binding.parent, entry)
	index.addSticky(binding.scrollportSticky, entry)
}

type htmlScrollbarSnapshot struct {
	scroll  scene.UIScroll
	size    scene.Size2D
	clip    scene.ClipRect2D
	rounded scene.RoundedClip2D
	active  [2]bool
	exists  bool
}

func changedHTMLScrollbars(world *ecs.World) *htmlScrollIndex[ScrollbarPart, htmlScrollbarSnapshot] {
	index := world.ScopeValue[htmlScrollbarSyncTag, htmlScrollIndex[ScrollbarPart, htmlScrollbarSnapshot]]()
	parts, _ := world.ComponentValueRevision[ScrollbarPart]()
	_, transforms := world.ComponentValueRevision[scene.UITransform]()
	_, sizes := world.ComponentValueRevision[scene.Size2D]()
	_, sprites := world.ComponentValueRevision[scene.Sprite2D]()
	exemptions, _ := world.ComponentValueRevision[scene.ClipExempt2D]()
	_, clips := world.ComponentValueRevision[scene.ClipRect2D]()
	_, rounded := world.ComponentValueRevision[scene.RoundedClip2D]()
	if index.rebuild(world, [7]uint64{parts, transforms, sizes, sprites, exemptions, clips, rounded}, 0) {
		for entity, access := range world.Query4[ScrollbarPart, scene.UITransform, scene.Size2D, scene.Sprite2D]().All {
			part := access.A.Get()
			index.add(part.Area, len(index.entries))
			index.entries = append(index.entries, htmlScrollEntry[ScrollbarPart]{entity: entity, value: part})
		}
		index.trim()
	} else {
		// Include the previous tick: another synchronizer may write after this
		// one within the same enclosing system invocation.
		index.context = world.ChangesSince(index.lastRun - 1)
		outputs := htmlScrollbarOutputVersions(world)
		index.dirtyOutput[scene.UITransform](world, outputs[0], 0, htmlScrollbarArea)
		index.dirtyOutput[scene.Size2D](world, outputs[1], 1, htmlScrollbarArea)
		index.dirtyOutput[scene.Sprite2D](world, outputs[2], 2, htmlScrollbarArea)
		index.dirtyOutput[scene.ClipRect2D](world, outputs[3], 3, htmlScrollbarArea)
		index.dirtyOutput[scene.RoundedClip2D](world, outputs[4], 4, htmlScrollbarArea)
	}
	index.pending = index.pending[:0]
	for area, group := range index.groups {
		snapshot := htmlScrollbarSnapshot{}
		snapshot.scroll, snapshot.exists = world.Get[scene.UIScroll](area)
		snapshot.size, _ = world.Get[scene.Size2D](area)
		snapshot.clip, _ = world.Get[scene.ClipRect2D](area)
		snapshot.rounded, _ = world.Get[scene.RoundedClip2D](area)
		for axis := range uint8(2) {
			snapshot.active[axis] = scene.UIScrollScrollbarActive(world, area, axis)
		}
		if !group.dirty && group.snapshot == snapshot {
			continue
		}
		group.snapshot, group.dirty = snapshot, false
		for _, entry := range group.indices {
			index.pending = append(index.pending, index.entries[entry].entity)
		}
	}
	return index
}
