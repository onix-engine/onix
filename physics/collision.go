package physics

import (
	"math"
	"slices"
)

const maxGridCellsPerCollider = 64

type worldCircle struct {
	center Vector
	radius float64
}

type worldBox struct {
	center Vector
	half   Vector
	xAxis  Vector
	yAxis  Vector
}

type worldSegment struct {
	start  Vector
	end    Vector
	radius float64
}

func (world *World) buildContacts() {
	world.buildBroadphase()
	world.contacts = world.contacts[:0]
	for _, pair := range world.pairs {
		colliderA := &world.colliders[pair.a]
		colliderB := &world.colliders[pair.b]
		bodyA := &world.bodies[colliderA.bodyIndex]
		bodyB := &world.bodies[colliderB.bodyIndex]
		contact, ok := collideShapes(pair, *colliderA, *bodyA, *colliderB, *bodyB)
		if !ok {
			continue
		}
		contact.sensor = colliderA.sensor || colliderB.sensor
		contact.generations = [2]uint32{colliderA.generation, colliderB.generation}
		contact.friction = math.Sqrt(colliderA.friction * colliderB.friction)
		contact.restitution = max(colliderA.restitution, colliderB.restitution)
		world.contacts = append(world.contacts, contact)
	}
}

func (world *World) buildBroadphase() {
	world.ensureBuckets(len(world.colliders))
	for index := range world.bucketHeads {
		world.bucketHeads[index] = -1
	}
	if cap(world.aabbs) < len(world.colliders) {
		world.aabbs = make([]aabb, len(world.colliders))
	} else {
		world.aabbs = world.aabbs[:len(world.colliders)]
	}
	world.cells = growCapacity(world.cells, len(world.colliders))[:len(world.colliders)]
	world.gridEntries = world.gridEntries[:0]
	world.large = world.large[:0]
	world.pairs = world.pairs[:0]
	cellSize := world.CellSize
	if cellSize <= 1e-9 || math.IsNaN(cellSize) || math.IsInf(cellSize, 0) {
		cellSize = 64
	}
	for index := range world.colliders {
		collider := world.colliders[index]
		if !collider.alive || int(collider.bodyIndex) >= len(world.bodies) || !world.bodies[collider.bodyIndex].alive {
			continue
		}
		bounds := colliderBounds(collider, world.bodies[collider.bodyIndex])
		world.aabbs[index] = bounds
		minX, minY := floorCell(bounds.min.X, cellSize), floorCell(bounds.min.Y, cellSize)
		maxX, maxY := floorCell(bounds.max.X, cellSize), floorCell(bounds.max.Y, cellSize)
		width, height := int64(maxX)-int64(minX)+1, int64(maxY)-int64(minY)+1
		large := width > maxGridCellsPerCollider || height > maxGridCellsPerCollider || width*height > maxGridCellsPerCollider
		world.cells[index] = cellRange{minX: minX, minY: minY, large: large}
		if large {
			world.large = append(world.large, index)
			continue
		}
		for dy := range height {
			y := int32(int64(minY) + dy)
			for dx := range width {
				x := int32(int64(minX) + dx)
				bucket := gridHash(x, y) & (len(world.bucketHeads) - 1)
				world.gridEntries = append(world.gridEntries, broadphaseEntry{collider: index, next: world.bucketHeads[bucket], x: x, y: y})
				world.bucketHeads[bucket] = len(world.gridEntries) - 1
			}
		}
	}
	for _, head := range world.bucketHeads {
		for left := head; left >= 0; left = world.gridEntries[left].next {
			entryA := world.gridEntries[left]
			for right := world.gridEntries[left].next; right >= 0; right = world.gridEntries[right].next {
				entryB := world.gridEntries[right]
				if entryA.x != entryB.x || entryA.y != entryB.y {
					continue
				}
				cellsA, cellsB := world.cells[entryA.collider], world.cells[entryB.collider]
				// Emit only in the first shared cell, even if the shapes span many.
				if entryA.x == max(cellsA.minX, cellsB.minX) && entryA.y == max(cellsA.minY, cellsB.minY) {
					world.appendPair(entryA.collider, entryB.collider)
				}
			}
		}
	}
	for _, large := range world.large {
		for other := range world.colliders {
			if other != large && world.colliders[other].alive && (!world.cells[other].large || large < other) {
				world.appendPair(large, other)
			}
		}
	}
	slices.SortFunc(world.pairs, func(a, b colliderPair) int {
		if a.a != b.a {
			return a.a - b.a
		}
		return a.b - b.b
	})
}

