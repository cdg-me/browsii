#!/bin/bash
# ─────────────────────────────────────────────────────────────
# WASM Script Runner — builds CLI, starts daemon, runs script
#
# Usage:
#   ./examples/run_wasm.sh examples/wasm/01_basics.go
#   ./examples/run_wasm.sh examples/wasm/02_events.go
#   ./examples/run_wasm.sh examples/wasm/03_network_capture.go
#   ./examples/run_wasm.sh /path/to/any_script.go
#   ./examples/run_wasm.sh /path/to/precompiled.wasm
#
# Available scripts in examples/wasm/:
#   01_basics.go          — navigate + wait for DOM visibility
#   02_events.go          — intercept network events via SSE
#   03_network_capture.go — capture requests with tab index
# ─────────────────────────────────────────────────────────────
set -e

SCRIPT="${1:-}"
PORT=9520
CLI="./browsii"

if [[ -z "$SCRIPT" ]]; then
    echo "Usage: $0 <script.go | script.wasm>" >&2
    echo "" >&2
    echo "Available scripts:" >&2
    for f in examples/wasm/*.go; do
        echo "  $f" >&2
    done
    exit 1
fi

if [[ ! -f "$SCRIPT" ]]; then
    echo "error: file not found: $SCRIPT" >&2
    exit 1
fi

echo "⚙️  Building CLI..."
go build -o "$CLI" cmd/browsii/*.go

echo "📦 Installing WASM runtimes..."
$CLI install-runtimes

echo "🚀 Starting daemon on port $PORT..."
$CLI start -p "$PORT" --mode headful
sleep 2

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "▶️  Running: $SCRIPT"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Capture exit code without triggering set -e
$CLI run -p "$PORT" "$SCRIPT" | python3 -m json.tool
RUN_EXIT=$?

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo "Cleaning up..."
$CLI stop -p "$PORT"

if [[ $RUN_EXIT -ne 0 ]]; then
    echo "❌ Script exited with code $RUN_EXIT"
    exit $RUN_EXIT
fi

echo "✅ Done."
