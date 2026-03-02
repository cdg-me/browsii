#!/usr/bin/env bash
# Build the browsii binary into the repo root.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

echo "Building browsii..."
go build -o browsii ./cmd/browsii
echo "Done → ./browsii"
