package game

import (
	"math"
	"testing"
)

// Shared test helpers for the game package. A file named *_test.go is compiled
// only into the test binary, so nothing here ships in the real build.

// eps is the slack allowed when comparing floats. Floating point arithmetic is
// not exact, so tests never use == on a float they computed.
const eps = 1e-9

// approxEqual fails the test unless got and want are within eps.
// t.Helper() makes a failure report the caller's line, not this one.
func approxEqual(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s = %v, want %v (diff %v)", label, got, want, math.Abs(got-want))
	}
}

// approxVec is the same check applied to both components of a Vec.
func approxVec(t *testing.T, got, want Vec, label string) {
	t.Helper()
	if math.Abs(got.X-want.X) > eps || math.Abs(got.Y-want.Y) > eps {
		t.Errorf("%s = %+v, want %+v", label, got, want)
	}
}

// sign reports -1, 0 or +1 for f, treating values within eps of zero as zero.
func sign(f float64) float64 {
	switch {
	case f > eps:
		return +1
	case f < -eps:
		return -1
	default:
		return 0
	}
}