func (world *World) appendPair(a, b int) {
	if a == b || !overlapAABB(world.aabbs[a], world.aabbs[b]) {
		return
	}
	colliderA, colliderB := &world.colliders[a], &world.colliders[b]
	if colliderA.bodyIndex == colliderB.bodyIndex || colliderA.filter.Reject(colliderB.filter) {
		return
	}
	bodyA, bodyB := &world.bodies[colliderA.bodyIndex], &world.bodies[colliderB.bodyIndex]
	if !bodyA.alive || !bodyB.alive || bodyA.bodyType != BodyDynamic && bodyB.bodyType != BodyDynamic && !colliderA.sensor && !colliderB.sensor {
		return
	}
	if a > b {
		a, b = b, a
	}
	world.pairs = append(world.pairs, colliderPair{a: a, b: b})
}

func (world *World) ensureBuckets(colliderCount int) {
	target := 64
	for target < colliderCount*4 {
		target <<= 1
	}
	if len(world.bucketHeads) < target {
		world.bucketHeads = make([]int, target)
	}
}

func floorCell(value, cellSize float64) int32 {
	result := math.Floor(value / cellSize)
	if result < math.MinInt32 {
		return math.MinInt32
	}
	if result > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(result)
}

func gridHash(x, y int32) int {
	return int(uint32(x)*0x8da6b343 ^ uint32(y)*0xd8163841)
}

func overlapAABB(a, b aabb) bool {
	return a.min.X <= b.max.X && a.max.X >= b.min.X && a.min.Y <= b.max.Y && a.max.Y >= b.min.Y
}

func colliderBounds(collider colliderState, body bodyState) aabb {
	switch collider.kind {
	case shapeCircle:
		circle := makeWorldCircle(collider, body)
		radius := Vector{X: circle.radius, Y: circle.radius}
		return aabb{min: circle.center.Sub(radius), max: circle.center.Add(radius)}
	case shapeBox:
		box := makeWorldBox(collider, body)
		extent := Vector{
			X: math.Abs(box.xAxis.X)*box.half.X + math.Abs(box.yAxis.X)*box.half.Y,
			Y: math.Abs(box.xAxis.Y)*box.half.X + math.Abs(box.yAxis.Y)*box.half.Y,
		}
		return aabb{min: box.center.Sub(extent), max: box.center.Add(extent)}
	case shapeSegment:
		segment := makeWorldSegment(collider, body)
		radius := Vector{X: segment.radius, Y: segment.radius}
		return aabb{
			min: Vector{X: min(segment.start.X, segment.end.X), Y: min(segment.start.Y, segment.end.Y)}.Sub(radius),
			max: Vector{X: max(segment.start.X, segment.end.X), Y: max(segment.start.Y, segment.end.Y)}.Add(radius),
		}
	default:
		return aabb{}
	}
}

func makeWorldCircle(collider colliderState, body bodyState) worldCircle {
	return worldCircle{center: body.position.Add(rotate(collider.a, body.rotation)), radius: collider.radius}
}

func makeWorldBox(collider colliderState, body bodyState) worldBox {
	angle := body.rotation + collider.rotation
	sine, cosine := math.Sincos(angle)
	return worldBox{
		center: body.position.Add(rotate(collider.a, body.rotation)), half: collider.b,
		xAxis: Vector{X: cosine, Y: sine}, yAxis: Vector{X: -sine, Y: cosine},
	}
}

func makeWorldSegment(collider colliderState, body bodyState) worldSegment {
	return worldSegment{
		start: body.position.Add(rotate(collider.a, body.rotation)),
		end:   body.position.Add(rotate(collider.b, body.rotation)), radius: collider.radius,
	}
}

