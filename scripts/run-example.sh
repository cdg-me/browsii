#!/usr/bin/env bash
# Run the Go client example (examples/go/01_basics.go).
# The example starts its own in-process daemon — no separate browsii binary needed.
#
# Usage:
#   ./scripts/run-example.sh                   # runs 01_basics.go
#   ./scripts/run-example.sh examples/go/01_basics.go
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

EXAMPLE="${1:-examples/go/01_basics.go}"

echo "Running $EXAMPLE ..."
go run "$EXAMPLE"
