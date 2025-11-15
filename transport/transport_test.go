package transport

import (
	"context"
	"crypto/rand"
	"io"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	lptransport "github.com/libp2p/go-libp2p/core/transport"
	ma "github.com/multiformats/go-multiaddr"

	"nymtrans/go-libp2p-nym/internal/testutil"
	"nymtrans/go-libp2p-nym/message"
)

func TestTransportDialAndStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	privA, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privB, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	recipientA := testRecipient(0x11)
	recipientB := testRecipient(0x22)

	inA, outA, inB, outB := testutil.PipeNetwork(ctx, recipientA, recipientB)

	transportA, err := newWithMixnet(ctx, privA, recipientA, inA, outA)
	if err != nil {
		t.Fatalf("create transportA: %v", err)
	}
	defer transportA.Close()

	transportB, err := newWithMixnet(ctx, privB, recipientB, inB, outB)
	if err != nil {
		t.Fatalf("create transportB: %v", err)
	}
	defer transportB.Close()

	listenerB, err := transportB.Listen(transportB.listenAddr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listenerB.Close()

	acceptCh := make(chan lptransport.CapableConn, 1)
	go func() {
		conn, err := listenerB.Accept()
		if err != nil {
			return
		}
		acceptCh <- conn
	}()

	dialAddr := transportB.listenAddr
	t.Logf("listen addr bytes: %v", dialAddr.Bytes())
	t.Logf("hasNymProtocol: %v", hasNymProtocol(dialAddr))
	proto := ma.ProtocolWithName(nymProtocolName)
	t.Logf("registered protocol code: %d", proto.Code)
	connAB, err := transportA.Dial(ctx, dialAddr, transportB.localPeer)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer connAB.Close()

	var connBA lptransport.CapableConn
	select {
	case raw := <-acceptCh:
		if raw == nil {
			t.Fatalf("listener closed")
		}
		connBA = raw
	case <-ctx.Done():
		t.Fatalf("accept timeout")
	}
	defer connBA.Close()

	streamAB, err := connAB.OpenStream(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	streamBA, err := connBA.AcceptStream()
	if err != nil {
		t.Fatalf("accept stream: %v", err)
	}

	payload := []byte("hello over nym")
	if _, err := streamAB.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(streamBA, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != string(payload) {
		t.Fatalf("unexpected payload %q", buf)
	}

	payload2 := []byte("response data")
	if _, err := streamBA.Write(payload2); err != nil {
		t.Fatalf("write back: %v", err)
	}

	buf2 := make([]byte, len(payload2))
	if _, err := io.ReadFull(streamAB, buf2); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(buf2) != string(payload2) {
		t.Fatalf("unexpected payload2 %q", buf2)
	}

	if err := streamAB.Close(); err != nil {
		t.Fatalf("close streamAB: %v", err)
	}
	if err := streamBA.Close(); err != nil {
		t.Fatalf("close streamBA: %v", err)
	}
}

func testRecipient(seed byte) message.Recipient {
	var r message.Recipient
	for i := 0; i < len(r.ClientIdentity); i++ {
		r.ClientIdentity[i] = seed
		r.ClientEncryptionKey[i] = seed + 1
		r.Gateway[i] = seed + 2
	}
	return r
}
