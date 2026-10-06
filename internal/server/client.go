package server

import (
	"context"
	"log"

	"github.com/coder/websocket"
	"github.com/maxgenovesi/Plink/internal/game"
	"github.com/maxgenovesi/Plink/internal/protocol"
	"golang.org/x/sync/errgroup"
)

// outgoingBufferSize is the capacity of client.outgoing: how many frames may
// queue for one connection before the hub starts dropping them for it.
const outgoingBufferSize = 16

// client owns one WebSocket connection and the goroutines that serve it. It
// holds no game state: the player it stands for lives in the hub's Game, and
// the client only carries bytes between that hub and the socket.
type client struct {
	id   game.PlayerID
	conn *websocket.Conn

	// hub is the world this client plays in. The client only ever uses its
	// channels; the Game behind them belongs to the hub's goroutine.
	hub *hub

	// outgoing carries encoded frames from the hub to writeLoop.
	// Buffered: a slow client must not stall the hub.
	outgoing chan []byte
}

// newClient returns a client for one accepted connection, with its outgoing
// buffer ready. id is the player it will be known as, conn is the socket it
// reads and writes, and h is the hub whose game it will play in.
//
// newClient neither joins the hub nor starts any goroutine; run does both.
func newClient(id game.PlayerID, conn *websocket.Conn, h *hub) *client {
	return &client{
		id:       id,
		conn:     conn,
		hub:      h,
		outgoing: make(chan []byte, outgoingBufferSize),
	}
}

// readLoop decodes frames off the socket and forwards each Input to the hub,
// tagged with this client's id. It runs until ctx is cancelled or the
// connection fails, and returns the error that ended it.
//
// A frame that fails to decode, or decodes to a message the browser has no
// business sending, is logged and skipped rather than ending the connection:
// one bad frame from an out-of-date client should not kick the player.
//
// The forward never blocks: if the hub's input buffer is full the frame is
// dropped, which costs nothing because the next one carries the full set of
// held keys again.
func (c *client) readLoop(ctx context.Context) error {
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return err
		}

		msg, err := protocol.Decode(data)
		if err != nil {
			log.Printf("client %s: dropping frame: %v", c.id, err)
			continue
		}

		switch m := msg.(type) {
		case *protocol.Input:
			select {
			case c.hub.inputs <- playerInput{id: c.id, input: *m}:
			default:
			}
		default:
			log.Printf("client %s: ignoring unexpected %q", c.id, msg.Type())
		}
	}
}

// writeLoop drains c.outgoing to the socket. It is the ONLY goroutine
// permitted to call conn.Write — coder/websocket forbids concurrent writes.
//
// Frames go out as MessageText, since everything queued on c.outgoing is a
// JSON envelope from protocol.Encode. writeLoop runs until ctx is cancelled or a write fails,
// returning ctx.Err() or that write error; frames still sitting in c.outgoing
// at that point are discarded, because a connection that is going away has no
// use for them.
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

// watchHub ends the connection when the hub stops, since a client with no hub
// would otherwise sit open receiving nothing. It returns errHubStopped in that
// case, or ctx.Err() if ctx is cancelled first.
func (c *client) watchHub(ctx context.Context) error {
	select {
	case <-c.hub.done:
		return errHubStopped
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run is the connection's whole life: it greets the browser, joins the hub,
// serves the socket until something ends it, then leaves the hub.
//
// ctx bounds the whole connection: cancelling it shuts every loop down. run
// returns the error from whichever loop returned first, and it always returns a
// non-nil error, since even a clean shutdown surfaces as ctx.Err(), a websocket
// close error or errHubStopped. Callers are expected to tell the ordinary
// endings apart from the real failures rather than logging all of them as
// problems.
//
// The welcome frame is queued before joining, so it is always the first thing
// the browser receives: the hub cannot broadcast to a client it has not been
// handed yet.
//
// errgroup is what couples the loops together: each takes the derived ctx, so
// the first non-nil return cancels it and unblocks the others.
func (c *client) run(ctx context.Context) error {
	welcome, err := protocol.Encode(&protocol.Welcome{PlayerID: string(c.id), TickRate: TickRate})
	if err != nil {
		return err
	}
	// Cannot block: outgoing is empty and nothing else can reach it yet.
	c.outgoing <- welcome

	// join is unbuffered, so a bare send would hang forever on a stopped hub.
	select {
	case c.hub.join <- c:
	case <-c.hub.done:
		return errHubStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	// ctx is usually already cancelled by the time this runs, so only the hub
	// stopping can release it. c.outgoing is deliberately never closed: the hub
	// may be mid-broadcast, and its non-blocking send would panic on a closed
	// channel.
	defer func() {
		select {
		case c.hub.leave <- c:
		case <-c.hub.done:
		}
	}()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return c.readLoop(ctx) })
	g.Go(func() error { return c.writeLoop(ctx) })
	g.Go(func() error { return c.watchHub(ctx) })

	return g.Wait()
}
