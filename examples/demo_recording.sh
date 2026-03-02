#!/bin/bash
# ─────────────────────────────────────────────────────────────
# Comprehensive Session Recording & Replay Demo
# Tests:
#   1. In-depth API recording (navigate, type, press, scroll, hover, click, back, forward, reload)
#   2. Relative/Local Path Recording
#   3. Absolute Path Recording
#   4. Replay Speeds (Real-time, 2x, Instant)
# ─────────────────────────────────────────────────────────────
set -e

PORT=9600
CLI="./browsii"

echo "⚙️  Building CLI..."
go build -o "$CLI" cmd/browsii/*.go

echo "🚀 Starting daemon..."
$CLI start -p "$PORT" --mode headful
sleep 2

REC_RELATIVE="./in_depth_demo.json"
REC_ABSOLUTE="/tmp/abs_recording.json"

rm -f "$REC_RELATIVE"
rm -f "$REC_ABSOLUTE"

# ── Test 1: Record In-Depth Session to Relative Path ──
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🎥 Test 1: Record In-Depth Session to Relative Path"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI navigate -p "$PORT" "about:blank"
sleep 1

echo "▶️  Starting recording: $REC_RELATIVE"
$CLI record start -p "$PORT" "$REC_RELATIVE"

echo "   - Navigating to pkg.go.dev"
$CLI navigate -p "$PORT" "https://pkg.go.dev/"
sleep 1

echo "   - Searching for 'http'"
$CLI type -p "$PORT" "input[name=q]" "http"
sleep 1
$CLI press -p "$PORT" "Enter"
sleep 2

echo "   - Scrolling results"
$CLI scroll -p "$PORT" --down
sleep 1

echo "   - Hovering on first result"
$CLI hover -p "$PORT" ".SearchSnippet a"
sleep 1

echo "   - Clicking first result"
$CLI click -p "$PORT" ".SearchSnippet a"
sleep 2

echo "   - Testing navigation history (Back & Forward)"
$CLI back -p "$PORT"
sleep 2
$CLI forward -p "$PORT"
sleep 2

echo "   - Testing page reload"
$CLI reload -p "$PORT"
sleep 2

echo "⏹️  Stopping recording... saved to $REC_RELATIVE"
$CLI record stop -p "$PORT"


# ── Test 2: Record to Absolute Path ──
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🎥 Test 2: Record to an absolute path ($REC_ABSOLUTE)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "▶️  Starting recording: $REC_ABSOLUTE"
$CLI record start -p "$PORT" "$REC_ABSOLUTE"

$CLI navigate -p "$PORT" "https://go.dev/"
sleep 1
$CLI navigate -p "$PORT" "about:blank"
sleep 1

echo "⏹️  Stopping recording... saved to $REC_ABSOLUTE"
$CLI record stop -p "$PORT"


# ── Test 3: Replay In-Depth Session at 2x Speed ──
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔄 Test 3: Replay Relative Path at 2x speed"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Resetting to about:blank before replay..."
$CLI navigate -p "$PORT" "about:blank"
sleep 2

echo "▶️  Replaying $REC_RELATIVE at 2x speed..."
$CLI record replay -p "$PORT" "$REC_RELATIVE" --speed 2.0
sleep 2


# ── Test 4: Replay Absolute Path Instantly ──
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "⚡ Test 4: Replay absolute path instantly (speed 0)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Resetting to about:blank before replay..."
$CLI navigate -p "$PORT" "about:blank"
sleep 1

echo "▶️  Replaying $REC_ABSOLUTE instantly..."
$CLI record replay -p "$PORT" "$REC_ABSOLUTE" --speed 0
sleep 2


# ── Test 5: Replay Preset Recordings ──
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🗂️ Test 5: Replay Preset Recordings"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
PRESET_WIKI="examples/recording-presets/preset_wikipedia.json"
PRESET_HN="examples/recording-presets/preset_hackernews.json"

echo "▶️  Replaying Wikipedia Preset..."
$CLI record replay -p "$PORT" "$PRESET_WIKI" --speed 1.0
sleep 2

echo "▶️  Replaying HackerNews Preset..."
$CLI record replay -p "$PORT" "$PRESET_HN" --speed 1.0
sleep 2


# ── Clean up ──
echo ""
echo "🗑️  Cleaning up..."
$CLI record delete -p "$PORT" "$REC_RELATIVE"
$CLI record delete -p "$PORT" "$REC_ABSOLUTE"
$CLI stop -p "$PORT"

echo ""
echo "✅ Comprehensive recording and replay demo completed successfully!"
