package physics

import "math"

// Traverse the cooked mesh in its own frame and combine aligned contacts
// across triangle seams before passing a single patch to the solver.
func collideBoxTriangleMesh(box worldBox3D, collider colliderState3D, body bodyState3D) (manifold3D, bool) {
	rotation := body.rotation.Mul(collider.rotation).Normalize()
	origin := body.position.Add(body.rotation.Rotate(collider.center))
	inverse := rotation.Conjugate()
	box.center = inverse.Rotate(box.center.Sub(origin))
	for i := range box.axis {
		box.axis[i] = inverse.Rotate(box.axis[i])
	}
	extent := V3(projectedBoxRadius3D(box, V3(1, 0, 0)), projectedBoxRadius3D(box, V3(0, 1, 0)), projectedBoxRadius3D(box, V3(0, 0, 1)))
	bounds := aabb3D{box.center.Sub(extent), box.center.Add(extent)}
	patch := manifold3D{penetration: -1}
	collider.mesh.visit(func(node Bounds3D) bool { return overlapAABB3D(bounds, boundsToAABB3D(node)) }, func(index uint32) {
		a, b, c := collider.mesh.triangle(index)
		contact, ok := boxTriangleContact(box, a, b, c)
		if !ok {
			return
		}
		if patch.pointCount == 0 || patch.normal.Dot(contact.normal) < 1-1e-7 {
			if contact.penetration > patch.penetration {
				patch = contact
			}
			return
		}
		patch.penetration = max(patch.penetration, contact.penetration)
		for i := range contact.pointCount {
			addConvexContactPoint3D(&patch, contact.points[i])
		}
	})
	if patch.pointCount == 0 {
		return manifold3D{}, false
	}
	patch.normal = rotation.Rotate(patch.normal)
	for i := range patch.pointCount {
		p := &patch.points[i]
		p.position = origin.Add(rotation.Rotate(p.position))
		patch.point = patch.point.Add(p.position)
	}
	patch.point = patch.point.Mul(1 / float64(patch.pointCount))
	return patch, true
}

func boxTriangleContact(box worldBox3D, a, b, c Vector3D) (manifold3D, bool) {
	vertices := [3]Vector3D{a, b, c}
	edges := [3]Vector3D{b.Sub(a), c.Sub(b), a.Sub(c)}
	face := edges[0].Cross(c.Sub(a)).Normalize()
	if face == (Vector3D{}) {
		return manifold3D{}, false
	}
	contact := manifold3D{penetration: math.Inf(1)}
	feature := 0 // triangle face, three box faces, then nine edge pairs
	test := func(axis Vector3D, index int) bool {
		p, q, r := a.Sub(box.center).Dot(axis), b.Sub(box.center).Dot(axis), c.Sub(box.center).Dot(axis)
		radius := projectedBoxRadius3D(box, axis)
		forward, backward := radius-min(p, q, r), radius+max(p, q, r)
		depth := min(forward, backward)
		if depth < -1e-10 {
			return false
		}
		if depth < contact.penetration-1e-9 {
			feature, contact.penetration, contact.normal = index, max(depth, 0), axis
			if backward < forward {
				contact.normal = axis.Mul(-1)
			}
		}
		return true
	}
	if !test(face, 0) {
		return manifold3D{}, false
	}
	for i, axis := range box.axis {
		if !test(axis, 1+i) {
			return manifold3D{}, false
		}
		for j, edge := range edges {
			cross := axis.Cross(edge)
			if length := cross.LengthSquared(); length > 1e-18*edge.LengthSquared() && !test(cross.Mul(1/math.Sqrt(length)), 4+3*i+j) {
				return manifold3D{}, false
			}
		}
	}

	normal := contact.normal
	var points [16]Vector3D
	var count int
	var plane Vector3D
	// The reference normal points towards the incident feature.
	reference := normal
	switch {
	case feature == 0:
		reference, plane = normal.Mul(-1), a
		points, count = incidentFaceQuad3D(box, reference), 4
		for i, edge := range edges {
			points, count = clipContactPolygon3D(points, count, vertices[i], edge.Cross(face).Normalize(), 0)
		}
	case feature <= 3:
		axis := feature - 1
		plane = box.center.Add(normal.Mul(box.half.component(axis)))
		points[0], points[1], points[2], count = a, b, c, 3
		for i, side := range box.axis {
			if i != axis {
				points, count = clipContactPolygon3D(points, count, box.center, side, box.half.component(i))
				points, count = clipContactPolygon3D(points, count, box.center, side.Mul(-1), box.half.component(i))
			}
		}
	default:
		i, j := (feature-4)/3, (feature-4)%3
		p, q := boxEdge3D(box, i, normal)
		start, end := vertices[j], vertices[(j+1)%3]
		if tip := vertices[(j+2)%3]; tip.Sub(start).Dot(normal) < -1e-9 {
			start, end = tip, tip
		}
		p, q = closestPointsOnSegments3D(p, q, start, end)
		contact.points[0] = contactPoint3D{position: p.Add(q).Mul(.5), penetration: contact.penetration}
		contact.pointCount = 1
		return contact, true
	}
	points, count = clipContactPolygon3D(points, count, plane, reference, 0)
	for _, point := range points[:count] {
		depth := max(plane.Sub(point).Dot(reference), 0)
		addConvexContactPoint3D(&contact, contactPoint3D{position: point.Add(reference.Mul(.5 * depth)), penetration: depth})
	}
	if contact.pointCount == 0 {
		// A grazing feature can fall just outside the clipping tolerance.
		point := closestPointTriangle3D(boxSupport3D(box, normal), a, b, c)
		contact.points[0] = contactPoint3D{position: point, penetration: contact.penetration}
		contact.pointCount = 1
	}
	return contact, true
}
