package transport

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/network"

	"banyan/transports/nym/message"
)

type Substream struct {
	conn *Conn
	id   message.SubstreamID

	inbound chan []byte
	buffer  []byte

	localClosed  atomic.Bool
	remoteClosed atomic.Bool

	channelOnce sync.Once

	readDeadline  atomic.Pointer[time.Time]
	writeDeadline atomic.Pointer[time.Time]
}

func newSubstream(conn *Conn, id message.SubstreamID) *Substream {
	return &Substream{
		conn:    conn,
		id:      id,
		inbound: make(chan []byte, 32),
	}
}

func (s *Substream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if len(s.buffer) == 0 {
		data, ok := <-s.inbound
		if !ok {
			return 0, io.EOF
		}
		if len(data) == 0 {
			return 0, nil
		}
		s.buffer = append(s.buffer, data...)
	}

	n := copy(p, s.buffer)
	s.buffer = s.buffer[n:]
	return n, nil
}

func (s *Substream) Write(p []byte) (int, error) {
	if s.localClosed.Load() {
		return 0, errors.New("substream closed")
	}
	buf := make([]byte, len(p))
	copy(buf, p)
	if err := s.conn.sendData(s.id, buf); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *Substream) Close() error {
	return s.closeWithControl(true)
}

func (s *Substream) CloseWrite() error {
	return s.closeWithControl(true)
}

func (s *Substream) CloseRead() error {
	return s.closeWithControl(false)
}

func (s *Substream) Reset() error {
	return s.closeWithControl(true)
}

func (s *Substream) ResetWithError(errCode network.StreamErrorCode) error {
	// Nym mixnet doesn't support sending error codes, just reset
	return s.Reset()
}

func (s *Substream) SetDeadline(time.Time) error {
	return nil
}

func (s *Substream) SetReadDeadline(time.Time) error {
	return nil
}

func (s *Substream) SetWriteDeadline(time.Time) error {
	return nil
}

func (s *Substream) closeWithControl(sendControl bool) error {
	if s.localClosed.Swap(true) {
		return nil
	}
	if sendControl {
		s.conn.closeLocalStream(s)
	}
	s.remoteClosed.Store(true)
	s.channelOnce.Do(func() {
		close(s.inbound)
	})
	return nil
}

func (s *Substream) pushData(data []byte) {
	if s.remoteClosed.Load() || s.localClosed.Load() {
		return
	}

	buf := make([]byte, len(data))
	copy(buf, data)

	defer func() {
		if r := recover(); r != nil {
			// channel closed concurrently; ignore
		}
	}()

	select {
	case <-s.conn.closeCh:
	case s.inbound <- buf:
	}
}

func (s *Substream) remoteClose() {
	if s.remoteClosed.Swap(true) {
		return
	}
	s.channelOnce.Do(func() {
		close(s.inbound)
	})
}

var _ network.MuxedStream = (*Substream)(nil)
