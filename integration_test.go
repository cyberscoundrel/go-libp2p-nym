//go:build integration
// +build integration

package libp2pnym_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	ma "github.com/multiformats/go-multiaddr"

	nymtransport "banyan/transports/nym/transport"
)

const (
	testProtocolID = protocol.ID("/test/echo/1.0.0")
	testMessage    = "Hello from libp2p over Nym mixnet!"
)

// TestNymTransportIntegration tests the full transport with real Nym mixnet communication
func TestNymTransportIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start two Nym client containers
	t.Log("Starting Nym client containers...")
	docker := NewDockerManager(t)
	defer docker.Cleanup()

	_, uri1 := docker.StartNymClient("peer1")
	nym2, uri2 := docker.StartNymClient("peer2")

	t.Logf("Nym client 1 URI: %s", uri1)
	t.Logf("Nym client 2 URI: %s", uri2)

	// Wait for Nym clients to be ready
	t.Log("Waiting for Nym clients to initialize...")
	time.Sleep(10 * time.Second)

	// Get recipient address for peer2 (the listener)
	recipient2, err := docker.GetNymRecipient(nym2)
	if err != nil {
		t.Fatalf("Failed to get recipient for peer2: %v", err)
	}

	t.Logf("Peer 2 recipient: %s", recipient2)

	// Create Nym transports directly
	t.Log("Creating Nym transports...")

	// Generate identities
	privKey1, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("Failed to generate key 1: %v", err)
	}

	privKey2, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("Failed to generate key 2: %v", err)
	}

	// Create transports
	transport1, err := nymtransport.New(ctx, uri1, privKey1)
	if err != nil {
		t.Fatalf("Failed to create transport 1: %v", err)
	}
	defer transport1.Close()

	transport2, err := nymtransport.New(ctx, uri2, privKey2)
	if err != nil {
		t.Fatalf("Failed to create transport 2: %v", err)
	}
	defer transport2.Close()

	// Get peer IDs from the public keys
	peerID2, err := peer.IDFromPublicKey(privKey2.GetPublic())
	if err != nil {
		t.Fatalf("Failed to get peer ID 2: %v", err)
	}

	t.Log("✓ Transports created")
	t.Logf("Peer 2 ID: %s", peerID2)

	// Build peer2's multiaddr
	peer2Addr, err := ma.NewMultiaddr(fmt.Sprintf("/nym/%s", recipient2))
	if err != nil {
		t.Fatalf("Failed to create peer2 multiaddr: %v", err)
	}

	// Start listening on transport2
	t.Log("Starting listener on transport 2...")
	listener, err := transport2.Listen(peer2Addr)
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer listener.Close()

	// Handle incoming connections in a goroutine
	acceptDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptDone <- fmt.Errorf("accept error: %w", err)
			return
		}
		defer conn.Close()

		t.Log("✓ Transport 2 accepted connection")

		// Accept a stream on the connection
		stream, err := conn.AcceptStream()
		if err != nil {
			acceptDone <- fmt.Errorf("accept stream error: %w", err)
			return
		}
		defer stream.Close()

		t.Log("✓ Transport 2 accepted stream")

		// Echo back whatever we receive
		buf := make([]byte, 1024)
		n, err := stream.Read(buf)
		if err != nil {
			acceptDone <- fmt.Errorf("read error: %w", err)
			return
		}

		t.Logf("Transport 2 received: %q", string(buf[:n]))

		_, err = stream.Write(buf[:n])
		if err != nil {
			acceptDone <- fmt.Errorf("write error: %w", err)
			return
		}

		t.Log("✓ Transport 2 echoed message")
		acceptDone <- nil
	}()

	// Give the listener time to start
	time.Sleep(2 * time.Second)

	// Dial from transport1 to transport2
	t.Logf("Dialing from transport 1 to %s...", peer2Addr)
	conn, err := transport1.Dial(ctx, peer2Addr, peerID2)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	t.Log("✓ Connection established over Nym mixnet!")

	// Open a stream on the connection
	stream, err := conn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("Failed to open stream: %v", err)
	}
	defer stream.Close()

	t.Log("✓ Stream opened")

	// Send a test message
	t.Logf("Sending test message: %q", testMessage)
	_, err = stream.Write([]byte(testMessage))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Read the echo response
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	response := string(buf[:n])
	t.Logf("Received response: %q", response)

	if response != testMessage {
		t.Fatalf("Unexpected response: got %q, want %q", response, testMessage)
	}

	// Wait for the accept goroutine to finish
	select {
	case err := <-acceptDone:
		if err != nil {
			t.Fatalf("Accept goroutine error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for accept goroutine")
	}

	// Verify the echo
	if response != testMessage {
		t.Errorf("Echo mismatch: sent %q, received %q", testMessage, response)
	}

	t.Log("✓ Integration test passed! Successfully communicated over Nym mixnet")
}

