package physics

import "math"

type convexPolyhedron3D interface {
	vertexCount() int
	vertex(int) Vector3D
	edgeCount() int
	edge(int) (Vector3D, Vector3D)
	faceCount() int
	face(int) (Vector3D, Vector3D)
}

type worldHull3D struct {
	hull     ConvexHull
	position Vector3D
	rotation Quaternion
}

func makeWorldHull3D(collider colliderState3D, body bodyState3D) worldHull3D {
	return worldHull3D{hull: collider.hull, position: worldPoint3D(collider, body, Vector3D{}), rotation: body.rotation.Mul(collider.rotation).Normalize()}
}

func (h worldHull3D) vertexCount() int { return len(h.hull.Vertices) }
func (h worldHull3D) vertex(index int) Vector3D {
	return h.position.Add(h.rotation.Rotate(h.hull.Vertices[index]))
}
func (h worldHull3D) edgeCount() int { return len(h.hull.Edges) }
func (h worldHull3D) edge(index int) (Vector3D, Vector3D) {
	edge := h.hull.Edges[index]
	return h.vertex(int(edge[0])), h.vertex(int(edge[1]))
}
func (h worldHull3D) faceCount() int { return len(h.hull.Faces) }
func (h worldHull3D) face(index int) (Vector3D, Vector3D) {
	face := h.hull.Faces[index]
	return h.vertex(int(face.Indices[0])), h.rotation.Rotate(face.Normal)
}

func (b worldBox3D) vertexCount() int { return 8 }
func (b worldBox3D) vertex(index int) Vector3D {
	return boxToWorld(b, V3(float64(2*(index&1)-1)*b.half.X, float64(2*((index>>1)&1)-1)*b.half.Y, float64(2*((index>>2)&1)-1)*b.half.Z))
}
func (b worldBox3D) edgeCount() int { return 12 }
func (b worldBox3D) edge(index int) (Vector3D, Vector3D) {
	axis, signs := index/4, index%4
	center := b.center.Add(b.axis[(axis+1)%3].Mul(float64(2*(signs&1)-1) * b.half.component((axis+1)%3))).Add(b.axis[(axis+2)%3].Mul(float64(2*((signs>>1)&1)-1) * b.half.component((axis+2)%3)))
	extent := b.axis[axis].Mul(b.half.component(axis))
	return center.Sub(extent), center.Add(extent)
}
func (b worldBox3D) faceCount() int { return 6 }
func (b worldBox3D) face(index int) (Vector3D, Vector3D) {
	normal := b.axis[index/2].Mul(float64(2*(index&1) - 1))
	return b.center.Add(normal.Mul(b.half.component(index / 2))), normal
}

func projectConvex3D[P convexPolyhedron3D](shape P, axis Vector3D) (float64, float64) {
	low, high := math.Inf(1), math.Inf(-1)
	for index := range shape.vertexCount() {
		value := shape.vertex(index).Dot(axis)
		low, high = min(low, value), max(high, value)
	}
	return low, high
}

func collideConvex3D[A, B convexPolyhedron3D](a A, b B) (manifold3D, bool) {
	if a.vertexCount() == 0 || b.vertexCount() == 0 || a.faceCount() == 0 || b.faceCount() == 0 {
		return manifold3D{}, false
	}
	contact := manifold3D{penetration: math.Inf(1)}
	edgeA, edgeB := -1, -1
	testAxis := func(axis Vector3D, first, second int) bool {
		if axis.LengthSquared() <= 1e-18 {
			return true
		}
		axis = axis.Normalize()
		minA, maxA := projectConvex3D(a, axis)
		minB, maxB := projectConvex3D(b, axis)
		forward, backward := maxA-minB, maxB-minA
		if min(forward, backward) < 0 {
			return false
		}
		if overlap := min(forward, backward); overlap < contact.penetration-1e-9 {
			contact.penetration, contact.normal = overlap, axis
			if backward < forward {
				contact.normal = axis.Mul(-1)
			}
			edgeA, edgeB = first, second
		}
		return true
	}
	for index := range a.faceCount() {
		_, normal := a.face(index)
		if !testAxis(normal, -1, -1) {
			return manifold3D{}, false
		}
	}
	for index := range b.faceCount() {
		_, normal := b.face(index)
		if !testAxis(normal, -1, -1) {
			return manifold3D{}, false
		}
	}
	for first := range a.edgeCount() {
		a0, a1 := a.edge(first)
		for second := range b.edgeCount() {
			b0, b1 := b.edge(second)
			if !testAxis(a1.Sub(a0).Cross(b1.Sub(b0)), first, second) {
				return manifold3D{}, false
			}
		}
	}
	if edgeA >= 0 {
		a0, a1 := a.edge(edgeA)
		b0, b1 := b.edge(edgeB)
		a0, a1 = supportEdge3D(a, a1.Sub(a0), contact.normal)
		b0, b1 = supportEdge3D(b, b1.Sub(b0), contact.normal.Mul(-1))
		p, q := closestPointsOnSegments3D(a0, a1, b0, b1)
		contact.point = p.Add(q).Mul(.5)
	} else {
		_, alignmentA := convexFace3D(a, contact.normal)
		_, alignmentB := convexFace3D(b, contact.normal.Mul(-1))
		if alignmentA >= alignmentB {
			convexFaceContacts3D(&contact, a, b, contact.normal)
		} else {
			convexFaceContacts3D(&contact, b, a, contact.normal.Mul(-1))
		}
	}
	contact.initializePoints()
	return contact, true
}

