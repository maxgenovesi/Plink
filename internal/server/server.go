package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/coder/websocket"
	"github.com/maxgenovesi/Plink/internal/game"
)

// Server holds process-wide dependencies and routes.
//
// hub is the one game world every connection joins. The Server only creates it
// and hands it to clients; the game inside belongs to the goroutine in RunHub.
//
// nextID needs no initialization: an atomic.Uint64 is usable at its zero value.
// It does mean a Server must never be copied, which is why New hands back a
// pointer and every method below takes a pointer receiver -- copying the struct
// would duplicate the counter and start handing out IDs twice.
type Server struct {
	mux    *http.ServeMux
	webDir string
	nextID atomic.Uint64
	hub    *hub
}

// New builds a Server with its routes registered and returns it ready to serve:
// "/ws" upgrades to a WebSocket, and everything else is served as a static file.
//
// webDir is the directory of built browser assets to serve, relative to the
// process's working directory (the Vite build writes to ./web/dist). It is not
// checked for existence here -- a missing directory surfaces as a 404 per
// request rather than a failure to start.
//
// New also creates the Server's hub but does not start it. Connections are
// accepted straight away, but each one waits to join until the caller runs
// RunHub, so nothing moves and nothing is broadcast before then.
func New(webDir string) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mux:    mux,
		webDir: webDir,
		hub:    newHub(),
	}

	// Registered on s, not on a bare function, so handleWS can reach nextID
	// and hub.
	// Pattern order does not matter to ServeMux: the most specific match wins,
	// so "/ws" beats the "/" catch-all regardless of which is registered first.
	mux.HandleFunc("/ws", s.handleWS)
	mux.Handle("/", http.FileServer(http.Dir(webDir)))

	return s
}

// ServeHTTP makes *Server satisfy http.Handler, so main.go can use it as the
// Handler of its http.Server. Idiomatic Go: your app is a Handler.
//
// It delegates to the mux New built, which keeps the routing table private: the
// only way in is through this method, so nothing outside the package can add or
// replace a route after construction.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// RunHub runs the game: it steps the world once per tick and broadcasts the
// result to every connected client. It blocks until ctx is cancelled and
// returns nothing, so the caller is expected to give it a goroutine of its own.
//
// Call it exactly once per Server. Cancelling ctx stops the game for good and
// disconnects every client; the HTTP side keeps serving static files, but a new
// WebSocket connection is turned away as soon as it tries to join.
func (s *Server) RunHub(ctx context.Context) {
	s.hub.Run(ctx)
}

// handleWS upgrades the request, builds a client for the Server's hub, and
// blocks in client.run until the connection dies.
//
// w and r are the usual handler pair; the response is written by
// websocket.Accept as part of the upgrade handshake, so nothing here writes to w
// directly. Blocking is the point: this handler's goroutine becomes the
// connection's lifetime, and net/http gives every request its own goroutine, so
// one slow player cannot hold up another's upgrade.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Skips the Origin check. Fine while the Vite dev server runs on a
		// different port than this one; must become OriginPatterns before deploy.
		InsecureSkipVerify: true,
	})
	if err != nil {
		// Accept has already written a 4xx to w, so only the log is left to do.
		log.Println("accept error:", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()

	// Add returns the post-increment value, so the first player is p1.
	id := game.PlayerID("p" + strconv.FormatUint(s.nextID.Add(1), 10))
	log.Printf("client %s connected", id)

	// run always returns non-nil, so the ordinary endings have to be told apart
	// from the failures -- otherwise every closed browser tab looks like a bug.
	err = newClient(id, conn, s.hub).run(r.Context())
	switch status := websocket.CloseStatus(err); {
	case status == websocket.StatusNormalClosure, status == websocket.StatusGoingAway:
		log.Printf("client %s disconnected", id)
	case errors.Is(err, context.Canceled), errors.Is(err, errHubStopped):
		log.Printf("client %s dropped: server shutting down", id)
	default:
		log.Printf("client %s failed: %v", id, err)
	}
}
