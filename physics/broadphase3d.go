package physics

import "math"

// Statistics reports work performed by the last step.
func (world *World3D) Statistics() StepStatistics { return world.stats }

func (world *World3D) markBody(body *bodyState3D, boundsDirty bool) {
	for id := body.firstCollider; id != 0; id = world.colliders[id-1].nextCollider {
		world.markProxy(id-1, boundsDirty)
	}
}

func (world *World3D) setPose(body *bodyState3D, position Vector3D, rotation Quaternion) {
	if body.position != position || body.rotation != rotation {
		if body.managed {
			world.writes.record(body.userData, true)
		}
		body.position, body.rotation = position, rotation
		world.markBody(body, true)
	}
}

func (world *World3D) setBodyType(body *bodyState3D, value BodyType) {
	if body.bodyType != value {
		body.bodyType = value
		world.markBody(body, false)
	}
}

func (world *World3D) updateProxy(index int, cellSize float64) {
	p, c := &world.proxies[index], &world.colliders[index]
	var span cellSpan3D
	staticSolid := false
	if c.alive {
		body := &world.bodies[c.bodyIndex]
		if p.boundsDirty {
			world.aabbs[index] = colliderBounds3D(*c, *body)
			world.stats.BoundsUpdates++
		}
		b := world.aabbs[index]
		span = cellSpan3D{
			min: cell3D{floorCell(b.min.X, cellSize), floorCell(b.min.Y, cellSize), floorCell(b.min.Z, cellSize)},
			max: cell3D{floorCell(b.max.X, cellSize), floorCell(b.max.Y, cellSize), floorCell(b.max.Z, cellSize)},
		}
		staticSolid = body.bodyType == BodyStatic && !c.sensor
	}
	world.updateMembership(index, span, staticSolid, c.alive, &world.stats)
}

func (world *World3D) buildBroadphase() {
	cellSize := world.CellSize
	if !(cellSize > 1e-9) || math.IsInf(cellSize, 0) {
		cellSize = 4
	}
	world.rebuild(cellSize, func(index int) bool { return world.colliders[index].alive }, world.updateProxy, world.appendPair, &world.stats)
}

func (world *World3D) appendPair(a, b int) {
	// Both dirty endpoints query their neighbours. Only the smaller index emits.
	if a == b || a > b && world.proxies[b].queued || !overlapAABB3D(world.aabbs[a], world.aabbs[b]) {
		return
	}
	ca, cb := &world.colliders[a], &world.colliders[b]
	if ca.bodyIndex == cb.bodyIndex || ca.filter.Reject(cb.filter) ||
		!ca.sensor && !cb.sensor && world.bodies[ca.bodyIndex].bodyType != BodyDynamic && world.bodies[cb.bodyIndex].bodyType != BodyDynamic {
		return
	}
	if a > b {
		a, b = b, a
	}
	world.pairs = append(world.pairs, colliderPair{a, b})
}
