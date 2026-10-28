package physics

import (
	"math"
	"slices"
)

type worldSphere struct {
	center Vector3D
	radius float64
}

type worldBox3D struct {
	center Vector3D
	half   Vector3D
	axis   [3]Vector3D
}

type worldCapsule struct {
	start  Vector3D
	end    Vector3D
	radius float64
}

func (world *World3D) buildContacts() {
	world.buildBroadphase()
	world.contacts = world.contacts[:0]
	previous := colliderPair{a: -1, b: -1}
	for _, pair := range world.pairs {
		if pair == previous {
			continue
		}
		previous = pair
		colliderA := &world.colliders[pair.a]
		colliderB := &world.colliders[pair.b]
		if !colliderA.alive || !colliderB.alive || colliderA.bodyIndex == colliderB.bodyIndex || colliderA.filter.Reject(colliderB.filter) {
			continue
		}
		bodyA := &world.bodies[colliderA.bodyIndex]
		bodyB := &world.bodies[colliderB.bodyIndex]
		if !bodyA.alive || !bodyB.alive || bodyA.bodyType != BodyDynamic && bodyB.bodyType != BodyDynamic && !colliderA.sensor && !colliderB.sensor {
			continue
		}
		if colliderA.kind == shapeTriangleMesh3D && bodyA.bodyType == BodyDynamic ||
			colliderB.kind == shapeTriangleMesh3D && bodyB.bodyType == BodyDynamic {
			continue
		}
		contact, ok := collideShapes3D(pair, *colliderA, *bodyA, *colliderB, *bodyB)
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

func (world *World3D) buildBroadphase() {
	world.ensureBuckets(len(world.colliders))
	for index := range world.bucketHeads {
		world.bucketHeads[index] = -1
	}
	if cap(world.aabbs) < len(world.colliders) {
		world.aabbs = make([]aabb3D, len(world.colliders))
	} else {
		world.aabbs = world.aabbs[:len(world.colliders)]
	}
	world.gridEntries = world.gridEntries[:0]
	world.large = world.large[:0]
	world.pairs = world.pairs[:0]
	cellSize := world.CellSize
	if cellSize <= 1e-9 {
		cellSize = 4
	}
	for index := range world.colliders {
		collider := world.colliders[index]
		if !collider.alive || int(collider.bodyIndex) >= len(world.bodies) || !world.bodies[collider.bodyIndex].alive {
			continue
		}
		bounds := colliderBounds3D(collider, world.bodies[collider.bodyIndex])
		world.aabbs[index] = bounds
		minX, minY, minZ := floorCell(bounds.min.X, cellSize), floorCell(bounds.min.Y, cellSize), floorCell(bounds.min.Z, cellSize)
		maxX, maxY, maxZ := floorCell(bounds.max.X, cellSize), floorCell(bounds.max.Y, cellSize), floorCell(bounds.max.Z, cellSize)
		cellCount := int64(maxX-minX+1) * int64(maxY-minY+1) * int64(maxZ-minZ+1)
		if cellCount > maxGridCellsPerCollider {
			world.large = append(world.large, index)
			continue
		}
		for z := minZ; z <= maxZ; z++ {
			for y := minY; y <= maxY; y++ {
				for x := minX; x <= maxX; x++ {
					bucket := gridHash3D(x, y, z) & (len(world.bucketHeads) - 1)
					world.gridEntries = append(world.gridEntries, broadphaseEntry3D{collider: index, next: world.bucketHeads[bucket]})
					world.bucketHeads[bucket] = len(world.gridEntries) - 1
				}
			}
		}
	}
	for _, head := range world.bucketHeads {
		for left := head; left >= 0; left = world.gridEntries[left].next {
			for right := world.gridEntries[left].next; right >= 0; right = world.gridEntries[right].next {
				world.appendPair(world.gridEntries[left].collider, world.gridEntries[right].collider)
			}
		}
	}
	for _, large := range world.large {
		for other := range world.colliders {
			if other != large && world.colliders[other].alive {
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

func (world *World3D) appendPair(a, b int) {
	if a == b || !overlapAABB3D(world.aabbs[a], world.aabbs[b]) {
		return
	}
	if a > b {
		a, b = b, a
	}
	world.pairs = append(world.pairs, colliderPair{a: a, b: b})
}

func (world *World3D) ensureBuckets(colliderCount int) {
	target := 64
	for target < colliderCount*4 {
		target <<= 1
	}
	if len(world.bucketHeads) < target {
		world.bucketHeads = make([]int, target)
	}
}

func gridHash3D(x, y, z int32) int {
	return int(uint32(x)*0x8da6b343 ^ uint32(y)*0xd8163841 ^ uint32(z)*0xcb1ab31f)
}

func overlapAABB3D(a, b aabb3D) bool {
	return a.min.X <= b.max.X && a.max.X >= b.min.X &&
		a.min.Y <= b.max.Y && a.max.Y >= b.min.Y &&
		a.min.Z <= b.max.Z && a.max.Z >= b.min.Z
}

func colliderBounds3D(collider colliderState3D, body bodyState3D) aabb3D {
	switch collider.kind {
	case shapeSphere3D:
		sphere := makeWorldSphere(collider, body)
		radius := V3(sphere.radius, sphere.radius, sphere.radius)
		return aabb3D{min: sphere.center.Sub(radius), max: sphere.center.Add(radius)}
	case shapeBox3D:
		box := makeWorldBox3D(collider, body)
		extent := Vector3D{
			X: math.Abs(box.axis[0].X)*box.half.X + math.Abs(box.axis[1].X)*box.half.Y + math.Abs(box.axis[2].X)*box.half.Z,
			Y: math.Abs(box.axis[0].Y)*box.half.X + math.Abs(box.axis[1].Y)*box.half.Y + math.Abs(box.axis[2].Y)*box.half.Z,
			Z: math.Abs(box.axis[0].Z)*box.half.X + math.Abs(box.axis[1].Z)*box.half.Y + math.Abs(box.axis[2].Z)*box.half.Z,
		}
		return aabb3D{min: box.center.Sub(extent), max: box.center.Add(extent)}
	case shapeCapsule3D:
		capsule := makeWorldCapsule(collider, body)
		radius := V3(capsule.radius, capsule.radius, capsule.radius)
		return aabb3D{
			min: V3(min(capsule.start.X, capsule.end.X), min(capsule.start.Y, capsule.end.Y), min(capsule.start.Z, capsule.end.Z)).Sub(radius),
			max: V3(max(capsule.start.X, capsule.end.X), max(capsule.start.Y, capsule.end.Y), max(capsule.start.Z, capsule.end.Z)).Add(radius),
		}
	case shapeTriangleMesh3D:
		return boundsToAABB3D(worldBounds3D(collider, body, collider.mesh.Bounds))
	case shapeConvexHull3D:
		return boundsToAABB3D(worldBounds3D(collider, body, collider.hull.Bounds))
	case shapeCompound3D:
		return boundsToAABB3D(worldBounds3D(collider, body, collider.compound.Bounds))
	default:
		return aabb3D{}
	}
}

func makeWorldSphere(collider colliderState3D, body bodyState3D) worldSphere {
	return worldSphere{center: body.position.Add(body.rotation.Rotate(collider.center)), radius: collider.radius}
}

func makeWorldBox3D(collider colliderState3D, body bodyState3D) worldBox3D {
	rotation := body.rotation.Mul(collider.rotation).Normalize()
	return worldBox3D{
		center: body.position.Add(body.rotation.Rotate(collider.center)), half: collider.half,
		axis: [3]Vector3D{rotation.Rotate(V3(1, 0, 0)), rotation.Rotate(V3(0, 1, 0)), rotation.Rotate(V3(0, 0, 1))},
	}
}

func makeWorldCapsule(collider colliderState3D, body bodyState3D) worldCapsule {
	center := body.position.Add(body.rotation.Rotate(collider.center))
	axis := body.rotation.Mul(collider.rotation).Rotate(V3(0, 1, 0)).Mul(collider.halfHeight)
	return worldCapsule{start: center.Sub(axis), end: center.Add(axis), radius: collider.radius}
}

func collideShapes3D(pair colliderPair, colliderA colliderState3D, bodyA bodyState3D, colliderB colliderState3D, bodyB bodyState3D) (manifold3D, bool) {
	contact := manifold3D{pair: pair}
	if custom, ok, handled := collideCustomShapes3D(contact, colliderA, bodyA, colliderB, bodyB); handled {
		custom.initializePoints()
		return custom, ok
	}
	var ok bool
	switch colliderA.kind {
	case shapeSphere3D:
		sphere := makeWorldSphere(colliderA, bodyA)
		switch colliderB.kind {
		case shapeSphere3D:
			contact.normal, contact.point, contact.penetration, ok = collideSphereSphere(sphere, makeWorldSphere(colliderB, bodyB))
		case shapeBox3D:
			contact.normal, contact.point, contact.penetration, ok = collideSphereBox(sphere, makeWorldBox3D(colliderB, bodyB))
		case shapeCapsule3D:
			contact.normal, contact.point, contact.penetration, ok = collideSphereCapsule(sphere, makeWorldCapsule(colliderB, bodyB))
		}
	case shapeBox3D:
		box := makeWorldBox3D(colliderA, bodyA)
		switch colliderB.kind {
		case shapeSphere3D:
			contact.normal, contact.point, contact.penetration, ok = collideSphereBox(makeWorldSphere(colliderB, bodyB), box)
			contact.normal = contact.normal.Mul(-1)
		case shapeBox3D:
			contact, ok = collideBoxBox3D(box, makeWorldBox3D(colliderB, bodyB))
			contact.pair = pair
		case shapeCapsule3D:
			contact.normal, contact.point, contact.penetration, ok = collideBoxCapsule(box, makeWorldCapsule(colliderB, bodyB))
			if ok {
				boxCapsuleContacts3D(&contact, box, makeWorldCapsule(colliderB, bodyB))
			}
		}
	case shapeCapsule3D:
		capsule := makeWorldCapsule(colliderA, bodyA)
		switch colliderB.kind {
		case shapeSphere3D:
			contact.normal, contact.point, contact.penetration, ok = collideSphereCapsule(makeWorldSphere(colliderB, bodyB), capsule)
			contact.normal = contact.normal.Mul(-1)
		case shapeBox3D:
			contact.normal, contact.point, contact.penetration, ok = collideBoxCapsule(makeWorldBox3D(colliderB, bodyB), capsule)
			if ok {
				boxCapsuleContacts3D(&contact, makeWorldBox3D(colliderB, bodyB), capsule)
			}
			contact.normal = contact.normal.Mul(-1)
		case shapeCapsule3D:
			contact.normal, contact.point, contact.penetration, ok = collideCapsuleCapsule(capsule, makeWorldCapsule(colliderB, bodyB))
		}
	}
	contact.initializePoints()
	return contact, ok
}

func collideSphereSphere(a, b worldSphere) (Vector3D, Vector3D, float64, bool) {
	delta := b.center.Sub(a.center)
	radius := a.radius + b.radius
	distanceSquared := delta.LengthSquared()
	if distanceSquared > radius*radius {
		return Vector3D{}, Vector3D{}, 0, false
	}
	distance := math.Sqrt(distanceSquared)
	normal := V3(1, 0, 0)
	if distance > 1e-12 {
		normal = delta.Mul(1 / distance)
	}
	return normal, a.center.Add(normal.Mul(a.radius)), radius - distance, true
}

func collideSphereBox(sphere worldSphere, box worldBox3D) (Vector3D, Vector3D, float64, bool) {
	local := boxToLocal(box, sphere.center)
	closest := clampToBox(local, box.half)
	inside := closest == local
	if inside {
		closest = nearestFacePoint(local, box.half)
	}
	point := boxToWorld(box, closest)
	towardBox := point.Sub(sphere.center)
	distance := towardBox.Length()
	if !inside && distance > sphere.radius {
		return Vector3D{}, Vector3D{}, 0, false
	}
	normal := towardBox.Normalize()
	if normal == (Vector3D{}) {
		normal = box.axis[0]
	}
	penetration := sphere.radius - distance
	if inside {
		penetration = sphere.radius + distance
		// The sphere is moved opposite this normal, toward its nearest exit.
		exit := closest.Sub(local).Normalize()
		if exit == (Vector3D{}) {
			for index := range 3 {
				if math.Abs(closest.component(index)) == box.half.component(index) {
					normal = box.axis[index].Mul(-nonzeroSign(closest.component(index)))
					break
				}
			}
		} else {
			normal = normal.Mul(-1)
		}
	}
	return normal, point, penetration, true
}

func collideSphereCapsule(sphere worldSphere, capsule worldCapsule) (Vector3D, Vector3D, float64, bool) {
	axisPoint := closestPointOnSegment3D(sphere.center, capsule.start, capsule.end)
	delta := axisPoint.Sub(sphere.center)
	radius := sphere.radius + capsule.radius
	distanceSquared := delta.LengthSquared()
	if distanceSquared > radius*radius {
		return Vector3D{}, Vector3D{}, 0, false
	}
	distance := math.Sqrt(distanceSquared)
	normal := delta.Normalize()
	if normal == (Vector3D{}) {
		normal = capsule.end.Sub(capsule.start).Cross(V3(1, 0, 0)).Normalize()
		if normal == (Vector3D{}) {
			normal = V3(0, 0, 1)
		}
	}
	return normal, sphere.center.Add(normal.Mul(sphere.radius)), radius - distance, true
}

func collideCapsuleCapsule(a, b worldCapsule) (Vector3D, Vector3D, float64, bool) {
	pointA, pointB := closestPointsOnSegments3D(a.start, a.end, b.start, b.end)
	delta := pointB.Sub(pointA)
	radius := a.radius + b.radius
	distanceSquared := delta.LengthSquared()
	if distanceSquared > radius*radius {
		return Vector3D{}, Vector3D{}, 0, false
	}
	distance := math.Sqrt(distanceSquared)
	normal := delta.Normalize()
	if normal == (Vector3D{}) {
		normal = b.start.Add(b.end).Sub(a.start.Add(a.end)).Normalize()
		if normal == (Vector3D{}) {
			normal = a.end.Sub(a.start).Cross(V3(1, 0, 0)).Normalize()
		}
		if normal == (Vector3D{}) {
			normal = V3(0, 0, 1)
		}
	}
	return normal, pointA.Add(normal.Mul(a.radius)), radius - distance, true
}

func collideBoxBox3D(a, b worldBox3D) (manifold3D, bool) {
	centerDelta := b.center.Sub(a.center)
	contact := manifold3D{penetration: math.MaxFloat64}
	feature := -1
	testAxis := func(axis Vector3D, index int) bool {
		lengthSquared := axis.LengthSquared()
		if lengthSquared <= 1e-12 {
			return true
		}
		axis = axis.Mul(1 / math.Sqrt(lengthSquared))
		overlap := projectedBoxRadius3D(a, axis) + projectedBoxRadius3D(b, axis) - math.Abs(centerDelta.Dot(axis))
		if overlap < 0 {
			return false
		}
		if overlap < contact.penetration-1e-9 {
			contact.penetration, contact.normal, feature = overlap, axis, index
			if centerDelta.Dot(axis) < 0 {
				contact.normal = axis.Mul(-1)
			}
		}
		return true
	}
	for index := range 3 {
		if !testAxis(a.axis[index], index) || !testAxis(b.axis[index], index+3) {
			return manifold3D{}, false
		}
	}
	for left := range 3 {
		for right := range 3 {
			if !testAxis(a.axis[left].Cross(b.axis[right]), 6+left*3+right) {
				return manifold3D{}, false
			}
		}
	}
	if feature < 3 {
		boxFaceContacts3D(&contact, a, b, feature, contact.normal)
	} else if feature < 6 {
		boxFaceContacts3D(&contact, b, a, feature-3, contact.normal.Mul(-1))
	} else {
		edge := feature - 6
		a0, a1 := boxEdge3D(a, edge/3, contact.normal)
		b0, b1 := boxEdge3D(b, edge%3, contact.normal.Mul(-1))
		p, q := closestPointsOnSegments3D(a0, a1, b0, b1)
		contact.point = p.Add(q).Mul(.5)
	}
	contact.initializePoints()
	return contact, true
}

func boxEdge3D(box worldBox3D, axis int, direction Vector3D) (Vector3D, Vector3D) {
	center := box.center
	for index := range 3 {
		if index != axis {
			center = center.Add(box.axis[index].Mul(math.Copysign(box.half.component(index), direction.Dot(box.axis[index]))))
		}
	}
	extent := box.axis[axis].Mul(box.half.component(axis))
	return center.Sub(extent), center.Add(extent)
}

// Clip the incident quad to the reference face and retain four spread-out
// support points. Fixed scratch bounds cover every intersection of two quads.
func boxFaceContacts3D(contact *manifold3D, reference, incident worldBox3D, face int, normal Vector3D) {
	center := reference.center.Add(normal.Mul(reference.half.component(face)))
	incidentFace := 0
	for index := 1; index < 3; index++ {
		if math.Abs(incident.axis[index].Dot(normal)) > math.Abs(incident.axis[incidentFace].Dot(normal)) {
			incidentFace = index
		}
	}
	incidentCenter := incident.center.Sub(incident.axis[incidentFace].Mul(math.Copysign(incident.half.component(incidentFace), incident.axis[incidentFace].Dot(normal))))
	u, v := (incidentFace+1)%3, (incidentFace+2)%3
	du, dv := incident.axis[u].Mul(incident.half.component(u)), incident.axis[v].Mul(incident.half.component(v))
	points := [16]Vector3D{incidentCenter.Sub(du).Sub(dv), incidentCenter.Add(du).Sub(dv), incidentCenter.Add(du).Add(dv), incidentCenter.Sub(du).Add(dv)}
	count := 4
	for _, index := range [2]int{(face + 1) % 3, (face + 2) % 3} {
		points, count = clipContactPolygon3D(points, count, center, reference.axis[index], reference.half.component(index))
		points, count = clipContactPolygon3D(points, count, center, reference.axis[index].Mul(-1), reference.half.component(index))
	}
	points, count = clipContactPolygon3D(points, count, center, normal, 0)
	var used [16]bool
	for slot := range min(count, len(contact.points)) {
		best, score := -1, -math.MaxFloat64
		for index := range count {
			if used[index] {
				continue
			}
			value := center.Sub(points[index]).Dot(normal)
			if slot > 0 {
				value = math.MaxFloat64
				for previous := range slot {
					value = min(value, points[index].Sub(contact.points[previous].position).LengthSquared())
				}
			}
			if value > score {
				best, score = index, value
			}
		}
		used[best] = true
		depth := center.Sub(points[best]).Dot(normal)
		point := points[best].Add(normal.Mul(depth * .5))
		contact.points[slot] = contactPoint3D{position: point, penetration: max(depth, 0)}
		contact.point = contact.point.Add(point)
		contact.pointCount++
	}
	if contact.pointCount > 0 {
		contact.point = contact.point.Mul(1 / float64(contact.pointCount))
	} else {
		contact.point = boxSupport3D(incident, normal.Mul(-1))
	}
}

func clipContactPolygon3D(points [16]Vector3D, count int, origin, normal Vector3D, offset float64) ([16]Vector3D, int) {
	var result [16]Vector3D
	if count == 0 {
		return result, 0
	}
	size := 0
	previous := points[count-1]
	previousDistance := previous.Sub(origin).Dot(normal) - offset
	for _, current := range points[:count] {
		distance := current.Sub(origin).Dot(normal) - offset
		if (distance <= 1e-9) != (previousDistance <= 1e-9) {
			result[size] = previous.Add(current.Sub(previous).Mul(clamp(previousDistance/(previousDistance-distance), 0, 1)))
			size++
		}
		if distance <= 1e-9 {
			result[size] = current
			size++
		}
		previous, previousDistance = current, distance
	}
	return result, size
}

func collideBoxCapsule(box worldBox3D, capsule worldCapsule) (Vector3D, Vector3D, float64, bool) {
	localStart, localEnd := boxToLocal(box, capsule.start), boxToLocal(box, capsule.end)
	boxPoint, axisPoint, inside := closestSegmentBox(localStart, localEnd, box.half)
	delta := axisPoint.Sub(boxPoint)
	distance := delta.Length()
	if !inside && distance > capsule.radius {
		return Vector3D{}, Vector3D{}, 0, false
	}
	localNormal := delta.Normalize()
	if localNormal == (Vector3D{}) {
		localNormal = V3(1, 0, 0)
	}
	normal := box.axis[0].Mul(localNormal.X).Add(box.axis[1].Mul(localNormal.Y)).Add(box.axis[2].Mul(localNormal.Z)).Normalize()
	penetration := capsule.radius - distance
	if inside {
		penetration = capsule.radius + distance
		normal = normal.Mul(-1)
		if distance <= 1e-12 {
			for index := range 3 {
				if math.Abs(boxPoint.component(index)) == box.half.component(index) {
					normal = box.axis[index].Mul(nonzeroSign(boxPoint.component(index)))
					break
				}
			}
		}
	}
	worldBoxPoint := boxToWorld(box, boxPoint)
	worldAxisPoint := boxToWorld(box, axisPoint)
	worldCapsulePoint := worldAxisPoint.Sub(normal.Mul(capsule.radius))
	return normal, worldBoxPoint.Add(worldCapsulePoint).Mul(.5), penetration, true
}

func boxCapsuleContacts3D(contact *manifold3D, box worldBox3D, capsule worldCapsule) {
	for _, center := range [2]Vector3D{capsule.start, capsule.end} {
		normal, point, depth, ok := collideSphereBox(worldSphere{center: center, radius: capsule.radius}, box)
		if !ok || normal.Dot(contact.normal) > -.99 {
			continue
		}
		if contact.pointCount == 0 || point.Sub(contact.points[0].position).LengthSquared() > 1e-12 {
			contact.points[contact.pointCount] = contactPoint3D{position: point, penetration: depth}
			contact.pointCount++
		}
	}
	if contact.pointCount > 0 {
		contact.point = contact.points[0].position
		if contact.pointCount == 2 {
			contact.point = contact.point.Add(contact.points[1].position).Mul(.5)
		}
	}
}

func projectedBoxRadius3D(box worldBox3D, axis Vector3D) float64 {
	return box.half.X*math.Abs(box.axis[0].Dot(axis)) +
		box.half.Y*math.Abs(box.axis[1].Dot(axis)) +
		box.half.Z*math.Abs(box.axis[2].Dot(axis))
}

func boxSupport3D(box worldBox3D, direction Vector3D) Vector3D {
	x := supportCoordinate(box.axis[0].Dot(direction), box.half.X)
	y := supportCoordinate(box.axis[1].Dot(direction), box.half.Y)
	z := supportCoordinate(box.axis[2].Dot(direction), box.half.Z)
	return box.center.Add(box.axis[0].Mul(x)).Add(box.axis[1].Mul(y)).Add(box.axis[2].Mul(z))
}

func supportCoordinate(projection, extent float64) float64 {
	if projection < -1e-9 {
		return -extent
	}
	if projection > 1e-9 {
		return extent
	}
	return 0
}

func boxToLocal(box worldBox3D, point Vector3D) Vector3D {
	delta := point.Sub(box.center)
	return V3(delta.Dot(box.axis[0]), delta.Dot(box.axis[1]), delta.Dot(box.axis[2]))
}

func boxToWorld(box worldBox3D, point Vector3D) Vector3D {
	return box.center.Add(box.axis[0].Mul(point.X)).Add(box.axis[1].Mul(point.Y)).Add(box.axis[2].Mul(point.Z))
}

func clampToBox(point, half Vector3D) Vector3D {
	return V3(
		clamp(point.X, -half.X, half.X),
		clamp(point.Y, -half.Y, half.Y),
		clamp(point.Z, -half.Z, half.Z),
	)
}

func nearestFacePoint(point, half Vector3D) Vector3D {
	result := point
	distanceX := half.X - math.Abs(point.X)
	distanceY := half.Y - math.Abs(point.Y)
	distanceZ := half.Z - math.Abs(point.Z)
	switch {
	case distanceX <= distanceY && distanceX <= distanceZ:
		result.X = math.Copysign(half.X, nonzeroSign(point.X))
	case distanceY <= distanceZ:
		result.Y = math.Copysign(half.Y, nonzeroSign(point.Y))
	default:
		result.Z = math.Copysign(half.Z, nonzeroSign(point.Z))
	}
	return result
}

func nonzeroSign(value float64) float64 {
	if value < 0 {
		return -1
	}
	return 1
}

func closestPointOnSegment3D(point, start, end Vector3D) Vector3D {
	delta := end.Sub(start)
	lengthSquared := delta.LengthSquared()
	if lengthSquared <= 1e-12 {
		return start
	}
	t := clamp(point.Sub(start).Dot(delta)/lengthSquared, 0, 1)
	return start.Add(delta.Mul(t))
}

func closestPointsOnSegments3D(a0, a1, b0, b1 Vector3D) (Vector3D, Vector3D) {
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

// closestSegmentBox finds the exact closest pair by minimizing each quadratic
// interval formed where the segment crosses an AABB slab.
func closestSegmentBox(start, end, half Vector3D) (Vector3D, Vector3D, bool) {
	if enter, exit, ok := segmentBoxInterval(start, end, half); ok {
		point := start.Add(end.Sub(start).Mul((enter + exit) * .5))
		return nearestFacePoint(point, half), point, true
	}
	delta := end.Sub(start)
	values := [8]float64{0, 1}
	count := 2
	for axis := range 3 {
		direction := delta.component(axis)
		if math.Abs(direction) <= 1e-12 {
			continue
		}
		origin, extent := start.component(axis), half.component(axis)
		for _, boundary := range [2]float64{-extent, extent} {
			t := (boundary - origin) / direction
			if t > 0 && t < 1 {
				values[count], count = t, count+1
			}
		}
	}
	slices.Sort(values[:count])
	bestDistance := math.MaxFloat64
	bestSegment, bestBox := start, clampToBox(start, half)
	evaluate := func(t float64) {
		point := start.Add(delta.Mul(clamp(t, 0, 1)))
		boxPoint := clampToBox(point, half)
		distance := point.Sub(boxPoint).LengthSquared()
		if distance < bestDistance {
			bestDistance, bestSegment, bestBox = distance, point, boxPoint
		}
	}
	for index := 0; index < count; index++ {
		evaluate(values[index])
		if index+1 == count || values[index+1]-values[index] <= 1e-12 {
			continue
		}
		low, high := values[index], values[index+1]
		midpoint := (low + high) * .5
		denominator, numerator := 0.0, 0.0
		for axis := range 3 {
			origin, direction, extent := start.component(axis), delta.component(axis), half.component(axis)
			value := origin + direction*midpoint
			if value < -extent {
				denominator += direction * direction
				numerator += direction * (origin + extent)
			} else if value > extent {
				denominator += direction * direction
				numerator += direction * (origin - extent)
			}
		}
		if denominator > 1e-12 {
			evaluate(clamp(-numerator/denominator, low, high))
		}
	}
	return bestBox, bestSegment, false
}

func segmentBoxInterval(start, end, half Vector3D) (float64, float64, bool) {
	enter, exit := 0.0, 1.0
	delta := end.Sub(start)
	for axis := range 3 {
		origin, direction, extent := start.component(axis), delta.component(axis), half.component(axis)
		if math.Abs(direction) <= 1e-12 {
			if origin < -extent || origin > extent {
				return 0, 0, false
			}
			continue
		}
		near, far := (-extent-origin)/direction, (extent-origin)/direction
		if near > far {
			near, far = far, near
		}
		enter, exit = max(enter, near), min(exit, far)
		if enter > exit {
			return 0, 0, false
		}
	}
	return enter, exit, true
}
