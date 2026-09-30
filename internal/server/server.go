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
// nextID needs no initialization: an atomic.Uint64 is usable at its zero value.
// It does mean a Server must never be copied, which is why New hands back a
// pointer and every method below takes a pointer receiver -- copying the struct
// would duplicate the counter and start handing out IDs twice.
type Server struct {
	mux    *http.ServeMux
	webDir string
	nextID atomic.Uint64
}

// New builds a Server with its routes registered and returns it ready to serve:
// "/ws" upgrades to a WebSocket, and everything else is served as a static file.
//
// webDir is the directory of built browser assets to serve, relative to the
// process's working directory (the Vite build writes to ./web/dist). It is not
// checked for existence here -- a missing directory surfaces as a 404 per
// request rather than a failure to start.
func New(webDir string) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mux:    mux,
		webDir: webDir,
	}

	// Registered on s, not on a bare function, so handleWS can reach nextID.
	// Pattern order does not matter to ServeMux: the most specific match wins,
	// so "/ws" beats the "/" catch-all regardless of which is registered first.
	mux.HandleFunc("/ws", s.handleWS)
	mux.Handle("/", http.FileServer(http.Dir(webDir)))

	return s
}

// ServeHTTP makes *Server satisfy http.Handler, so main.go can pass it
// straight to http.ListenAndServe. Idiomatic Go: your app is a Handler.
//
// It delegates to the mux New built, which keeps the routing table private: the
// only way in is through this method, so nothing outside the package can add or
// replace a route after construction.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleWS upgrades the request, builds a client, and blocks in client.run
// until the connection dies.
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
	err = newClient(id, conn).run(r.Context())
	switch status := websocket.CloseStatus(err); {
	case status == websocket.StatusNormalClosure, status == websocket.StatusGoingAway:
		log.Printf("client %s disconnected", id)
	case errors.Is(err, context.Canceled):
		log.Printf("client %s dropped: server shutting down", id)
	default:
		log.Printf("client %s failed: %v", id, err)
	}
}