func supportEdge3D[P convexPolyhedron3D](shape P, direction, normal Vector3D) (Vector3D, Vector3D) {
	var start, end Vector3D
	best := math.Inf(-1)
	for index := range shape.edgeCount() {
		a, b := shape.edge(index)
		delta := b.Sub(a)
		if delta.Cross(direction).LengthSquared() > 1e-12*delta.LengthSquared()*direction.LengthSquared() {
			continue
		}
		if score := a.Add(b).Dot(normal); score > best {
			start, end, best = a, b, score
		}
	}
	return start, end
}

func convexFace3D[P convexPolyhedron3D](shape P, direction Vector3D) (int, float64) {
	best, alignment := 0, math.Inf(-1)
	for index := range shape.faceCount() {
		_, normal := shape.face(index)
		if score := normal.Dot(direction); score > alignment {
			best, alignment = index, score
		}
	}
	return best, alignment
}

// Intersection vertices lie on an edge of either polyhedron. Clipping edges
// avoids variable-size polygon scratch even for hulls with many coplanar faces.
func convexFaceContacts3D[A, B convexPolyhedron3D](contact *manifold3D, reference A, incident B, normal Vector3D) {
	face, _ := convexFace3D(incident, normal.Mul(-1))
	origin, incidentNormal := incident.face(face)
	_, plane := projectConvex3D(reference, normal)
	add := func(point Vector3D) {
		if math.Abs(point.Sub(origin).Dot(incidentNormal)) > 1e-7 {
			return
		}
		depth := max(plane-point.Dot(normal), 0)
		addConvexContactPoint3D(contact, contactPoint3D{position: point.Add(normal.Mul(depth * .5)), penetration: depth})
	}
	for index := range incident.edgeCount() {
		a, b := incident.edge(index)
		if start, end, ok := clipConvexEdge3D(reference, a, b); ok {
			add(start)
			add(end)
		}
	}
	for index := range reference.edgeCount() {
		a, b := reference.edge(index)
		if start, end, ok := clipConvexEdge3D(incident, a, b); ok {
			add(start)
			add(end)
		}
	}
	if contact.pointCount == 0 {
		// SAT can retain an edge touch a few ULPs outside the clipping planes.
		best := math.Inf(1)
		for index := range incident.vertexCount() {
			point := incident.vertex(index)
			if projection := point.Dot(normal); projection < best {
				contact.point, best = point, projection
			}
		}
		return
	}
	for index := range contact.pointCount {
		contact.point = contact.point.Add(contact.points[index].position)
	}
	contact.point = contact.point.Mul(1 / float64(contact.pointCount))
}

func clipConvexEdge3D[P convexPolyhedron3D](shape P, start, end Vector3D) (Vector3D, Vector3D, bool) {
	delta := end.Sub(start)
	enter, exit := 0.0, 1.0
	for index := range shape.faceCount() {
		origin, normal := shape.face(index)
		distance, slope := start.Sub(origin).Dot(normal), delta.Dot(normal)
		if math.Abs(slope) <= 1e-12 {
			if distance > 1e-9 {
				return Vector3D{}, Vector3D{}, false
			}
			continue
		}
		if slope > 0 {
			exit = min(exit, -distance/slope)
		} else {
			enter = max(enter, -distance/slope)
		}
		if enter > exit+1e-9 {
			return Vector3D{}, Vector3D{}, false
		}
	}
	return start.Add(delta.Mul(clamp(enter, 0, 1))), start.Add(delta.Mul(clamp(exit, 0, 1))), true
}

func addConvexContactPoint3D(contact *manifold3D, point contactPoint3D) {
	var candidates [5]contactPoint3D
	for index := range contact.pointCount {
		if contact.points[index].position.Sub(point.position).LengthSquared() < 1e-14 {
			return
		}
		candidates[index] = contact.points[index]
	}
	candidates[contact.pointCount] = point
	count := contact.pointCount + 1
	var used [5]bool
	for slot := range min(count, len(contact.points)) {
		best, score := -1, math.Inf(-1)
		for index := range count {
			if used[index] {
				continue
			}
			value := candidates[index].penetration
			if slot > 0 {
				value = math.Inf(1)
				for previous := range slot {
					value = min(value, candidates[index].position.Sub(contact.points[previous].position).LengthSquared())
				}
			}
			if value > score {
				best, score = index, value
			}
		}
		used[best], contact.points[slot] = true, candidates[best]
	}
	contact.pointCount = min(count, len(contact.points))
}
