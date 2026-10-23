package ecs

//go:generate go run ../internal/ecsgen

import "slices"

type filterKind uint8

const (
	filterAdded filterKind = iota
	filterChanged
	filterWith
	filterWithout
)

type rowFilter struct {
	component componentID
	kind      filterKind
	lastRun   Tick
	thisRun   Tick
}

type querySpec struct {
	required componentMask
	without  componentMask
}

const inlineQueryFilterCount = 8

type queryFilters struct {
	inline [inlineQueryFilterCount]rowFilter
	extra  []rowFilter
	count  int
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
	spec       querySpec
	archetypes []uint32
	scanned    int
}

type simpleQueryKey struct {
	count uint8
	ids   [7]componentID
}

func (p *queryPlan) update(world *World) {
	for _, archetype := range world.archetypes[p.scanned:] {
		if archetype.mask.contains(p.spec.required) && !archetype.mask.intersects(p.spec.without) {
			p.archetypes = append(p.archetypes, archetype.id)
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

func (withOption[T]) resolve(world *World) rowFilter {
	return rowFilter{component: registerComponent[T](world), kind: filterWith}
}

func (withoutOption[T]) resolve(world *World) rowFilter {
	return rowFilter{component: registerComponent[T](world), kind: filterWithout}
}

func (option addedOption[T]) resolve(world *World) rowFilter {
	return resolveChangeFilter[T](world, option.context, filterAdded)
}

func (option changedOption[T]) resolve(world *World) rowFilter {
	return resolveChangeFilter[T](world, option.context, filterChanged)
}

func With[T any]() QueryOption {
	return withOption[T]{}
}

func Without[T any]() QueryOption {
	return withoutOption[T]{}
}

func Added[T any](context *SystemContext) QueryOption {
	return addedOption[T]{context: context}
}

func Changed[T any](context *SystemContext) QueryOption {
	return changedOption[T]{context: context}
}

func resolveChangeFilter[T any](world *World, context *SystemContext, kind filterKind) rowFilter {
	if context == nil {
		panic("ecs: system context is nil")
	}
	if context.World != world {
		panic("ecs: change filter context belongs to a different world")
	}
	return rowFilter{component: registerComponent[T](world), kind: kind, lastRun: context.lastRun, thisRun: context.thisRun}
}

type queryView struct {
	world    *World
	plan     *queryPlan
	filters  queryFilters
	entities []Entity
	limited  bool
}

func (v queryView) begin(callback bool) bool {
	if !callback || v.world == nil || v.plan == nil {
		return false
	}
	v.plan.update(v.world)
	v.world.activeQueryIterators++
	return true
}

func (v queryView) end() {
	v.world.activeQueryIterators--
}

// filterBinding resolves one change filter against one bound table: its tick
// column, its block bounds and the read window the filter was created with.
// All three are constant for the whole traversal of that table.
type filterBinding struct {
	ticks   []Tick
	ranges  []tickRange
	lastRun Tick
	thisRun Tick
}

// queryCursor shares query lifetime, table binding, tick filters and restricted
// entity traversal. Typed adapters traverse ordinary table rows directly.
type queryCursor struct {
	view           queryView
	archetype      *archetype
	archetypeIndex int
	row            int
	entityIndex    int
	tick           Tick
	inline         [inlineQueryFilterCount]filterBinding
	extra          *[]filterBinding
}

func (v queryView) cursor(callback bool) (queryCursor, bool) {
	if !v.limited && v.world != nil && v.world.queryScope != nil {
		v.entities, v.limited = v.world.queryScope.entities, true
	}
	if !v.begin(callback) {
		return queryCursor{}, false
	}
	cursor := queryCursor{view: v, archetypeIndex: -1, row: -1}
	if count := v.filters.count; count > len(cursor.inline) {
		cursor.extra = queryBindingPool.get(count - len(cursor.inline))
	}
	return cursor, true
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
		if cursor.view.filters.count != 0 && !cursor.rowMatches() {
			continue
		}
		return true
	}
	return false
}

// nextArchetype binds one matching table. Typed adapters traverse its contiguous
// rows directly, keeping cursor calls and column binding out of the hot loop.
func (cursor *queryCursor) nextArchetype() bool {
	cursor.archetypeIndex++
	if cursor.archetypeIndex >= len(cursor.view.plan.archetypes) {
		return false
	}
	cursor.bindArchetype(cursor.view.world.archetypes[cursor.view.plan.archetypes[cursor.archetypeIndex]])
	return true
}

func (cursor *queryCursor) bindArchetype(archetype *archetype) {
	cursor.archetype = archetype
	for index := 0; index < cursor.view.filters.count; index++ {
		filter := cursor.view.filters.at(index)
		column := archetype.column(filter.component)
		binding := cursor.binding(index)
		binding.ticks = column.ticks(filter.kind)
		binding.ranges = column.ranges(filter.kind)
		binding.lastRun = filter.lastRun
		binding.thisRun = filter.thisRun
	}
}

func (cursor *queryCursor) blockMatches() bool {
	block := cursor.row / queryTickBlockSize
	for index := 0; index < cursor.view.filters.count; index++ {
		binding := cursor.binding(index)
		if !binding.ranges[block].matches(rowFilter{lastRun: binding.lastRun, thisRun: binding.thisRun}) {
			return false
		}
	}
	return true
}

func (cursor *queryCursor) mutationTick() Tick {
	if cursor.tick == 0 {
		cursor.tick = cursor.view.world.mutationTick()
	}
	return cursor.tick
}

func (cursor *queryCursor) close() {
	if cursor.extra != nil {
		queryBindingPool.put(cursor.extra)
	}
	cursor.view.end()
}

func buildQuery(world *World, key simpleQueryKey, options []QueryOption) queryView {
	key.sort()
	for index := 1; index < int(key.count); index++ {
		if key.ids[index-1] == key.ids[index] {
			panic("ecs: query contains duplicate component type")
		}
	}
	if len(options) == 0 {
		return queryView{world: world, plan: world.simpleQueryPlan(key)}
	}
	// Reuse the world's scratch masks so a warm query builds no new storage.
	spec := &world.queryMasks
	spec.required = maskForInto(spec.required, key.ids[:key.count])
	spec.without = spec.without[:0]
	view := queryView{world: world}
	for _, option := range options {
		if option == nil {
			continue
		}
		filter := option.resolve(world)
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
	return view
}

func (cursor *queryCursor) rowMatches() bool {
	row := cursor.row
	for index := 0; index < cursor.view.filters.count; index++ {
		binding := cursor.binding(index)
		if !tickInRange(binding.ticks[row], binding.lastRun, binding.thisRun) {
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
	for _, plan := range w.queryPlans[hash] {
		if plan.spec.required.equal(spec.required) && plan.spec.without.equal(spec.without) {
			return plan
		}
	}
	plan := &queryPlan{spec: querySpec{required: slices.Clone(spec.required), without: slices.Clone(spec.without)}}
	w.queryPlans[hash] = append(w.queryPlans[hash], plan)
	return plan
}

func (w *World) simpleQueryPlan(key simpleQueryKey) *queryPlan {
	if plan := w.simpleQueryPlans[key]; plan != nil {
		return plan
	}
	ids := key.ids[:key.count]
	plan := &queryPlan{spec: querySpec{required: maskFor(ids)}}
	w.simpleQueryPlans[key] = plan
	return plan
}

func (key *simpleQueryKey) sort() {
	slices.Sort(key.ids[:key.count])
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
