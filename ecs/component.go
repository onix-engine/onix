package ecs

import (
	"reflect"
	"slices"
)

type Tick uint32

const maxChangeAge = ^uint32(0) - (2*518_400_000 - 1)

type tickWindow struct {
	thisRun Tick
	age     uint32
}

func newTickWindow(lastRun, thisRun Tick) tickWindow {
	return tickWindow{thisRun: thisRun, age: min(uint32(thisRun-lastRun), maxChangeAge)}
}

func (w tickWindow) contains(value Tick) bool {
	return value != 0 && uint32(w.thisRun-value) < w.age
}

func tickInRange(value, lastRun, thisRun Tick) bool {
	return newTickWindow(lastRun, thisRun).contains(value)
}

type componentID uint32

type componentInfo struct {
	typ       reflect.Type
	newColumn func(*World, componentID) *columnState
	onRemove  func(*World, Entity, *columnState, int)
	version   uint64
	structure uint64
	values    uint64
	members   uint64
	detached  uint64
}

// columnValues only dispatches operations that copy or allocate values of T.
type columnValues interface {
	reserveValues(int)
	compactValues(int)
	removeValue(row int, destination columnValues)
}

// Keep state named so its methods do not enter every component type's method set.
type typedColumn[T any] struct {
	values []T
	state  columnState
}

// columnState keeps change tracking independent of the component value type.
// Query access can hold this metadata and an already resolved value pointer.
type columnState struct {
	storage       columnValues
	world         *World
	added         []Tick
	changed       []Tick
	addedRanges   []tickRange
	changedRanges []tickRange
	id            componentID
}

const queryTickBlockSize = 64

const queryTickGroupBlocks = 64
const queryTickGroupSize = queryTickBlockSize * queryTickGroupBlocks

// Blocks grow from the front and group caches from the back of the same slice.
// This preserves direct block indexing on writes without extra slice headers.
func tickRangeCount(rows int) int {
	blocks := (rows + queryTickBlockSize - 1) / queryTickBlockSize
	return blocks + (blocks+queryTickGroupBlocks-1)/queryTickGroupBlocks
}

func growTickRanges(ranges []tickRange, rows, capacity int) []tickRange {
	needed := tickRangeCount(capacity)
	if needed <= len(ranges) {
		return ranges
	}
	ranges = slices.Grow(ranges, needed-len(ranges))
	ranges = ranges[:cap(ranges)]
	// Moving the group cache to the new tail invalidates it. Only leaf block
	// bounds are persistent; the gap and all group slots start empty.
	clear(ranges[(rows+queryTickBlockSize-1)/queryTickBlockSize:])
	return ranges
}

func includeTick(ranges []tickRange, row int, tick Tick) {
	// Component insertion and mutation clocks never produce the zero sentinel.
	ranges[row/queryTickBlockSize].includeNonzero(tick)
	// Zero invalidates the group cache. Rebuild only when a query needs it,
	// instead of updating a second min/max pair on every component write.
	ranges[len(ranges)-1-row/queryTickGroupSize].max = 0
}

func tickGroupRange(ranges []tickRange, group, rows int) tickRange {
	index := len(ranges) - 1 - group
	if bounds := ranges[index]; bounds.max != 0 {
		return bounds
	}
	var bounds tickRange
	start := group * queryTickGroupBlocks
	end := min(start+queryTickGroupBlocks, (rows+queryTickBlockSize-1)/queryTickBlockSize)
	for _, block := range ranges[start:end] {
		bounds.invertedMin = max(bounds.invertedMin, block.invertedMin)
		bounds.max = max(bounds.max, block.max)
	}
	ranges[index] = bounds
	return bounds
}

// Inverting the lower bound lets both bounds accumulate with max, starting at
// zero without a special case for the first tick. Removed ticks are retained
// conservatively and may cause extra scans.
type tickRange struct{ invertedMin, max Tick }

func (bounds *tickRange) include(tick Tick) {
	if tick == 0 {
		return
	}
	bounds.includeNonzero(tick)
}

func (bounds *tickRange) includeNonzero(tick Tick) {
	bounds.invertedMin = max(bounds.invertedMin, ^tick)
	bounds.max = max(bounds.max, tick)
}