// TestNymTransportMultipleStreams tests multiple concurrent streams over Nym
func TestNymTransportMultipleStreams(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start two Nym client containers
	t.Log("Starting Nym client containers...")
	docker := NewDockerManager(t)
	defer docker.Cleanup()

	_, uri1 := docker.StartNymClient("multi1")
	nym2, uri2 := docker.StartNymClient("multi2")

	t.Log("Waiting for Nym clients to initialize...")
	time.Sleep(10 * time.Second)

	recipient2, err := docker.GetNymRecipient(nym2)
	if err != nil {
		t.Fatalf("Failed to get recipient: %v", err)
	}

	t.Logf("Peer 2 recipient: %s", recipient2)

	// Create Nym transports
	t.Log("Creating Nym transports...")

	privKey1, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("Failed to generate key 1: %v", err)
	}

	privKey2, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("Failed to generate key 2: %v", err)
	}

	transport1, err := nymtransport.New(ctx, uri1, privKey1)
	if err != nil {
		t.Fatalf("Failed to create transport 1: %v", err)
	}
	defer transport1.Close()

	transport2, err := nymtransport.New(ctx, uri2, privKey2)
	if err != nil {
		t.Fatalf("Failed to create transport 2: %v", err)
	}
	defer transport2.Close()

	peerID2, err := peer.IDFromPublicKey(privKey2.GetPublic())
	if err != nil {
		t.Fatalf("Failed to get peer ID 2: %v", err)
	}

	t.Log("✓ Transports created")

	// Build peer2's multiaddr
	peer2Addr, err := ma.NewMultiaddr(fmt.Sprintf("/nym/%s", recipient2))
	if err != nil {
		t.Fatalf("Failed to create peer2 multiaddr: %v", err)
	}

	// Start listening on transport2
	t.Log("Starting listener on transport 2...")
	listener, err := transport2.Listen(peer2Addr)
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer listener.Close()

	// Handle incoming connection and multiple streams
	acceptDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptDone <- fmt.Errorf("accept error: %w", err)
			return
		}
		defer conn.Close()

		t.Log("✓ Transport 2 accepted connection")

		// Handle multiple streams
		streamCount := 0
		for {
			stream, err := conn.AcceptStream()
			if err != nil {
				// Connection closed or error
				break
			}

			streamCount++
			streamNum := streamCount
			go func(s network.MuxedStream, num int) {
				defer s.Close()
				t.Logf("Transport 2: Handling stream #%d", num)

				buf := make([]byte, 1024)
				n, err := s.Read(buf)
				if err != nil {
					t.Logf("Stream %d read error: %v", num, err)
					return
				}

				_, err = s.Write(buf[:n])
				if err != nil {
					t.Logf("Stream %d write error: %v", num, err)
					return
				}

				t.Logf("✓ Stream %d echoed message", num)
			}(stream, streamNum)
		}

		acceptDone <- nil
	}()

	// Give the listener time to start
	time.Sleep(2 * time.Second)

	// Dial from transport1 to transport2
	t.Logf("Dialing from transport 1 to %s...", peer2Addr)
	conn, err := transport1.Dial(ctx, peer2Addr, peerID2)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	t.Log("✓ Connection established over Nym mixnet!")

	// Open multiple streams concurrently
	numStreams := 5
	results := make(chan error, numStreams)

	for i := 0; i < numStreams; i++ {
		go func(streamNum int) {
			msg := fmt.Sprintf("Message from stream %d", streamNum)

			stream, err := conn.OpenStream(ctx)
			if err != nil {
				results <- fmt.Errorf("stream %d: failed to open: %w", streamNum, err)
				return
			}
			defer stream.Close()

			_, err = stream.Write([]byte(msg))
			if err != nil {
				results <- fmt.Errorf("stream %d: failed to write: %w", streamNum, err)
				return
			}

			buf := make([]byte, 1024)
			n, err := stream.Read(buf)
			if err != nil && err != io.EOF {
				results <- fmt.Errorf("stream %d: failed to read: %w", streamNum, err)
				return
			}

			if string(buf[:n]) != msg {
				results <- fmt.Errorf("stream %d: echo mismatch: got %q, want %q", streamNum, string(buf[:n]), msg)
				return
			}

			t.Logf("✓ Stream %d: Success", streamNum)
			results <- nil
		}(i)
	}

	// Wait for all streams to complete
	for i := 0; i < numStreams; i++ {
		if err := <-results; err != nil {
			t.Errorf("Stream error: %v", err)
		}
	}

	t.Log("✓ All streams completed successfully!")

	// Close connection to trigger listener exit
	conn.Close()

	// Wait for accept goroutine
	select {
	case <-acceptDone:
	case <-time.After(5 * time.Second):
		t.Log("Timeout waiting for accept goroutine (expected)")
	}

	t.Logf("✓ Successfully handled %d concurrent streams over Nym mixnet", numStreams)
}

