package physics

import "math"

func (body *bodyState3D) velocityAt(offset Vector3D) Vector3D {
	if body.bodyType == BodyStatic {
		return Vector3D{}
	}
	if body.fixedRotation {
		return body.velocity
	}
	return body.velocity.Add(body.angularVelocity.Cross(offset))
}

func (world *World3D) solveContacts(delta float64) {
	world.solveVelocities(delta, true)
	world.integratePositions(delta)
	world.buildContacts()
	world.solveVelocities(delta, false)
	for iteration := range max(world.Iterations, 1) {
		if iteration != 0 {
			world.buildContacts()
		}
		corrected := false
		for index := len(world.contacts) - 1; index >= 0; index-- {
			if !world.contacts[index].sensor {
				corrected = world.solvePosition(&world.contacts[index]) || corrected
			}
		}
		if !corrected {
			break
		}
	}
	world.buildContacts()
}

func (world *World3D) solveVelocities(delta float64, warmStart bool) {
	for index := range world.contacts {
		contact := &world.contacts[index]
		if contact.sensor {
			continue
		}
		a := &world.bodies[world.colliders[contact.pair.a].bodyIndex]
		b := &world.bodies[world.colliders[contact.pair.b].bodyIndex]
		threshold := max(1, 2*world.Gravity.Length()*delta*max(math.Abs(a.gravityScale), math.Abs(b.gravityScale)))
		for pointIndex := range contact.pointCount {
			point := &contact.points[pointIndex]
			speed := b.velocityAt(point.position.Sub(b.position)).Sub(a.velocityAt(point.position.Sub(a.position))).Dot(contact.normal)
			if speed < -threshold {
				point.bounceVelocity = -contact.restitution * speed
			}
		}
	}
	world.restoreContactImpulses(delta, warmStart)
	for range max(world.Iterations, 1) {
		for index := range world.contacts {
			if !world.contacts[index].sensor {
				world.solveVelocity(&world.contacts[index])
			}
		}
	}
	world.velocityContacts = append(world.velocityContacts[:0], world.contacts...)
	world.contactDelta = delta
}

func (world *World3D) restoreContactImpulses(delta float64, warmStart bool) {
	previousIndex := 0
	for index := range world.contacts {
		contact := &world.contacts[index]
		key := pairKey(contact.pair.a, contact.pair.b)
		for previousIndex < len(world.velocityContacts) && pairKey(world.velocityContacts[previousIndex].pair.a, world.velocityContacts[previousIndex].pair.b) < key {
			previousIndex++
		}
		if previousIndex == len(world.velocityContacts) {
			break
		}
		previous := &world.velocityContacts[previousIndex]
		if contact.sensor || previous.sensor || contact.pair != previous.pair || contact.generations != previous.generations ||
			contact.pointCount != previous.pointCount || contact.normal.Dot(previous.normal) < .99 {
			continue
		}
		a := &world.bodies[world.colliders[contact.pair.a].bodyIndex]
		b := &world.bodies[world.colliders[contact.pair.b].bodyIndex]
		scale := 1.0
		if warmStart && world.contactDelta > 0 {
			scale = delta / world.contactDelta
		}
		var used [4]bool
		for pointIndex := range contact.pointCount {
			point := &contact.points[pointIndex]
			best, distance := -1, math.MaxFloat64
			for oldIndex := range previous.pointCount {
				if next := point.position.Sub(previous.points[oldIndex].position).LengthSquared(); !used[oldIndex] && next < distance {
					best, distance = oldIndex, next
				}
			}
			used[best] = true
			old := &previous.points[best]
			point.normalImpulse = scale * old.normalImpulse
			point.tangentImpulse = old.tangentImpulse.Sub(contact.normal.Mul(old.tangentImpulse.Dot(contact.normal))).Mul(scale)
			if warmStart {
				impulse := contact.normal.Mul(point.normalImpulse).Add(point.tangentImpulse)
				applyPairImpulse3D(a, b, point.position.Sub(a.position), point.position.Sub(b.position), impulse)
			} else {
				point.bounceVelocity = max(point.bounceVelocity, old.bounceVelocity)
			}
		}
	}
}

