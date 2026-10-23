package physics

import "math"

func boundsToAABB3D(bounds Bounds3D) aabb3D {
	return aabb3D{min: bounds.Min, max: bounds.Max}
}

func worldBounds3D(collider colliderState3D, body bodyState3D, local Bounds3D) Bounds3D {
	if !local.Valid() {
		return Bounds3D{}
	}
	result := emptyBounds3D()
	for mask := 0; mask < 8; mask++ {
		point := V3(local.Min.X, local.Min.Y, local.Min.Z)
		if mask&1 != 0 {
			point.X = local.Max.X
		}
		if mask&2 != 0 {
			point.Y = local.Max.Y
		}
		if mask&4 != 0 {
			point.Z = local.Max.Z
		}
		result = result.expand(worldPoint3D(collider, body, point))
	}
	return result
}

func worldPoint3D(collider colliderState3D, body bodyState3D, point Vector3D) Vector3D {
	return body.position.Add(body.rotation.Rotate(collider.center.Add(collider.rotation.Rotate(point))))
}

func collideCustomShapes3D(contact manifold3D, colliderA colliderState3D, bodyA bodyState3D, colliderB colliderState3D, bodyB bodyState3D) (manifold3D, bool, bool) {
	customA := isCookedShape3D(colliderA.kind)
	customB := isCookedShape3D(colliderB.kind)
	if !customA && !customB {
		return contact, false, false
	}
	if colliderA.kind == shapeCapsule3D && colliderB.kind == shapeTriangleMesh3D {
		contact.normal, contact.point, contact.penetration = collideCapsuleTriangleMesh(makeWorldCapsule(colliderA, bodyA), colliderB, bodyB)
		return contact, contact.penetration >= 0, true
	}
	if colliderB.kind == shapeCapsule3D && colliderA.kind == shapeTriangleMesh3D {
		contact.normal, contact.point, contact.penetration = collideCapsuleTriangleMesh(makeWorldCapsule(colliderB, bodyB), colliderA, bodyA)
		contact.normal = contact.normal.Mul(-1)
		return contact, contact.penetration >= 0, true
	}
	if colliderA.kind == shapeSphere3D && customB {
		contact.normal, contact.point, contact.penetration, _ = collideSphereCooked(makeWorldSphere(colliderA, bodyA), colliderB, bodyB)
		return contact, contact.penetration >= 0, true
	}
	if colliderB.kind == shapeSphere3D && customA {
		contact.normal, contact.point, contact.penetration, _ = collideSphereCooked(makeWorldSphere(colliderB, bodyB), colliderA, bodyA)
		contact.normal = contact.normal.Mul(-1)
		return contact, contact.penetration >= 0, true
	}
	if colliderA.kind == shapeConvexHull3D && colliderB.kind == shapeConvexHull3D {
		result, ok := collideConvex3D(makeWorldHull3D(colliderA, bodyA), makeWorldHull3D(colliderB, bodyB))
		result.pair = contact.pair
		return result, ok, true
	}
	if colliderA.kind == shapeBox3D && colliderB.kind == shapeConvexHull3D {
		result, ok := collideConvex3D(makeWorldBox3D(colliderA, bodyA), makeWorldHull3D(colliderB, bodyB))
		result.pair = contact.pair
		return result, ok, true
	}
	if colliderA.kind == shapeConvexHull3D && colliderB.kind == shapeBox3D {
		result, ok := collideConvex3D(makeWorldHull3D(colliderA, bodyA), makeWorldBox3D(colliderB, bodyB))
		result.pair = contact.pair
		return result, ok, true
	}
	if colliderA.kind == shapeCompound3D && colliderB.kind == shapeConvexHull3D {
		return collideCompoundHull(contact, colliderA, bodyA, colliderB, bodyB)
	}
	if colliderA.kind == shapeConvexHull3D && colliderB.kind == shapeCompound3D {
		result, ok, handled := collideCompoundHull(contact, colliderB, bodyB, colliderA, bodyA)
		result.normal = result.normal.Mul(-1)
		return result, ok, handled
	}
	// Two concave triangle meshes are intentionally not collidable. Every
	// other unsupported pair uses the cooked bounds as a conservative fallback
	// until a convex support narrow phase is requested for that pair.
	if colliderA.kind == shapeTriangleMesh3D && colliderB.kind == shapeTriangleMesh3D {
		return contact, false, true
	}
	contact.normal, contact.point, contact.penetration = collideAABBShapes3D(colliderA, bodyA, colliderB, bodyB)
	return contact, contact.penetration >= 0, true
}

