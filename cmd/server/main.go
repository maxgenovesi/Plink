// Command server runs the Plink game server: static assets plus one WebSocket
// endpoint per player. It wires dependencies together and does nothing else --
// the routing lives in internal/server, the simulation in internal/game.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maxgenovesi/Plink/internal/server"
)

// webDir is where Vite writes the built client. Served relative to the
// process's working directory, so run the binary from the repository root.
const webDir = "./web/dist"

// shutdownTimeout is how long in-flight HTTP requests get to finish once the
// process has been asked to stop.
const shutdownTimeout = 5 * time.Second

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	// ctx is the process's lifetime: Ctrl-C or SIGTERM cancels it, which stops
	// the game loop and, below, the HTTP server.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(webDir)
	go srv.RunHub(ctx)

	httpServer := &http.Server{Addr: addr, Handler: srv}

	// Buffered so the goroutine can exit even if main stopped listening.
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.ListenAndServe() }()

	log.Printf("Plink listening on %s, serving %s", addr, webDir)

	select {
	case err := <-serveErr:
		// ListenAndServe only returns by itself on failure, e.g. port in use.
		log.Fatal(err)
	case <-ctx.Done():
	}

	// A second Ctrl-C from here on kills the process the default way.
	stop()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown does not wait for WebSockets, which are hijacked connections;
	// those end by themselves when the hub stops.
	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("shutdown: %v", err)
	}
}
