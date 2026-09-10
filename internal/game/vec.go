package game

import "math"

// Vec is a 2D vector. Used for positions, velocities, and directions.
// Methods are on the same value receiver: a vec is small and always copied.
type Vec struct {
	X, Y float64
}

// Add returns the component-wise sum v+o. It does not modify v.
func (v Vec) Add(o Vec) Vec {
	return Vec{v.X + o.X, v.Y + o.Y}
}

// Sub returns the component-wise difference v-o.
// Direction matters: a.Sub(b) points from b towards a.
func (v Vec) Sub(o Vec) Vec {
	return Vec{v.X - o.X, v.Y - o.Y}
}

// Scale returns v with both components multiplied by s.
// Scaling by a negative s reverses the direction.
func (v Vec) Scale(s float64) Vec {
	return Vec{v.X * s, v.Y * s}
}

// Len returns the magnitude of v.
func (v Vec) Len() float64 {
	return math.Sqrt((v.X * v.X) + (v.Y * v.Y))
}

// Normalized returns a unit vector in the same direction.
// It must return the zero Vec if v is the zero Vec (do not divide by zero).
func (v Vec) Normalized() Vec {
	l := v.Len()
	if l == 0 {
		return Vec{}
	}
	return Vec{v.X / l, v.Y / l}
}
