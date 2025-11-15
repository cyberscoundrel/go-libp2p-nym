package testutil

import (
	"context"
	"sync"

	"nymtrans/go-libp2p-nym/message"
	"nymtrans/go-libp2p-nym/mixnet"
)

// PipeNetwork creates two mixnet endpoints connected via in-memory channels.
// Messages are routed based on recipient strings.
func PipeNetwork(ctx context.Context, aRecipient, bRecipient message.Recipient) (inboundA <-chan mixnet.InboundMessage, outboundA chan<- mixnet.OutboundMessage, inboundB <-chan mixnet.InboundMessage, outboundB chan<- mixnet.OutboundMessage) {
	aIn := make(chan mixnet.InboundMessage, 64)
	bIn := make(chan mixnet.InboundMessage, 64)
	aOut := make(chan mixnet.OutboundMessage, 64)
	bOut := make(chan mixnet.OutboundMessage, 64)

	recipients := map[string]chan<- mixnet.InboundMessage{
		aRecipient.String(): aIn,
		bRecipient.String(): bIn,
	}

	var once sync.Once
	closeAll := func() {
		once.Do(func() {
			close(aIn)
			close(bIn)
			close(aOut)
			close(bOut)
		})
	}

	go func() {
		defer closeAll()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-aOut:
				if !ok {
					return
				}
				target := recipients[msg.Recipient.String()]
				if target == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case target <- mixnet.InboundMessage{Message: msg.Message}:
				}
			case msg, ok := <-bOut:
				if !ok {
					return
				}
				target := recipients[msg.Recipient.String()]
				if target == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case target <- mixnet.InboundMessage{Message: msg.Message}:
				}
			}
		}
	}()

	return aIn, aOut, bIn, bOut
}

