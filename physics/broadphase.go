package physics

import (
	"math"
	"slices"
)

// StepStatistics reports work performed by the last Step, without allocating.
type StepStatistics struct {
	BoundsUpdates, IndexUpdates, PairQueries, ContactBuilds int
	SleepingBodies                                          int
}

func (world *World) Statistics() StepStatistics { return world.stats }

type proxyGeometry struct {
	generation, bodyGeneration, bodyIndex uint32
	kind                                  shapeKind
	a, b, position                        Vector
	radius, rotation, bodyRotation        float64
}

type proxyKey struct {
	proxyGeometry
	bodyType BodyType
	filter   Filter
	sensor   bool
}
type broadphaseProxy struct {
	key                    proxyKey
	alive, dirty, large    bool
	minX, minY, maxX, maxY int32
	first                  int // entry indices are one based; zero is the end of a chain
}
type spatialEntry struct {
	collider, next, previous, ownNext int
	x, y                              int32
}
type spatialGrid struct {
	heads   []int
	entries []spatialEntry
	free    int
}

func (grid *spatialGrid) reserve(count int) {
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
		bucket := gridHash(entry.x, entry.y) & (target - 1)
		entry.previous, entry.next = 0, grid.heads[bucket]
		if entry.next != 0 {
			grid.entries[entry.next-1].previous = i + 1
		}
		grid.heads[bucket] = i + 1
	}
}
func (grid *spatialGrid) remove(proxy *broadphaseProxy) {
	for id := proxy.first; id != 0; {
		entry := &grid.entries[id-1]
		next := entry.ownNext
		if entry.previous != 0 {
			grid.entries[entry.previous-1].next = entry.next
		} else {
			grid.heads[gridHash(entry.x, entry.y)&(len(grid.heads)-1)] = entry.next
		}
		if entry.next != 0 {
			grid.entries[entry.next-1].previous = entry.previous
		}
		entry.collider, entry.next = -1, grid.free
		grid.free = id
		id = next
	}
	proxy.first = 0
}
func (grid *spatialGrid) insert(index int, proxy *broadphaseProxy) {
	if proxy.large {
		return
	}
	for y := int64(proxy.minY); y <= int64(proxy.maxY); y++ {
		for x := int64(proxy.minX); x <= int64(proxy.maxX); x++ {
			id := grid.free
			if id == 0 {
				grid.entries = append(grid.entries, spatialEntry{})
				id = len(grid.entries)
			} else {
				grid.free = grid.entries[id-1].next
			}
			bucket := gridHash(int32(x), int32(y)) & (len(grid.heads) - 1)
			grid.entries[id-1] = spatialEntry{collider: index, next: grid.heads[bucket], ownNext: proxy.first, x: int32(x), y: int32(y)}
			if grid.heads[bucket] != 0 {
				grid.entries[grid.heads[bucket]-1].previous = id
			}
			grid.heads[bucket], proxy.first = id, id
		}
	}
}
func (world *World) proxyGrid(bodyType BodyType) *spatialGrid {
	if bodyType == BodyStatic {
		return &world.staticGrid
	}
	return &world.dynamicGrid
}
func (world *World) buildBroadphase() {
	count := len(world.colliders)
	world.staticGrid.reserve(count)
	world.dynamicGrid.reserve(count)
	world.aabbs = growCapacity(world.aabbs, count)[:count]
	previous := len(world.proxies)
	world.proxies = growCapacity(world.proxies, count)[:count]
	clear(world.proxies[previous:])
	cellSize := world.CellSize
	if cellSize <= 1e-9 || math.IsNaN(cellSize) || math.IsInf(cellSize, 0) {
		cellSize = 64
	}
	resized := cellSize != world.gridCellSize
	world.gridCellSize = cellSize
	changed := false
	world.large = world.large[:0]
	world.staticSensors = world.staticSensors[:0]
	for i := range world.colliders {
		collider := &world.colliders[i]
		proxy := &world.proxies[i]
		proxy.dirty = false
		alive := collider.alive && int(collider.bodyIndex) < len(world.bodies) && world.bodies[collider.bodyIndex].alive
		if !alive {
			if proxy.alive {
				world.proxyGrid(proxy.key.bodyType).remove(proxy)
				proxy.alive, proxy.dirty, changed = false, true, true
			}
			continue
		}
		body := &world.bodies[collider.bodyIndex]
		key := proxyKey{proxyGeometry{collider.generation, body.generation, collider.bodyIndex, collider.kind, collider.a, collider.b, body.position, collider.radius, collider.rotation, body.rotation}, body.bodyType, collider.filter, collider.sensor}
		if !proxy.alive || key != proxy.key || resized {
			next := broadphaseProxy{key: key, alive: true, dirty: true}
			bounds := world.aabbs[i]
			if !proxy.alive || key.proxyGeometry != proxy.key.proxyGeometry {
				bounds = colliderBounds(*collider, *body)
				world.stats.BoundsUpdates++
				world.aabbs[i] = bounds
			}
			next.minX, next.minY = floorCell(bounds.min.X, cellSize), floorCell(bounds.min.Y, cellSize)
			next.maxX, next.maxY = floorCell(bounds.max.X, cellSize), floorCell(bounds.max.Y, cellSize)
			width, height := int64(next.maxX)-int64(next.minX)+1, int64(next.maxY)-int64(next.minY)+1
			next.large = width > maxGridCellsPerCollider || height > maxGridCellsPerCollider || width*height > maxGridCellsPerCollider
			if !proxy.alive || resized || next.key.bodyType != proxy.key.bodyType || next.minX != proxy.minX || next.minY != proxy.minY || next.maxX != proxy.maxX || next.maxY != proxy.maxY || next.large != proxy.large {
				if proxy.alive {
					world.proxyGrid(proxy.key.bodyType).remove(proxy)
				}
				*proxy = next
				world.proxyGrid(key.bodyType).insert(i, proxy)
				world.stats.IndexUpdates++
			} else {
				next.first = proxy.first
				*proxy = next
			}
			changed = true
		}
		if key.bodyType == BodyStatic && key.sensor {
			world.staticSensors = append(world.staticSensors, i)
		}
		if proxy.large {
			world.large = append(world.large, i)
		}
	}
	if !changed {
		return
	}
	// Retain pairs whose endpoints did not change; only query changed proxies.
	kept := world.pairs[:0]
	for _, pair := range world.pairs {
		if !world.proxies[pair.a].dirty && !world.proxies[pair.b].dirty {
			kept = append(kept, pair)
		}
	}
	world.pairs = kept
	for i := range world.proxies {
		proxy := &world.proxies[i]
		if !proxy.dirty || !proxy.alive {
			continue
		}
		world.stats.PairQueries++
		if proxy.large {
			for j := range world.proxies {
				if world.proxies[j].alive {
					world.appendPair(i, j)
				}
			}
			continue
		}
		if proxy.key.bodyType != BodyStatic || proxy.key.sensor {
			world.queryGrid(i, proxy, &world.staticGrid)
		} else {
			for _, j := range world.staticSensors {
				world.appendPair(i, j)
			}
		}
		world.queryGrid(i, proxy, &world.dynamicGrid)
		for _, j := range world.large {
			world.appendPair(i, j)
		}
	}
	slices.SortFunc(world.pairs, func(a, b colliderPair) int {
		if a.a != b.a {
			return a.a - b.a
		}
		return a.b - b.b
	})
	world.pairs = slices.Compact(world.pairs)
}
func (world *World) queryGrid(index int, proxy *broadphaseProxy, grid *spatialGrid) {
	for y := int64(proxy.minY); y <= int64(proxy.maxY); y++ {
		for x := int64(proxy.minX); x <= int64(proxy.maxX); x++ {
			for id := grid.heads[gridHash(int32(x), int32(y))&(len(grid.heads)-1)]; id != 0; id = grid.entries[id-1].next {
				entry := &grid.entries[id-1]
				if entry.x != int32(x) || entry.y != int32(y) {
					continue
				}
				other := &world.proxies[entry.collider]
				if entry.x == max(proxy.minX, other.minX) && entry.y == max(proxy.minY, other.minY) {
					world.appendPair(index, entry.collider)
				}
			}
		}
	}
}
