package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	nymtransport "nymtrans/go-libp2p-nym/transport"
)

func main() {
	// Parse command line flags
	nymURI := flag.String("nym-uri", "", "Nym websocket URI (e.g., ws://localhost:1977)")
	flag.Parse()

	if *nymURI == "" {
		fmt.Println("Error: -nym-uri is required")
		fmt.Println("Usage: chat -nym-uri <ws://host:port>")
		fmt.Println()
		fmt.Println("This example demonstrates the Nym transport layer.")
		fmt.Println("It requires a running Nym network.")
		fmt.Println("See the Rust implementation for Docker-based setup.")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Generate a new keypair for this host
	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		panic(err)
	}

	// Create the Nym transport
	transport, err := nymtransport.New(ctx, *nymURI, priv)
	if err != nil {
		panic(err)
	}
	defer transport.Close()

	// Get local peer ID
	pub := priv.GetPublic()
	localPeerID, err := peer.IDFromPublicKey(pub)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Local peer ID: %s\n", localPeerID)
	fmt.Println()
	fmt.Println("Nym transport initialized successfully!")
	fmt.Println("This is a basic example showing transport initialization.")
	fmt.Println()
	fmt.Println("For a full chat application with gossipsub, you would need to:")
	fmt.Println("1. Create a libp2p host with this transport")
	fmt.Println("2. Set up gossipsub for pub/sub messaging")
	fmt.Println("3. Handle incoming connections and messages")
	fmt.Println()
	fmt.Println("Press Ctrl+C to exit.")

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
}
