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
const (
	PlayerRadius   = 15.0
	PlayerAccel    = 3000.0 // units/s^2 while a key is held
	PlayerMaxSpeed = 400.0  // units/s
	PlayerFriction = 8.0    // velocity lost per second, proportional to speed
)

// Input is the set of controls a client is holding during one tick.
// It is a snapshot of state, not an event: "W is currently down",
// not "W was just pressed".
type Input struct {
	Up, Down, Left, Right bool
}

func (p *Player) Update(in Input, dt float64) {}
