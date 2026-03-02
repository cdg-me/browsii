#!/usr/bin/env bash
# Run end-to-end tests against a real headless browser.
# Builds the browsii binary first so the tests always use the latest code.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# Build first — the e2e suite launches the binary as a subprocess.
echo "Building browsii for e2e tests..."
go build -o browsii ./cmd/browsii

echo "Running e2e tests (this launches a headless browser)..."
go test -v -count=1 -timeout 120s ./tests/... "$@"
