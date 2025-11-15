package message

import (
	"encoding/binary"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Encode serialises the message to the on-wire representation used by rust-libp2p-nym.
func Encode(msg *Message) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("message: encode nil message")
	}

	var payload []byte
	switch msg.Type {
	case MessageTypeConnectionRequest, MessageTypeConnectionResponse:
		cm := msg.Connection
		if cm == nil {
			return nil, fmt.Errorf("message: missing connection payload")
		}
		payload = encodeConnectionMessage(cm)
	case MessageTypeTransport:
		tm := msg.Transport
		if tm == nil {
			return nil, fmt.Errorf("message: missing transport payload")
		}
		payload = encodeTransportMessage(tm)
	default:
		return nil, fmt.Errorf("message: unknown type %d", msg.Type)
	}

	out := make([]byte, 1+len(payload))
	out[0] = byte(msg.Type)
	copy(out[1:], payload)
	return out, nil
}

// Decode parses a binary message emitted by rust-libp2p-nym.
func Decode(data []byte) (*Message, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("message: decode short buffer")
	}

	msgType := MessageType(data[0])
	payload := data[1:]
	switch msgType {
	case MessageTypeConnectionRequest, MessageTypeConnectionResponse:
		cm, err := decodeConnectionMessage(payload)
		if err != nil {
			return nil, err
		}
		return &Message{Type: msgType, Connection: cm}, nil
	case MessageTypeTransport:
		tm, err := decodeTransportMessage(payload)
		if err != nil {
			return nil, err
		}
		return &Message{Type: msgType, Transport: tm}, nil
	default:
		return nil, fmt.Errorf("message: unknown type %d", msgType)
	}
}

func encodeConnectionMessage(cm *ConnectionMessage) []byte {
	peerBytes := []byte(cm.PeerID)
	var flag byte
	var recipientBytes []byte
	if cm.Recipient != nil {
		flag = 1
		recipientBytes = cm.Recipient.Bytes()
	}

	out := make([]byte, ConnectionIDLength+1+len(recipientBytes)+len(peerBytes))
	copy(out[:ConnectionIDLength], cm.ID[:])
	out[ConnectionIDLength] = flag
	offset := ConnectionIDLength + 1
	copy(out[offset:], recipientBytes)
	offset += len(recipientBytes)
	copy(out[offset:], peerBytes)
	return out
}

func decodeConnectionMessage(data []byte) (*ConnectionMessage, error) {
	minLen := ConnectionIDLength + 1
	if len(data) < minLen {
		return nil, fmt.Errorf("message: connection payload too short")
	}
	var id ConnectionID
	copy(id[:], data[:ConnectionIDLength])
	flag := data[ConnectionIDLength]
	cursor := ConnectionIDLength + 1

	var recipient *Recipient
	if flag == 1 {
		if len(data) < cursor+RecipientLength {
			return nil, fmt.Errorf("message: connection recipient truncated")
		}
		rec, err := RecipientFromBytes(data[cursor : cursor+RecipientLength])
		if err != nil {
			return nil, fmt.Errorf("message: parse recipient: %w", err)
		}
		recipient = &rec
		cursor += RecipientLength
	} else if flag != 0 {
		return nil, fmt.Errorf("message: invalid recipient flag %d", flag)
	}

	if len(data) <= cursor {
		return nil, fmt.Errorf("message: missing peer id bytes")
	}

	peerID, err := peer.IDFromBytes(data[cursor:])
	if err != nil {
		return nil, fmt.Errorf("message: parse peer id: %w", err)
	}

	return &ConnectionMessage{
		PeerID:    peerID,
		Recipient: recipient,
		ID:        id,
	}, nil
}

func encodeTransportMessage(tm *TransportMessage) []byte {
	substreamBytes := encodeSubstreamMessage(&tm.Message)

	out := make([]byte, 8+ConnectionIDLength+len(substreamBytes))
	binary.BigEndian.PutUint64(out[:8], tm.Nonce)
	copy(out[8:8+ConnectionIDLength], tm.ID[:])
	copy(out[8+ConnectionIDLength:], substreamBytes)
	return out
}

func decodeTransportMessage(data []byte) (*TransportMessage, error) {
	minLen := 8 + ConnectionIDLength + SubstreamIDLength + 1
	if len(data) < minLen {
		return nil, fmt.Errorf("message: transport payload too short")
	}

	nonce := binary.BigEndian.Uint64(data[:8])
	var id ConnectionID
	copy(id[:], data[8:8+ConnectionIDLength])

	substream, err := decodeSubstreamMessage(data[8+ConnectionIDLength:])
	if err != nil {
		return nil, err
	}

	return &TransportMessage{
		Nonce:   nonce,
		Message: *substream,
		ID:      id,
	}, nil
}

func encodeSubstreamMessage(sm *SubstreamMessage) []byte {
	out := make([]byte, SubstreamIDLength+1+len(sm.Data))
	copy(out[:SubstreamIDLength], sm.ID[:])
	out[SubstreamIDLength] = byte(sm.Type)
	copy(out[SubstreamIDLength+1:], sm.Data)
	return out
}

func decodeSubstreamMessage(data []byte) (*SubstreamMessage, error) {
	if len(data) < SubstreamIDLength+1 {
		return nil, fmt.Errorf("message: substream payload too short")
	}
	var id SubstreamID
	copy(id[:], data[:SubstreamIDLength])
	msgType := SubstreamMessageType(data[SubstreamIDLength])

	payload := data[SubstreamIDLength+1:]
	switch msgType {
	case SubstreamMessageOpenRequest, SubstreamMessageOpenResponse, SubstreamMessageClose:
		if len(payload) != 0 {
			return nil, fmt.Errorf("message: unexpected payload for substream control message")
		}
		return &SubstreamMessage{ID: id, Type: msgType}, nil
	case SubstreamMessageData:
		buf := make([]byte, len(payload))
		copy(buf, payload)
		return &SubstreamMessage{ID: id, Type: msgType, Data: buf}, nil
	default:
		return nil, fmt.Errorf("message: unknown substream type %d", msgType)
	}
}
