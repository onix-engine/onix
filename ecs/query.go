package ecs

//go:generate go run ../internal/ecsgen

import (
	"math/bits"
	"reflect"
	"slices"
)

type filterKind uint8

const (
	filterAdded filterKind = iota
	filterChanged
	filterWith
	filterWithout
	filterUnscoped
)

type rowFilter struct {
	component componentID
	kind      filterKind
	window    tickWindow
}

type querySpec struct {
	required componentMask
	without  componentMask
}

const inlineQueryFilterCount = 8

type queryFilters struct {
	extra  []rowFilter
	count  int
	inline [inlineQueryFilterCount]rowFilter
}

func (filters *queryFilters) append(filter rowFilter) {
	if filters.count < len(filters.inline) {
		filters.inline[filters.count] = filter
	} else {
		filters.extra = append(filters.extra, filter)
	}
	filters.count++
}

func (filters *queryFilters) at(index int) rowFilter {
	if index < len(filters.inline) {
		return filters.inline[index]
	}
	return filters.extra[index-len(filters.inline)]
}

type queryPlan struct {
	nextHash *queryPlan // next plan with the same mask hash
	spec     querySpec
	blocks   []queryArchetypeBlock
	scanned  int
}

// Store only words containing matching tables. Dense matches use one bit per
// table; sparse matches skip gaps instead of scanning the world's whole mask.
type queryArchetypeBlock struct {
	mask  uint64
	index uint32
}

type simpleQueryKey struct {
	count uint8
	ids   [7]componentID
}

func (p *queryPlan) update(world *World) {
	for _, archetype := range world.archetypes[p.scanned:] {
		if archetype.mask.contains(p.spec.required) && !archetype.mask.intersects(p.spec.without) {
			word, bit := archetype.id/64, uint64(1)<<(archetype.id%64)
			if last := len(p.blocks) - 1; last >= 0 && p.blocks[last].index == word {
				p.blocks[last].mask |= bit
			} else {
				p.blocks = append(p.blocks, queryArchetypeBlock{index: word, mask: bit})
			}
		}
	}
	p.scanned = len(world.archetypes)
}

type QueryOption interface {
	resolve(*World) rowFilter
}

type withOption[T any] struct{}
type withoutOption[T any] struct{}
type addedOption[T any] struct{ context *SystemContext }
type changedOption[T any] struct{ context *SystemContext }
type unscopedOption struct{}

func (unscopedOption) resolve(*World) rowFilter { return rowFilter{kind: filterUnscoped} }

// Unscoped visits matching tables independently of the current execution scope.
// It is intended for world-owned indexes that also include suspended scenes.
func Unscoped() QueryOption { return unscopedOption{} }

func (withOption[T]) resolve(world *World) rowFilter {
	return rowFilter{component: world.identifyComponent(reflect.TypeFor[T]()), kind: filterWith}
}

func (withoutOption[T]) resolve(world *World) rowFilter {
	return rowFilter{component: world.identifyComponent(reflect.TypeFor[T]()), kind: filterWithout}
}

func (option addedOption[T]) resolve(world *World) rowFilter {
	return world.resolveChangeFilter(reflect.TypeFor[T](), option.context, filterAdded)
}

func (option changedOption[T]) resolve(world *World) rowFilter {
	return world.resolveChangeFilter(reflect.TypeFor[T](), option.context, filterChanged)
}

func With[T any]() QueryOption {
	return withOption[T]{}
}

func Without[T any]() QueryOption {
	return withoutOption[T]{}
}

func (context *SystemContext) Added[T any]() QueryOption {
	return addedOption[T]{context: context}
}

func (context *SystemContext) Changed[T any]() QueryOption {
	return changedOption[T]{context: context}
}

func (world *World) resolveChangeFilter(typ reflect.Type, context *SystemContext, kind filterKind) rowFilter {
	if context == nil {
		panic("ecs: system context is nil")
	}
	if context.World != world {
		panic("ecs: change filter context belongs to a different world")
	}
	return rowFilter{component: world.identifyComponent(typ), kind: kind, window: newTickWindow(context.lastRun, context.thisRun)}
}

