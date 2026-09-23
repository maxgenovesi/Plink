package game

import (
	"math"
	"testing"
)

// tick is the server's fixed timestep: 60 updates per second.
const tick = 1.0 / 60.0

// The arena the ClampTo tests run in. Any size comfortably larger than
// 2*PlayerRadius works; these are the Phase 1 dimensions.
const (
	arenaW = 800.0
	arenaH = 600.0
)

// TestPlayerUpdateSingleTickFromRest pins the exact arithmetic of one tick.
// The constants are chosen so the numbers come out round:
//
//	accel     = 5400 * 1/60      = 90
//	damped    = 90 * (1 - 8/60)  = 78
//	position  = 78 * 1/60        = 1.3
//
// This is the test that breaks loudly if the order of operations in Update
// changes (for example, if position were integrated before damping).
func TestPlayerUpdateSingleTickFromRest(t *testing.T) {
	p := Player{ID: "p1"}

	p.Update(Input{Right: true}, tick)

	approxVec(t, p.Vel, Vec{78, 0}, "Vel")
	approxVec(t, p.Pos, Vec{1.3, 0}, "Pos")
}

// TestPlayerUpdateDirections is a table-driven test: one slice of cases, one
// loop, one subtest per case. This is the standard Go shape — adding a case is
// one line, and `go test -run 'TestPlayerUpdateDirections/up'` runs just one.
func TestPlayerUpdateDirections(t *testing.T) {
	// wantX/wantY are the *signs* we expect: -1, 0 or +1. The magnitude is
	// already covered by the exact test above, so here we only care about
	// which way the player went.
	tests := []struct {
		name         string
		in           Input
		wantX, wantY float64
	}{
		{"up", Input{Up: true}, 0, -1},
		{"down", Input{Down: true}, 0, +1},
		{"left", Input{Left: true}, -1, 0},
		{"right", Input{Right: true}, +1, 0},
		{"up right", Input{Up: true, Right: true}, +1, -1},
		{"down left", Input{Down: true, Left: true}, -1, +1},
		{"idle", Input{}, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Player{ID: "p1"}

			p.Update(tt.in, tick)

			if got := sign(p.Vel.X); got != tt.wantX {
				t.Errorf("sign(Vel.X) = %v, want %v (Vel = %+v)", got, tt.wantX, p.Vel)
			}
			if got := sign(p.Vel.Y); got != tt.wantY {
				t.Errorf("sign(Vel.Y) = %v, want %v (Vel = %+v)", got, tt.wantY, p.Vel)
			}
			// Position must always follow velocity in the same direction.
			if sign(p.Pos.X) != sign(p.Vel.X) || sign(p.Pos.Y) != sign(p.Vel.Y) {
				t.Errorf("Pos %+v does not follow Vel %+v", p.Pos, p.Vel)
			}
		})
	}
}

// TestPlayerUpdateDiagonalIsNotFaster guards the Normalized() call in Update.
// Without it, holding W+D would move you sqrt(2) times faster than holding D —
// the classic "diagonal strafing" bug.
func TestPlayerUpdateDiagonalIsNotFaster(t *testing.T) {
	straight := Player{ID: "p1"}
	diagonal := Player{ID: "p2"}

	straight.Update(Input{Right: true}, tick)
	diagonal.Update(Input{Right: true, Up: true}, tick)

	approxEqual(t, diagonal.Vel.Len(), straight.Vel.Len(), "diagonal speed")
}

// TestPlayerUpdateOppositeKeysCancel: holding both Left and Right is the same
// as holding neither. A key-cancelling bug here would make the player drift.
func TestPlayerUpdateOppositeKeysCancel(t *testing.T) {
	p := Player{ID: "p1"}

	p.Update(Input{Left: true, Right: true, Up: true, Down: true}, tick)

	approxVec(t, p.Vel, Vec{0, 0}, "Vel")
	approxVec(t, p.Pos, Vec{0, 0}, "Pos")
}

// TestPlayerUpdateFrictionDecaysVelocity: with no keys held, a moving player
// loses PlayerFriction of its speed per second and eventually stops.
func TestPlayerUpdateFrictionDecaysVelocity(t *testing.T) {
	p := Player{ID: "p1", Vel: Vec{100, 0}}

	p.Update(Input{}, tick)

	// 100 * (1 - 8/60) = 86.666...
	approxEqual(t, p.Vel.X, 100*(1-PlayerFriction*tick), "Vel.X after one tick")

	// Over a few seconds of no input the player should be very nearly stopped.
	for range 600 {
		p.Update(Input{}, tick)
	}
	if p.Vel.Len() > 0.01 {
		t.Errorf("after 10s of no input Vel = %+v, want ~zero", p.Vel)
	}
}

