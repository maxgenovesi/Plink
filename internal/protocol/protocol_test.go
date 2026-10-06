package protocol

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// samples holds one populated value per message type. TestSamplesCoverRegistry
// fails if a type is registered without a sample here, so every new message
// gets the round-trip test for free.
var samples = []Message{
	&Welcome{PlayerID: "p1", TickRate: 60},
	&State{Tick: 42, Players: []PlayerState{{ID: "p1", X: 1.5, Y: 2.5, AckSeq: 7}}},
	&Input{Seq: 9, Up: true, Right: true},
}

func TestSamplesCoverRegistry(t *testing.T) {
	seen := map[MessageType]bool{}
	for _, s := range samples {
		seen[s.Type()] = true
	}
	for typ := range registry {
		if !seen[typ] {
			t.Errorf("registry has %q but samples has no value for it", typ)
		}
	}
}

func TestRegistryConstructorsMatchTheirKey(t *testing.T) {
	for typ, newMsg := range registry {
		if got := newMsg().Type(); got != typ {
			t.Errorf("registry[%q] builds a message whose Type() is %q", typ, got)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	for _, want := range samples {
		t.Run(string(want.Type()), func(t *testing.T) {
			frame, err := Encode(want)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			got, err := Decode(frame)
			if err != nil {
				t.Fatalf("Decode(%s): %v", frame, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("round trip = %#v, want %#v", got, want)
			}
		})
	}
}

func TestEncodeWritesEnvelope(t *testing.T) {
	frame, err := Encode(&Welcome{PlayerID: "p3", TickRate: 60})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"type":"welcome","data":{"playerId":"p3","tickRate":60}}`
	if string(frame) != want {
		t.Errorf("Encode = %s, want %s", frame, want)
	}
}

func TestEncodeEmptyPlayersIsArrayNotNull(t *testing.T) {
	frame, err := Encode(&State{Players: []PlayerState{}})
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Data struct {
			Players json.RawMessage `json:"players"`
		} `json:"data"`
	}
	if err := json.Unmarshal(frame, &env); err != nil {
		t.Fatal(err)
	}
	if string(env.Data.Players) != "[]" {
		t.Errorf("players = %s, want []", env.Data.Players)
	}
}

// unregistered satisfies Message but has no registry entry.
type unregistered struct{}

func (unregistered) Type() MessageType { return "nope" }

func TestEncodeRejectsUnregisteredType(t *testing.T) {
	if _, err := Encode(unregistered{}); !errors.Is(err, ErrUnknownType) {
		t.Errorf("Encode(unregistered) error = %v, want ErrUnknownType", err)
	}
}

func TestEncodeRejectsNil(t *testing.T) {
	if _, err := Encode(nil); err == nil {
		t.Error("Encode(nil) returned no error")
	}
}

func TestDecodeRejectsUnknownType(t *testing.T) {
	_, err := Decode([]byte(`{"type":"nope","data":{}}`))
	if !errors.Is(err, ErrUnknownType) {
		t.Errorf("error = %v, want ErrUnknownType", err)
	}
}

func TestDecodeRejectsMalformedFrames(t *testing.T) {
	cases := map[string]string{
		"not json":           `hello`,
		"payload wrong type": `{"type":"input","data":{"seq":"nine"}}`,
		"data not an object": `{"type":"input","data":[1,2]}`,
		"missing type":       `{"data":{}}`,
	}
	for name, frame := range cases {
		t.Run(name, func(t *testing.T) {
			if msg, err := Decode([]byte(frame)); err == nil {
				t.Errorf("Decode(%s) = %#v, want an error", frame, msg)
			}
		})
	}
}

func TestDecodeMissingOrNullDataIsZeroValue(t *testing.T) {
	for _, frame := range []string{`{"type":"input"}`, `{"type":"input","data":null}`} {
		msg, err := Decode([]byte(frame))
		if err != nil {
			t.Fatalf("Decode(%s): %v", frame, err)
		}
		if in, ok := msg.(*Input); !ok || *in != (Input{}) {
			t.Errorf("Decode(%s) = %#v, want a zero *Input", frame, msg)
		}
	}
}

func TestDecodeIgnoresUnknownFields(t *testing.T) {
	// Lets a newer client add a field without breaking an older server.
	msg, err := Decode([]byte(`{"type":"input","data":{"seq":3,"jump":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if in := msg.(*Input); in.Seq != 3 {
		t.Errorf("Seq = %d, want 3", in.Seq)
	}
}
