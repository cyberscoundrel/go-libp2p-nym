package transport

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	lptransport "github.com/libp2p/go-libp2p/core/transport"
	ma "github.com/multiformats/go-multiaddr"

	"nymtrans/go-libp2p-nym/message"
	"nymtrans/go-libp2p-nym/mixnet"
	"nymtrans/go-libp2p-nym/queue"
)

// Transport implements the go-libp2p transport interface over the Nym mixnet.
type Transport struct {
	ctx    context.Context
	cancel context.CancelFunc

	privKey   crypto.PrivKey
	localPeer peer.ID

	selfRecipient message.Recipient
	listenAddr    ma.Multiaddr

	mixnetInbound  <-chan mixnet.InboundMessage
	mixnetOutbound chan<- mixnet.OutboundMessage

	handshakeTimeout time.Duration

	mu           sync.RWMutex
	listeners    map[*listener]struct{}
	connections  map[string]*Conn
	pendingDials map[string]*dialState
}

type dialState struct {
	remoteRecipient message.Recipient
	resultCh        chan *Conn
}

// New creates a new transport instance that connects to the provided Nym websocket URI.
func New(ctx context.Context, uri string, privKey crypto.PrivKey) (*Transport, error) {
	ensureProtocolRegistered()

	self, inbound, outbound, err := mixnet.Initialize(ctx, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("nym transport: initialize mixnet: %w", err)
	}

	return newWithMixnet(ctx, privKey, self, inbound, outbound)
}

func newWithMixnet(ctx context.Context, privKey crypto.PrivKey, self message.Recipient, inbound <-chan mixnet.InboundMessage, outbound chan<- mixnet.OutboundMessage) (*Transport, error) {
	ensureProtocolRegistered()
	ctx, cancel := context.WithCancel(ctx)

	pub := privKey.GetPublic()
	localPeer, err := peer.IDFromPublicKey(pub)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("nym transport: derive peer id: %w", err)
	}

	addr, err := multiaddrFromRecipient(self)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("nym transport: build listen address: %w", err)
	}

	t := &Transport{
		ctx:              ctx,
		cancel:           cancel,
		privKey:          privKey,
		localPeer:        localPeer,
		selfRecipient:    self,
		listenAddr:       addr,
		mixnetInbound:    inbound,
		mixnetOutbound:   outbound,
		handshakeTimeout: 5 * time.Second,
		listeners:        make(map[*listener]struct{}),
		connections:      make(map[string]*Conn),
		pendingDials:     make(map[string]*dialState),
	}

	go t.processInbound()

	return t, nil
}

// Proxy indicates whether the transport is a proxy transport.
func (t *Transport) Proxy() bool {
	return false
}

// Protocols returns the set of supported multiaddr protocol codes.
func (t *Transport) Protocols() []int {
	return []int{nymProtocolCode}
}

// CanDial determines whether this transport can dial the given multiaddr.
func (t *Transport) CanDial(addr ma.Multiaddr) bool {
	return hasNymProtocol(addr)
}

// Close releases transport resources.
func (t *Transport) Close() error {
	t.cancel()

	// Collect listeners to shutdown
	t.mu.Lock()
	listeners := make([]*listener, 0, len(t.listeners))
	for l := range t.listeners {
		listeners = append(listeners, l)
		delete(t.listeners, l)
	}

	// Collect connections to close
	connections := make([]*Conn, 0, len(t.connections))
	for key, conn := range t.connections {
		connections = append(connections, conn)
		delete(t.connections, key)
	}

	// Close pending dials
	for key, dial := range t.pendingDials {
		close(dial.resultCh)
		delete(t.pendingDials, key)
	}
	t.mu.Unlock()

	// Shutdown listeners without holding the lock
	for _, l := range listeners {
		l.shutdown()
	}

	// Close connections without holding the lock
	// This prevents deadlock since conn.Close() calls removeConnection()
	for _, conn := range connections {
		conn.Close()
	}

	return nil
}

