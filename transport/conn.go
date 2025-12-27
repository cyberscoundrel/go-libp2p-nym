package transport

import (
	"context"
	"encoding/hex"
	"sync"
	"sync/atomic"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	lptransport "github.com/libp2p/go-libp2p/core/transport"
	ma "github.com/multiformats/go-multiaddr"

	"banyan/transports/nym/message"
	"banyan/transports/nym/queue"
)

type Conn struct {
	transport *Transport
	id        message.ConnectionID

	localPeer  peer.ID
	remotePeer peer.ID

	localAddr  ma.Multiaddr
	remoteAddr ma.Multiaddr

	remoteRecipient message.Recipient

	queue *queue.MessageQueue

	inboundSubstreams chan *Substream
	closeCh           chan struct{}
	closed            atomic.Bool

	streamsMu       sync.Mutex
	streams         map[string]*Substream
	pendingOutbound map[string]*pendingSubstream

	nonce uint64

	scope network.ConnScope
}

type pendingSubstream struct {
	stream *Substream
	ready  chan struct{}
}

func newConn(t *Transport, connID message.ConnectionID, remotePeer peer.ID, remoteRecipient message.Recipient, q *queue.MessageQueue) (*Conn, error) {
	remoteAddr, err := multiaddrFromRecipient(remoteRecipient)
	if err != nil {
		return nil, err
	}

	conn := &Conn{
		transport:         t,
		id:                connID,
		localPeer:         t.localPeer,
		remotePeer:        remotePeer,
		localAddr:         t.listenAddr,
		remoteAddr:        remoteAddr,
		remoteRecipient:   remoteRecipient,
		queue:             q,
		inboundSubstreams: make(chan *Substream, 8),
		closeCh:           make(chan struct{}),
		streams:           make(map[string]*Substream),
		pendingOutbound:   make(map[string]*pendingSubstream),
		scope:             &network.NullScope{},
	}

	return conn, nil
}

func (c *Conn) handleTransportMessage(msg message.TransportMessage) {
	if ready, ok := c.queue.TryPush(msg); ok && ready != nil {
		c.processOrderedMessage(*ready)
	}
	for {
		next, ok := c.queue.Pop()
		if !ok || next == nil {
			break
		}
		c.processOrderedMessage(*next)
	}
}

func (c *Conn) processOrderedMessage(msg message.TransportMessage) {
	subMsg := msg.Message
	switch subMsg.Type {
	case message.SubstreamMessageOpenRequest:
		c.handleOpenRequest(subMsg.ID)
	case message.SubstreamMessageOpenResponse:
		c.handleOpenResponse(subMsg.ID)
	case message.SubstreamMessageData:
		c.handleData(subMsg.ID, subMsg.Data)
	case message.SubstreamMessageClose:
		c.handleClose(subMsg.ID)
	}
}

func (c *Conn) handleOpenRequest(id message.SubstreamID) {
	stream := newSubstream(c, id)

	c.streamsMu.Lock()
	c.streams[substreamKey(id)] = stream
	c.streamsMu.Unlock()

	_ = c.sendControl(id, message.SubstreamMessageOpenResponse)
	c.enqueueInboundStream(stream)
}

func (c *Conn) handleOpenResponse(id message.SubstreamID) {
	key := substreamKey(id)

	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()

	pending, ok := c.pendingOutbound[key]
	if !ok {
		return
	}
	delete(c.pendingOutbound, key)
	c.streams[key] = pending.stream
	close(pending.ready)
}

func (c *Conn) handleData(id message.SubstreamID, data []byte) {
	stream := c.getStream(id)
	if stream == nil {
		return
	}
	stream.pushData(data)
}

func (c *Conn) handleClose(id message.SubstreamID) {
	stream := c.removeStream(id)
	if stream != nil {
		stream.remoteClose()
	}
}

func (c *Conn) enqueueInboundStream(stream *Substream) {
	select {
	case <-c.closeCh:
		return
	case c.inboundSubstreams <- stream:
	default:
		go func() {
			select {
			case <-c.closeCh:
			case c.inboundSubstreams <- stream:
			}
		}()
	}
}

func (c *Conn) getStream(id message.SubstreamID) *Substream {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	return c.streams[substreamKey(id)]
}

func (c *Conn) removeStream(id message.SubstreamID) *Substream {
	key := substreamKey(id)
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	stream := c.streams[key]
	delete(c.streams, key)
	if pending, ok := c.pendingOutbound[key]; ok {
		delete(c.pendingOutbound, key)
		close(pending.ready)
	}
	return stream
}