func collideCompoundHull(contact manifold3D, compoundState colliderState3D, compoundBody bodyState3D, hullState colliderState3D, hullBody bodyState3D) (manifold3D, bool, bool) {
	contact.penetration = -1
	for _, child := range compoundState.compound.Children {
		childState := colliderState3D{
			kind:     shapeConvexHull3D,
			center:   compoundState.center.Add(compoundState.rotation.Rotate(child.Position)),
			rotation: compoundState.rotation.Mul(child.Rotation).Normalize(),
			hull:     child.Shape,
		}
		current, ok := collideConvex3D(makeWorldHull3D(childState, compoundBody), makeWorldHull3D(hullState, hullBody))
		if ok && current.penetration > contact.penetration {
			current.pair = contact.pair
			contact = current
		}
	}
	return contact, contact.penetration >= 0, true
}

func isCookedShape3D(kind shapeKind3D) bool {
	return kind >= shapeTriangleMesh3D
}

func collideAABBShapes3D(colliderA colliderState3D, bodyA bodyState3D, colliderB colliderState3D, bodyB bodyState3D) (Vector3D, Vector3D, float64) {
	a, b := colliderBounds3D(colliderA, bodyA), colliderBounds3D(colliderB, bodyB)
	overlapX := min(a.max.X, b.max.X) - max(a.min.X, b.min.X)
	overlapY := min(a.max.Y, b.max.Y) - max(a.min.Y, b.min.Y)
	overlapZ := min(a.max.Z, b.max.Z) - max(a.min.Z, b.min.Z)
	if overlapX < 0 || overlapY < 0 || overlapZ < 0 {
		return Vector3D{}, Vector3D{}, -1
	}
	centerA := a.min.Add(a.max).Mul(.5)
	centerB := b.min.Add(b.max).Mul(.5)
	minimum, normal := overlapX, V3(1, 0, 0)
	if overlapY < minimum {
		minimum, normal = overlapY, V3(0, 1, 0)
	}
	if overlapZ < minimum {
		minimum, normal = overlapZ, V3(0, 0, 1)
	}
	if centerB.Sub(centerA).Dot(normal) < 0 {
		normal = normal.Mul(-1)
	}
	point := V3(
		(max(a.min.X, b.min.X)+min(a.max.X, b.max.X))*.5,
		(max(a.min.Y, b.min.Y)+min(a.max.Y, b.max.Y))*.5,
		(max(a.min.Z, b.min.Z)+min(a.max.Z, b.max.Z))*.5,
	)
	return normal, point, minimum
}

func collideSphereCooked(sphere worldSphere, collider colliderState3D, body bodyState3D) (Vector3D, Vector3D, float64, bool) {
	switch collider.kind {
	case shapeTriangleMesh3D:
		return collideSphereTriangleMesh(sphere, collider.mesh, collider, body)
	case shapeConvexHull3D:
		return collideSphereHull(sphere, collider.hull, collider, body)
	case shapeCompound3D:
		bestPenetration := -1.0
		var bestNormal, bestPoint Vector3D
		for _, child := range collider.compound.Children {
			childCollider := colliderState3D{
				kind:     shapeConvexHull3D,
				center:   collider.center.Add(collider.rotation.Rotate(child.Position)),
				rotation: collider.rotation.Mul(child.Rotation).Normalize(),
				hull:     child.Shape,
			}
			normal, point, penetration, ok := collideSphereHull(sphere, child.Shape, childCollider, body)
			if ok && penetration > bestPenetration {
				bestNormal, bestPoint, bestPenetration = normal, point, penetration
			}
		}
		return bestNormal, bestPoint, bestPenetration, bestPenetration >= 0
	default:
		return Vector3D{}, Vector3D{}, -1, false
	}
}

func collideSphereTriangleMesh(sphere worldSphere, mesh TriangleMesh, collider colliderState3D, body bodyState3D) (Vector3D, Vector3D, float64, bool) {
	bestDistanceSquared := math.Inf(1)
	var bestPoint, bestNormal Vector3D
	testTriangle := func(triangle uint32) {
		base := int(triangle) * 3
		if base+2 >= len(mesh.Indices) {
			return
		}
		a := worldPoint3D(collider, body, mesh.Vertices[mesh.Indices[base]])
		b := worldPoint3D(collider, body, mesh.Vertices[mesh.Indices[base+1]])
		c := worldPoint3D(collider, body, mesh.Vertices[mesh.Indices[base+2]])
		normal := b.Sub(a).Cross(c.Sub(a)).Normalize()
		if normal == (Vector3D{}) || normal.Dot(sphere.center.Sub(a)) < -1e-9 {
			return
		}
		point := closestPointTriangle3D(sphere.center, a, b, c)
		delta := point.Sub(sphere.center)
		distanceSquared := delta.LengthSquared()
		if distanceSquared > sphere.radius*sphere.radius || distanceSquared >= bestDistanceSquared {
			return
		}
		bestDistanceSquared, bestPoint = distanceSquared, point
		bestNormal = delta.Normalize()
		if bestNormal == (Vector3D{}) {
			bestNormal = normal.Mul(-1)
		}
	}
	if len(mesh.BVH) != 0 {
		var stack [128]int32
		count := 1
		stack[0] = 0
		for count > 0 {
			count--
			node := mesh.BVH[stack[count]]
			worldNodeBounds := worldBounds3D(collider, body, node.Bounds)
			if distanceSquaredBounds3D(sphere.center, worldNodeBounds) > sphere.radius*sphere.radius {
				continue
			}
			if node.Count != 0 {
				for index := uint32(0); index < node.Count; index++ {
					testTriangle(mesh.TriangleIndices[node.First+index])
				}
				continue
			}
			if count+2 > len(stack) {
				continue
			}
			stack[count], count = node.Left, count+1
			stack[count], count = node.Right, count+1
		}
	} else {
		for triangle := 0; triangle < len(mesh.Indices)/3; triangle++ {
			testTriangle(uint32(triangle))
		}
	}
	if math.IsInf(bestDistanceSquared, 1) {
		return Vector3D{}, Vector3D{}, -1, false
	}
	distance := math.Sqrt(bestDistanceSquared)
	return bestNormal, bestPoint, sphere.radius - distance, true
}

