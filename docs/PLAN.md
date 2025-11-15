# Go Nym Transport Port Plan

## Goals
- Port core functionality of ust-libp2p-nym into Go using go-libp2p.
- Expose a transport that libp2p hosts can dial/listen on using /nym/<recipient> multiaddrs.
- Provide chat example for manual testing.

## Architecture Overview
### Packages
- 	ransport: Implements 	ransport.Transport, entry point for go-libp2p integration.
- mixnet: Manages websocket connection with Nym gateways, handles binary framing, inbound/outbound channels, self-address discovery.
- message: Defines wire formats (connection/substream messages), serialization helpers.
- queue: Message reordering queue keyed by nonce to compensate for mixnet reordering.
- connection: Implements 	ransport.CapableConn, 
etwork.MuxedConn, stream management, handshake flows.
- substream: Wraps per-substream state implementing 
etwork.MuxedStream semantics.
- internal/testutil: Re-usable helpers for integration tests (mock mixnet server, deterministic IDs).

### Handshake Flow
1. Transport connects to mixnet via websocket and fetches self recipient.
2. Dialer generates ConnectionId, sends ConnectionRequest message with local peer.ID and recipient.
3. Listener sees request, constructs connection, stores mapping, replies with ConnectionResponse containing its peer.ID.
4. Dialer resolves pending dial, both sides mark MessageQueue ready (
ext_expected_nonce = 1).
5. Subsequent TransportMessage frames carry SubstreamMessages ordered by nonce.

### Stream Handling
- Outbound OpenStream increments connection nonce, enqueues request.
- Listener handles request, creates 
ymStream, responds with OpenResponse.
- Data frames piped via per-stream channels; close operations send Close message.

### Multiaddr Encoding
- Convert between /nym/<recipient> and Recipient base58 using helper functions mirroring Rust implementation.

### Concurrency Model
- mixnet.Client goroutine handles websocket read/write using gorilla/websocket.
- 	ransport.Transport event loop listens to inbound messages and dispatches to connections via channels. Uses context.Context for cancellation.
- Use sync.RWMutex protected maps for connection/pending dial state.

### Resource Management & Security
- 	ransport.CapableConn satisfies ConnSecurity by exposing peer IDs and (if present) decoded public keys.
- ConnScope returns 
etwork.NullScope{} initially; allow injection of resource manager later.
- Implement keepalive timers? (future work).

### Chat Example
- Directory cmd/chat with CLI supporting --dial flag to connect to peer multiaddr.
- Uses go-libp2p host with our transport for simple stdin/stdout chat.

### Testing Strategy
- Unit tests for message encode/decode, queue ordering.
- Integration tests with fake mixnet server verifying dial/listen handshake and stream data.
- Chat example manual instructions in README.

### Open Questions / TODOs
- Backpressure for websocket channels (bounded queue?).
- Error propagation semantics on websocket disconnect.
- Security handshake beyond peer ID (needs follow-up).
