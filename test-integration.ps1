# PowerShell script to run integration tests on Windows

Write-Host "go-libp2p-nym Integration Tests" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""

# Check if Docker is running
Write-Host "Checking Docker..." -ForegroundColor Yellow
try {
    $dockerVersion = docker --version
    Write-Host "✓ Docker found: $dockerVersion" -ForegroundColor Green
} catch {
    Write-Host "✗ Docker not found or not running!" -ForegroundColor Red
    Write-Host "Please install Docker Desktop and ensure it's running." -ForegroundColor Red
    exit 1
}

# Check if Docker daemon is accessible
try {
    docker ps | Out-Null
    Write-Host "✓ Docker daemon is accessible" -ForegroundColor Green
} catch {
    Write-Host "✗ Cannot connect to Docker daemon!" -ForegroundColor Red
    Write-Host "Please start Docker Desktop." -ForegroundColor Red
    exit 1
}

Write-Host ""

# Check if Nym image exists
Write-Host "Checking for Nym Docker image..." -ForegroundColor Yellow
$imageExists = docker images -q chainsafe/nym:1.1.12
if ($imageExists) {
    Write-Host "✓ Nym Docker image already exists" -ForegroundColor Green
} else {
    Write-Host "! Nym Docker image not found, will be built during tests" -ForegroundColor Yellow
    Write-Host "  This may take a few minutes on first run..." -ForegroundColor Yellow
}

Write-Host ""

# Run the integration tests
Write-Host "Running integration tests..." -ForegroundColor Cyan
Write-Host "This will take several minutes (Nym clients need to initialize)" -ForegroundColor Yellow
Write-Host ""

go test -v -tags=integration -timeout=10m ./...

$exitCode = $LASTEXITCODE

Write-Host ""
if ($exitCode -eq 0) {
    Write-Host "✓ All integration tests passed!" -ForegroundColor Green
} else {
    Write-Host "✗ Integration tests failed!" -ForegroundColor Red
    Write-Host ""
    Write-Host "Troubleshooting tips:" -ForegroundColor Yellow
    Write-Host "1. Check Docker logs: docker ps -a" -ForegroundColor Gray
    Write-Host "2. Clean up containers: docker ps -a | Select-String 'chainsafe/nym' | ForEach-Object { docker rm -f `$_.ToString().Split()[0] }" -ForegroundColor Gray
    Write-Host "3. Check your internet connection (Nym needs to connect to mixnet)" -ForegroundColor Gray
    Write-Host "4. See INTEGRATION_TESTS.md for more details" -ForegroundColor Gray
}

Write-Host ""
Write-Host "Cleaning up any remaining containers..." -ForegroundColor Yellow
docker ps -a | Select-String "chainsafe/nym" | ForEach-Object {
    $containerId = $_.ToString().Split()[0]
    Write-Host "Stopping container $containerId..." -ForegroundColor Gray
    docker stop $containerId 2>&1 | Out-Null
    docker rm $containerId 2>&1 | Out-Null
}
Write-Host "✓ Cleanup complete" -ForegroundColor Green

exit $exitCode