// TestPlayerUpdateClampsToMaxSpeed holds a key long enough to hit terminal
// velocity and checks the player never exceeds the limit on any tick.
func TestPlayerUpdateClampsToMaxSpeed(t *testing.T) {
	p := Player{ID: "p1"}

	for i := range 600 {
		p.Update(Input{Right: true}, tick)
		if speed := p.Vel.Len(); speed > PlayerMaxSpeed+eps {
			t.Fatalf("tick %d: speed %v exceeds PlayerMaxSpeed %v", i, speed, PlayerMaxSpeed)
		}
	}

	// Acceleration outruns friction here, so the clamp is what decides the
	// steady state: the player should be pinned exactly at the limit.
	approxEqual(t, p.Vel.Len(), PlayerMaxSpeed, "terminal speed")
}

// TestPlayerUpdateZeroDtIsNoOp: a tick of no elapsed time must change nothing.
// Worth having because dt reaching zero is a realistic edge case on a laggy or
// paused server loop.
func TestPlayerUpdateZeroDtIsNoOp(t *testing.T) {
	before := Player{ID: "p1", Pos: Vec{10, 20}, Vel: Vec{3, 4}}
	after := before

	after.Update(Input{Up: true}, 0)

	approxVec(t, after.Pos, before.Pos, "Pos")
	approxVec(t, after.Vel, before.Vel, "Vel")
}

// TestPlayerUpdateLargeDtDoesNotReverseVelocity guards the max(0, ...) in the
// friction step. With dt = 1s the naive damping factor is 1 - 8*1 = -7, which
// would fling a player backwards at seven times their speed after one long
// stall. It must clamp to a dead stop instead.
func TestPlayerUpdateLargeDtDoesNotReverseVelocity(t *testing.T) {
	p := Player{ID: "p1", Vel: Vec{100, 0}}

	p.Update(Input{}, 1.0)

	if p.Vel.X < 0 {
		t.Errorf("Vel reversed under a large dt: %+v", p.Vel)
	}
	approxVec(t, p.Vel, Vec{0, 0}, "Vel")
}

// TestPlayerUpdateIsDeterministic is the property the doc comment on Update
// promises, and the one the whole server design leans on: same state + same
// input + same dt produces the same result, every time.
func TestPlayerUpdateIsDeterministic(t *testing.T) {
	start := Player{ID: "p1", Pos: Vec{5, -7}, Vel: Vec{31, 12}}
	in := Input{Up: true, Right: true}

	a, b := start, start
	for range 100 {
		a.Update(in, tick)
		b.Update(in, tick)
	}

	if a != b {
		t.Errorf("divergence: %+v vs %+v", a, b)
	}
}

// TestPlayerClampToInsideArena: a player nowhere near a wall must come out
// byte-identical. A clamp that "helpfully" adjusts something in the common case
// would be far worse than one that fails at the edges.
func TestPlayerClampToInsideArena(t *testing.T) {
	before := Player{ID: "p1", Pos: Vec{400, 300}, Vel: Vec{50, -25}}
	after := before

	after.ClampTo(arenaW, arenaH)

	if after != before {
		t.Errorf("ClampTo modified a player inside the arena: %+v, want %+v", after, before)
	}
}

// TestPlayerClampToWalls checks each wall in turn. The want values are written
// in terms of PlayerRadius on purpose: Pos is the circle's *centre*, so a
// clamp to a bare 0 or w would leave half the player outside the arena.
func TestPlayerClampToWalls(t *testing.T) {
	tests := []struct {
		name             string
		pos, vel         Vec
		wantPos, wantVel Vec
	}{
		{
			"past the left wall",
			Vec{-50, 300}, Vec{-200, 0},
			Vec{PlayerRadius, 300}, Vec{0, 0},
		},
		{
			"past the right wall",
			Vec{900, 300}, Vec{200, 0},
			Vec{arenaW - PlayerRadius, 300}, Vec{0, 0},
		},
		{
			"past the top wall",
			Vec{400, -20}, Vec{0, -200},
			Vec{400, PlayerRadius}, Vec{0, 0},
		},
		{
			"past the bottom wall",
			Vec{400, 700}, Vec{0, 200},
			Vec{400, arenaH - PlayerRadius}, Vec{0, 0},
		},
		{
			// Exactly on the boundary is inside, not out: nothing is touched.
			// Without this the bounds could drift to <= and nobody would notice.
			"resting exactly against the wall",
			Vec{PlayerRadius, 300}, Vec{-5, 40},
			Vec{PlayerRadius, 300}, Vec{-5, 40},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Player{ID: "p1", Pos: tt.pos, Vel: tt.vel}

			p.ClampTo(arenaW, arenaH)

			approxVec(t, p.Pos, tt.wantPos, "Pos")
			approxVec(t, p.Vel, tt.wantVel, "Vel")
		})
	}
}

