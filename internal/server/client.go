package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/maxgenovesi/Plink/internal/game"
	"golang.org/x/sync/errgroup"
)

const (
	TickRate     = 60
	TickDuration = time.Second / TickRate
	TickDelta    = 1.0 / float64(TickRate) // the fixed dt passed to Update
)

const (
	inputBufferSize    = 8
	outgoingBufferSize = 16
)

// client owns one WebSocket connection and the goroutines that serve it.
type client struct {
	id   game.PlayerID
	conn *websocket.Conn

	// inputs carries decoded input from readLoop to simLoop.
	// Buffered: a slow tick must not block the reader.
	inputs chan clientInput

	// outgoing carries encoded frames from simLoop to writeLoop.
	// Buffered: a slow client must not stall the simulation.
	outgoing chan []byte
}

func newClient(id game.PlayerID, conn *websocket.Conn) *client {
	return &client{
		id,
		conn,
		make(chan clientInput, inputBufferSize),
		make(chan []byte, outgoingBufferSize),
	}
}

// readLoop decodes frames off the socket until ctx is cancelled or the
// connection fails. It returns the error that ended it.
func (c *client) readLoop(ctx context.Context) error {
	for {
		var in clientInput
		if err := wsjson.Read(ctx, c.conn, &in); err != nil {
			return err
		}

		select {
		case c.inputs <- in:
		default:
		}
	}
}

// simLoop is the ONLY place the player is allowed to move. It owns the Player
// outright -- no other goroutine holds a reference to it -- so the simulation
// needs no mutex.
//
// The player spawns at the arena centre and advances once per TickDuration
// until ctx is cancelled, which is the normal way simLoop returns: it hands back
// ctx.Err(). Every tick writes one encoded serverState onto c.outgoing for
// writeLoop to send.
//
// Input arriving on c.inputs is recorded, not applied. Only the ticker advances
// the simulation, which is what keeps movement speed independent of how often a
// client sends.
func (c *client) simLoop(ctx context.Context) error {
	arenaCenter := game.Vec{X: game.ArenaWidth / 2, Y: game.ArenaHeight / 2}
	player := &game.Player{ID: c.id, Pos: arenaCenter}

	var latest clientInput

	ticker := time.NewTicker(TickDuration)
	defer ticker.Stop()

	var tick uint64

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case in := <-c.inputs:
			latest = in

		case <-ticker.C:
			player.Update(game.Input{
				Up:    latest.Up,
				Down:  latest.Down,
				Left:  latest.Left,
				Right: latest.Right,
			}, TickDelta)

			player.ClampTo(game.ArenaWidth, game.ArenaHeight)

			tick++

			frame, err := json.Marshal(serverState{
				Type:   "state",
				Tick:   tick,
				AckSeq: latest.Seq,
				You: playerState{
					ID: string(c.id),
					X:  player.Pos.X,
					Y:  player.Pos.Y,
				},
			})
			if err != nil {
				return err
			}

			select {
			case c.outgoing <- frame:
			default:
			}
		}
	}
}

// writeLoop drains c.outgoing to the socket. It is the ONLY goroutine
// permitted to call conn.Write — coder/websocket forbids concurrent writes.
//
// Frames go out as MessageText, since simLoop encodes them as JSON. writeLoop
// runs until ctx is cancelled or a write fails, returning ctx.Err() or that
// write error; frames still sitting in c.outgoing at that point are discarded,
// because a connection that is going away has no use for them.
func (c *client) writeLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case frame := <-c.outgoing:
			if err := c.conn.Write(ctx, websocket.MessageText, frame); err != nil {
				return err
			}
		}
	}
}

// run starts all three loops and blocks until the first one fails,
// then cancels the other two.
//
// ctx bounds the whole connection: cancelling it shuts every loop down. run
// returns the error from whichever loop returned first, and it always returns a
// non-nil error, since even a clean shutdown surfaces as ctx.Err() or a
// websocket close error. Callers are expected to tell the ordinary endings
// apart from the real failures rather than logging all of them as problems.
//
// errgroup is what couples the three together: each loop takes the derived ctx,
// so the first non-nil return cancels it and unblocks the other two.
func (c *client) run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return c.readLoop(ctx) })
	g.Go(func() error { return c.simLoop(ctx) })
	g.Go(func() error { return c.writeLoop(ctx) })

	return g.Wait()
}
