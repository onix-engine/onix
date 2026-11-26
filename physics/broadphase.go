package physics

import (
	"math"
	"slices"
)

// StepStatistics reports work performed by the last Step, without allocating.
type StepStatistics struct {
	WritebackBodies                                         int // Managed ECS bodies visited for writeback after this step.
	BoundsUpdates, IndexUpdates, PairQueries, ContactBuilds int
	SleepingBodies                                          int
	CandidatePairs                                          int // Number of unique candidate pairs in the final broadphase pass.
}

func (world *World) Statistics() StepStatistics { return world.stats }

type cell3D struct{ x, y, z int32 }

// Static solids and possible sensor/dynamic contacts use separate buckets
// while sharing the same pool of grid entries.
type collisionCell3D struct {
	cell3D
	staticSolid bool
}

func (c collisionCell3D) hash() int {
	hash := gridHash3D(c.x, c.y, c.z) << 1
	if c.staticSolid {
		return hash | 1
	}
	return hash
}

type cellSpan3D struct{ min, max cell3D }

func (s cellSpan3D) large() bool {
	x := int64(s.max.x) - int64(s.min.x) + 1
	y := int64(s.max.y) - int64(s.min.y) + 1
	z := int64(s.max.z) - int64(s.min.z) + 1
	return x > maxGridCellsPerCollider || y > maxGridCellsPerCollider || z > maxGridCellsPerCollider || x*y*z > maxGridCellsPerCollider
}

func (s cellSpan3D) cells(yield func(cell3D) bool) {
	for z := int64(s.min.z); z <= int64(s.max.z); z++ {
		for y := int64(s.min.y); y <= int64(s.max.y); y++ {
			for x := int64(s.min.x); x <= int64(s.max.x); x++ {
				if !yield(cell3D{int32(x), int32(y), int32(z)}) {
					return
				}
			}
		}
	}
}

type broadphaseProxy struct {
	span                             cellSpan3D
	first, largeSlot                 int // one-based grid entry or large-list slot
	staticSolid, queued, boundsDirty bool
}

type spatialCell interface{ hash() int }

type spatialEntry[C spatialCell] struct {
	cell                              C
	collider, next, previous, ownNext int
}
type spatialGrid[C spatialCell] struct {
	heads   []int
	entries []spatialEntry[C]
	free    int
}

func (grid *spatialGrid[C]) reserve(count int) {
	target := 64
	for target < count*4 {
		target <<= 1
	}
	if len(grid.heads) >= target {
		return
	}
	grid.heads = make([]int, target)
	for i := range grid.entries {
		entry := &grid.entries[i]
		if entry.collider < 0 {
			continue
		}
		bucket := entry.cell.hash() & (target - 1)
		entry.previous, entry.next = 0, grid.heads[bucket]
		if entry.next != 0 {
			grid.entries[entry.next-1].previous = i + 1
		}
		grid.heads[bucket] = i + 1
	}
}

func (grid *spatialGrid[C]) remove(first int) {
	for id := first; id != 0; {
		entry := &grid.entries[id-1]
		next := entry.ownNext
		if entry.previous != 0 {
			grid.entries[entry.previous-1].next = entry.next
		} else {
			grid.heads[entry.cell.hash()&(len(grid.heads)-1)] = entry.next
		}
		if entry.next != 0 {
			grid.entries[entry.next-1].previous = entry.previous
		}
		entry.collider, entry.next = -1, grid.free
		grid.free = id
		id = next
	}
}

func (grid *spatialGrid[C]) insertEntry(index, first int, cell C) int {
	id := grid.free
	if id == 0 {
		grid.entries = append(grid.entries, spatialEntry[C]{})
		id = len(grid.entries)
	} else {
		grid.free = grid.entries[id-1].next
	}
	bucket := cell.hash() & (len(grid.heads) - 1)
	head := grid.heads[bucket]
	grid.entries[id-1] = spatialEntry[C]{collider: index, next: head, ownNext: first, cell: cell}
	if head != 0 {
		grid.entries[head-1].previous = id
	}
	grid.heads[bucket] = id
	return id
}

// Both solvers index static solids separately from other colliders.
// The 2D world uses a single Z plane.
type broadphaseIndex struct {
	proxies      []broadphaseProxy
	dirtyProxies []int
	pairs        []colliderPair
	large        []int
	grid         spatialGrid[collisionCell3D]
	gridCellSize float64
}

func (world *broadphaseIndex) reserve(count int) {
	world.proxies = growCapacity(world.proxies, count)
	world.dirtyProxies = growCapacity(world.dirtyProxies, count)
	world.large = growCapacity(world.large, max(count/8, 8))
	world.pairs = growCapacity(world.pairs, count*8)
	world.grid.entries = growCapacity(world.grid.entries, count*8)
	world.grid.reserve(count)
}

func (world *broadphaseIndex) markProxy(index uint32, boundsDirty bool) {
	p := &world.proxies[index]
	p.boundsDirty = p.boundsDirty || boundsDirty
	if !p.queued {
		p.queued = true
		world.dirtyProxies = append(world.dirtyProxies, int(index))
	}
}