// ChangedEntityQuery visits entity IDs whose component was written during a
// system's change window. It shares the table and tick indexes of typed queries.
type ChangedEntityQuery struct {
	view queryView
}

// ChangedEntities snapshots the current change window for one component type.
func (context *SystemContext) ChangedEntities(component ComponentType) (result ChangedEntityQuery) {
	if component.typ == nil {
		return result
	}
	world := context.World
	filter := world.resolveChangeFilter(component.typ, context, filterChanged)
	result.view.init(world, []componentID{filter.component}, nil)
	result.view.filters.append(filter)
	return result
}

// Unscoped includes matching entities outside the current execution scope.
func (q ChangedEntityQuery) Unscoped() ChangedEntityQuery {
	q.view.unscoped = true
	return q
}

// All visits matching entity IDs in the same order as a typed component query.
func (q ChangedEntityQuery) All(yield func(Entity) bool) {
	cursor := queryCursor{view: q.view}
	if cursor.open(yield != nil) == 0 {
		return
	}
	defer cursor.close()
	for start, end := cursor.nextRows(0); start < end; start, end = cursor.nextRows(end) {
		filter := cursor.inline[0]
		entities := cursor.archetype.entities
		for row := start; row < end; row++ {
			if filter.matches(row) && !yield(entities[row]) {
				return
			}
		}
	}
}

type queryView struct {
	world    *World
	plan     *queryPlan
	entities []Entity
	filters  queryFilters
	limited  bool
	unscoped bool
}

// filterBinding resolves one change filter against one bound table: its tick
// column, its block bounds and the read window the filter was created with.
// All three are constant for the whole traversal of that table.
type filterBinding struct {
	ticks  []Tick
	ranges []tickRange
	window tickWindow
}

// queryCursor shares query lifetime, table binding, tick filters and restricted
// entity traversal. Typed adapters traverse ordinary table rows directly.
type queryCursor struct {
	archetype      *archetype
	extra          *[]filterBinding
	scopeTable     *scopeTable
	scope          *QueryScope
	inline         [inlineQueryFilterCount]filterBinding
	view           queryView
	entityIndex    int
	row            int
	scopeBlock     int
	archetypeBits  uint64
	archetypeIndex int
	archetypeBase  uint32
}

// open initializes the cursor in place. Zero means there is no iteration;
// otherwise all mutable accesses in the iteration share the returned tick.
func (cursor *queryCursor) open(callback bool) Tick {
	v := &cursor.view
	if !callback || v.world == nil || v.plan == nil {
		return 0
	}
	v.plan.update(v.world)
	v.world.activeQueryIterators++
	cursor.archetypeIndex = -1
	if !v.limited && !v.unscoped && v.world.queryScope != nil {
		cursor.scope = v.world.queryScope
		cursor.scope.bind(v.world)
	}
	if count := v.filters.count; count > len(cursor.inline) {
		cursor.extra = queryBindingPool.get(count - len(cursor.inline))
	}
	return v.world.mutationTick()
}

// binding returns the resolved storage of one change filter.
func (cursor *queryCursor) binding(index int) *filterBinding {
	if index < len(cursor.inline) {
		return &cursor.inline[index]
	}
	return &(*cursor.extra)[index-len(cursor.inline)]
}

// nextEntity visits a caller-owned entity list in its supplied order. Validate
// generations on every pass because a saved list may contain retired entities.
func (cursor *queryCursor) nextEntity() bool {
	for cursor.entityIndex < len(cursor.view.entities) {
		entity := cursor.view.entities[cursor.entityIndex]
		cursor.entityIndex++
		_, meta, alive := cursor.view.world.entityMeta(entity)
		if !alive {
			continue
		}
		archetype := cursor.view.world.archetypes[meta.archetypeID]
		if archetype != cursor.archetype {
			if !archetype.mask.contains(cursor.view.plan.spec.required) || archetype.mask.intersects(cursor.view.plan.spec.without) {
				continue
			}
			cursor.bindArchetype(archetype)
		}
		cursor.row = int(meta.row)
		return true
	}
	return false
}

