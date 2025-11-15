.PHONY: test test-unit test-integration build clean docker-build help

# Default target
help:
	@echo "Available targets:"
	@echo "  make test              - Run all tests (unit + integration)"
	@echo "  make test-unit         - Run unit tests only"
	@echo "  make test-integration  - Run integration tests with Docker"
	@echo "  make build             - Build all packages"
	@echo "  make docker-build      - Build the Nym Docker image"
	@echo "  make clean             - Clean build artifacts and stop containers"

# Run all tests
test: test-unit test-integration

# Run unit tests only
test-unit:
	@echo "Running unit tests..."
	go test -v ./...

# Run integration tests (requires Docker)
test-integration:
	@echo "Running integration tests..."
	@echo "Note: This will build and launch Docker containers"
	go test -v -tags=integration -timeout=10m ./...

# Build all packages
build:
	@echo "Building all packages..."
	go build ./...

# Build the Nym Docker image
docker-build:
	@echo "Building Nym Docker image..."
	docker build -t chainsafe/nym:1.1.12 -f ../rust-libp2p-nym/Dockerfile.nym ../rust-libp2p-nym

# Clean up
clean:
	@echo "Cleaning up..."
	@echo "Stopping any running Nym containers..."
	-docker ps -a | grep chainsafe/nym | awk '{print $$1}' | xargs -r docker stop
	-docker ps -a | grep chainsafe/nym | awk '{print $$1}' | xargs -r docker rm
	@echo "Removing build artifacts..."
	go clean
	@echo "Done!"

