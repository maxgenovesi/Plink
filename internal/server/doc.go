// Package server hosts the game over HTTP: static assets plus one WebSocket per
// player.
//
// A Server owns a single hub, and the hub owns the one game.Game. Each
// connection is a client that forwards the browser's input to the hub and
// writes the hub's per-tick broadcast back to the socket; no connection ever
// touches the Game directly.
package server
