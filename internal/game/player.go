package game

// PlayerID identifies a player for the lifetime of one connection.
type PlayerID string

// Player is one controllable circle in the arena.
type Player struct {
	ID  PlayerID
	Pos Vec
	Vel Vec
}

// Input is the set of controls a client is holding during one tick.
// It is a snapshot of state, not an event: "W is currently down",
// not "W was just pressed".
type Input struct {
	Up, Down, Left, Right bool
}

func (p *Player) Update(in Input, dt float64) {}
