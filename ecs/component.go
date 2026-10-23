package ecs

import (
	"reflect"
	"slices"
)

type Tick uint32

const maxChangeAge = ^uint32(0) - (2*518_400_000 - 1)

func tickInRange(value, lastRun, thisRun Tick) bool {
	if value == 0 {
		return false
	}
	ticksSinceChange := min(uint32(thisRun-value), maxChangeAge)
	ticksSinceSystem := min(uint32(thisRun-lastRun), maxChangeAge)
	return ticksSinceSystem > ticksSinceChange
}

type componentID uint32

type componentInfo struct {
	typ       reflect.Type
	newColumn func() column
	onRemove  func(*World, Entity, column, int)
	version   uint64
	structure uint64
}

type column interface {
	reserve(int)
	moveTo(column, int)
	swapRemove(int)
	ticks(filterKind) []Tick
	ranges(filterKind) []tickRange
	markChanged(int, Tick)
}

type typedColumn[T any] struct {
	columnState
	values []T
}

// columnState keeps change tracking independent of the component value type.
// Query access can hold this metadata and an already resolved value pointer.
type columnState struct {
	world         *World
	id            componentID
	added         []Tick
	changed       []Tick
	addedRanges   []tickRange
	changedRanges []tickRange
}

const queryTickBlockSize = 64

// Bounds may retain removed ticks, which only causes conservative extra scans.
type tickRange struct{ min, max Tick }

func (bounds *tickRange) include(tick Tick) {
	if tick == 0 {
		return
	}
	if bounds.min == 0 || tick < bounds.min {
		bounds.min = tick
	}
	if tick > bounds.max {
		bounds.max = tick
	}
}

func (bounds tickRange) matches(filter rowFilter) bool {
	window := min(uint32(filter.thisRun-filter.lastRun), maxChangeAge)
	if window == 0 || bounds.max == 0 {
		return false
	}
	first := filter.thisRun - Tick(window-1)
	if first <= filter.thisRun {
		return bounds.max >= first && bounds.min <= filter.thisRun
	}
	return bounds.max >= first || bounds.min <= filter.thisRun
}

func (c *columnState) touch() {
	c.world.componentInfos[c.id].version++
	c.world.mutationVersion++
}

func (c *typedColumn[T]) reserve(additional int) {
	if additional <= 0 {
		return
	}
	c.values = slices.Grow(c.values, additional)
	c.added = slices.Grow(c.added, additional)
	c.changed = slices.Grow(c.changed, additional)
	blocks := (len(c.values) + additional + queryTickBlockSize - 1) / queryTickBlockSize
	c.addedRanges = slices.Grow(c.addedRanges, max(0, blocks-len(c.addedRanges)))
	c.changedRanges = slices.Grow(c.changedRanges, max(0, blocks-len(c.changedRanges)))
}

func (c *typedColumn[T]) append(value T, tick Tick) {
	c.appendWithTicks(value, tick, tick)
}

func (c *typedColumn[T]) appendWithTicks(value T, added, changed Tick) {
	c.values = append(c.values, value)
	c.added = append(c.added, added)
	c.changed = append(c.changed, changed)
	block := (len(c.values) - 1) / queryTickBlockSize
	if block == len(c.addedRanges) {
		c.addedRanges = append(c.addedRanges, tickRange{})
		c.changedRanges = append(c.changedRanges, tickRange{})
	}
	c.addedRanges[block].include(added)
	c.changedRanges[block].include(changed)
	c.touch()
	c.world.componentInfos[c.id].structure++
}

func (c *typedColumn[T]) replace(row int, value T, tick Tick) {
	c.values[row] = value
	c.markChanged(row, tick)
}

// moveTo transfers one row into target and drops it from the source. Change
// ticks are preserved; both tables record the migration.
func (c *typedColumn[T]) moveTo(target column, row int) {
	dst := target.(*typedColumn[T])
	dst.appendWithTicks(c.values[row], c.added[row], c.changed[row])
	c.swapRemove(row)
}

func (c *typedColumn[T]) swapRemove(row int) {
	last := len(c.values) - 1
	if row != last {
		c.values[row] = c.values[last]
		c.added[row] = c.added[last]
		c.changed[row] = c.changed[last]
		c.addedRanges[row/queryTickBlockSize].include(c.added[row])
		c.changedRanges[row/queryTickBlockSize].include(c.changed[row])
	}
	var zero T
	c.values[last] = zero
	c.values = c.values[:last]
	c.added = c.added[:last]
	c.changed = c.changed[:last]
	blocks := (last + queryTickBlockSize - 1) / queryTickBlockSize
	c.addedRanges = c.addedRanges[:blocks]
	c.changedRanges = c.changedRanges[:blocks]
	c.touch()
	c.world.componentInfos[c.id].structure++
}

func (c *columnState) ticks(kind filterKind) []Tick {
	if kind == filterAdded {
		return c.added
	}
	return c.changed
}

func (c *columnState) markChanged(row int, tick Tick) {
	c.changed[row] = tick
	c.changedRanges[row/queryTickBlockSize].include(tick)
	c.touch()
}

func (c *columnState) ranges(kind filterKind) []tickRange {
	if kind == filterAdded {
		return c.addedRanges
	}
	return c.changedRanges
}

func registerComponent[T any](world *World) componentID {
	if world == nil {
		panic("ecs: world is nil")
	}
	typ := reflect.TypeFor[T]()
	if id, ok := world.componentTypes[typ]; ok {
		return id
	}
	id := componentID(len(world.componentInfos))
	world.componentTypes[typ] = id
	world.componentInfos = append(world.componentInfos, componentInfo{
		typ: typ,
		newColumn: func() column {
			return &typedColumn[T]{columnState: columnState{world: world, id: id}}
		},
	})
	return id
}

// ComponentRevision reads the content and structural versions in one lookup.
// Missing types return zero without registering storage or advancing ticks.
func ComponentRevision[T any](world *World) (version, structure uint64) {
	requireWorld(world)
	if id, ok := world.componentTypes[reflect.TypeFor[T]()]; ok {
		info := &world.componentInfos[id]
		return info.version, info.structure
	}
	return 0, 0
}

// ComponentVersion changes for writes, insertion, removal and row migration.
func ComponentVersion[T any](world *World) uint64 {
	version, _ := ComponentRevision[T](world)
	return version
}

// ComponentStructureVersion changes when rows are inserted or removed.
func ComponentStructureVersion[T any](world *World) uint64 {
	_, structure := ComponentRevision[T](world)
	return structure
}

func componentColumn[T any](archetype *archetype, id componentID) *typedColumn[T] {
	return archetype.column(id).(*typedColumn[T])
}

// OnComponentRemove installs a cleanup hook for removal and despawn. Hooks
// must not change entity structure; archetype migration does not invoke them.
func OnComponentRemove[T any](world *World, cleanup func(*World, Entity, *T)) {
	id := registerComponent[T](world)
	if cleanup == nil {
		world.componentInfos[id].onRemove = nil
		return
	}
	world.componentInfos[id].onRemove = func(world *World, entity Entity, storage column, row int) {
		cleanup(world, entity, &storage.(*typedColumn[T]).values[row])
	}
	for _, archetype := range world.archetypes {
		if archetype.has(id) {
			archetype.hasCleanup = true
		}
	}
}

func (w *World) cleanupComponent(id componentID, entity Entity, storage column, row int) {
	cleanup := w.componentInfos[id].onRemove
	if cleanup == nil {
		return
	}
	w.activeComponentCleanups++
	defer func() { w.activeComponentCleanups-- }()
	cleanup(w, entity, storage, row)
}
