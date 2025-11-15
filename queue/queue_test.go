package queue

import (
	"testing"

	"nymtrans/go-libp2p-nym/message"
)

func createTestMessage(nonce uint64, data []byte) message.TransportMessage {
	connID, _ := message.GenerateConnectionID()
	streamID, _ := message.GenerateSubstreamID()

	return message.TransportMessage{
		ID:    connID,
		Nonce: nonce,
		Message: message.SubstreamMessage{
			ID:   streamID,
			Type: message.SubstreamMessageData,
			Data: data,
		},
	}
}

func TestQueueInOrder(t *testing.T) {
	q := New()
	q.SetConnectionMessageReceived()

	// Push messages in order
	for i := uint64(1); i <= 10; i++ {
		msg := createTestMessage(i, []byte{byte(i)})
		returned, ok := q.TryPush(msg)
		if !ok {
			t.Errorf("TryPush(%d) returned false", i)
		}
		if returned == nil {
			t.Errorf("TryPush(%d) returned nil message", i)
		}
		if returned.Nonce != i {
			t.Errorf("TryPush(%d) returned wrong nonce: got %d", i, returned.Nonce)
		}
	}

	// Queue should be empty now (all messages were returned immediately)
	if _, ok := q.Pop(); ok {
		t.Error("Pop() succeeded on empty queue")
	}
}

func TestQueueOutOfOrder(t *testing.T) {
	q := New()
	q.SetConnectionMessageReceived()

	// Push message 3 (should be buffered, waiting for 1)
	msg3 := createTestMessage(3, []byte{3})
	returned, ok := q.TryPush(msg3)
	if ok {
		t.Error("TryPush(3) should have buffered (waiting for 1)")
	}

	// Push message 5 (should be buffered)
	msg5 := createTestMessage(5, []byte{5})
	returned, ok = q.TryPush(msg5)
	if ok {
		t.Error("TryPush(5) should have buffered (waiting for 1)")
	}

	// Push message 1 (should be returned immediately)
	msg1 := createTestMessage(1, []byte{1})
	returned, ok = q.TryPush(msg1)
	if !ok || returned == nil {
		t.Fatal("TryPush(1) should have returned message")
	}
	if returned.Nonce != 1 {
		t.Errorf("TryPush(1) returned wrong nonce: got %d", returned.Nonce)
	}

	// Push message 4 (should be buffered, waiting for 2)
	msg4 := createTestMessage(4, []byte{4})
	returned, ok = q.TryPush(msg4)
	if ok {
		t.Error("TryPush(4) should have buffered (waiting for 2)")
	}

	// Push message 2 (should be returned immediately, and then 3 should be available)
	msg2 := createTestMessage(2, []byte{2})
	returned, ok = q.TryPush(msg2)
	if !ok || returned == nil {
		t.Fatal("TryPush(2) should have returned message")
	}
	if returned.Nonce != 2 {
		t.Errorf("TryPush(2) returned wrong nonce: got %d", returned.Nonce)
	}

	// Now pop messages 3 and 4 in order
	for i := uint64(3); i <= 4; i++ {
		msg, ok := q.Pop()
		if !ok {
			t.Errorf("Pop() failed at nonce %d", i)
			continue
		}
		if msg == nil {
			t.Errorf("Pop() returned nil at nonce %d", i)
			continue
		}
		if msg.Nonce != i {
			t.Errorf("Pop() returned wrong nonce: got %d, want %d", msg.Nonce, i)
		}
	}

	// Should not be able to pop 5 yet (gap at 5, next expected is 5 but we need to check)
	// Actually, after popping 4, next expected is 5, so we should be able to pop it
	msg, ok := q.Pop()
	if !ok {
		t.Error("Pop() failed for nonce 5")
	}
	if msg.Nonce != 5 {
		t.Errorf("Pop() returned wrong nonce: got %d, want 5", msg.Nonce)
	}

	// Queue should be empty now
	if _, ok := q.Pop(); ok {
		t.Error("Pop() succeeded on empty queue")
	}
}