func collideSphereHull(sphere worldSphere, hull ConvexHull, collider colliderState3D, body bodyState3D) (Vector3D, Vector3D, float64, bool) {
	bestDistanceSquared := math.Inf(1)
	var bestPoint, bestNormal Vector3D
	separation := math.Inf(-1)
	var exitNormal Vector3D
	rotation := body.rotation.Mul(collider.rotation).Normalize()
	for _, face := range hull.Faces {
		a := worldPoint3D(collider, body, hull.Vertices[face.Indices[0]])
		faceNormal := rotation.Rotate(face.Normal)
		if distance := sphere.center.Sub(a).Dot(faceNormal); distance > separation {
			separation, exitNormal = distance, faceNormal
		}
		b := worldPoint3D(collider, body, hull.Vertices[face.Indices[1]])
		c := worldPoint3D(collider, body, hull.Vertices[face.Indices[2]])
		point := closestPointTriangle3D(sphere.center, a, b, c)
		delta := point.Sub(sphere.center)
		distanceSquared := delta.LengthSquared()
		if distanceSquared > sphere.radius*sphere.radius || distanceSquared >= bestDistanceSquared {
			continue
		}
		bestDistanceSquared, bestPoint = distanceSquared, point
		bestNormal = delta.Normalize()
		if bestNormal == (Vector3D{}) {
			bestNormal = faceNormal.Mul(-1)
		}
	}
	if separation <= 0 && !math.IsInf(separation, -1) {
		return exitNormal.Mul(-1), sphere.center.Sub(exitNormal.Mul(separation)), sphere.radius - separation, true
	}
	if math.IsInf(bestDistanceSquared, 1) {
		return Vector3D{}, Vector3D{}, -1, false
	}
	return bestNormal, bestPoint, sphere.radius - math.Sqrt(bestDistanceSquared), true
}

func distanceSquaredBounds3D(point Vector3D, bounds Bounds3D) float64 {
	delta := Vector3D{
		X: max(bounds.Min.X-point.X, 0, point.X-bounds.Max.X),
		Y: max(bounds.Min.Y-point.Y, 0, point.Y-bounds.Max.Y),
		Z: max(bounds.Min.Z-point.Z, 0, point.Z-bounds.Max.Z),
	}
	return delta.LengthSquared()
}

func closestPointTriangle3D(point, a, b, c Vector3D) Vector3D {
	ab, ac, ap := b.Sub(a), c.Sub(a), point.Sub(a)
	d1, d2 := ab.Dot(ap), ac.Dot(ap)
	if d1 <= 0 && d2 <= 0 {
		return a
	}
	bp := point.Sub(b)
	d3, d4 := ab.Dot(bp), ac.Dot(bp)
	if d3 >= 0 && d4 <= d3 {
		return b
	}
	vc := d1*d4 - d3*d2
	if vc <= 0 && d1 >= 0 && d3 <= 0 {
		return a.Add(ab.Mul(d1 / (d1 - d3)))
	}
	cp := point.Sub(c)
	d5, d6 := ab.Dot(cp), ac.Dot(cp)
	if d6 >= 0 && d5 <= d6 {
		return c
	}
	vb := d5*d2 - d1*d6
	if vb <= 0 && d2 >= 0 && d6 <= 0 {
		return a.Add(ac.Mul(d2 / (d2 - d6)))
	}
	va := d3*d6 - d5*d4
	if va <= 0 && d4-d3 >= 0 && d5-d6 >= 0 {
		return b.Add(c.Sub(b).Mul((d4 - d3) / ((d4 - d3) + (d5 - d6))))
	}
	denominator := 1 / (va + vb + vc)
	v, w := vb*denominator, vc*denominator
	return a.Add(ab.Mul(v)).Add(ac.Mul(w))
}
