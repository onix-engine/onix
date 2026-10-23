package physics

import "math"

// MeshRayHit is a hit in the triangle mesh's local coordinate system.
type MeshRayHit struct {
	Distance      float64
	Point, Normal Vector3D
	Triangle      uint32
}

// Raycast returns the closest two-sided triangle hit. Direction must be unit
// length. The cooked BVH and traversal stack are reused without allocation.
func (mesh TriangleMesh) Raycast(origin, direction Vector3D, limit float64) (MeshRayHit, bool) {
	hit := MeshRayHit{Distance: limit}
	if !mesh.Cooked() || limit <= 0 || !finiteVector3D(origin) || !finiteVector3D(direction) {
		return hit, false
	}
	found := false
	mesh.visit(func(bounds Bounds3D) bool { return rayBounds3D(origin, direction, bounds, hit.Distance) }, func(triangle uint32) {
		a, b, c := mesh.triangle(triangle)
		distance, ok := rayTriangle3D(origin, direction, a, b, c)
		if ok && distance <= hit.Distance {
			normal := b.Sub(a).Cross(c.Sub(a)).Normalize()
			if normal.Dot(direction) > 0 {
				normal = normal.Mul(-1)
			}
			hit = MeshRayHit{distance, origin.Add(direction.Mul(distance)), normal, triangle}
			found = true
		}
	})
	return hit, found
}

// OverlapsCapsule tests a capsule given its segment endpoints and radius in
// mesh-local space. It tests actual triangles, including edges and vertices.
func (mesh TriangleMesh) OverlapsCapsule(start, end Vector3D, radius float64) bool {
	_, _, depth := mesh.capsuleContact(start, end, radius)
	return depth >= 0
}

func (mesh TriangleMesh) triangle(index uint32) (Vector3D, Vector3D, Vector3D) {
	i := int(index) * 3
	return mesh.Vertices[mesh.Indices[i]], mesh.Vertices[mesh.Indices[i+1]], mesh.Vertices[mesh.Indices[i+2]]
}

func (mesh TriangleMesh) visit(intersects func(Bounds3D) bool, triangle func(uint32)) {
	if !mesh.Cooked() {
		return
	}
	var stack [64]int32 // A balanced binary tree with uint32 triangles is at most 32 levels deep.
	count := 1
	for count > 0 {
		count--
		node := mesh.BVH[stack[count]]
		if !intersects(node.Bounds) {
			continue
		}
		if node.Count != 0 {
			for i := uint32(0); i < node.Count; i++ {
				triangle(mesh.TriangleIndices[node.First+i])
			}
		} else {
			stack[count], stack[count+1] = node.Left, node.Right
			count += 2
		}
	}
}

func rayBounds3D(origin, direction Vector3D, bounds Bounds3D, limit float64) bool {
	near, far := 0.0, limit
	for _, axis := range [3][4]float64{{origin.X, direction.X, bounds.Min.X, bounds.Max.X}, {origin.Y, direction.Y, bounds.Min.Y, bounds.Max.Y}, {origin.Z, direction.Z, bounds.Min.Z, bounds.Max.Z}} {
		if math.Abs(axis[1]) < 1e-12 {
			if axis[0] < axis[2] || axis[0] > axis[3] {
				return false
			}
			continue
		}
		a, b := (axis[2]-axis[0])/axis[1], (axis[3]-axis[0])/axis[1]
		near, far = max(near, min(a, b)), min(far, max(a, b))
		if near > far {
			return false
		}
	}
	return true
}

func rayTriangle3D(origin, direction, a, b, c Vector3D) (float64, bool) {
	e1, e2 := b.Sub(a), c.Sub(a)
	p := direction.Cross(e2)
	det := e1.Dot(p)
	if math.Abs(det) < 1e-12 {
		return 0, false
	}
	t := origin.Sub(a)
	u := t.Dot(p) / det
	if u < 0 || u > 1 {
		return 0, false
	}
	q := t.Cross(e1)
	v := direction.Dot(q) / det
	if v < 0 || u+v > 1 {
		return 0, false
	}
	distance := e2.Dot(q) / det
	return distance, distance >= 0
}

func (mesh TriangleMesh) capsuleContact(start, end Vector3D, radius float64) (normal, point Vector3D, depth float64) {
	depth = -1
	if radius < 0 || !finiteVector3D(start) || !finiteVector3D(end) {
		return
	}
	r := V3(radius, radius, radius)
	bounds := Bounds3D{Min: V3(min(start.X, end.X), min(start.Y, end.Y), min(start.Z, end.Z)).Sub(r), Max: V3(max(start.X, end.X), max(start.Y, end.Y), max(start.Z, end.Z)).Add(r)}
	mesh.visit(func(b Bounds3D) bool {
		return b.Min.X <= bounds.Max.X && b.Max.X >= bounds.Min.X && b.Min.Y <= bounds.Max.Y && b.Max.Y >= bounds.Min.Y && b.Min.Z <= bounds.Max.Z && b.Max.Z >= bounds.Min.Z
	}, func(triangle uint32) {
		a, b, c := mesh.triangle(triangle)
		axis, surface := closestSegmentTriangle3D(start, end, a, b, c)
		delta := surface.Sub(axis)
		distance := delta.Length()
		penetration := radius - distance
		if penetration < 0 || penetration <= depth {
			return
		}
		n := delta.Normalize()
		if distance < 1e-10 {
			n = b.Sub(a).Cross(c.Sub(a)).Normalize()
			if n.Dot(start.Add(end).Mul(.5).Sub(a)) >= 0 {
				n = n.Mul(-1)
			}
		}
		normal, point, depth = n, surface, penetration
	})
	return
}

func closestSegmentTriangle3D(start, end, a, b, c Vector3D) (axis, surface Vector3D) {
	direction := end.Sub(start)
	if t, ok := rayTriangle3D(start, direction, a, b, c); ok && t <= 1 {
		p := start.Add(direction.Mul(t))
		return p, p
	}
	axis, surface = start, closestPointTriangle3D(start, a, b, c)
	best := axis.Sub(surface).LengthSquared()
	p := closestPointTriangle3D(end, a, b, c)
	if d := end.Sub(p).LengthSquared(); d < best {
		axis, surface, best = end, p, d
	}
	for _, edge := range [3][2]Vector3D{{a, b}, {b, c}, {c, a}} {
		p, q := closestPointsOnSegments3D(start, end, edge[0], edge[1])
		if d := p.Sub(q).LengthSquared(); d < best {
			axis, surface, best = p, q, d
		}
	}
	return
}

func collideCapsuleTriangleMesh(capsule worldCapsule, collider colliderState3D, body bodyState3D) (Vector3D, Vector3D, float64) {
	rotation := body.rotation.Mul(collider.rotation).Normalize()
	origin := body.position.Add(body.rotation.Rotate(collider.center))
	inverse := rotation.Conjugate()
	normal, point, depth := collider.mesh.capsuleContact(inverse.Rotate(capsule.start.Sub(origin)), inverse.Rotate(capsule.end.Sub(origin)), capsule.radius)
	return rotation.Rotate(normal), origin.Add(rotation.Rotate(point)), depth
}
