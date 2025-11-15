#!/bin/bash
# Bash script to run integration tests on Linux/Mac

set -e

echo -e "\033[36mgo-libp2p-nym Integration Tests\033[0m"
echo -e "\033[36m================================\033[0m"
echo ""

# Check if Docker is running
echo -e "\033[33mChecking Docker...\033[0m"
if ! command -v docker &> /dev/null; then
    echo -e "\033[31m✗ Docker not found!\033[0m"
    echo -e "\033[31mPlease install Docker: https://docs.docker.com/get-docker/\033[0m"
    exit 1
fi

DOCKER_VERSION=$(docker --version)
echo -e "\033[32m✓ Docker found: $DOCKER_VERSION\033[0m"

# Check if Docker daemon is accessible
if ! docker ps &> /dev/null; then
    echo -e "\033[31m✗ Cannot connect to Docker daemon!\033[0m"
    echo -e "\033[31mPlease start Docker daemon.\033[0m"
    exit 1
fi
echo -e "\033[32m✓ Docker daemon is accessible\033[0m"

echo ""

# Check if Nym image exists
echo -e "\033[33mChecking for Nym Docker image...\033[0m"
if docker images -q chainsafe/nym:1.1.12 | grep -q .; then
    echo -e "\033[32m✓ Nym Docker image already exists\033[0m"
else
    echo -e "\033[33m! Nym Docker image not found, will be built during tests\033[0m"
    echo -e "\033[33m  This may take a few minutes on first run...\033[0m"
fi

echo ""

# Run the integration tests
echo -e "\033[36mRunning integration tests...\033[0m"
echo -e "\033[33mThis will take several minutes (Nym clients need to initialize)\033[0m"
echo ""

if go test -v -tags=integration -timeout=10m ./...; then
    echo ""
    echo -e "\033[32m✓ All integration tests passed!\033[0m"
    EXIT_CODE=0
else
    echo ""
    echo -e "\033[31m✗ Integration tests failed!\033[0m"
    echo ""
    echo -e "\033[33mTroubleshooting tips:\033[0m"
    echo -e "\033[90m1. Check Docker logs: docker ps -a\033[0m"
    echo -e "\033[90m2. Clean up containers: docker ps -a | grep chainsafe/nym | awk '{print \$1}' | xargs docker rm -f\033[0m"
    echo -e "\033[90m3. Check your internet connection (Nym needs to connect to mixnet)\033[0m"
    echo -e "\033[90m4. See INTEGRATION_TESTS.md for more details\033[0m"
    EXIT_CODE=1
fi

echo ""
echo -e "\033[33mCleaning up any remaining containers...\033[0m"
docker ps -a | grep "chainsafe/nym" | awk '{print $1}' | xargs -r docker stop 2>/dev/null || true
docker ps -a | grep "chainsafe/nym" | awk '{print $1}' | xargs -r docker rm 2>/dev/null || true
echo -e "\033[32m✓ Cleanup complete\033[0m"

exit $EXIT_CODE

