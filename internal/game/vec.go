package game

import "math"

// Vec is a 2D vector, used for positions, velocities, and directions.
// Every method takes a value receiver and returns a new Vec: a Vec is small,
// always copied, and never mutated in place.
type Vec struct {
	X, Y float64
}

// Add returns the component-wise sum of v and o. Neither operand is modified.
func (v Vec) Add(o Vec) Vec {
	return Vec{v.X + o.X, v.Y + o.Y}
}

// Sub returns the component-wise difference v-o. Neither operand is modified.
// Direction matters: a.Sub(b) points from b towards a.
func (v Vec) Sub(o Vec) Vec {
	return Vec{v.X - o.X, v.Y - o.Y}
}

// Scale returns v with both components multiplied by s, which resizes it
// without turning it. A negative s reverses the direction; an s of 0 gives the
// zero Vec. v is unmodified.
func (v Vec) Scale(s float64) Vec {
	return Vec{v.X * s, v.Y * s}
}

// Len returns the magnitude of v, which is never negative.
func (v Vec) Len() float64 {
	return math.Sqrt((v.X * v.X) + (v.Y * v.Y))
}

// Normalized returns a Vec of length 1 pointing the same way as v, or the zero
// Vec if v is the zero Vec. v is unmodified.
func (v Vec) Normalized() Vec {
	l := v.Len()
	if l == 0 {
		return v
	}

	return Vec{v.X / l, v.Y / l}
}