func TestQueueGaps(t *testing.T) {
	q := New()
	q.SetConnectionMessageReceived()

	// Push messages with gaps
	msg1 := createTestMessage(1, []byte{1})
	returned, ok := q.TryPush(msg1)
	if !ok || returned == nil {
		t.Fatal("TryPush(1) failed")
	}

	msg3 := createTestMessage(3, []byte{3})
	returned, ok = q.TryPush(msg3)
	if ok {
		t.Error("TryPush(3) should have buffered (gap at 2)")
	}

	msg5 := createTestMessage(5, []byte{5})
	returned, ok = q.TryPush(msg5)
	if ok {
		t.Error("TryPush(5) should have buffered (gap at 2)")
	}

	// Next pop should fail because of gap at nonce 2
	if _, ok := q.Pop(); ok {
		t.Error("Pop() succeeded despite gap at nonce 2")
	}

	// Fill the gap at 2
	msg2 := createTestMessage(2, []byte{2})
	returned, ok = q.TryPush(msg2)
	if !ok || returned == nil {
		t.Fatal("TryPush(2) should have returned message")
	}
	if returned.Nonce != 2 {
		t.Errorf("TryPush(2) returned wrong nonce: got %d", returned.Nonce)
	}

	// Now we should be able to pop message 3
	msg, ok := q.Pop()
	if !ok {
		t.Fatal("Pop() failed after filling gap")
	}
	if msg.Nonce != 3 {
		t.Errorf("Pop() returned wrong nonce: got %d, want 3", msg.Nonce)
	}

	// Should not be able to pop message 5 yet (gap at 4)
	if _, ok := q.Pop(); ok {
		t.Error("Pop() succeeded despite gap at nonce 4")
	}

	// Fill the gap at 4
	msg4 := createTestMessage(4, []byte{4})
	returned, ok = q.TryPush(msg4)
	if !ok || returned == nil {
		t.Fatal("TryPush(4) should have returned message")
	}

	// Now we should be able to pop message 5
	msg, ok = q.Pop()
	if !ok {
		t.Fatal("Pop() failed after filling second gap")
	}
	if msg.Nonce != 5 {
		t.Errorf("Pop() returned wrong nonce: got %d, want 5", msg.Nonce)
	}
}

func TestQueueBeforeHandshake(t *testing.T) {
	q := New()

	// Push messages before handshake completes
	for i := uint64(1); i <= 5; i++ {
		msg := createTestMessage(i, []byte{byte(i)})
		returned, ok := q.TryPush(msg)
		if ok {
			t.Errorf("TryPush(%d) should have buffered before handshake", i)
		}
		if returned != nil {
			t.Errorf("TryPush(%d) should have returned nil before handshake", i)
		}
	}

	// Pop should fail before handshake
	if _, ok := q.Pop(); ok {
		t.Error("Pop() succeeded before handshake")
	}

	// Complete handshake
	q.SetConnectionMessageReceived()

	// Now we should be able to pop all messages in order
	for i := uint64(1); i <= 5; i++ {
		msg, ok := q.Pop()
		if !ok {
			t.Errorf("Pop() failed at nonce %d after handshake", i)
		}
		if msg.Nonce != i {
			t.Errorf("Pop() returned wrong nonce: got %d, want %d", msg.Nonce, i)
		}
	}
}

func TestQueueReset(t *testing.T) {
	q := New()
	q.SetConnectionMessageReceived()

	// Push some messages
	for i := uint64(1); i <= 5; i++ {
		msg := createTestMessage(i, []byte{byte(i)})
		q.TryPush(msg)
	}

	// Reset the queue
	q.Reset()

	// Pop should fail after reset
	if _, ok := q.Pop(); ok {
		t.Error("Pop() succeeded after reset")
	}

	// Should be able to push messages again after reset
	q.SetConnectionMessageReceived()
	msg := createTestMessage(1, []byte{1})
	returned, ok := q.TryPush(msg)
	if !ok || returned == nil {
		t.Error("TryPush(1) failed after reset and handshake")
	}
}

func TestQueuePendingNonces(t *testing.T) {
	q := New()
	q.SetConnectionMessageReceived()

	// Push message 1 (should be returned immediately)
	msg1 := createTestMessage(1, []byte{1})
	q.TryPush(msg1)

	// Push messages 3, 5, 7 (should be buffered)
	for _, nonce := range []uint64{3, 5, 7} {
		msg := createTestMessage(nonce, []byte{byte(nonce)})
		q.TryPush(msg)
	}

	// Check pending nonces
	pending := q.PendingNonces()
	if len(pending) != 3 {
		t.Errorf("PendingNonces() returned %d nonces, want 3", len(pending))
	}

	// Verify the nonces are in order
	expected := []uint64{3, 5, 7}
	for i, nonce := range pending {
		if nonce != expected[i] {
			t.Errorf("PendingNonces()[%d] = %d, want %d", i, nonce, expected[i])
		}
	}
}

func BenchmarkQueuePushPop(b *testing.B) {
	q := New()
	q.SetConnectionMessageReceived()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := createTestMessage(uint64(i+1), []byte{byte(i)})
		q.TryPush(msg)
	}
}

func BenchmarkQueuePushOutOfOrder(b *testing.B) {
	q := New()
	q.SetConnectionMessageReceived()

	// Pre-populate with some messages
	for i := 0; i < 1000; i += 2 {
		msg := createTestMessage(uint64(i+1), []byte{byte(i)})
		q.TryPush(msg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nonce := uint64(i*2 + 2)
		msg := createTestMessage(nonce, []byte{byte(nonce)})
		q.TryPush(msg)
	}
}