func (bounds tickRange) matches(window tickWindow) bool {
	if window.age == 0 || bounds.max == 0 {
		return false
	}
	first := window.thisRun - Tick(window.age-1)
	if first <= window.thisRun {
		return bounds.max >= first && ^bounds.invertedMin <= window.thisRun
	}
	return bounds.max >= first || ^bounds.invertedMin <= window.thisRun
}

func (c *columnState) touch() {
	c.world.componentInfos[c.id].version++
	c.world.mutationVersion++
}

func (c *typedColumn[T]) reserveValues(additional int) {
	c.values = slices.Grow(c.values, additional)
}

func (c *columnState) reserve(additional int) {
	if additional <= 0 {
		return
	}
	c.storage.reserveValues(additional)
	c.added = slices.Grow(c.added, additional)
	c.changed = slices.Grow(c.changed, additional)
	c.addedRanges = growTickRanges(c.addedRanges, len(c.added), len(c.added)+additional)
	c.changedRanges = growTickRanges(c.changedRanges, len(c.changed), len(c.changed)+additional)
}

func (c *typedColumn[T]) append(value T, tick Tick) {
	c.appendWithTicks(value, tick, tick)
	c.state.world.componentInfos[c.state.id].values++
	c.state.world.componentInfos[c.state.id].members++
}

func (c *typedColumn[T]) appendWithTicks(value T, added, changed Tick) {
	c.values = append(c.values, value)
	c.state.appendTicks(added, changed)
}

func (c *columnState) appendTicks(added, changed Tick) {
	c.added = append(c.added, added)
	c.changed = append(c.changed, changed)
	row := len(c.added) - 1
	if tickRangeCount(len(c.added)) > len(c.addedRanges) {
		c.addedRanges = growTickRanges(c.addedRanges, row, len(c.added))
		c.changedRanges = growTickRanges(c.changedRanges, row, len(c.added))
	}
	if row%queryTickBlockSize == 0 {
		c.addedRanges[row/queryTickBlockSize] = tickRange{}
		c.changedRanges[row/queryTickBlockSize] = tickRange{}
	}
	includeTick(c.addedRanges, row, added)
	includeTick(c.changedRanges, row, changed)
	c.touch()
	c.world.componentInfos[c.id].structure++
}

func (c *typedColumn[T]) replace(row int, value T, tick Tick) {
	c.values[row] = value
	c.state.markChanged(row, tick)
}

// moveTo transfers one row into target and drops it from the source. Change
// ticks are preserved; both tables record the migration.
func (c *columnState) moveTo(target *columnState, row int) {
	c.storage.removeValue(row, target.storage)
	target.appendTicks(c.added[row], c.changed[row])
	c.removeTicks(row)
}

func (c *columnState) swapRemove(row int) {
	c.removeRow(row)
	c.world.componentInfos[c.id].values++
	c.world.componentInfos[c.id].members++
}

func (c *columnState) removeRow(row int) {
	c.storage.removeValue(row, nil)
	c.removeTicks(row)
}

// Migrations keep the removed value in a destination column. Despawns and
// component removals pass nil. Both paths swap out and clear the source row.
func (c *typedColumn[T]) removeValue(row int, destination columnValues) {
	if destination != nil {
		dst := destination.(*typedColumn[T])
		dst.values = append(dst.values, c.values[row])
	}
	last := len(c.values) - 1
	if row != last {
		c.values[row] = c.values[last]
	}
	var zero T
	c.values[last] = zero
	c.values = c.values[:last]
}

func (c *columnState) removeTicks(row int) {
	last := len(c.added) - 1
	if row != last {
		c.added[row] = c.added[last]
		c.changed[row] = c.changed[last]
		includeTick(c.addedRanges, row, c.added[row])
		includeTick(c.changedRanges, row, c.changed[row])
	}
	c.added = c.added[:last]
	c.changed = c.changed[:last]
	c.touch()
	c.world.componentInfos[c.id].structure++
}

func (c *columnState) ticks(kind filterKind) []Tick {
	if kind == filterAdded {
		return c.added
	}
	return c.changed
}

// Queries reuse their iteration tick. Direct writes pass zero for a fresh tick.
func (c *columnState) markChanged(row int, tick Tick) {
	if tick == 0 {
		tick = c.world.mutationTick()
	}
	c.changed[row] = tick
	includeTick(c.changedRanges, row, tick)
	c.touch()
	c.world.componentInfos[c.id].values++
}