// TestPlayerClampToSlidesAlongWall is the case that justifies clearing one
// component instead of the whole vector.
//
// Holding left+down against the left wall, the player should stop moving left
// but keep sliding down. Zeroing all of Vel pins them in place, so holding two
// keys would move you *less* than holding one -- a bug you can only find by
// playing, which is exactly why it is worth a test.
func TestPlayerClampToSlidesAlongWall(t *testing.T) {
	p := Player{ID: "p1", Pos: Vec{-10, 300}, Vel: Vec{-200, 150}}

	p.ClampTo(arenaW, arenaH)

	approxEqual(t, p.Pos.X, PlayerRadius, "Pos.X (pushed back inside)")
	approxEqual(t, p.Vel.X, 0, "Vel.X (into the wall, killed)")

	approxEqual(t, p.Pos.Y, 300, "Pos.Y (untouched)")
	approxEqual(t, p.Vel.Y, 150, "Vel.Y (along the wall, preserved)")
}

// TestPlayerClampToCorners: two walls at once zeroes both components, but only
// because two independent checks each fired -- not because the whole vector is
// thrown away.
func TestPlayerClampToCorners(t *testing.T) {
	tests := []struct {
		name    string
		pos     Vec
		wantPos Vec
	}{
		{"top left", Vec{-10, -10}, Vec{PlayerRadius, PlayerRadius}},
		{"top right", Vec{900, -10}, Vec{arenaW - PlayerRadius, PlayerRadius}},
		{"bottom left", Vec{-10, 700}, Vec{PlayerRadius, arenaH - PlayerRadius}},
		{"bottom right", Vec{900, 700}, Vec{arenaW - PlayerRadius, arenaH - PlayerRadius}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Player{ID: "p1", Pos: tt.pos, Vel: Vec{-300, -300}}

			p.ClampTo(arenaW, arenaH)

			approxVec(t, p.Pos, tt.wantPos, "Pos")
			approxVec(t, p.Vel, Vec{0, 0}, "Vel")
		})
	}
}

// TestPlayerUpdateThenClampStaysInArena runs the two together in the order the
// server's tick loop will: Update, then ClampTo, every tick. Holding into a
// corner for ten seconds must never once put the player outside the arena, and
// must settle exactly in the corner rather than jittering against it.
func TestPlayerUpdateThenClampStaysInArena(t *testing.T) {
	p := Player{ID: "p1", Pos: Vec{arenaW / 2, arenaH / 2}}

	for i := range 600 {
		p.Update(Input{Up: true, Left: true}, tick)
		p.ClampTo(arenaW, arenaH)

		if p.Pos.X < PlayerRadius-eps || p.Pos.X > arenaW-PlayerRadius+eps ||
			p.Pos.Y < PlayerRadius-eps || p.Pos.Y > arenaH-PlayerRadius+eps {
			t.Fatalf("tick %d: Pos %+v is outside the arena", i, p.Pos)
		}
	}

	approxVec(t, p.Pos, Vec{PlayerRadius, PlayerRadius}, "resting Pos")
	approxVec(t, p.Vel, Vec{0, 0}, "resting Vel")
}

// FuzzPlayerUpdate throws random inputs at Update and asserts the invariants
// that must hold no matter what: the state stays finite, and the speed limit is
// never breached. Run the generated corpus with:
//
//	go test ./internal/game -run Fuzz -fuzz FuzzPlayerUpdate -fuzztime 30s
func FuzzPlayerUpdate(f *testing.F) {
	f.Add(true, false, false, false, tick)
	f.Add(true, false, true, false, 0.5)
	f.Add(false, false, false, false, 0.0)

	f.Fuzz(func(t *testing.T, up, down, left, right bool, dt float64) {
		// The server only ever calls Update with a small positive dt, so
		// anything else is out of contract and not worth reporting.
		if math.IsNaN(dt) || math.IsInf(dt, 0) || dt < 0 || dt > 1 {
			t.Skip()
		}

		p := Player{ID: "p1"}
		for range 100 {
			p.Update(Input{Up: up, Down: down, Left: left, Right: right}, dt)

			if math.IsNaN(p.Vel.X) || math.IsNaN(p.Vel.Y) || math.IsInf(p.Pos.X, 0) || math.IsInf(p.Pos.Y, 0) {
				t.Fatalf("state went non-finite: %+v (dt = %v)", p, dt)
			}
			if speed := p.Vel.Len(); speed > PlayerMaxSpeed+eps {
				t.Fatalf("speed %v exceeds PlayerMaxSpeed (dt = %v)", speed, dt)
			}
		}
	})
}

// BenchmarkPlayerUpdate measures one tick. Update runs once per player per
// tick, 60 times a second, so it is worth knowing it stays in the nanoseconds.
//
//	go test ./internal/game -bench PlayerUpdate -benchmem
func BenchmarkPlayerUpdate(b *testing.B) {
	p := Player{ID: "p1"}
	in := Input{Up: true, Right: true}

	for b.Loop() {
		p.Update(in, tick)
	}
}
