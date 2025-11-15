package mixnet

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"nymtrans/go-libp2p-nym/message"
)

const (
	requestTagSend        = 0x00
	requestTagSelfAddress = 0x03
)

const (
	responseTagError       = 0x00
	responseTagReceived    = 0x01
	responseTagSelfAddress = 0x02
)

// senderTagSize defines the length of the optional reply SURB sender tag.
// The current transport doesn't support anonymous replies and will reject frames
// carrying the tag to avoid mis-parsing the payload.
const senderTagSize = 32

var errUnsupportedSenderTag = errors.New("mixnet: received message with sender tag is unsupported")

func serializeSelfAddressRequest() []byte {
	return []byte{requestTagSelfAddress}
}

func serializeSendRequest(recipient message.Recipient, payload []byte) []byte {
	size := 1 + message.RecipientLength + 8 + 8 + len(payload)
	buf := make([]byte, size)
	buf[0] = requestTagSend
	copy(buf[1:1+message.RecipientLength], recipient.Bytes())
	// connection id is currently unused, set to zero.
	// it occupies bytes [1+RecipientLength, 1+RecipientLength+8)
	offset := 1 + message.RecipientLength
	// connection id -> zero (already zeroed by make)
	offset += 8
	binary.BigEndian.PutUint64(buf[offset:offset+8], uint64(len(payload)))
	offset += 8
	copy(buf[offset:], payload)
	return buf
}

func decodeServerResponse(data []byte) (serverResponse, error) {
	if len(data) == 0 {
		return serverResponse{}, fmt.Errorf("mixnet: empty response")
	}

	switch data[0] {
	case responseTagReceived:
		payload, err := decodeReceivedPayload(data)
		if err != nil {
			return serverResponse{}, err
		}
		return serverResponse{kind: responseTagReceived, payload: payload}, nil
	case responseTagSelfAddress:
		if len(data) != 1+message.RecipientLength {
			return serverResponse{}, fmt.Errorf("mixnet: invalid self address response length %d", len(data))
		}
		recipient, err := message.RecipientFromBytes(data[1:])
		if err != nil {
			return serverResponse{}, fmt.Errorf("mixnet: decode self address: %w", err)
		}
		return serverResponse{kind: responseTagSelfAddress, payload: recipient}, nil
	case responseTagError:
		if len(data) < 2+8 {
			return serverResponse{}, fmt.Errorf("mixnet: error response too short")
		}
		code := data[1]
		msgLen := binary.BigEndian.Uint64(data[2 : 2+8])
		if int(msgLen) != len(data)-(2+8) {
			return serverResponse{}, fmt.Errorf("mixnet: malformed error response length")
		}
		return serverResponse{
			kind:    responseTagError,
			payload: fmt.Sprintf("remote error code=%d msg=%s", code, string(data[10:])),
		}, nil
	default:
		return serverResponse{}, fmt.Errorf("mixnet: unknown response tag %d", data[0])
	}
}

type serverResponse struct {
	kind    byte
	payload any
}

func decodeReceivedPayload(data []byte) ([]byte, error) {
	if len(data) < 2+8 {
		return nil, fmt.Errorf("mixnet: received response too short")
	}

	hasTag := data[1]
	offset := 2
	if hasTag == 1 {
		// The current transport implementation does not support sender tags.
		// Bail out explicitly to avoid parsing inconsistent payloads.
		if len(data) < offset+senderTagSize+8 {
			return nil, fmt.Errorf("mixnet: received response missing sender tag bytes")
		}
		return nil, errUnsupportedSenderTag
	} else if hasTag != 0 {
		return nil, fmt.Errorf("mixnet: invalid sender tag marker %d", hasTag)
	}

	if len(data) < offset+8 {
		return nil, fmt.Errorf("mixnet: received response missing length")
	}

	length := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8
	if int(length) != len(data)-offset {
		return nil, fmt.Errorf("mixnet: received response malformed length expected %d got %d", length, len(data)-offset)
	}

	msg := make([]byte, length)
	copy(msg, data[offset:])
	return msg, nil
}

func encodeMessagePayload(msg *message.Message) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("mixnet: nil message")
	}
	return message.Encode(msg)
}

func decodeMessagePayload(data []byte) (*message.Message, error) {
	return message.Decode(data)
}

// isContextDone returns true if the context has been cancelled.
func isContextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

