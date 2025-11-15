package transport

import (
	"context"
	"net"
	"sync"

	lptransport "github.com/libp2p/go-libp2p/core/transport"
	ma "github.com/multiformats/go-multiaddr"
)

type listener struct {
	t        *Transport
	incoming chan *Conn
	closed   chan struct{}
	once     sync.Once
}

func newListener(t *Transport) *listener {
	return &listener{
		t:        t,
		incoming: make(chan *Conn, 16),
		closed:   make(chan struct{}),
	}
}

func (l *listener) Accept() (lptransport.CapableConn, error) {
	select {
	case <-l.closed:
		return nil, lptransport.ErrListenerClosed
	case <-l.t.ctx.Done():
		return nil, context.Canceled
	case conn, ok := <-l.incoming:
		if !ok {
			return nil, lptransport.ErrListenerClosed
		}
		return conn, nil
	}
}

func (l *listener) Close() error {
	l.shutdown()
	l.t.mu.Lock()
	delete(l.t.listeners, l)
	l.t.mu.Unlock()
	return nil
}

func (l *listener) shutdown() {
	l.once.Do(func() {
		close(l.closed)
		close(l.incoming)
	})
}

func (l *listener) Addr() net.Addr {
	return maNetAddr{ma: l.t.listenAddr}
}

func (l *listener) Multiaddr() ma.Multiaddr {
	return l.t.listenAddr
}

func (l *listener) enqueue(conn *Conn) {
	select {
	case <-l.closed:
		return
	case l.incoming <- conn:
	default:
		go func() {
			select {
			case <-l.closed:
			case l.incoming <- conn:
			}
		}()
	}
}

type maNetAddr struct {
	ma ma.Multiaddr
}

func (a maNetAddr) Network() string {
	return nymProtocolName
}

func (a maNetAddr) String() string {
	return a.ma.String()
}