func contactMass3D(a, b *bodyState3D, ra, rb, first, second Vector3D) float64 {
	return (a.inverseMass+b.inverseMass)*first.Dot(second) + ra.Cross(first).Dot(ra.Cross(second))*a.inverseInertia + rb.Cross(first).Dot(rb.Cross(second))*b.inverseInertia
}

func (world *World3D) solveVelocity(contact *manifold3D) {
	a := &world.bodies[world.colliders[contact.pair.a].bodyIndex]
	b := &world.bodies[world.colliders[contact.pair.b].bodyIndex]
	axis := V3(1, 0, 0)
	if math.Abs(contact.normal.X) > .8 {
		axis = V3(0, 1, 0)
	}
	u := contact.normal.Cross(axis).Normalize()
	v := contact.normal.Cross(u)
	for index := range contact.pointCount {
		point := &contact.points[index]
		ra, rb := point.position.Sub(a.position), point.position.Sub(b.position)
		relative := b.velocityAt(rb).Sub(a.velocityAt(ra))
		k11, k22, k12 := contactMass3D(a, b, ra, rb, u, u), contactMass3D(a, b, ra, rb, v, v), contactMass3D(a, b, ra, rb, u, v)
		determinant := k11*k22 - k12*k12
		if determinant <= 1e-12*k11*k22 || k11 <= 1e-12 || k22 <= 1e-12 {
			continue
		}
		su, sv := relative.Dot(u), relative.Dot(v)
		impulse := point.tangentImpulse.Add(u.Mul((k12*sv - k22*su) / determinant)).Add(v.Mul((k12*su - k11*sv) / determinant))
		if length, limit := impulse.Length(), contact.friction*point.normalImpulse; length > limit {
			impulse = impulse.Mul(limit / length)
		}
		applyPairImpulse3D(a, b, ra, rb, impulse.Sub(point.tangentImpulse))
		point.tangentImpulse = impulse
	}
	matrix := normalContactMass3D(a, b, contact)
	var rhs [4]float64
	for index := range contact.pointCount {
		point := &contact.points[index]
		rhs[index] = point.bounceVelocity - b.velocityAt(point.position.Sub(b.position)).Sub(a.velocityAt(point.position.Sub(a.position))).Dot(contact.normal)
		for other := range contact.pointCount {
			rhs[index] += matrix[index][other] * contact.points[other].normalImpulse
		}
	}
	if impulses, ok := solveContactBlock3D(matrix, rhs, contact.pointCount); ok {
		for index := range contact.pointCount {
			point := &contact.points[index]
			applyPairImpulse3D(a, b, point.position.Sub(a.position), point.position.Sub(b.position), contact.normal.Mul(impulses[index]-point.normalImpulse))
			point.normalImpulse = impulses[index]
		}
		return
	}
	for index := range contact.pointCount {
		point := &contact.points[index]
		ra, rb := point.position.Sub(a.position), point.position.Sub(b.position)
		if mass := matrix[index][index]; mass > 1e-12 {
			speed := b.velocityAt(rb).Sub(a.velocityAt(ra)).Dot(contact.normal)
			impulse := max(point.normalImpulse+(point.bounceVelocity-speed)/mass, 0)
			applyPairImpulse3D(a, b, ra, rb, contact.normal.Mul(impulse-point.normalImpulse))
			point.normalImpulse = impulse
		}
	}
}

func normalContactMass3D(a, b *bodyState3D, contact *manifold3D) [4][4]float64 {
	var ca, cb [4]Vector3D
	for index := range contact.pointCount {
		ca[index] = contact.points[index].position.Sub(a.position).Cross(contact.normal)
		cb[index] = contact.points[index].position.Sub(b.position).Cross(contact.normal)
	}
	var result [4][4]float64
	for row := range contact.pointCount {
		for column := range contact.pointCount {
			result[row][column] = a.inverseMass + b.inverseMass + ca[row].Dot(ca[column])*a.inverseInertia + cb[row].Dot(cb[column])*b.inverseInertia
		}
	}
	return result
}

