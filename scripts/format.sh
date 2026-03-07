#!/usr/bin/env bash
# Format Go source files.
# Default: check mode — reports unformatted files and exits non-zero.
# With --fix: rewrites files in place.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [ "${1:-}" = "--fix" ]; then
  echo "Formatting files (gofmt -w)..."
  gofmt -w .
  echo "  Done"
else
  echo "Checking formatting (gofmt)..."
  UNFORMATTED=$(gofmt -l .)
  if [ -n "$UNFORMATTED" ]; then
    echo "The following files are not formatted (run: ./scripts/format.sh --fix):"
    echo "$UNFORMATTED"
    exit 1
  fi
  echo "  OK"
fi
