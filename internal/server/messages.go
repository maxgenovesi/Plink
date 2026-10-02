package server

// The wire format, mirrored by web/src/network.ts. Every message the server
// sends carries a "type" so the browser can tell them apart.

// clientInput is what the browser sends us, ~30x/second.
type clientInput struct {
	Seq   uint32 `json:"seq"`
	Up    bool   `json:"up"`
	Down  bool   `json:"down"`
	Left  bool   `json:"left"`
	Right bool   `json:"right"`
}

// serverWelcome is the first frame on every connection, sent once. It tells
// the browser which entry in worldState.Players is its own.
type serverWelcome struct {
	Type string `json:"type"` // always "welcome"
	ID   string `json:"id"`
}

// worldState is what the hub sends every client each tick: the same bytes for
// everyone, so anything per-player lives inside Players.
type worldState struct {
	Type    string        `json:"type"` // always "state"
	Tick    uint64        `json:"tick"`
	Players []playerState `json:"players"`
}

// playerState is one player's entry in a worldState. X and Y are the centre of
// the player's circle, in arena-units.
type playerState struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	AckSeq uint32  `json:"ackSeq"` // last clientInput.Seq the hub recorded for this player
}