// network.ConnMultiaddrs

func (c *Conn) LocalMultiaddr() ma.Multiaddr {
	return c.localAddr
}

func (c *Conn) RemoteMultiaddr() ma.Multiaddr {
	return c.remoteAddr
}

// network.ConnSecurity

func (c *Conn) LocalPeer() peer.ID {
	return c.localPeer
}

func (c *Conn) RemotePeer() peer.ID {
	return c.remotePeer
}

func (c *Conn) RemotePublicKey() crypto.PubKey {
	return nil
}

func (c *Conn) ConnState() network.ConnectionState {
	return network.ConnectionState{
		Transport: nymProtocolName,
	}
}

// network.ConnScoper

func (c *Conn) Scope() network.ConnScope {
	return c.scope
}

// network.MuxedConn

func (c *Conn) OpenStream(ctx context.Context) (network.MuxedStream, error) {
	if c.closed.Load() {
		return nil, network.ErrReset
	}

	id, err := message.GenerateSubstreamID()
	if err != nil {
		return nil, err
	}

	stream := newSubstream(c, id)
	key := substreamKey(id)
	pending := &pendingSubstream{
		stream: stream,
		ready:  make(chan struct{}),
	}

	c.streamsMu.Lock()
	c.pendingOutbound[key] = pending
	c.streamsMu.Unlock()

	if err := c.sendControl(id, message.SubstreamMessageOpenRequest); err != nil {
		c.streamsMu.Lock()
		delete(c.pendingOutbound, key)
		c.streamsMu.Unlock()
		return nil, err
	}

	select {
	case <-pending.ready:
		return stream, nil
	case <-ctx.Done():
		c.streamsMu.Lock()
		delete(c.pendingOutbound, key)
		c.streamsMu.Unlock()
		return nil, ctx.Err()
	case <-c.closeCh:
		return nil, network.ErrReset
	}
}

func (c *Conn) AcceptStream() (network.MuxedStream, error) {
	select {
	case <-c.closeCh:
		return nil, network.ErrReset
	case stream, ok := <-c.inboundSubstreams:
		if !ok {
			return nil, network.ErrReset
		}
		return stream, nil
	}
}

func (c *Conn) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(c.closeCh)
	close(c.inboundSubstreams)

	c.transport.removeConnection(c)

	c.streamsMu.Lock()
	for key, pending := range c.pendingOutbound {
		close(pending.ready)
		delete(c.pendingOutbound, key)
	}
	for key, stream := range c.streams {
		delete(c.streams, key)
		stream.remoteClose()
	}
	c.streamsMu.Unlock()

	return nil
}

func (c *Conn) IsClosed() bool {
	return c.closed.Load()
}

func (c *Conn) CloseWithError(errCode network.ConnErrorCode) error {
	// Nym mixnet doesn't support sending error codes, just close
	return c.Close()
}

func (c *Conn) As(target any) bool {
	// No wrapped connections
	return false
}

func (c *Conn) sendControl(id message.SubstreamID, typ message.SubstreamMessageType) error {
	return c.sendSubstreamMessage(message.SubstreamMessage{
		ID:   id,
		Type: typ,
	})
}

func (c *Conn) sendData(id message.SubstreamID, data []byte) error {
	return c.sendSubstreamMessage(message.SubstreamMessage{
		ID:   id,
		Type: message.SubstreamMessageData,
		Data: data,
	})
}

func (c *Conn) sendSubstreamMessage(sub message.SubstreamMessage) error {
	nonce := atomic.AddUint64(&c.nonce, 1)
	msg := &message.Message{
		Type: message.MessageTypeTransport,
		Transport: &message.TransportMessage{
			Nonce:   nonce,
			Message: sub,
			ID:      c.id,
		},
	}
	return c.transport.sendMixnetMessage(c.remoteRecipient, msg)
}

func (c *Conn) closeLocalStream(stream *Substream) {
	if stream == nil {
		return
	}
	if c.closed.Load() {
		return
	}
	c.sendControl(stream.id, message.SubstreamMessageClose)
	c.removeStream(stream.id)
}

func substreamKey(id message.SubstreamID) string {
	return hex.EncodeToString(id[:])
}

func (c *Conn) Transport() lptransport.Transport {
	return c.transport
}

var _ network.ConnMultiaddrs = (*Conn)(nil)
var _ network.ConnSecurity = (*Conn)(nil)
var _ network.ConnScoper = (*Conn)(nil)
var _ network.MuxedConn = (*Conn)(nil)
var _ lptransport.CapableConn = (*Conn)(nil)