// nextArchetype binds one matching nonempty table. Typed adapters traverse its contiguous
// rows directly, keeping cursor calls and column binding out of the hot loop.
func (cursor *queryCursor) nextArchetype() bool {
	for {
		for cursor.archetypeBits == 0 {
			cursor.archetypeIndex++
			if cursor.archetypeIndex >= len(cursor.view.plan.blocks) {
				return false
			}
			block := cursor.view.plan.blocks[cursor.archetypeIndex]
			cursor.archetypeBits = block.mask & cursor.view.world.occupiedArchetypes[block.index]
			cursor.archetypeBase = block.index * 64
		}
		id := cursor.archetypeBase + uint32(bits.TrailingZeros64(cursor.archetypeBits))
		cursor.archetypeBits &= cursor.archetypeBits - 1
		archetype := cursor.view.world.archetypes[id]
		cursor.scopeTable = nil
		if cursor.scope != nil {
			table := cursor.scope.table(id)
			if table == nil {
				continue
			}
			if table.count == 0 {
				continue
			}
			if int(table.count) != len(archetype.entities) {
				cursor.scopeTable, cursor.scopeBlock = table, 0
			}
		}
		cursor.bindArchetype(archetype)
		return true
	}
}

func (cursor *queryCursor) bindArchetype(archetype *archetype) {
	cursor.archetype = archetype
	for index := 0; index < cursor.view.filters.count; index++ {
		filter := cursor.view.filters.at(index)
		column := archetype.column(filter.component)
		binding := cursor.binding(index)
		binding.ticks = column.ticks(filter.kind)
		binding.ranges = column.ranges(filter.kind)
		binding.window = filter.window
	}
}

func (cursor *queryCursor) rangeMatches(rangeIndex int) bool {
	for index := 0; index < cursor.view.filters.count; index++ {
		binding := cursor.binding(index)
		if !binding.ranges[rangeIndex].matches(binding.window) {
			return false
		}
	}
	return true
}

func (cursor *queryCursor) groupMatches(group int) bool {
	for index := 0; index < cursor.view.filters.count; index++ {
		binding := cursor.binding(index)
		if !tickGroupRange(binding.ranges, group, len(binding.ticks)).matches(binding.window) {
			return false
		}
	}
	return true
}

// nextRows returns a half-open range in the current table. Explicit lists
// yield one row at a time to preserve their order and repeated entities.
func (cursor *queryCursor) nextRows(after int) (int, int) {
	if cursor.view.limited {
		if cursor.nextEntity() {
			return cursor.row, cursor.row + 1
		}
		return 0, 0
	}
	if cursor.archetype != nil && after < len(cursor.archetype.entities) {
		if start, end := cursor.nextBlock(after, len(cursor.archetype.entities)); start < end {
			return start, end
		}
	}
	for cursor.nextArchetype() {
		if start, end := cursor.nextBlock(0, len(cursor.archetype.entities)); start < end {
			return start, end
		}
	}
	return 0, 0
}

// nextBlock returns a whole table for dense traversal. With row filters or
// a partial scope, it skips excluded groups and blocks.
func (cursor *queryCursor) nextBlock(start, length int) (int, int) {
	if cursor.view.filters.count == 0 && cursor.scopeTable == nil {
		return start, length
	}
	for start < length {
		if table := cursor.scopeTable; table != nil && len(table.sparse) != 0 {
			for cursor.scopeBlock < len(table.sparse) && int(table.sparse[cursor.scopeBlock]) < start {
				cursor.scopeBlock++
			}
			if cursor.scopeBlock == len(table.sparse) {
				return length, length
			}
			start = int(table.sparse[cursor.scopeBlock])
		}
		if cursor.view.filters.count != 0 && start%queryTickGroupSize == 0 && !cursor.groupMatches(start/queryTickGroupSize) {
			start = (start/queryTickGroupSize + 1) * queryTickGroupSize
			continue
		}
		block := start / queryTickBlockSize
		end := min((block+1)*queryTickBlockSize, length)
		if cursor.view.filters.count != 0 && !cursor.rangeMatches(block) {
			start = end
			continue
		}
		if table := cursor.scopeTable; table != nil {
			if len(table.dense) != 0 {
				if block >= len(table.dense) {
					return length, length
				}
				word := table.dense[block] & (^uint64(0) << uint(start%64))
				if word == 0 {
					start = end
					continue
				}
				start = block*64 + bits.TrailingZeros64(word)
				end = min(end, start+bits.TrailingZeros64(^(word>>uint(start%64))))
			} else {
				end = start + 1
				for cursor.scopeBlock+1 < len(table.sparse) && int(table.sparse[cursor.scopeBlock+1]) == end && end%64 != 0 {
					cursor.scopeBlock++
					end++
				}
			}
		}
		return start, end
	}
	return length, length
}

