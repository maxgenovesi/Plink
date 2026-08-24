package main

import (
	"context"
	"log"
	"net/http"

	"github.com/coder/websocket"
)

func main() {
	mux := http.NewServeMux()

	// Serve static files from the "web" directory
	mux.Handle("/", http.FileServer(http.Dir("./web")))

	// Handle WebSocket connections at the "/ws" endpoint
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // allows localhost connections during dev
		})
		if err != nil {
			log.Println("accept error:", err)
			return
		}
		defer conn.CloseNow()

		ctx := context.Background()

		// Read loop — just log messages for now
		for {
			_, msg, err := conn.Read(ctx)
			if err != nil {
				log.Println("read error:", err)
				return
			}
			log.Println("got message:", string(msg))
		}
	})

	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))

}