// createNymHost creates a libp2p host with Nym transport
func createNymHost(ctx context.Context, nymURI string) (host.Host, error) {
	// Generate a new identity for this host
	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		return nil, fmt.Errorf("failed to generate identity: %w", err)
	}

	// Create the Nym transport directly
	transport, err := nymtransport.New(ctx, nymURI, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nym transport: %w", err)
	}

	// Create a basic host with no transports
	// We'll add the Nym transport manually
	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.NoTransports,
		libp2p.NoListenAddrs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Add the Nym transport to the swarm
	swarmInst := h.Network().(*swarm.Swarm)
	err = swarmInst.AddTransport(transport)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("failed to add Nym transport: %w", err)
	}

	// Now start listening on the Nym address
	// The transport knows its own listen address
	listenAddrs := transport.Protocols()
	for _, proto := range listenAddrs {
		if proto == 999 { // Nym protocol code
			// Build the multiaddr for this transport
			// We need to get the actual listen address from the transport
			// For now, we'll skip explicit listening since the transport
			// is already set up to receive connections
			break
		}
	}

	return h, nil
}

// DockerManager manages Docker containers for testing
type DockerManager struct {
	t          *testing.T
	containers []string
}

// NewDockerManager creates a new Docker manager
func NewDockerManager(t *testing.T) *DockerManager {
	return &DockerManager{
		t:          t,
		containers: make([]string, 0),
	}
}

// StartNymClient starts a Nym client container and returns the container ID and WebSocket URI
func (dm *DockerManager) StartNymClient(nymID string) (containerID string, wsURI string) {
	dm.t.Logf("Starting Nym client container with ID: %s", nymID)

	// Build the Nym Docker image if it doesn't exist
	dm.ensureNymImage()

	// Run the container
	cmd := exec.Command("docker", "run", "-d",
		"-e", fmt.Sprintf("NYM_ID=%s", nymID),
		"-p", "0:1977", // Map random host port to container port 1977
		"chainsafe/nym:latest",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		dm.t.Fatalf("Failed to start Nym container: %v\nOutput: %s", err, string(output))
	}

	containerID = strings.TrimSpace(string(output))
	dm.containers = append(dm.containers, containerID)
	dm.t.Logf("Started container: %s", containerID)

	// Wait for container to be ready
	dm.waitForContainer(containerID, "Client startup finished!")

	// Get the mapped port
	port := dm.getContainerPort(containerID, "1977")
	wsURI = fmt.Sprintf("ws://127.0.0.1:%s", port)

	dm.t.Logf("Nym client %s ready at %s", nymID, wsURI)
	return containerID, wsURI
}

