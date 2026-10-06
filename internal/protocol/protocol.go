// Package protocol is the wire format between the server and the browser,
// mirrored on the client by web/src/protocol.ts.
//
// Every frame, in both directions, is one JSON Envelope: a "type" naming the
// message and a "data" object holding it. The message structs themselves and
// the table of known types live in messages.go, so adding a message touches
// that file here and protocol.ts on the client, and nothing else.
//
// Dependencies point one way: server imports protocol, and protocol imports
// nothing of ours.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// MessageType is the discriminator carried in Envelope.Type. Each one names
// exactly one message struct in messages.go.
type MessageType string

// Message is any frame that can go on the wire. Type reports which
// MessageType the value encodes as; it must agree with the entry for that
// type in the registry, which the tests check.
//
// Making this an interface rather than taking any means passing a non-message
// to Encode is a compile error, not a runtime one.
type Message interface {
	Type() MessageType
}

// Envelope is the outer shape of every frame. Decode reads Type first,
// then unmarshals Data into the right concrete struct.
type Envelope struct {
	Type MessageType     `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ErrUnknownType is returned, wrapped, by Encode and Decode for a MessageType
// that has no entry in the registry.
var ErrUnknownType = errors.New("unknown message type")

// Encode wraps msg in an Envelope tagged with msg.Type() and returns the JSON
// bytes, ready to write to the socket as a text frame.
//
// It returns an error wrapping ErrUnknownType if msg.Type() is not in the
// registry, so a message nobody can decode never leaves the process, and an
// error if msg is nil or fails to marshal.
func Encode(msg Message) ([]byte, error) {
	if msg == nil {
		return nil, errors.New("encode: nil message")
	}
	t := msg.Type()
	if _, ok := registry[t]; !ok {
		return nil, fmt.Errorf("encode %q: %w", t, ErrUnknownType)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encode %q: %w", t, err)
	}
	return json.Marshal(Envelope{Type: t, Data: data})
}

// Decode parses one frame and returns the message inside it as a pointer to
// its concrete struct, e.g. *Input. Callers type-switch on the result.
//
// data is the raw bytes of one WebSocket text frame. Decode returns an error
// wrapping ErrUnknownType if the envelope's type is not in the registry, and
// an error if data is not a valid envelope or the payload does not fit the
// struct its type names. A missing or null "data" decodes as the zero value
// of that struct, so messages with no fields need not send one.
func Decode(data []byte) (Message, error) {
	// First pass: only the envelope. Data stays as raw bytes until we know
	// which struct it belongs in.
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}

	newMsg, ok := registry[env.Type]
	if !ok {
		return nil, fmt.Errorf("decode %q: %w", env.Type, ErrUnknownType)
	}

	// Second pass: the payload, into the struct the type names.
	msg := newMsg()
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, msg); err != nil {
			return nil, fmt.Errorf("decode %q: %w", env.Type, err)
		}
	}
	return msg, nil
}
