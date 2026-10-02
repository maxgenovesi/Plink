package game

// PlayerID identifies a player for the lifetime of one connection.
type PlayerID string

// Player is one controllable circle in the arena.
type Player struct {
	ID  PlayerID
	Pos Vec
	Vel Vec
}

// Tuning constants. Units are arena-units and seconds.
// Rates are per second so the tick rate can change without changing feel.
const (
	PlayerRadius   = 15.0
	PlayerAccel    = 5400.0 // units/s^2 while a key is held
	PlayerMaxSpeed = 400.0  // units/s
	PlayerFriction = 8.0    // fraction of current speed lost per second
)

// Input is the set of controls a client is holding during one tick.
// It is a snapshot of state, not an event: "W is currently down",
// not "W was just pressed".
type Input struct {
	Up, Down, Left, Right bool
}

// Update advances the player one tick, applying acceleration, friction, the
// speed limit, and movement in that order. It writes p.Vel and p.Pos in place
// rather than returning a new Player.
//
// in holds the controls pressed for the whole tick, and dt is the length of
// that tick in seconds, which must be positive (the server uses 1.0/60).
//
// Update is deterministic: the same Player, Input and dt always produce the
// same result. The server depends on that to stay in agreement with its clients.
func (p *Player) Update(in Input, dt float64) {
	// Held keys become a direction. Opposite keys cancel.
	dir := Vec{0, 0}
	if in.Up {
		dir.Y -= 1
	}
	if in.Down {
		dir.Y += 1
	}
	if in.Left {
		dir.X -= 1
	}
	if in.Right {
		dir.X += 1
	}

	// Normalizing first keeps diagonals from being faster than the axes.
	p.Vel = p.Vel.Add(dir.Normalized().Scale(PlayerAccel * dt))

	// max(0, ...) stops a large dt from flinging the player backwards.
	p.Vel = p.Vel.Scale(max(0, 1-PlayerFriction*dt))

	if speed := p.Vel.Len(); speed > PlayerMaxSpeed {
		p.Vel = p.Vel.Scale(PlayerMaxSpeed / speed)
	}

	// Integrating with the new velocity (semi-implicit Euler) stays stable
	// where integrating with the old one drifts.
	p.Pos = p.Pos.Add(p.Vel.Scale(dt))
}

// ClampTo keeps the player fully inside a w x h arena, accounting for
// PlayerRadius, and zeroes the velocity component that hit the wall.
//
// w and h are the arena's dimensions in arena-units; both must be larger than
// 2*PlayerRadius, or the low and high bounds cross. Pos and Vel are written in
// place; a player already inside the arena is left exactly as it was.
func (p *Player) ClampTo(w, h float64) {
	// Pos is the circle's centre, so the usable range is inset by the radius at
	// both ends -- otherwise half the player hangs outside the wall.
	if p.Pos.X < PlayerRadius {
		p.Pos.X = PlayerRadius
		p.Vel.X = 0
	} else if p.Pos.X > w-PlayerRadius {
		p.Pos.X = w - PlayerRadius
		p.Vel.X = 0
	}

	// Each axis is handled on its own, and only the component that hit the wall
	// is cleared. Zeroing the whole vector would stop a player pressed against
	// the left wall from sliding down it, so holding two keys would move you
	// less than holding one.
	if p.Pos.Y < PlayerRadius {
		p.Pos.Y = PlayerRadius
		p.Vel.Y = 0
	} else if p.Pos.Y > h-PlayerRadius {
		p.Pos.Y = h - PlayerRadius
		p.Vel.Y = 0
	}
}
