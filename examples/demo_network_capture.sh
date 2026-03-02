#!/bin/bash
# ─────────────────────────────────────────────────────────────
# Network Capture Demo — all-tabs, --tab active, --tab next
# ─────────────────────────────────────────────────────────────
set -e

PORT=9510
CLI="./browsii"

echo "⚙️  Building CLI..."
go build -o "$CLI" cmd/browsii/*.go

echo "🚀 Starting daemon on port $PORT..."
$CLI start -p "$PORT" --mode headful
sleep 2

# ── Demo 1: All-tabs capture (default) ───────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 1: all-tabs capture (default — no --tab flag)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI navigate -p "$PORT" "about:blank"
$CLI tab new  -p "$PORT"                      # open second tab (index 1)

$CLI network capture start -p "$PORT"         # captures all tabs

$CLI navigate      -p "$PORT" "https://example.com"   # tab 1 (active after tab new)
$CLI tab switch    -p "$PORT" 0
$CLI navigate      -p "$PORT" "https://example.org"   # tab 0
sleep 1

echo "📡 Captured events (all tabs, showing tab index per request):"
$CLI network capture stop -p "$PORT" | python3 -m json.tool

# ── Demo 2: --tab active ──────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 2: --tab active (only the currently active tab)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI tab switch -p "$PORT" 0
$CLI network capture start -p "$PORT" --tab active

# Both tabs navigate; only tab-0 events should appear
$CLI navigate   -p "$PORT" "https://example.com"
$CLI tab switch -p "$PORT" 1
$CLI navigate   -p "$PORT" "https://example.org"
sleep 1

echo "📡 Captured events (active tab = 0 only):"
$CLI network capture stop -p "$PORT" | python3 -m json.tool

# ── Demo 3: --tab next ────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 3: --tab next (tab opened after capture starts)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI network capture start -p "$PORT" --tab next

$CLI tab new    -p "$PORT"                    # this new tab (index 2) is "next"
$CLI navigate   -p "$PORT" "https://example.net"
$CLI tab switch -p "$PORT" 0
$CLI navigate   -p "$PORT" "https://example.com"  # tab 0 — should NOT appear
sleep 1

echo "📡 Captured events (next tab = 2 only):"
$CLI network capture stop -p "$PORT" | python3 -m json.tool

# ── Cleanup ───────────────────────────────────────────────────
echo ""
echo "Cleaning up..."
$CLI stop -p "$PORT"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✨ Network capture demo complete!"
echo "   Each event carries a 'tab' field — the 0-based index"
echo "   of the tab that fired the request."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