func hasNymProtocol(addr ma.Multiaddr) bool {
	found := false
	ma.ForEach(addr, func(c ma.Component) bool {
		if c.Protocol().Code == nymProtocolCode {
			found = true
			return false // stop iteration
		}
		return true // continue
	})
	return found
}

// Listen listens on the transport's Nym address.
func (t *Transport) Listen(laddr ma.Multiaddr) (lptransport.Listener, error) {
	if !laddr.Equal(t.listenAddr) {
		return nil, fmt.Errorf("nym transport: can only listen on %s", t.listenAddr)
	}

	l := newListener(t)
	t.mu.Lock()
	t.listeners[l] = struct{}{}
	t.mu.Unlock()
	return l, nil
}

// Dial dials a remote peer via the mixnet.
func (t *Transport) Dial(ctx context.Context, addr ma.Multiaddr, p peer.ID) (lptransport.CapableConn, error) {
	if !hasNymProtocol(addr) {
		return nil, fmt.Errorf("nym transport: unsupported address")
	}

	recipient, err := parseRecipientFromMultiaddr(addr)
	if err != nil {
		return nil, fmt.Errorf("nym transport: parse recipient: %w", err)
	}

	connID, err := message.GenerateConnectionID()
	if err != nil {
		return nil, fmt.Errorf("nym transport: generate connection id: %w", err)
	}

	resultCh := make(chan *Conn, 1)
	state := &dialState{
		remoteRecipient: recipient,
		resultCh:        resultCh,
	}
	key := connKey(connID)

	t.mu.Lock()
	if _, exists := t.pendingDials[key]; exists {
		t.mu.Unlock()
		return nil, fmt.Errorf("nym transport: connection id collision")
	}
	t.pendingDials[key] = state
	t.mu.Unlock()

	self := t.selfRecipient
	msg := &message.Message{
		Type: message.MessageTypeConnectionRequest,
		Connection: &message.ConnectionMessage{
			PeerID:    t.localPeer,
			Recipient: &self,
			ID:        connID,
		},
	}

	if err := t.sendMixnetMessage(recipient, msg); err != nil {
		t.removePendingDial(key)
		return nil, err
	}

	handshakeCtx, cancel := context.WithTimeout(ctx, t.handshakeTimeout)
	defer cancel()

	select {
	case conn, ok := <-resultCh:
		if !ok || conn == nil {
			return nil, fmt.Errorf("nym transport: dial aborted")
		}
		if p != "" && conn.remotePeer != p {
			conn.Close()
			return nil, fmt.Errorf("nym transport: remote peer mismatch")
		}
		return conn, nil
	case <-handshakeCtx.Done():
		t.removePendingDial(key)
		return nil, handshakeCtx.Err()
	case <-t.ctx.Done():
		t.removePendingDial(key)
		return nil, context.Canceled
	}
}

func (t *Transport) removePendingDial(key string) {
	t.mu.Lock()
	if state, ok := t.pendingDials[key]; ok {
		delete(t.pendingDials, key)
		close(state.resultCh)
	}
	t.mu.Unlock()
}

func (t *Transport) processInbound() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case inbound, ok := <-t.mixnetInbound:
			if !ok {
				return
			}
			if inbound.Message == nil {
				continue
			}
			if err := t.handleInboundMessage(inbound.Message); err != nil {
				log.Printf("nym transport: inbound message error: %v", err)
			}
		}
	}
}

func (t *Transport) handleInboundMessage(msg *message.Message) error {
	switch msg.Type {
	case message.MessageTypeConnectionRequest:
		if msg.Connection == nil {
			return fmt.Errorf("missing connection request payload")
		}
		return t.handleConnectionRequest(msg.Connection)
	case message.MessageTypeConnectionResponse:
		if msg.Connection == nil {
			return fmt.Errorf("missing connection response payload")
		}
		return t.handleConnectionResponse(msg.Connection)
	case message.MessageTypeTransport:
		if msg.Transport == nil {
			return fmt.Errorf("missing transport payload")
		}
		return t.handleTransportMessage(msg.Transport)
	default:
		return fmt.Errorf("unknown message type %d", msg.Type)
	}
}

