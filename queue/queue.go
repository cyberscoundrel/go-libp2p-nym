package queue

import (
	"sort"
	"sync"

	"banyan/transports/nym/message"
)

// MessageQueue reorders transport messages by nonce for a connection.
type MessageQueue struct {
	mu                sync.Mutex
	nextExpectedNonce uint64
	pending           map[uint64]message.TransportMessage
	nonces            []uint64
}

// New returns an empty queue.
func New() *MessageQueue {
	return &MessageQueue{
		pending: make(map[uint64]message.TransportMessage),
	}
}

// SetConnectionMessageReceived initialises the queue once the handshake completed.
func (mq *MessageQueue) SetConnectionMessageReceived() {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if mq.nextExpectedNonce != 0 {
		panic("queue: connection message received twice")
	}
	mq.nextExpectedNonce = 1
}

// TryPush inserts a transport message.
// If the message has the next expected nonce, it is returned immediately for processing.
func (mq *MessageQueue) TryPush(msg message.TransportMessage) (*message.TransportMessage, bool) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	nonce := msg.Nonce
	if mq.nextExpectedNonce == 0 {
		// handshake not completed yet, buffer everything
		mq.insertLocked(msg)
		return nil, false
	}

	if nonce == mq.nextExpectedNonce {
		mq.nextExpectedNonce++
		return &msg, true
	}
	if nonce < mq.nextExpectedNonce {
		return nil, false
	}

	if _, exists := mq.pending[nonce]; !exists {
		mq.insertLocked(msg)
	}
	return nil, false
}

// Pop returns queued messages in order if available.
func (mq *MessageQueue) Pop() (*message.TransportMessage, bool) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	if mq.nextExpectedNonce == 0 {
		return nil, false
	}

	if len(mq.nonces) == 0 {
		return nil, false
	}

	smallest := mq.nonces[0]
	if smallest != mq.nextExpectedNonce {
		return nil, false
	}

	msg := mq.pending[smallest]
	delete(mq.pending, smallest)
	mq.nonces = mq.nonces[1:]
	mq.nextExpectedNonce++
	return &msg, true
}

// PendingNonces returns a snapshot of queued nonces (useful for debugging).
func (mq *MessageQueue) PendingNonces() []uint64 {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	out := make([]uint64, len(mq.nonces))
	copy(out, mq.nonces)
	return out
}

func (mq *MessageQueue) insertLocked(msg message.TransportMessage) {
	nonce := msg.Nonce
	mq.pending[nonce] = msg
	idx := sort.Search(len(mq.nonces), func(i int) bool {
		return mq.nonces[i] >= nonce
	})
	if idx == len(mq.nonces) {
		mq.nonces = append(mq.nonces, nonce)
	} else if mq.nonces[idx] == nonce {
		// duplicate, nothing else to do because msg already stored.
	} else {
		mq.nonces = append(mq.nonces, 0)
		copy(mq.nonces[idx+1:], mq.nonces[idx:])
		mq.nonces[idx] = nonce
	}
}

// Reset clears the queue (used when tearing down connections).
func (mq *MessageQueue) Reset() {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	mq.nextExpectedNonce = 0
	mq.pending = make(map[uint64]message.TransportMessage)
	mq.nonces = mq.nonces[:0]
}
