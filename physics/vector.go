package physics

import "math"

// Vector is the scalar-independent two-dimensional value used by the physics
// runtime. The solver uses float64 so long simulations do not accumulate the
// visible drift caused by render-space float32 transforms.
type Vector struct {
	X float64
	Y float64
}

func V(x, y float64) Vector { return Vector{X: x, Y: y} }

func (v Vector) Add(other Vector) Vector   { return Vector{X: v.X + other.X, Y: v.Y + other.Y} }
func (v Vector) Sub(other Vector) Vector   { return Vector{X: v.X - other.X, Y: v.Y - other.Y} }
func (v Vector) Mul(scalar float64) Vector { return Vector{X: v.X * scalar, Y: v.Y * scalar} }
func (v Vector) Dot(other Vector) float64  { return v.X*other.X + v.Y*other.Y }
func (v Vector) LengthSquared() float64    { return v.Dot(v) }
func (v Vector) Length() float64           { return math.Sqrt(v.LengthSquared()) }

func (v Vector) Normalize() Vector {
	length := v.Length()
	if length <= 1e-12 {
		return Vector{}
	}
	return v.Mul(1 / length)
}

func (v Vector) Perpendicular() Vector { return Vector{X: -v.Y, Y: v.X} }

func cross(a, b Vector) float64 { return a.X*b.Y - a.Y*b.X }

func crossScalarVector(value float64, vector Vector) Vector {
	return Vector{X: -value * vector.Y, Y: value * vector.X}
}

func rotate(vector Vector, angle float64) Vector {
	sine, cosine := math.Sincos(angle)
	return Vector{X: cosine*vector.X - sine*vector.Y, Y: sine*vector.X + cosine*vector.Y}
}

func clamp(value, low, high float64) float64 {
	return min(max(value, low), high)
}
