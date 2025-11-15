package message

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	ConnectionIDLength = 32
	SubstreamIDLength  = 32
)

// MessageType mirrors the Rust enum discriminants.
type MessageType byte

const (
	MessageTypeConnectionRequest MessageType = iota
	MessageTypeConnectionResponse
	MessageTypeTransport
)

// ConnectionID uniquely identifies a logical connection.
type ConnectionID [ConnectionIDLength]byte

// GenerateConnectionID returns a random identifier.
func GenerateConnectionID() (ConnectionID, error) {
	var id ConnectionID
	if _, err := rand.Read(id[:]); err != nil {
		return ConnectionID{}, fmt.Errorf("message: generate connection id: %w", err)
	}
	return id, nil
}

// String implements fmt.Stringer for debugging.
func (c ConnectionID) String() string {
	return hex.EncodeToString(c[:])
}

// Bytes returns the raw identifier bytes.
func (c ConnectionID) Bytes() []byte {
	b := make([]byte, len(c))
	copy(b, c[:])
	return b
}

// SubstreamID uniquely identifies a substream on a connection.
type SubstreamID [SubstreamIDLength]byte

// GenerateSubstreamID returns a random substream identifier.
func GenerateSubstreamID() (SubstreamID, error) {
	var id SubstreamID
	if _, err := rand.Read(id[:]); err != nil {
		return SubstreamID{}, fmt.Errorf("message: generate substream id: %w", err)
	}
	return id, nil
}

func (s SubstreamID) String() string {
	return hex.EncodeToString(s[:])
}

func (s SubstreamID) Bytes() []byte {
	b := make([]byte, len(s))
	copy(b, s[:])
	return b
}

// ConnectionMessage is exchanged during handshake.
type ConnectionMessage struct {
	PeerID    peer.ID
	Recipient *Recipient
	ID        ConnectionID
}

// TransportMessage carries substream payloads with ordering information.
type TransportMessage struct {
	Nonce   uint64
	Message SubstreamMessage
	ID      ConnectionID
}

// Message represents a top-level transport message.
type Message struct {
	Type       MessageType
	Connection *ConnectionMessage
	Transport  *TransportMessage
}

// SubstreamMessageType mirrors Rust SubstreamMessageType discriminants.
type SubstreamMessageType byte

const (
	SubstreamMessageOpenRequest SubstreamMessageType = iota
	SubstreamMessageOpenResponse
	SubstreamMessageClose
	SubstreamMessageData
)

// SubstreamMessage is sent over a logical substream.
type SubstreamMessage struct {
	ID   SubstreamID
	Type SubstreamMessageType
	Data []byte
}

// ErrInvalidMessage indicates decoding failure.
var ErrInvalidMessage = errors.New("message: invalid data")
