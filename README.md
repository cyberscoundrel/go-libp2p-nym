# go-libp2p-nym

A Go implementation of libp2p transport over the Nym mixnet, providing privacy-preserving peer-to-peer networking.

## Overview

This package implements a libp2p transport that routes traffic through the Nym mixnet, providing network-level privacy and anonymity for libp2p applications. It is a port of the Rust implementation found in `rust-libp2p-nym`.

## Features

- **libp2p Transport Interface**: Implements the standard `transport.Transport` interface from go-libp2p
- **Nym Mixnet Integration**: Routes all traffic through the Nym mixnet for privacy
- **Custom Multiaddr Protocol**: Uses `/nym/<recipient>` addressing format
- **Connection Management**: Handles connection establishment, stream multiplexing, and message ordering
- **Message Reordering**: Compensates for mixnet-induced message reordering using sequence numbers

## Architecture

The implementation consists of several key components:

### Transport Layer (`transport/`)
- **transport.go**: Main transport implementation
- **conn.go**: Connection management and stream multiplexing
- **substream.go**: Individual stream implementation
- **listener.go**: Listener for incoming connections
- **addr.go**: Custom multiaddr protocol registration

### Message Layer (`message/`)
- **encoding.go**: Message serialization/deserialization
- **recipient.go**: Nym recipient address parsing
- **types.go**: Message type definitions

### Mixnet Layer (`mixnet/`)
- **client.go**: WebSocket client for Nym gateway communication

### Queue Layer (`queue/`)
- **queue.go**: Message reordering queue implementation

## Installation

```bash
go get nymtrans/go-libp2p-nym
```

## Testing

### Unit Tests

Run unit tests:
```bash
make test-unit
# or
go test ./...
```

### Integration Tests

Run full integration tests with real Nym mixnet communication:
```bash
make test-integration
# or
go test -v -tags=integration -timeout=10m ./...
```

**Requirements for integration tests:**
- Docker installed and running
- Internet connection (to connect to Nym mixnet)
- ~500MB disk space for Nym Docker image

The integration tests will:
1. Build the Nym Docker image (if needed)
2. Launch two Nym client containers
3. Create two libp2p hosts with Nym transport
4. Test real communication over the Nym mixnet
5. Verify message delivery and stream multiplexing
6. Clean up all containers

See [INTEGRATION_TESTS.md](INTEGRATION_TESTS.md) for detailed information.

## Usage

### Basic Transport Creation

```go
import (
    "context"
    "github.com/libp2p/go-libp2p/core/crypto"
    nymtransport "nymtrans/go-libp2p-nym/transport"
)

func main() {
    ctx := context.Background()
    
    // Generate a keypair
    priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
    if err != nil {
        panic(err)
    }
    
    // Create the transport
    // Requires a running Nym network with a gateway at the specified URI
    transport, err := nymtransport.New(ctx, "ws://localhost:1977", priv)
    if err != nil {
        panic(err)
    }
    defer transport.Close()
    
    // Use the transport with libp2p...
}
```

### Running the Chat Example

The `cmd/chat` directory contains a basic example demonstrating transport initialization:

```bash
# Build the example
go build ./cmd/chat

# Run with a Nym gateway URI
./chat -nym-uri ws://localhost:1977
```

**Note**: This example requires a running Nym network. See the Rust implementation for Docker-based setup instructions.

## Building

```bash
# Build all packages
go build ./...

# Run tests
go test ./...

# Build the chat example
go build ./cmd/chat
```

## Testing

The package includes integration tests that use a simulated mixnet:

```bash
go test -v ./transport
```

## Multiaddr Format

The transport uses a custom multiaddr protocol with code `999`:

```
/nym/<recipient>
```

Where `<recipient>` is a Nym recipient address in the format:
```
<identity>.<encryption>@<gateway>
```

All three components are base58-encoded public keys.

Example:
```
/nym/CytBseW6yFXUMzz4SGAKdNLGR7q3sJLLYxyBGvutNEQV.4QXYyEVc5fUDjmmi8PrHN9tdUFV4PCvSJE1278cHyvoe@4sBbL1ngf1vtNqykydQKTFh26sQCw888GpUqvPvyNB4f
```

## Protocol Details

### Connection Establishment

1. Dialer sends a `ConnectionRequest` message with:
   - Connection ID
   - Peer ID
   - Recipient address

2. Listener responds with a `ConnectionResponse` message

3. Both sides create a connection object and can begin exchanging data

### Message Types

- **ConnectionRequest**: Initiates a new connection
- **ConnectionResponse**: Acknowledges a connection request
- **TransportMessage**: Carries data for an established connection
  - **Data**: Application data
  - **Close**: Closes a substream
  - **OpenStream**: Opens a new substream

### Message Ordering

The mixnet may deliver messages out of order. The transport handles this by:
1. Assigning sequence numbers to all messages
2. Using a reordering queue to buffer out-of-order messages
3. Delivering messages to the application in the correct order

## Differences from Rust Implementation

- Uses Go's standard library and go-libp2p packages
- Simplified mixnet client (WebSocket-based)
- Test utilities use in-memory channels instead of Docker containers
- Some protocol details may differ slightly

## Requirements

- Go 1.24 or later
- A running Nym network (for production use)

## Development Status

This is a port of the Rust implementation and is currently in development. The core transport functionality is implemented and tested, but full libp2p host integration and production deployment require additional work.

## Contributing

Contributions are welcome! Please ensure that:
- All tests pass (`go test ./...`)
- Code follows Go conventions
- New features include tests

## License

This project follows the same license as the Rust implementation.

## Related Projects

- [rust-libp2p-nym](../rust-libp2p-nym): The original Rust implementation
- [Nym](https://nymtech.net/): The Nym mixnet project
- [go-libp2p](https://github.com/libp2p/go-libp2p): The Go implementation of libp2p

## References

- [libp2p Transport Specification](https://github.com/libp2p/specs/tree/master/transport)
- [Nym Documentation](https://nymtech.net/docs/)
- [Multiaddr Specification](https://github.com/multiformats/multiaddr)

