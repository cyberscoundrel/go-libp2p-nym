package mixnet

import (
	"context"
	"log"
	"sync"

	"github.com/gorilla/websocket"

	"nymtrans/go-libp2p-nym/message"
)

// InboundMessage represents a message received from the mixnet which should be
// forwarded to the libp2p transport.
type InboundMessage struct {
	Message *message.Message
}

// OutboundMessage represents a message destined for the mixnet.
type OutboundMessage struct {
	Recipient message.Recipient
	Message   *message.Message
}

// Initialize establishes a websocket connection to the Nym client mixnet gateway,
// returning the local recipient address alongside inbound/outbound channels.
// If notifyInbound is non-nil, it will receive a signal every time an inbound
// message is delivered.
func Initialize(ctx context.Context, uri string, notifyInbound chan<- struct{}) (message.Recipient, <-chan InboundMessage, chan<- OutboundMessage, error) {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, uri, nil)
	if err != nil {
		return message.Recipient{}, nil, nil, err
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, serializeSelfAddressRequest()); err != nil {
		conn.Close()
		return message.Recipient{}, nil, nil, err
	}

	inbound := make(chan InboundMessage, 32)
	outbound := make(chan OutboundMessage, 32)

	var self message.Recipient
	// Fetch self address synchronously before launching the workers.
	for {
		if isContextDone(ctx) {
			conn.Close()
			return message.Recipient{}, nil, nil, context.Canceled
		}

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			conn.Close()
			return message.Recipient{}, nil, nil, err
		}
		if msgType != websocket.BinaryMessage {
			continue
		}
		resp, err := decodeServerResponse(data)
		if err != nil {
			log.Printf("mixnet: failed to decode handshake response: %v", err)
			continue
		}
		switch resp.kind {
		case responseTagSelfAddress:
			self = resp.payload.(message.Recipient)
		case responseTagReceived:
			payload := resp.payload.([]byte)
			m, err := decodeMessagePayload(payload)
			if err != nil {
				log.Printf("mixnet: failed to decode pre-handshake message: %v", err)
				continue
			}
			select {
			case inbound <- InboundMessage{Message: m}:
			default:
				log.Printf("mixnet: dropping pre-handshake message due to full queue")
			}
		case responseTagError:
			log.Printf("mixnet: gateway error during handshake: %v", resp.payload)
		default:
			log.Printf("mixnet: ignoring unexpected handshake response tag %d", resp.kind)
		}
		if self != (message.Recipient{}) {
			break
		}
	}

	var (
		writeOnce sync.Once
		closer    = func() {
			writeOnce.Do(func() {
				conn.Close()
			})
		}
	)

	// Writer goroutine.
	go func() {
		defer closer()
		for {
			select {
			case <-ctx.Done():
				return
			case outboundMsg, ok := <-outbound:
				if !ok {
					return
				}
				payload, err := encodeMessagePayload(outboundMsg.Message)
				if err != nil {
					log.Printf("mixnet: encode outbound message: %v", err)
					continue
				}
				req := serializeSendRequest(outboundMsg.Recipient, payload)
				if err := conn.WriteMessage(websocket.BinaryMessage, req); err != nil {
					log.Printf("mixnet: failed to write message: %v", err)
					return
				}
			}
		}
	}()

	// Reader goroutine.
	go func() {
		defer func() {
			closer()
			close(inbound)
		}()
		for {
			if isContextDone(ctx) {
				return
			}
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				log.Printf("mixnet: read error: %v", err)
				return
			}
			if msgType != websocket.BinaryMessage {
				continue
			}

			resp, err := decodeServerResponse(data)
			if err != nil {
				log.Printf("mixnet: failed to decode response: %v", err)
				continue
			}

			switch resp.kind {
			case responseTagReceived:
				payload := resp.payload.([]byte)
				m, err := decodeMessagePayload(payload)
				if err != nil {
					log.Printf("mixnet: failed to decode message payload: %v", err)
					continue
				}
				select {
				case inbound <- InboundMessage{Message: m}:
					if notifyInbound != nil {
						select {
						case notifyInbound <- struct{}{}:
						default:
						}
					}
				default:
					log.Printf("mixnet: inbound queue full, dropping message")
				}
			case responseTagSelfAddress:
				// Additional self address responses are unexpected but harmless.
				log.Printf("mixnet: received duplicate self address response")
			case responseTagError:
				log.Printf("mixnet: gateway error: %v", resp.payload)
			default:
				log.Printf("mixnet: unknown response tag %d", resp.kind)
			}
		}
	}()

	return self, inbound, outbound, nil
}

