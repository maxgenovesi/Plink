// Command server runs the Plink game server: static assets plus one WebSocket
// endpoint per player. It wires dependencies together and does nothing else --
// the routing lives in internal/server, the simulation in internal/game.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/maxgenovesi/Plink/internal/server"
)

// webDir is where Vite writes the built client. Served relative to the
// process's working directory, so run the binary from the repository root.
const webDir = "./web/dist"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	log.Printf("Plink listening on %s, serving %s", addr, webDir)

	// ListenAndServe only ever returns on failure, so reaching Fatal is correct.
	log.Fatal(http.ListenAndServe(addr, server.New(webDir)))
}
