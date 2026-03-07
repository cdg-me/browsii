#!/usr/bin/env bash
# Run linters. Reports issues but never modifies files.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if ! command -v golangci-lint &>/dev/null; then
  echo "golangci-lint not found. Install it:"
  echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
  echo "  # or: brew install golangci-lint"
  exit 1
fi

echo "Running linters (golangci-lint)..."
golangci-lint run ./...
echo "  OK"