func (c *columnState) ranges(kind filterKind) []tickRange {
	if kind == filterAdded {
		return c.addedRanges
	}
	return c.changedRanges
}

func (world *World) registerComponent[T any]() componentID {
	id := world.identifyComponent(reflect.TypeFor[T]())
	if world.componentInfos[id].newColumn == nil {
		world.componentInfos[id].newColumn = newTypedColumn[T]
	}
	return id
}

func newTypedColumn[T any](world *World, id componentID) *columnState {
	c := &typedColumn[T]{state: columnState{world: world, id: id}}
	// A table keeps the header, while typed queries borrow values from the same allocation.
	c.state.storage = c
	return &c.state
}

// Queries can obtain an ID before a writer installs the column factory.
// Share the registry lookup and growth code across every component type.
//
//go:noinline
func (world *World) identifyComponent(typ reflect.Type) componentID {
	requireWorld(world)
	if id, ok := world.componentTypes[typ]; ok {
		return id
	}
	id := componentID(len(world.componentInfos))
	world.componentTypes[typ] = id
	world.componentInfos = append(world.componentInfos, componentInfo{typ: typ})
	return id
}

// ComponentRevision reads the content and structural versions in one lookup.
// Missing types return zero without registering storage or advancing ticks.
func (world *World) ComponentRevision[T any]() (version, structure uint64) {
	requireWorld(world)
	if id, ok := world.componentTypes[reflect.TypeFor[T]()]; ok {
		info := &world.componentInfos[id]
		return info.version, info.structure
	}
	return 0, 0
}

// ComponentValueRevision reports writes and membership changes, excluding
// migrations that only relocate unchanged values between archetypes.
// The second result changes only when values are inserted or removed.
func (world *World) ComponentValueRevision[T any]() (version, membership uint64) {
	return Type[T]().ValueRevision(world)
}

// ValueRevision reads the value and membership revisions for this component
// type in world. Reading an unregistered type returns zero for both.
func (component ComponentType) ValueRevision(world *World) (version, membership uint64) {
	requireWorld(world)
	if id, ok := world.componentTypes[component.typ]; ok {
		info := &world.componentInfos[id]
		return info.values, info.members
	}
	return 0, 0
}

// ComponentDetachRevision changes when a component is removed from a live
// entity. Despawns can be handled separately through EntityChangesSince.
func (world *World) ComponentDetachRevision[T any]() uint64 {
	requireWorld(world)
	if id, ok := world.componentTypes[reflect.TypeFor[T]()]; ok {
		return world.componentInfos[id].detached
	}
	return 0
}

// ComponentVersion changes for writes, insertion, removal and row migration.
func (world *World) ComponentVersion[T any]() uint64 {
	version, _ := world.ComponentRevision[T]()
	return version
}

// ComponentStructureVersion changes when rows are inserted or removed.
func (world *World) ComponentStructureVersion[T any]() uint64 {
	_, structure := world.ComponentRevision[T]()
	return structure
}

func (archetype *archetype) componentColumn[T any](id componentID) *typedColumn[T] {
	return archetype.column(id).typed[T]()
}

// Keep the checked conversion in one place per type across queries, inserts
// and direct accesses. A missing column is a normal lookup result.
//
//go:noinline
func (c *columnState) typed[T any]() *typedColumn[T] {
	if c == nil {
		return nil
	}
	return c.storage.(*typedColumn[T])
}

// OnComponentRemove installs a cleanup hook for removal and despawn. Hooks
// must not change entity structure; archetype migration does not invoke them.
func (world *World) OnComponentRemove[T any](cleanup func(*World, Entity, *T)) {
	id := world.identifyComponent(reflect.TypeFor[T]())
	if cleanup == nil {
		world.componentInfos[id].onRemove = nil
		return
	}
	world.componentInfos[id].onRemove = func(world *World, entity Entity, storage *columnState, row int) {
		cleanup(world, entity, &storage.typed[T]().values[row])
	}
	for _, archetype := range world.archetypes {
		if archetype.has(id) {
			archetype.hasCleanup = true
		}
	}
}

func (w *World) cleanupComponent(id componentID, entity Entity, storage *columnState, row int) {
	cleanup := w.componentInfos[id].onRemove
	if cleanup == nil {
		return
	}
	w.activeComponentCleanups++
	defer func() { w.activeComponentCleanups-- }()
	cleanup(w, entity, storage, row)
}
