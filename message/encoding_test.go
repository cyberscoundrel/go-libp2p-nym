package message

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestConnectionMessageEncoding(t *testing.T) {
	// Create a test peer ID
	peerID, err := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	if err != nil {
		t.Fatalf("Failed to decode peer ID: %v", err)
	}

	// Create a test recipient
	recipient, err := ParseRecipient("CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f")
	if err != nil {
		t.Fatalf("Failed to parse recipient: %v", err)
	}

	// Create test connection ID
	connID, err := GenerateConnectionID()
	if err != nil {
		t.Fatalf("Failed to generate connection ID: %v", err)
	}

	msg := &Message{
		Type: MessageTypeConnectionRequest,
		Connection: &ConnectionMessage{
			PeerID:    peerID,
			Recipient: &recipient,
			ID:        connID,
		},
	}

	// Encode the message
	encoded, err := Encode(msg)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode the message
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify the message type
	if decoded.Type != msg.Type {
		t.Errorf("Message type mismatch: got %d, want %d", decoded.Type, msg.Type)
	}

	// Verify connection message fields
	if decoded.Connection == nil {
		t.Fatal("Connection message is nil")
	}
	if decoded.Connection.PeerID != msg.Connection.PeerID {
		t.Errorf("PeerID mismatch: got %s, want %s", decoded.Connection.PeerID, msg.Connection.PeerID)
	}
	if decoded.Connection.ID != msg.Connection.ID {
		t.Errorf("Connection ID mismatch")
	}
}

func TestTransportMessageEncoding(t *testing.T) {
	// Create test IDs
	connID, err := GenerateConnectionID()
	if err != nil {
		t.Fatalf("Failed to generate connection ID: %v", err)
	}

	streamID, err := GenerateSubstreamID()
	if err != nil {
		t.Fatalf("Failed to generate substream ID: %v", err)
	}

	msg := &Message{
		Type: MessageTypeTransport,
		Transport: &TransportMessage{
			ID:    connID,
			Nonce: 42,
			Message: SubstreamMessage{
				ID:   streamID,
				Type: SubstreamMessageData,
				Data: []byte("hello world"),
			},
		},
	}

	// Encode the message
	encoded, err := Encode(msg)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode the message
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify the message type
	if decoded.Type != msg.Type {
		t.Errorf("Message type mismatch: got %d, want %d", decoded.Type, msg.Type)
	}

	// Verify transport message fields
	if decoded.Transport == nil {
		t.Fatal("Transport message is nil")
	}
	if decoded.Transport.ID != msg.Transport.ID {
		t.Errorf("Transport ID mismatch")
	}
	if decoded.Transport.Nonce != msg.Transport.Nonce {
		t.Errorf("Nonce mismatch: got %d, want %d", decoded.Transport.Nonce, msg.Transport.Nonce)
	}
	if decoded.Transport.Message.Type != msg.Transport.Message.Type {
		t.Errorf("Substream type mismatch: got %d, want %d", decoded.Transport.Message.Type, msg.Transport.Message.Type)
	}
	if string(decoded.Transport.Message.Data) != string(msg.Transport.Message.Data) {
		t.Errorf("Data mismatch: got %s, want %s", decoded.Transport.Message.Data, msg.Transport.Message.Data)
	}
}

func TestConnectionIDGeneration(t *testing.T) {
	// Generate multiple connection IDs and ensure they're unique
	ids := make(map[ConnectionID]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateConnectionID()
		if err != nil {
			t.Fatalf("GenerateConnectionID failed: %v", err)
		}
		if ids[id] {
			t.Errorf("Duplicate connection ID generated: %x", id)
		}
		ids[id] = true
	}
}

func TestSubstreamIDGeneration(t *testing.T) {
	// Generate multiple substream IDs and ensure they're unique
	ids := make(map[SubstreamID]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateSubstreamID()
		if err != nil {
			t.Fatalf("GenerateSubstreamID failed: %v", err)
		}
		if ids[id] {
			t.Errorf("Duplicate substream ID generated: %x", id)
		}
		ids[id] = true
	}
}

func TestDecodeInvalidData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "EmptyData",
			data: []byte{},
		},
		{
			name: "InvalidMessageType",
			data: []byte{0xFF},
		},
		{
			name: "TruncatedData",
			data: []byte{0x00, 0x01},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(tt.data)
			if err == nil {
				t.Error("Decode() should have failed but succeeded")
			}
		})
	}
}

func BenchmarkEncodeConnectionMessage(b *testing.B) {
	peerID, _ := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	recipient, _ := ParseRecipient("CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f")
	connID, _ := GenerateConnectionID()

	msg := &Message{
		Type: MessageTypeConnectionRequest,
		Connection: &ConnectionMessage{
			PeerID:    peerID,
			Recipient: &recipient,
			ID:        connID,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Encode(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeConnectionMessage(b *testing.B) {
	peerID, _ := peer.Decode("12D3KooWEyoppNCUx8Yx66oV9fJnriXwCcXwDDUA2kj6vnc6iDEp")
	recipient, _ := ParseRecipient("CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f")
	connID, _ := GenerateConnectionID()

	msg := &Message{
		Type: MessageTypeConnectionRequest,
		Connection: &ConnectionMessage{
			PeerID:    peerID,
			Recipient: &recipient,
			ID:        connID,
		},
	}

	encoded, _ := Encode(msg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Decode(encoded)
		if err != nil {
			b.Fatal(err)
		}
	}
}
