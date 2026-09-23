package game

import (
	"math"
	"testing"
)

// TestVecAdd checks component-wise addition, including the two cases that
// define what addition means: adding zero changes nothing, and adding the
// opposite vector gets you back to zero.
func TestVecAdd(t *testing.T) {
	tests := []struct {
		name string
		v, o Vec
		want Vec
	}{
		{"positive components", Vec{1, 2}, Vec{3, 4}, Vec{4, 6}},
		{"mixed signs", Vec{-1, -2}, Vec{3, 4}, Vec{2, 2}},
		{"zero is the identity", Vec{5, 7}, Vec{0, 0}, Vec{5, 7}},
		{"opposite cancels to zero", Vec{5, 7}, Vec{-5, -7}, Vec{0, 0}},
		{"fractional", Vec{0.1, 0.2}, Vec{0.2, 0.3}, Vec{0.3, 0.5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approxVec(t, tt.v.Add(tt.o), tt.want, "Add")
		})
	}
}

// TestVecSub checks component-wise subtraction.
func TestVecSub(t *testing.T) {
	tests := []struct {
		name string
		v, o Vec
		want Vec
	}{
		{"basic", Vec{5, 7}, Vec{2, 3}, Vec{3, 4}},
		{"zero is the identity", Vec{5, 7}, Vec{0, 0}, Vec{5, 7}},
		{"self cancels to zero", Vec{5, 7}, Vec{5, 7}, Vec{0, 0}},
		{"result goes negative", Vec{1, 1}, Vec{4, 5}, Vec{-3, -4}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approxVec(t, tt.v.Sub(tt.o), tt.want, "Sub")
		})
	}
}

// TestVecSubPointsFromOperandToReceiver pins the direction convention promised
// by Sub's doc comment: a.Sub(b) is the vector that carries you from b to a.
// Get this backwards and every "which way is the enemy" calculation in the game
// comes out reversed, so it is worth a test of its own.
func TestVecSubPointsFromOperandToReceiver(t *testing.T) {
	a := Vec{10, 10}
	b := Vec{4, 6}

	delta := a.Sub(b)

	// Starting at b and travelling along delta must land exactly on a.
	approxVec(t, b.Add(delta), a, "b + (a-b)")
}

// TestVecScale covers resizing, including the three factors with special
// meaning: 1 leaves the vector alone, 0 collapses it, -1 reverses it.
func TestVecScale(t *testing.T) {
	tests := []struct {
		name string
		v    Vec
		s    float64
		want Vec
	}{
		{"double", Vec{3, 4}, 2, Vec{6, 8}},
		{"half", Vec{3, 4}, 0.5, Vec{1.5, 2}},
		{"one is the identity", Vec{3, 4}, 1, Vec{3, 4}},
		{"zero collapses", Vec{3, 4}, 0, Vec{0, 0}},
		{"negative one reverses", Vec{3, 4}, -1, Vec{-3, -4}},
		{"negative scales and reverses", Vec{3, 4}, -2, Vec{-6, -8}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approxVec(t, tt.v.Scale(tt.s), tt.want, "Scale")
		})
	}
}

// TestVecScaleChangesLengthProportionally states the property behind Scale:
// scaling by s multiplies the length by |s|. This is what the speed clamp in
// Player.Update relies on.
func TestVecScaleChangesLengthProportionally(t *testing.T) {
	v := Vec{3, 4} // length 5

	approxEqual(t, v.Scale(3).Len(), 15, "length after Scale(3)")
	approxEqual(t, v.Scale(-3).Len(), 15, "length after Scale(-3)")
}

// TestVecLen uses triangles with whole-number answers so a failure is readable.
func TestVecLen(t *testing.T) {
	tests := []struct {
		name string
		v    Vec
		want float64
	}{
		{"3-4-5 triangle", Vec{3, 4}, 5},
		{"negative components", Vec{-3, -4}, 5},
		{"zero vector", Vec{0, 0}, 0},
		{"along x axis", Vec{7, 0}, 7},
		{"along negative y axis", Vec{0, -7}, 7},
		{"unit vector", Vec{1, 0}, 1},
		{"5-12-13 triangle", Vec{5, 12}, 13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approxEqual(t, tt.v.Len(), tt.want, "Len")
		})
	}
}

// TestVecLenIsNeverNegative: length is a magnitude, so no combination of signs
// should ever produce a negative result.
func TestVecLenIsNeverNegative(t *testing.T) {
	for _, v := range []Vec{{3, 4}, {-3, 4}, {3, -4}, {-3, -4}, {0, 0}} {
		if got := v.Len(); got < 0 {
			t.Errorf("Vec%+v.Len() = %v, want >= 0", v, got)
		}
	}
}

