package protocol

// Every message on the wire, mirrored by web/src/protocol.ts.
//
// Adding a message means, in this file: a MessageType constant, the struct, a
// Type method on it, and an entry in registry. Then the same type in
// protocol.ts. Nothing else in either package changes.

const (
	// server -> client
	TypeWelcome MessageType = "welcome"
	TypeState   MessageType = "state"

	// client -> server
	TypeInput MessageType = "input"
)

// registry maps each MessageType to a constructor for its struct. Decode uses
// it to pick the struct to unmarshal into, and Encode uses it to refuse types
// nobody could decode. A type missing from here does not exist on the wire.
var registry = map[MessageType]func() Message{
	TypeWelcome: func() Message { return &Welcome{} },
	TypeState:   func() Message { return &State{} },
	TypeInput:   func() Message { return &Input{} },
}

// Welcome is the first frame on every connection, sent once. PlayerID tells
// the browser which entry in State.Players is its own, and TickRate is how
// many States per second to expect.
type Welcome struct {
	PlayerID string `json:"playerId"`
	TickRate int    `json:"tickRate"`
}

// State is the whole world as of one tick, sent to every client each tick.
// It is the same bytes for everyone, so anything per-player lives inside
// Players rather than at the top level.
type State struct {
	Tick    uint64        `json:"tick"`
	Players []PlayerState `json:"players"`
}

// PlayerState is one player's entry in a State. X and Y are the centre of the
// player's circle, in arena-units.
//
// AckSeq is the last Input.Seq the server recorded for this player. It sits
// here rather than on State so that one encoded State still serves every
// client: each finds its own ack in its own entry.
type PlayerState struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	AckSeq uint32  `json:"ackSeq"`
}

// Input is what the browser sends, ~30 times a second: the movement keys held
// right now, not presses or releases, so a dropped frame costs nothing.
//
// Seq goes up by one with every Input a client sends and is echoed back as
// PlayerState.AckSeq, which client-side prediction relies on.
type Input struct {
	Seq   uint32 `json:"seq"`
	Up    bool   `json:"up"`
	Down  bool   `json:"down"`
	Left  bool   `json:"left"`
	Right bool   `json:"right"`
}

// Type reports TypeWelcome, satisfying Message.
func (*Welcome) Type() MessageType { return TypeWelcome }

// Type reports TypeState, satisfying Message.
func (*State) Type() MessageType { return TypeState }

// Type reports TypeInput, satisfying Message.
func (*Input) Type() MessageType { return TypeInput }
