package server

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/maxgenovesi/Plink/internal/game"
	"github.com/maxgenovesi/Plink/internal/protocol"
)

// The simulation's clock. Rates in internal/game are per second, so TickRate
// can change without changing how the game feels.
const (
	TickRate     = 60
	TickDuration = time.Second / TickRate
	TickDelta    = 1.0 / float64(TickRate) // the fixed dt passed to Game.Step
)

// errHubStopped is what a client's run returns when its hub has shut down
// underneath it, which only happens when the server is stopping.
var errHubStopped = errors.New("hub stopped")

// hub owns exactly one game.Game and is the only goroutine allowed to touch it.
// Every connection reaches the game through the hub's channels and never
// through the Game itself, which is why nothing here needs a mutex.
type hub struct {
	game *game.Game

	// join and leave are unbuffered, so a client's send returns only once the
	// hub has taken it: after a join the client is in the game, and after a
	// leave the hub holds no reference to it.
	join  chan *client
	leave chan *client

	// inputs carries decoded input from every client's readLoop.
	// Buffered: a busy tick must not block the readers.
	inputs chan playerInput

	// done is closed when Run returns. Nothing is ever sent on it; clients
	// select on it so a send to join or leave cannot hang on a stopped hub.
	done chan struct{}

	clients map[game.PlayerID]*client // hub goroutine only
	ackSeq  map[game.PlayerID]uint32  // last input Seq recorded per player; hub goroutine only
}

// playerInput is one decoded input frame on its way to the hub, tagged with the
// player who sent it, since hub.inputs is shared by every connection.
type playerInput struct {
	id    game.PlayerID
	input protocol.Input
}

// hubInputBufferSize is the capacity of hub.inputs. Every client's readLoop
// sends into that one channel, so it is sized for all players at once rather
// than for a single connection.
const hubInputBufferSize = 256

// newHub returns a hub holding an empty game and no clients, with its channels
// ready to use. It takes no arguments.
//
// newHub does not start the hub's loop: nothing is read from join, leave or
// inputs until the caller runs Run in a goroutine of its own. Until then a
// client trying to join simply waits.
func newHub() *hub {
	return &hub{
		game:    game.NewGame(),
		join:    make(chan *client),
		leave:   make(chan *client),
		inputs:  make(chan playerInput, hubInputBufferSize),
		done:    make(chan struct{}),
		clients: make(map[game.PlayerID]*client),
		ackSeq:  make(map[game.PlayerID]uint32),
	}
}

// Run is the world's loop, and the only code that touches h.game, h.clients and
// h.ackSeq. It blocks until ctx is cancelled, then returns nothing; the caller
// is expected to run it in a goroutine of its own, exactly once per hub.
//
// Returning closes h.done, which disconnects every client still attached and
// turns away any that try to join afterwards. A hub cannot be restarted.
//
// Clients arriving on join are added to the game and those on leave are taken
// out of it. Input arriving on inputs is recorded, not applied: only the ticker
// advances the simulation, once per TickDuration, and each tick ends with a
// broadcast of the new snapshot.
func (h *hub) Run(ctx context.Context) {
	ticker := time.NewTicker(TickDuration)
	defer ticker.Stop()
	defer close(h.done)

	for {
		select {
		case <-ctx.Done():
			return

		case c := <-h.join:
			h.clients[c.id] = c
			h.game.AddPlayer(c.id)

		case c := <-h.leave:
			delete(h.clients, c.id)
			delete(h.ackSeq, c.id)
			h.game.RemovePlayer(c.id)

		case pi := <-h.inputs:
			// An input can still be in the buffer after its client has left.
			if _, ok := h.clients[pi.id]; !ok {
				continue
			}
			h.ackSeq[pi.id] = pi.input.Seq
			h.game.SetInput(pi.id, game.Input{
				Up:    pi.input.Up,
				Down:  pi.input.Down,
				Left:  pi.input.Left,
				Right: pi.input.Right,
			})

		case <-ticker.C:
			h.game.Step(TickDelta)
			h.broadcast(h.game.Snapshot())
		}
	}
}

// broadcast marshals a snapshot once and fans the bytes out to every client.
// snap is the world as of the tick just stepped; broadcast returns nothing.
//
// The snapshot is converted to a protocol.State first, pairing each player with
// the last input Seq recorded for them, so one frame serves every client and
// each finds its own entry by ID. The frame is shared, not copied, which is safe
// because nothing writes to it after this.
//
// Sends never block: a client whose outgoing buffer is full misses this frame
// and catches up on the next, since every frame is the whole world. A snapshot
// that fails to encode is logged and sent to nobody.
func (h *hub) broadcast(snap game.Snapshot) {
	state := &protocol.State{
		Tick:    snap.Tick,
		Players: make([]protocol.PlayerState, 0, len(snap.Players)),
	}
	for _, p := range snap.Players {
		state.Players = append(state.Players, protocol.PlayerState{
			ID:     string(p.ID),
			X:      p.X,
			Y:      p.Y,
			AckSeq: h.ackSeq[p.ID],
		})
	}

	frame, err := protocol.Encode(state)
	if err != nil {
		log.Printf("broadcast tick %d: %v", snap.Tick, err)
		return
	}

	for _, c := range h.clients {
		select {
		case c.outgoing <- frame:
		default:
		}
	}
}