// TestVecNormalized checks the expected unit vector for each input.
func TestVecNormalized(t *testing.T) {
	tests := []struct {
		name string
		v    Vec
		want Vec
	}{
		{"along x axis", Vec{10, 0}, Vec{1, 0}},
		{"along negative y axis", Vec{0, -4}, Vec{0, -1}},
		{"3-4-5 triangle", Vec{3, 4}, Vec{0.6, 0.8}},
		{"already unit length", Vec{1, 0}, Vec{1, 0}},
		{"negative components", Vec{-3, -4}, Vec{-0.6, -0.8}},
		{"very small vector", Vec{0.003, 0.004}, Vec{0.6, 0.8}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approxVec(t, tt.v.Normalized(), tt.want, "Normalized")
		})
	}
}

// TestVecNormalizedZeroVector is the guard clause that keeps the whole game
// free of NaN. Without the l == 0 early return this divides 0 by 0, producing
// NaN, and NaN spreads through every later calculation and never clears.
// Player.Update calls Normalized on the zero Vec on every tick a player is
// standing still, so this is the common case, not an edge case.
func TestVecNormalizedZeroVector(t *testing.T) {
	got := Vec{0, 0}.Normalized()

	if math.IsNaN(got.X) || math.IsNaN(got.Y) {
		t.Fatalf("Normalized() on the zero Vec = %+v, want no NaN", got)
	}
	approxVec(t, got, Vec{0, 0}, "Normalized")
}

// TestVecNormalizedHasUnitLength states the defining property: whatever goes
// in, what comes out has length 1 (or is the zero Vec).
func TestVecNormalizedHasUnitLength(t *testing.T) {
	for _, v := range []Vec{{3, 4}, {-17, 0}, {0.001, 0.002}, {1e6, -1e6}, {1, 1}} {
		if got := v.Normalized().Len(); math.Abs(got-1) > eps {
			t.Errorf("Vec%+v.Normalized().Len() = %v, want 1", v, got)
		}
	}
}

// TestVecNormalizedPreservesDirection: the unit vector must point the same way
// as the original, not the opposite way. A sign slip inside Normalized would
// still pass the unit-length test above, so it needs checking separately.
//
// Two vectors are parallel when their cross product is zero, and point the
// same way (rather than opposite ways) when their dot product is positive.
func TestVecNormalizedPreservesDirection(t *testing.T) {
	for _, v := range []Vec{{3, 4}, {-3, 4}, {3, -4}, {-3, -4}, {0, 9}} {
		n := v.Normalized()

		if cross := v.X*n.Y - v.Y*n.X; math.Abs(cross) > eps {
			t.Errorf("Vec%+v.Normalized() = %+v is not parallel to it (cross = %v)", v, n, cross)
		}
		if dot := v.X*n.X + v.Y*n.Y; dot <= 0 {
			t.Errorf("Vec%+v.Normalized() = %+v points the wrong way (dot = %v)", v, n, dot)
		}
	}
}

// TestVecMethodsDoNotMutateReceiver backs up the promise in the Vec type
// comment. Value receivers make this true automatically, but the test pins the
// guarantee so it survives someone later switching a method to a pointer
// receiver, which would silently change the meaning of every call site.
func TestVecMethodsDoNotMutateReceiver(t *testing.T) {
	v := Vec{3, 4}
	other := Vec{1, 1}

	v.Add(other)
	v.Sub(other)
	v.Scale(10)
	v.Len()
	v.Normalized()

	approxVec(t, v, Vec{3, 4}, "receiver after five method calls")
}

// FuzzVecNormalized throws random components at Normalized and asserts the
// invariant that matters: the result is either the zero Vec or exactly unit
// length, and is never NaN.
//
//	go test ./internal/game -run Fuzz -fuzz FuzzVecNormalized -fuzztime 30s
func FuzzVecNormalized(f *testing.F) {
	f.Add(3.0, 4.0)
	f.Add(0.0, 0.0)
	f.Add(-1.0, 0.0)

	f.Fuzz(func(t *testing.T, x, y float64) {
		v := Vec{x, y}

		// Len overflows to +Inf once a component passes roughly 1.3e154,
		// because it squares before taking the root. Arena coordinates live in
		// the thousands, so that range is out of contract rather than a bug.
		l := v.Len()
		if math.IsNaN(l) || math.IsInf(l, 0) {
			t.Skip()
		}

		n := v.Normalized()

		if math.IsNaN(n.X) || math.IsNaN(n.Y) {
			t.Fatalf("Vec%+v.Normalized() = %+v, want no NaN", v, n)
		}
		if l == 0 {
			approxVec(t, n, Vec{0, 0}, "Normalized of zero-length Vec")
			return
		}
		if got := n.Len(); math.Abs(got-1) > 1e-9 {
			t.Errorf("Vec%+v.Normalized().Len() = %v, want 1", v, got)
		}
	})
}