func collideShapes(pair colliderPair, colliderA colliderState, bodyA bodyState, colliderB colliderState, bodyB bodyState) (manifold, bool) {
	contact := manifold{pair: pair}
	var ok bool
	switch colliderA.kind {
	case shapeCircle:
		circle := makeWorldCircle(colliderA, bodyA)
		switch colliderB.kind {
		case shapeCircle:
			contact.normal, contact.point, contact.penetration, ok = collideCircleCircle(circle, makeWorldCircle(colliderB, bodyB))
		case shapeBox:
			contact.normal, contact.point, contact.penetration, ok = collideCircleBox(circle, makeWorldBox(colliderB, bodyB))
		case shapeSegment:
			contact.normal, contact.point, contact.penetration, ok = collideCircleSegment(circle, makeWorldSegment(colliderB, bodyB))
		}
	case shapeBox:
		box := makeWorldBox(colliderA, bodyA)
		switch colliderB.kind {
		case shapeCircle:
			contact.normal, contact.point, contact.penetration, ok = collideCircleBox(makeWorldCircle(colliderB, bodyB), box)
			contact.normal = contact.normal.Mul(-1)
		case shapeBox:
			contact, ok = collideBoxBox(box, makeWorldBox(colliderB, bodyB))
			contact.pair = pair
		case shapeSegment:
			contact.normal, contact.point, contact.penetration, ok = collideBoxSegment(box, makeWorldSegment(colliderB, bodyB))
		}
	case shapeSegment:
		segment := makeWorldSegment(colliderA, bodyA)
		switch colliderB.kind {
		case shapeCircle:
			contact.normal, contact.point, contact.penetration, ok = collideCircleSegment(makeWorldCircle(colliderB, bodyB), segment)
			contact.normal = contact.normal.Mul(-1)
		case shapeBox:
			contact.normal, contact.point, contact.penetration, ok = collideBoxSegment(makeWorldBox(colliderB, bodyB), segment)
			contact.normal = contact.normal.Mul(-1)
		case shapeSegment:
			contact.normal, contact.point, contact.penetration, ok = collideSegmentSegment(segment, makeWorldSegment(colliderB, bodyB))
		}
	}
	if ok && contact.pointCount == 0 {
		contact.points[0].position, contact.pointCount = contact.point, 1
		contact.points[0].penetration = contact.penetration
	}
	return contact, ok
}

func collideCircleCircle(a, b worldCircle) (Vector, Vector, float64, bool) {
	delta := b.center.Sub(a.center)
	radius := a.radius + b.radius
	distanceSquared := delta.LengthSquared()
	if distanceSquared > radius*radius {
		return Vector{}, Vector{}, 0, false
	}
	distance := math.Sqrt(distanceSquared)
	normal := Vector{X: 1}
	if distance > 1e-12 {
		normal = delta.Mul(1 / distance)
	}
	return normal, a.center.Add(normal.Mul(a.radius)), radius - distance, true
}

func collideCircleBox(circle worldCircle, box worldBox) (Vector, Vector, float64, bool) {
	delta := circle.center.Sub(box.center)
	local := Vector{X: delta.Dot(box.xAxis), Y: delta.Dot(box.yAxis)}
	closest := Vector{X: clamp(local.X, -box.half.X, box.half.X), Y: clamp(local.Y, -box.half.Y, box.half.Y)}
	inside := closest.Sub(local).LengthSquared() <= 1e-24
	insideNormal := Vector{}
	if inside {
		distanceX := box.half.X - math.Abs(local.X)
		distanceY := box.half.Y - math.Abs(local.Y)
		if distanceX < distanceY {
			sign := 1.0
			if local.X < 0 {
				sign = -1
			}
			closest.X = sign * box.half.X
			insideNormal = box.xAxis.Mul(-sign)
		} else {
			sign := 1.0
			if local.Y < 0 {
				sign = -1
			}
			closest.Y = sign * box.half.Y
			insideNormal = box.yAxis.Mul(-sign)
		}
	}
	point := box.center.Add(box.xAxis.Mul(closest.X)).Add(box.yAxis.Mul(closest.Y))
	towardBox := point.Sub(circle.center)
	distance := towardBox.Length()
	if !inside && distance > circle.radius {
		return Vector{}, Vector{}, 0, false
	}
	normal := towardBox.Normalize()
	if normal == (Vector{}) {
		normal = box.xAxis
	}
	penetration := circle.radius - distance
	if inside {
		penetration = circle.radius + distance
		normal = insideNormal
	}
	return normal, point, penetration, true
}