func (index *broadphaseIndex) updateMembership(slot int, span cellSpan3D, staticSolid, alive bool, stats *StepStatistics) {
	p := &index.proxies[slot]
	if alive && (p.first != 0 || p.largeSlot != 0) && p.span == span && p.staticSolid == staticSolid {
		return
	}
	if p.largeSlot != 0 {
		last := index.large[len(index.large)-1]
		index.large[p.largeSlot-1] = last
		index.proxies[last].largeSlot = p.largeSlot
		index.large = index.large[:len(index.large)-1]
		p.largeSlot = 0
	} else if p.first != 0 {
		index.grid.remove(p.first)
		p.first = 0
	}
	stats.IndexUpdates++
	if !alive {
		return
	}
	p.span, p.staticSolid = span, staticSolid
	if span.large() {
		index.large = append(index.large, slot)
		p.largeSlot = len(index.large)
		return
	}
	for cell := range span.cells {
		p.first = index.grid.insertEntry(slot, p.first, collisionCell3D{cell, staticSolid})
	}
}

func (world *broadphaseIndex) rebuild(cellSize float64, alive func(int) bool, update func(int, float64), appendPair func(int, int), stats *StepStatistics) {
	if cellSize != world.gridCellSize {
		world.gridCellSize = cellSize
		for i := range world.proxies {
			if alive(i) {
				world.markProxy(uint32(i), false)
			}
		}
	}
	if len(world.dirtyProxies) == 0 {
		return
	}
	world.grid.reserve(len(world.proxies))
	for _, index := range world.dirtyProxies {
		update(index, cellSize)
	}
	kept := world.pairs[:0]
	for _, pair := range world.pairs {
		if !world.proxies[pair.a].queued && !world.proxies[pair.b].queued {
			kept = append(kept, pair)
		}
	}
	world.pairs = kept
	for _, index := range world.dirtyProxies {
		if !alive(index) {
			continue
		}
		stats.PairQueries++
		p := &world.proxies[index]
		if p.largeSlot != 0 {
			for other := range world.proxies {
				if alive(other) {
					appendPair(index, other)
				}
			}
			continue
		}
		for cell := range p.span.cells {
			if !p.staticSolid {
				world.queryCell(index, collisionCell3D{cell, true}, appendPair)
			}
			world.queryCell(index, collisionCell3D{cell, false}, appendPair)
		}
		for _, other := range world.large {
			appendPair(index, other)
		}
	}
	if len(world.pairs) > len(kept) {
		slices.SortFunc(world.pairs, compareColliderPairs)
	}
	for _, index := range world.dirtyProxies {
		world.proxies[index].queued, world.proxies[index].boundsDirty = false, false
	}
	world.dirtyProxies = world.dirtyProxies[:0]
}

func (world *broadphaseIndex) queryCell(index int, cell collisionCell3D, appendPair func(int, int)) {
	grid := &world.grid
	start := world.proxies[index].span.min
	for id := grid.heads[cell.hash()&(len(grid.heads)-1)]; id != 0; id = grid.entries[id-1].next {
		entry := &grid.entries[id-1]
		if entry.cell == cell {
			other := world.proxies[entry.collider].span.min
			if cell.cell3D == (cell3D{max(start.x, other.x), max(start.y, other.y), max(start.z, other.z)}) {
				appendPair(index, entry.collider)
			}
		}
	}
}

func (world *World) markBody(body *bodyState, boundsDirty bool) {
	for id := body.firstCollider; id != 0; id = world.colliders[id-1].nextCollider {
		world.markProxy(id-1, boundsDirty)
	}
}

func (world *World) setPose(body *bodyState, position Vector, rotation float64) {
	if body.position != position || body.rotation != rotation {
		if body.managed {
			world.writes.record(body.userData, true)
		}
		body.position, body.rotation = position, rotation
		world.markBody(body, true)
	}
}

func (world *World) setBodyType(body *bodyState, value BodyType) {
	if body.bodyType != value {
		body.bodyType = value
		world.markBody(body, false)
	}
}

func (world *World) updateProxy(index int, cellSize float64) {
	p, c := &world.proxies[index], &world.colliders[index]
	var span cellSpan3D
	staticSolid := false
	if c.alive {
		body := &world.bodies[c.bodyIndex]
		if p.boundsDirty {
			world.aabbs[index] = colliderBounds(*c, *body)
			world.stats.BoundsUpdates++
		}
		b := world.aabbs[index]
		span = cellSpan3D{
			min: cell3D{floorCell(b.min.X, cellSize), floorCell(b.min.Y, cellSize), 0},
			max: cell3D{floorCell(b.max.X, cellSize), floorCell(b.max.Y, cellSize), 0},
		}
		staticSolid = body.bodyType == BodyStatic && !c.sensor
	}
	world.updateMembership(index, span, staticSolid, c.alive, &world.stats)
}

func (world *World) buildBroadphase() {
	cellSize := world.CellSize
	if !(cellSize > 1e-9) || math.IsInf(cellSize, 0) {
		cellSize = 64
	}
	world.rebuild(cellSize, func(index int) bool { return world.colliders[index].alive }, world.updateProxy, world.appendPair, &world.stats)
}

func compareColliderPairs(a, b colliderPair) int {
	if a.a != b.a {
		return a.a - b.a
	}
	return a.b - b.b
}
