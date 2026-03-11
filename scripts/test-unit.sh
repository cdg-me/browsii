#!/usr/bin/env bash
# Run unit tests (no browser required).
# Covers internal/daemon package only — pure Go, no subprocess.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

echo "Running unit tests..."
go test -v -count=1 ./internal/daemon/... ./client/... ./cmd/browsii/... "$@"