func collideCircleSegment(circle worldCircle, segment worldSegment) (Vector, Vector, float64, bool) {
	point := closestPointOnSegment(circle.center, segment.start, segment.end)
	delta := point.Sub(circle.center)
	radius := circle.radius + segment.radius
	distanceSquared := delta.LengthSquared()
	if distanceSquared > radius*radius {
		return Vector{}, Vector{}, 0, false
	}
	distance := math.Sqrt(distanceSquared)
	normal := delta.Normalize()
	if normal == (Vector{}) {
		normal = segment.end.Sub(segment.start).Perpendicular().Normalize()
		if normal == (Vector{}) {
			normal.X = 1
		}
	}
	return normal, point.Sub(normal.Mul(segment.radius)), radius - distance, true
}

func collideBoxBox(a, b worldBox) (manifold, bool) {
	centerDelta := b.center.Sub(a.center)
	minimum := math.MaxFloat64
	normal := Vector{}
	referenceA := true
	for index, axis := range [4]Vector{a.xAxis, a.yAxis, b.xAxis, b.yAxis} {
		radiusA := projectedBoxRadius(a, axis)
		radiusB := projectedBoxRadius(b, axis)
		overlap := radiusA + radiusB - math.Abs(centerDelta.Dot(axis))
		if overlap < 0 {
			return manifold{}, false
		}
		if overlap < minimum {
			minimum = overlap
			referenceA = index < 2
			normal = axis
			if centerDelta.Dot(normal) < 0 {
				normal = normal.Mul(-1)
			}
		}
	}
	contact := manifold{normal: normal, penetration: minimum}
	if referenceA {
		contact.points, contact.pointCount = boxContactPoints(a, b, normal)
	} else {
		contact.points, contact.pointCount = boxContactPoints(b, a, normal.Mul(-1))
	}
	contact.point = contact.points[0].position
	if contact.pointCount == 2 {
		contact.point = contact.point.Add(contact.points[1].position).Mul(.5)
	}
	return contact, true
}

// Keep both ends of the clipped face for independent support constraints.
func boxContactPoints(reference, incident worldBox, normal Vector) ([2]contactPoint, int) {
	r0, r1 := boxFace(reference, normal)
	i0, i1 := boxFace(incident, normal.Mul(-1))
	tangent := normal.Perpendicular()
	lo, hi := r0.Dot(tangent), r1.Dot(tangent)
	if lo > hi {
		lo, hi = hi, lo
	}
	start, delta := i0.Dot(tangent), i1.Sub(i0).Dot(tangent)
	t0, t1 := 0.0, 1.0
	if math.Abs(delta) > 1e-12 {
		a, b := (lo-start)/delta, (hi-start)/delta
		if a > b {
			a, b = b, a
		}
		t0, t1 = clamp(a, 0, 1), clamp(b, 0, 1)
	}
	separation, slope := i0.Sub(r0).Dot(normal), i1.Sub(i0).Dot(normal)
	if slope > 1e-12 {
		t1 = min(t1, clamp(-separation/slope, t0, t1))
	} else if slope < -1e-12 {
		t0 = max(t0, clamp(-separation/slope, t0, t1))
	}
	var points [2]contactPoint
	for index, t := range [2]float64{t0, t1} {
		point := i0.Add(i1.Sub(i0).Mul(t))
		points[index].penetration = r0.Sub(point).Dot(normal)
		points[index].position = point.Add(normal.Mul(points[index].penetration * .5))
	}
	if points[1].position.Sub(points[0].position).LengthSquared() <= 1e-12 {
		return points, 1
	}
	return points, 2
}

func boxFace(box worldBox, direction Vector) (Vector, Vector) {
	axis, tangent, half, span := box.xAxis, box.yAxis, box.half.X, box.half.Y
	if math.Abs(box.yAxis.Dot(direction)) > math.Abs(box.xAxis.Dot(direction)) {
		axis, tangent, half, span = box.yAxis, box.xAxis, box.half.Y, box.half.X
	}
	if axis.Dot(direction) < 0 {
		half = -half
	}
	center := box.center.Add(axis.Mul(half))
	return center.Sub(tangent.Mul(span)), center.Add(tangent.Mul(span))
}