func (t *Transport) handleConnectionRequest(connMsg *message.ConnectionMessage) error {
	if connMsg.Recipient == nil {
		return fmt.Errorf("connection request missing recipient")
	}

	key := connKey(connMsg.ID)

	t.mu.Lock()
	if _, exists := t.connections[key]; exists {
		t.mu.Unlock()
		return fmt.Errorf("connection already exists")
	}

	queue := queue.New()
	queue.SetConnectionMessageReceived()

	conn, err := newConn(t, connMsg.ID, connMsg.PeerID, *connMsg.Recipient, queue)
	if err != nil {
		t.mu.Unlock()
		return err
	}
	t.connections[key] = conn
	t.mu.Unlock()

	resp := &message.Message{
		Type: message.MessageTypeConnectionResponse,
		Connection: &message.ConnectionMessage{
			PeerID: t.localPeer,
			ID:     connMsg.ID,
		},
	}

	if err := t.sendMixnetMessage(*connMsg.Recipient, resp); err != nil {
		conn.Close()
		return err
	}

	t.notifyListeners(conn)
	return nil
}

func (t *Transport) handleConnectionResponse(connMsg *message.ConnectionMessage) error {
	key := connKey(connMsg.ID)

	t.mu.Lock()
	state, ok := t.pendingDials[key]
	if !ok {
		t.mu.Unlock()
		return fmt.Errorf("no pending dial for response")
	}
	delete(t.pendingDials, key)

	queue := queue.New()
	queue.SetConnectionMessageReceived()

	conn, err := newConn(t, connMsg.ID, connMsg.PeerID, state.remoteRecipient, queue)
	if err != nil {
		t.mu.Unlock()
		return err
	}
	t.connections[key] = conn
	t.mu.Unlock()

	select {
	case state.resultCh <- conn:
	default:
		conn.Close()
	}
	return nil
}

func (t *Transport) handleTransportMessage(transportMsg *message.TransportMessage) error {
	key := connKey(transportMsg.ID)
	t.mu.RLock()
	conn, ok := t.connections[key]
	t.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no connection for transport message")
	}

	conn.handleTransportMessage(*transportMsg)
	return nil
}

func (t *Transport) notifyListeners(conn *Conn) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for l := range t.listeners {
		l.enqueue(conn)
	}
}

func (t *Transport) removeConnection(conn *Conn) {
	key := connKey(conn.id)
	t.mu.Lock()
	delete(t.connections, key)
	t.mu.Unlock()
}

func (t *Transport) sendMixnetMessage(recipient message.Recipient, msg *message.Message) error {
	select {
	case <-t.ctx.Done():
		return context.Canceled
	case t.mixnetOutbound <- mixnet.OutboundMessage{
		Recipient: recipient,
		Message:   msg,
	}:
		return nil
	}
}

func multiaddrFromRecipient(rec message.Recipient) (ma.Multiaddr, error) {
	return ma.NewMultiaddr(fmt.Sprintf("/%s/%s", nymProtocolName, rec.String()))
}

func parseRecipientFromMultiaddr(addr ma.Multiaddr) (message.Recipient, error) {
	data := addr.Bytes()
	code, n, err := ma.ReadVarintCode(data)
	if err != nil {
		return message.Recipient{}, err
	}
	if code != nymProtocolCode {
		return message.Recipient{}, fmt.Errorf("unexpected protocol code %d", code)
	}
	data = data[n:]
	size, m, err := ma.ReadVarintCode(data)
	if err != nil {
		return message.Recipient{}, err
	}
	data = data[m:]
	if len(data) < size {
		return message.Recipient{}, fmt.Errorf("invalid nym multiaddr payload")
	}
	value := string(data[:size])
	return message.ParseRecipient(value)
}

func connKey(id message.ConnectionID) string {
	return hex.EncodeToString(id[:])
}

var _ lptransport.Transport = (*Transport)(nil)