// ensureNymImage ensures the Nym Docker image exists
func (dm *DockerManager) ensureNymImage() {
	// Check if image exists
	cmd := exec.Command("docker", "images", "-q", "chainsafe/nym:latest")
	output, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		dm.t.Log("Nym Docker image already exists")
		return
	}

	dm.t.Log("Building Nym Docker image...")

	// Build from the rust-libp2p-nym directory
	cmd = exec.Command("docker", "build",
		"-t", "chainsafe/nym:latest",
		"-f", "../rust-libp2p-nym/Dockerfile.nym",
		"../rust-libp2p-nym",
	)

	output, err = cmd.CombinedOutput()
	if err != nil {
		dm.t.Fatalf("Failed to build Nym image: %v\nOutput: %s", err, string(output))
	}

	dm.t.Log("Nym Docker image built successfully")
}

// waitForContainer waits for a specific log message in the container
func (dm *DockerManager) waitForContainer(containerID, message string) {
	dm.t.Logf("Waiting for container %s to be ready...", containerID[:12])

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "logs", "-f", containerID)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		dm.t.Fatalf("Failed to get stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		dm.t.Fatalf("Failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		dm.t.Fatalf("Failed to start docker logs: %v", err)
	}

	// Read from both stdout and stderr in separate goroutines
	done := make(chan bool, 1)

	readPipe := func(reader io.Reader, name string) {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			dm.t.Logf("[%s] %s", name, line)
			if strings.Contains(line, message) {
				dm.t.Logf("Container %s is ready!", containerID[:12])
				select {
				case done <- true:
				default:
				}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			dm.t.Logf("Scanner error on %s: %v", name, err)
		}
	}

	go readPipe(stdout, "stdout")
	go readPipe(stderr, "stderr")

	select {
	case <-done:
		cmd.Process.Kill()
	case <-ctx.Done():
		cmd.Process.Kill()
		dm.t.Fatalf("Timeout waiting for container %s to be ready", containerID[:12])
	}
}

// getContainerPort gets the host port mapped to a container port
func (dm *DockerManager) getContainerPort(containerID, containerPort string) string {
	cmd := exec.Command("docker", "port", containerID, containerPort)
	output, err := cmd.Output()
	if err != nil {
		dm.t.Fatalf("Failed to get container port: %v", err)
	}

	// Output format: "0.0.0.0:12345" or "127.0.0.1:12345"
	portStr := strings.TrimSpace(string(output))
	parts := strings.Split(portStr, ":")
	if len(parts) < 2 {
		dm.t.Fatalf("Invalid port output: %s", portStr)
	}

	return parts[len(parts)-1]
}

// GetNymRecipient extracts the Nym recipient address from container logs
func (dm *DockerManager) GetNymRecipient(containerID string) (string, error) {
	dm.t.Logf("Extracting Nym recipient from container %s...", containerID[:12])

	// Get container logs
	cmd := exec.Command("docker", "logs", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %w", err)
	}

	// Look for the recipient address in the logs
	// Format: "The address of this client is: <recipient>"
	re := regexp.MustCompile(`The address of this client is:\s*([A-Za-z0-9]+\.[A-Za-z0-9]+@[A-Za-z0-9]+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("could not find recipient address in logs")
	}

	recipient := matches[1]
	dm.t.Logf("Found recipient: %s", recipient)
	return recipient, nil
}

// Cleanup stops and removes all containers
func (dm *DockerManager) Cleanup() {
	dm.t.Log("Cleaning up Docker containers...")
	for _, containerID := range dm.containers {
		dm.t.Logf("Stopping container %s...", containerID[:12])

		// Stop the container
		cmd := exec.Command("docker", "stop", containerID)
		if err := cmd.Run(); err != nil {
			dm.t.Logf("Warning: failed to stop container %s: %v", containerID[:12], err)
		}

		// Remove the container
		cmd = exec.Command("docker", "rm", containerID)
		if err := cmd.Run(); err != nil {
			dm.t.Logf("Warning: failed to remove container %s: %v", containerID[:12], err)
		}

		dm.t.Logf("Container %s cleaned up", containerID[:12])
	}
	dm.t.Log("All containers cleaned up")
}
