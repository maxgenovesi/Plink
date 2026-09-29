package server

// clientInput is what the browser sends us, ~30x/second.
type clientInput struct {
	Seq   uint32 `json:"seq"`
	Up    bool   `json:"up"`
	Down  bool   `json:"down"`
	Left  bool   `json:"left"`
	Right bool   `json:"right"`
}

// serverState is what we send back every tick.
type serverState struct {
	Type   string      `json:"type"` // always "state" for now
	Tick   uint64      `json:"tick"`
	AckSeq uint32      `json:"ackSeq"` // last Seq we processed
	You    playerState `json:"you"`
}

type playerState struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}
