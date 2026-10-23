package physics

import "math"

// Vector3D is the float64 simulation-space vector used by the 3D solver.
type Vector3D struct {
	X float64
	Y float64
	Z float64
}

func V3(x, y, z float64) Vector3D { return Vector3D{X: x, Y: y, Z: z} }

func (v Vector3D) Add(other Vector3D) Vector3D {
	return Vector3D{X: v.X + other.X, Y: v.Y + other.Y, Z: v.Z + other.Z}
}

func (v Vector3D) Sub(other Vector3D) Vector3D {
	return Vector3D{X: v.X - other.X, Y: v.Y - other.Y, Z: v.Z - other.Z}
}

func (v Vector3D) Mul(scalar float64) Vector3D {
	return Vector3D{X: v.X * scalar, Y: v.Y * scalar, Z: v.Z * scalar}
}

func (v Vector3D) Dot(other Vector3D) float64 {
	return v.X*other.X + v.Y*other.Y + v.Z*other.Z
}

func (v Vector3D) Cross(other Vector3D) Vector3D {
	return Vector3D{
		X: v.Y*other.Z - v.Z*other.Y,
		Y: v.Z*other.X - v.X*other.Z,
		Z: v.X*other.Y - v.Y*other.X,
	}
}

func (v Vector3D) LengthSquared() float64 { return v.Dot(v) }
func (v Vector3D) Length() float64        { return math.Sqrt(v.LengthSquared()) }

func (v Vector3D) Normalize() Vector3D {
	length := v.Length()
	if length <= 1e-12 {
		return Vector3D{}
	}
	return v.Mul(1 / length)
}

func (v Vector3D) component(index int) float64 {
	switch index {
	case 0:
		return v.X
	case 1:
		return v.Y
	default:
		return v.Z
	}
}

// Quaternion stores an orientation as an (x, y, z, w) unit quaternion.
type Quaternion struct {
	X float64
	Y float64
	Z float64
	W float64
}

func Q(x, y, z, w float64) Quaternion { return Quaternion{X: x, Y: y, Z: z, W: w} }
func QuaternionIdentity() Quaternion  { return Quaternion{W: 1} }

func QuaternionFromAxisAngle(axis Vector3D, angle float64) Quaternion {
	axis = axis.Normalize()
	if axis == (Vector3D{}) {
		return QuaternionIdentity()
	}
	sine, cosine := math.Sincos(angle * .5)
	return Quaternion{X: axis.X * sine, Y: axis.Y * sine, Z: axis.Z * sine, W: cosine}
}

func (q Quaternion) Normalize() Quaternion {
	lengthSquared := q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W
	if lengthSquared <= 1e-24 {
		return QuaternionIdentity()
	}
	scale := 1 / math.Sqrt(lengthSquared)
	return Quaternion{X: q.X * scale, Y: q.Y * scale, Z: q.Z * scale, W: q.W * scale}
}

func (q Quaternion) Conjugate() Quaternion {
	return Quaternion{X: -q.X, Y: -q.Y, Z: -q.Z, W: q.W}
}

func (q Quaternion) Mul(other Quaternion) Quaternion {
	return Quaternion{
		X: q.W*other.X + q.X*other.W + q.Y*other.Z - q.Z*other.Y,
		Y: q.W*other.Y - q.X*other.Z + q.Y*other.W + q.Z*other.X,
		Z: q.W*other.Z + q.X*other.Y - q.Y*other.X + q.Z*other.W,
		W: q.W*other.W - q.X*other.X - q.Y*other.Y - q.Z*other.Z,
	}
}

func (q Quaternion) Rotate(vector Vector3D) Vector3D {
	q = q.Normalize()
	part := Vector3D{X: q.X, Y: q.Y, Z: q.Z}
	twiceCross := part.Cross(vector).Mul(2)
	return vector.Add(twiceCross.Mul(q.W)).Add(part.Cross(twiceCross))
}

func (q Quaternion) inverseRotate(vector Vector3D) Vector3D {
	return q.Normalize().Conjugate().Rotate(vector)
}

func integrateRotation(rotation Quaternion, angularVelocity Vector3D, delta float64) Quaternion {
	spin := Quaternion{X: angularVelocity.X, Y: angularVelocity.Y, Z: angularVelocity.Z}.Mul(rotation)
	return Quaternion{
		X: rotation.X + spin.X*delta*.5,
		Y: rotation.Y + spin.Y*delta*.5,
		Z: rotation.Z + spin.Z*delta*.5,
		W: rotation.W + spin.W*delta*.5,
	}.Normalize()
}
