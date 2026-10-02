package game

// Game is the whole world. It is NOT safe for concurrent use — by design.
// Exactly one goroutine may touch a Game. Everything else talks to that
// goroutine over channels. This is the "share memory by communicating" rule,
// and following it is why you will never need a mutex in this package.
type Game struct {
	// Width and Height are the arena's size in arena-units. They are the only
	// place the rest of the program should read the arena size from.
	Width, Height float64
	Tick          uint64

	players map[PlayerID]*Player
	inputs  map[PlayerID]Input // latest input per player, applied each tick
}

// The size NewGame gives every arena, in arena-units. Unexported so that only
// NewGame reads them; everything else goes through Game.Width and Game.Height.
// Both must stay larger than 2*PlayerRadius, or the low and high bounds ClampTo
// derives from them cross.
const (
	arenaWidth  = 1500.0
	arenaHeight = 1500.0
)

// NewGame returns an empty world at tick zero with no players, ready for
// AddPlayer. It takes no size: every Game starts at the default arena
// dimensions, which callers read back from Width and Height.
//
// The returned Game belongs to whichever goroutine goes on to call Step.
func NewGame() *Game {
	return &Game{
		Width:   arenaWidth,
		Height:  arenaHeight,
		players: make(map[PlayerID]*Player),
		inputs:  make(map[PlayerID]Input),
	}
}

// AddPlayer puts a new player into the world, at rest in the centre of the
// arena, and returns it. id is the key the player is stored under and the one
// SetInput and RemovePlayer expect later.
//
// If id is already in the game, AddPlayer returns that existing player
// untouched rather than replacing it, so a repeated call cannot orphan a
// player that Step is still moving.
//
// The returned pointer is the Game's own Player, not a copy. It must stay on
// the goroutine that owns the Game; other goroutines get Snapshot instead.
func (g *Game) AddPlayer(id PlayerID) *Player {
	if p, ok := g.players[id]; ok {
		return p
	}

	arenaCenter := Vec{g.Width / 2, g.Height / 2}
	p := &Player{ID: id, Pos: arenaCenter}
	g.players[id] = p

	return p
}

// RemovePlayer takes the player stored under id out of the world, along with
// the input recorded for them. It returns nothing, and an id that is not in the
// game is a no-op.
//
// A *Player that AddPlayer handed out for id is no longer part of the Game
// afterwards: Step stops moving it and Snapshot stops reporting it.
func (g *Game) RemovePlayer(id PlayerID) {
	delete(g.players, id)
	delete(g.inputs, id)
}

// SetInput records in as the controls the player stored under id is holding.
// It returns nothing and does not simulate: nothing moves until the next Step.
//
// in is the full set of keys currently down, so it replaces whatever was
// recorded for that player before rather than combining with it. It then stays
// in effect on every Step until the next SetInput for the same id.
//
// An id that is not in the game is ignored, so input arriving after
// RemovePlayer cannot leave an entry behind.
func (g *Game) SetInput(id PlayerID, in Input) {
	if _, ok := g.players[id]; ok {
		g.inputs[id] = in
	}
}

// Step advances the whole world by one fixed timestep: every player is moved by
// the input most recently recorded for them, then kept inside the arena, and
// Tick goes up by one. It changes the Game in place and returns nothing.
//
// dt is the length of the step in seconds, which must be positive (the server
// uses 1.0/60). A player with no recorded input is treated as holding no keys.
//
// Recorded inputs are read, not consumed, so they carry over to the next Step.
func (g *Game) Step(dt float64) {
	for id, p := range g.players {
		latest := g.inputs[id]
		p.Update(latest, dt)
		p.ClampTo(g.Width, g.Height)
	}
	g.Tick++
}

// Snapshot is a frozen copy of the world at one tick. Tick is the number of
// Steps completed when it was taken, and Players holds one entry per player in
// the game at that moment, in no particular order.
type Snapshot struct {
	Tick    uint64
	Players []PlayerSnapshot
}

// PlayerSnapshot is one player's state inside a Snapshot. X and Y are the
// centre of the player's circle, in arena-units.
type PlayerSnapshot struct {
	ID   PlayerID
	X, Y float64
}

// Snapshot returns a value copy of the world as it stands now: the current Tick
// and one PlayerSnapshot per player, holding their ID and position. It takes no
// arguments and leaves the Game unchanged.
//
// The result shares no memory with the Game, so it is safe to hand to other
// goroutines and to serialise while Step carries on. Players is never nil, even
// with nobody in the game, so it encodes as an empty JSON array, not null.
//
// The order of Players is unspecified and can differ between calls; match
// players up by ID rather than by index.
func (g *Game) Snapshot() Snapshot {
	s := Snapshot{
		Tick:    g.Tick,
		Players: make([]PlayerSnapshot, 0, len(g.players)),
	}
	for id, p := range g.players {
		s.Players = append(s.Players, PlayerSnapshot{id, p.Pos.X, p.Pos.Y})
	}
	return s
}
