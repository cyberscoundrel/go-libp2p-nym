# Integration Tests

This document describes how to run the integration tests for go-libp2p-nym, which test real communication over the Nym mixnet using Docker containers.

## Prerequisites

1. **Docker**: You must have Docker installed and running
   - Install from: https://docs.docker.com/get-docker/
   - Verify with: `docker --version`

2. **Go**: Go 1.24 or later
   - Verify with: `go version`

3. **Sufficient disk space**: The Nym Docker image is ~500MB

4. **Internet connectivity**: The Nym client needs to connect to the Nym mixnet
   - Requires access to Nym validator nodes
   - May require firewall configuration

## Current Status

⚠️ **Note**: The integration tests use Nym client version 1.1.12, which may have compatibility issues with the current Nym network. If tests fail with "Failed to setup gateway" or "Validator client error", this indicates the Nym client cannot connect to the mixnet.

**Possible solutions:**
- Update to a newer Nym client version in the Dockerfile
- Check Nym network status at https://status.nymtech.net/
- Verify network connectivity and firewall settings
- Use a local Nym testnet for testing

## Running Integration Tests

### Quick Start

Run all tests (unit + integration):
```bash
make test
```

Run only integration tests:
```bash
make test-integration
```

Or directly with go:
```bash
go test -v -tags=integration -timeout=10m ./...
```

### What the Tests Do

The integration tests:

1. **Build the Nym Docker image** (if not already built)
   - Uses the Dockerfile from `../rust-libp2p-nym/Dockerfile.nym`
   - Downloads Nym client binaries (v1.1.12)
   - Tagged as `chainsafe/nym:1.1.12`

2. **Launch two Nym client containers**
   - Each container runs a separate Nym client
   - Containers connect to the Nym mixnet
   - WebSocket endpoints are exposed on random host ports

3. **Create two libp2p hosts**
   - Each host uses the Nym transport
   - Hosts connect to their respective Nym clients

4. **Test communication**
   - Host 1 connects to Host 2 via Nym multiaddr
   - Opens a stream and sends a test message
   - Verifies the echo response
   - Tests multiple concurrent streams

5. **Cleanup**
   - Stops and removes all Docker containers
   - Cleans up resources

## Test Details

### TestNymTransportIntegration

Tests basic end-to-end communication:
- Launches 2 Nym clients in Docker
- Creates 2 libp2p hosts with Nym transport
- Establishes connection over Nym mixnet
- Sends and receives a test message
- Verifies echo response

**Expected duration**: 2-3 minutes (including Nym client initialization)

### TestNymTransportMultipleStreams

Tests concurrent stream multiplexing:
- Launches 2 Nym clients in Docker
- Creates 2 libp2p hosts with Nym transport
- Opens 5 concurrent streams
- Sends different messages on each stream
- Verifies all messages are echoed correctly

**Expected duration**: 2-3 minutes

## Troubleshooting

### Docker not running
```
Error: Cannot connect to the Docker daemon
```
**Solution**: Start Docker Desktop or the Docker daemon

### Port conflicts
```
Error: Bind for 0.0.0.0:1977 failed: port is already allocated
```
**Solution**: The test uses random ports, but if you see this, stop any running Nym containers:
```bash
make clean
```

### Timeout waiting for container
```
Timeout waiting for container to be ready
```
**Solution**: 
- Check your internet connection (Nym needs to connect to the mixnet)
- Increase timeout in the test (currently 2 minutes)
- Check Docker logs: `docker logs <container-id>`

### Image build fails
```
Failed to build Nym image
```
**Solution**: 
- Ensure you're in the `go-libp2p-nym` directory
- Ensure `../rust-libp2p-nym/Dockerfile.nym` exists
- Try building manually: `make docker-build`

### Tests hang
If tests hang indefinitely:
1. Check Docker container status: `docker ps`
2. Check container logs: `docker logs <container-id>`
3. Stop all containers: `make clean`
4. Re-run tests

## Manual Testing

You can also manually test with Docker:

### 1. Build the image
```bash
make docker-build
```

### 2. Start a Nym client
```bash
docker run -d --name nym1 -e NYM_ID=test1 -p 1977:1977 chainsafe/nym:1.1.12
```

### 3. Check logs
```bash
docker logs -f nym1
```

Wait for: `Client startup finished!`

### 4. Get the recipient address
```bash
docker logs nym1 | grep "The address of this client is"
```

### 5. Test with the chat example
```bash
go run ./cmd/chat -nym-uri ws://127.0.0.1:1977
```

### 6. Cleanup
```bash
docker stop nym1
docker rm nym1
```

## CI/CD Integration

To run integration tests in CI/CD:

```yaml
# Example GitHub Actions workflow
- name: Run integration tests
  run: |
    make test-integration
  timeout-minutes: 10
```

**Note**: Ensure the CI environment has:
- Docker installed and running
- Sufficient resources (2GB+ RAM recommended)
- Network access to Nym mixnet

## Performance Notes

- **First run**: Takes longer due to Docker image build (~2-3 minutes)
- **Subsequent runs**: Faster as image is cached (~1-2 minutes)
- **Nym initialization**: Each client takes ~30-60 seconds to connect to mixnet
- **Message latency**: Expect 5-10 second RTT due to mixnet routing

## Skipping Integration Tests

To run only unit tests (skip integration tests):
```bash
make test-unit
# or
go test -v -short ./...
```

Integration tests are automatically skipped when using `-short` flag.