func collideBoxSegment(box worldBox, segment worldSegment) (Vector, Vector, float64, bool) {
	segmentDelta := segment.end.Sub(segment.start)
	segmentCenter := segment.start.Add(segment.end).Mul(.5)
	centerDelta := segmentCenter.Sub(box.center)
	segmentAxis := segmentDelta.Perpendicular().Normalize()
	if segmentAxis == (Vector{}) {
		circle := worldCircle{center: segment.start, radius: segment.radius}
		normal, point, penetration, ok := collideCircleBox(circle, box)
		return normal.Mul(-1), point, penetration, ok
	}
	minimum := math.MaxFloat64
	normal := Vector{}
	axes := [7]Vector{box.xAxis, box.yAxis, segmentAxis}
	for index, corner := range [4]Vector{V(-1, -1), V(-1, 1), V(1, -1), V(1, 1)} {
		vertex := box.center.Add(box.xAxis.Mul(corner.X * box.half.X)).Add(box.yAxis.Mul(corner.Y * box.half.Y))
		axes[index+3] = closestPointOnSegment(vertex, segment.start, segment.end).Sub(vertex).Normalize()
	}
	for _, axis := range axes {
		if axis == (Vector{}) {
			continue
		}
		boxRadius := projectedBoxRadius(box, axis)
		segmentProjection := math.Abs(segmentDelta.Dot(axis))*.5 + segment.radius
		overlap := boxRadius + segmentProjection - math.Abs(centerDelta.Dot(axis))
		if overlap < 0 {
			return Vector{}, Vector{}, 0, false
		}
		if overlap < minimum {
			minimum = overlap
			normal = axis
			if centerDelta.Dot(normal) < 0 {
				normal = normal.Mul(-1)
			}
		}
	}
	point := closestPointOnSegment(box.center, segment.start, segment.end).Sub(normal.Mul(segment.radius))
	return normal, point, minimum, true
}

func collideSegmentSegment(a, b worldSegment) (Vector, Vector, float64, bool) {
	pointA, pointB := closestPointsOnSegments(a.start, a.end, b.start, b.end)
	delta := pointB.Sub(pointA)
	radius := a.radius + b.radius
	distanceSquared := delta.LengthSquared()
	if distanceSquared > radius*radius {
		return Vector{}, Vector{}, 0, false
	}
	distance := math.Sqrt(distanceSquared)
	normal := delta.Normalize()
	if normal == (Vector{}) {
		normal = b.start.Add(b.end).Sub(a.start.Add(a.end)).Normalize()
		if normal == (Vector{}) {
			normal = a.end.Sub(a.start).Perpendicular().Normalize()
		}
		if normal == (Vector{}) {
			normal.X = 1
		}
	}
	point := pointA.Add(normal.Mul(a.radius))
	return normal, point, radius - distance, true
}

func projectedBoxRadius(box worldBox, axis Vector) float64 {
	return box.half.X*math.Abs(box.xAxis.Dot(axis)) + box.half.Y*math.Abs(box.yAxis.Dot(axis))
}

func closestPointOnSegment(point, start, end Vector) Vector {
	delta := end.Sub(start)
	lengthSquared := delta.LengthSquared()
	if lengthSquared <= 1e-12 {
		return start
	}
	t := clamp(point.Sub(start).Dot(delta)/lengthSquared, 0, 1)
	return start.Add(delta.Mul(t))
}

func closestPointsOnSegments(a0, a1, b0, b1 Vector) (Vector, Vector) {
	d1, d2, offset := a1.Sub(a0), b1.Sub(b0), a0.Sub(b0)
	a, e := d1.LengthSquared(), d2.LengthSquared()
	if a <= 1e-12 && e <= 1e-12 {
		return a0, b0
	}
	var s, t float64
	if a <= 1e-12 {
		t = clamp(d2.Dot(offset)/e, 0, 1)
	} else {
		c := d1.Dot(offset)
		if e <= 1e-12 {
			s = clamp(-c/a, 0, 1)
		} else {
			b := d1.Dot(d2)
			denominator := a*e - b*b
			if denominator != 0 {
				s = clamp((b*d2.Dot(offset)-c*e)/denominator, 0, 1)
			}
			t = (b*s + d2.Dot(offset)) / e
			if t < 0 {
				t, s = 0, clamp(-c/a, 0, 1)
			} else if t > 1 {
				t, s = 1, clamp((b-c)/a, 0, 1)
			}
		}
	}
	return a0.Add(d1.Mul(s)), b0.Add(d2.Mul(t))
}