func (cursor *queryCursor) close() {
	if cursor.extra != nil {
		queryBindingPool.put(cursor.extra)
	}
	cursor.view.world.activeQueryIterators--
}

func (view *queryView) init(world *World, ids []componentID, options []QueryOption) {
	view.world = world
	// Insert the at most seven IDs in sorted order for the cache. Accessors
	// retain the original order supplied by the caller.
	key := simpleQueryKey{count: uint8(len(ids))}
	for i, id := range ids {
		j := i
		for j > 0 && key.ids[j-1] > id {
			key.ids[j] = key.ids[j-1]
			j--
		}
		if j > 0 && key.ids[j-1] == id {
			panic("ecs: query contains duplicate component type")
		}
		key.ids[j] = id
	}
	if len(options) == 0 {
		view.plan = world.simpleQueryPlan(key)
		return
	}
	// Reuse the world's scratch masks so a warm query builds no new storage.
	spec := &world.queryMasks
	spec.required = maskForInto(spec.required, key.ids[:key.count])
	spec.without = spec.without[:0]
	for _, option := range options {
		if option == nil {
			continue
		}
		filter := option.resolve(world)
		if filter.kind == filterUnscoped {
			view.unscoped = true
			continue
		}
		if filter.kind == filterWithout {
			spec.without = maskWith(spec.without, filter.component)
			continue
		}
		spec.required = maskWith(spec.required, filter.component)
		if filter.kind != filterWith {
			view.filters.append(filter)
		}
	}
	view.plan = world.queryPlan(*spec)
}

func (binding *filterBinding) matches(row int) bool {
	return binding.window.contains(binding.ticks[row])
}

func (cursor *queryCursor) remainingFiltersMatch(row int) bool {
	for index := 1; index < cursor.view.filters.count; index++ {
		if !cursor.binding(index).matches(row) {
			return false
		}
	}
	return true
}

func (w *World) queryPlan(spec querySpec) *queryPlan {
	spec.required = spec.required[:lastMaskWord(spec.required)+1]
	spec.without = spec.without[:lastMaskWord(spec.without)+1]
	hash := spec.required.hash(0x243f6a8885a308d3)
	hash = spec.without.hash(hash ^ 0x13198a2e03707344)
	head := w.queryPlans[hash]
	for plan := head; plan != nil; plan = plan.nextHash {
		if slices.Equal(plan.spec.required, spec.required) && slices.Equal(plan.spec.without, spec.without) {
			return plan
		}
	}
	plan := &queryPlan{
		nextHash: head,
		spec:     querySpec{required: slices.Clone(spec.required), without: slices.Clone(spec.without)},
	}
	w.queryPlans[hash] = plan
	return plan
}

func (w *World) simpleQueryPlan(key simpleQueryKey) *queryPlan {
	if plan := w.simpleQueryPlans[key]; plan != nil {
		return plan
	}
	ids := key.ids[:key.count]
	w.queryMasks.required = maskForInto(w.queryMasks.required, ids)
	plan := w.queryPlan(querySpec{required: w.queryMasks.required})
	w.simpleQueryPlans[key] = plan
	return plan
}

// maskWith sets one bit, growing the mask word-wise. Reused capacity is
// cleared so an enlarged mask never inherits stale bits.
func maskWith(mask componentMask, id componentID) componentMask {
	word := int(id) / 64
	if len(mask) <= word {
		length := len(mask)
		mask = slices.Grow(mask, word+1-length)[:word+1]
		clear(mask[length:])
	}
	mask[word] |= uint64(1) << (uint(id) % 64)
	return mask
}