// A planar patch has at most three independent normal constraints. Enumerate
// its small active sets so redundant face corners never make support singular.
func solveContactBlock3D(matrix [4][4]float64, rhs [4]float64, count int) ([4]float64, bool) {
	scale, tolerance := 0.0, 1e-9
	for index := range count {
		scale = max(scale, matrix[index][index])
		tolerance = max(tolerance, math.Abs(rhs[index])*1e-9)
	}
	for mask := (1 << count) - 1; mask >= 0; mask-- {
		var indices [4]int
		size := 0
		for index := range count {
			if mask&(1<<index) != 0 {
				indices[size], size = index, size+1
			}
		}
		var rows [4][5]float64
		for row := range size {
			for column := range size {
				rows[row][column] = matrix[indices[row]][indices[column]]
			}
			rows[row][size] = rhs[indices[row]]
		}
		valid := true
		for column := range size {
			pivot := column
			for row := column + 1; row < size; row++ {
				if math.Abs(rows[row][column]) > math.Abs(rows[pivot][column]) {
					pivot = row
				}
			}
			if math.Abs(rows[pivot][column]) <= max(1e-12, scale*1e-9) {
				valid = false
				break
			}
			rows[column], rows[pivot] = rows[pivot], rows[column]
			divisor := rows[column][column]
			for entry := column; entry <= size; entry++ {
				rows[column][entry] /= divisor
			}
			for row := range size {
				if row == column {
					continue
				}
				factor := rows[row][column]
				for entry := column; entry <= size; entry++ {
					rows[row][entry] -= factor * rows[column][entry]
				}
			}
		}
		var impulses [4]float64
		for row := range size {
			valid = valid && rows[row][size] >= -tolerance
			impulses[indices[row]] = max(rows[row][size], 0)
		}
		for row := range count {
			residual := -rhs[row]
			for column := range count {
				residual += matrix[row][column] * impulses[column]
			}
			valid = valid && residual >= -tolerance
		}
		if valid {
			return impulses, true
		}
	}
	return [4]float64{}, false
}

func applyPairImpulse3D(a, b *bodyState3D, ra, rb, impulse Vector3D) {
	a.velocity = a.velocity.Sub(impulse.Mul(a.inverseMass))
	a.angularVelocity = a.angularVelocity.Sub(ra.Cross(impulse).Mul(a.inverseInertia))
	b.velocity = b.velocity.Add(impulse.Mul(b.inverseMass))
	b.angularVelocity = b.angularVelocity.Add(rb.Cross(impulse).Mul(b.inverseInertia))
}

func (world *World3D) solvePosition(contact *manifold3D) bool {
	ca, cb := &world.colliders[contact.pair.a], &world.colliders[contact.pair.b]
	a, b := &world.bodies[ca.bodyIndex], &world.bodies[cb.bodyIndex]
	current, ok := collideShapes3D(contact.pair, *ca, *a, *cb, *b)
	slop := max(world.CollisionSlop, 0)
	if !ok || current.penetration-slop <= 1e-9 {
		return false
	}
	matrix := normalContactMass3D(a, b, &current)
	var rhs [4]float64
	for index := range current.pointCount {
		rhs[index] = current.points[index].penetration - slop + 1e-7
	}
	impulses, solved := solveContactBlock3D(matrix, rhs, current.pointCount)
	if !solved {
		for index := range current.pointCount {
			if matrix[index][index] > 1e-12 {
				impulses[index] = max(rhs[index], 0) / (matrix[index][index] * float64(current.pointCount))
			}
		}
	}
	var correction, angleA, angleB Vector3D
	for index := range current.pointCount {
		impulse := current.normal.Mul(impulses[index])
		correction = correction.Add(impulse)
		angleA = angleA.Add(current.points[index].position.Sub(a.position).Cross(impulse).Mul(a.inverseInertia))
		angleB = angleB.Add(current.points[index].position.Sub(b.position).Cross(impulse).Mul(b.inverseInertia))
	}
	if correction.LengthSquared() == 0 {
		return false
	}
	scale := 1.0
	if angle := max(angleA.Length(), angleB.Length()); angle > .2 {
		scale = .2 / angle
	}
	a.position = a.position.Sub(correction.Mul(scale * a.inverseMass))
	b.position = b.position.Add(correction.Mul(scale * b.inverseMass))
	a.rotation = integrateRotation(a.rotation, angleA, -scale)
	b.rotation = integrateRotation(b.rotation, angleB, scale)
	return true
}
