#!/bin/bash
# ─────────────────────────────────────────────────────────────
# Console Capture Demo — all levels, --level filter, --tab filter
# ─────────────────────────────────────────────────────────────
set -e

PORT=9530
CLI="./browsii"

echo "⚙️  Building CLI..."
go build -o "$CLI" cmd/browsii/*.go

echo "🚀 Starting daemon on port $PORT..."
$CLI start -p "$PORT" --mode headless
sleep 2

# ── Demo 1: Capture all console levels ───────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 1: Capture all console levels"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI console capture start -p "$PORT"
# Navigate to a page that fires console.log/warn/error/info/debug
$CLI navigate -p "$PORT" "data:text/html,<script>
  console.log('hello log');
  console.warn('hello warn');
  console.error('hello error');
  console.info('hello info');
  console.debug('hello debug');
</script>"
sleep 0.5

echo "📋 Captured entries (all levels):"
$CLI console capture stop -p "$PORT" | python3 -m json.tool

# ── Demo 2: --level filter (errors and warnings only) ─────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 2: --level error,warn (errors and warnings only)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI console capture start -p "$PORT" --level "error,warn"
$CLI navigate -p "$PORT" "data:text/html,<script>
  console.log('this log will be filtered out');
  console.warn('this warn IS captured');
  console.error('this error IS captured');
  console.info('this info will be filtered out');
</script>"
sleep 0.5

echo "📋 Captured entries (error + warn only):"
$CLI console capture stop -p "$PORT" | python3 -m json.tool

# ── Demo 3: --tab active (single tab filter) ──────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 3: --tab active (only capture from the active tab)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# tab 0 is active; open a second tab
$CLI navigate -p "$PORT" "about:blank"
$CLI tab new  -p "$PORT"          # tab 1, now active
$CLI tab switch -p "$PORT" 0      # switch back to tab 0

$CLI console capture start -p "$PORT" --tab active  # locked to tab 0

# tab 0 fires some console calls
$CLI navigate -p "$PORT" "data:text/html,<script>console.log('from tab 0');</script>"
# tab 1 also fires console calls — these should NOT be captured
$CLI tab switch -p "$PORT" 1
$CLI navigate -p "$PORT" "data:text/html,<script>console.log('from tab 1 — filtered out');</script>"
sleep 0.5

echo "📋 Captured entries (tab 0 only — tab field should always be 0):"
$CLI console capture stop -p "$PORT" | python3 -m json.tool

# ── Demo 4: Structured args ───────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 4: Multiple args — structured 'args' array"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI console capture start -p "$PORT"
$CLI navigate -p "$PORT" "data:text/html,<script>console.log('alpha', 'beta', 42, {key: 'value'});</script>"
sleep 0.5

echo "📋 Captured entries (args array with type/value/description per argument):"
$CLI console capture stop -p "$PORT" | python3 -m json.tool

# ── Cleanup ───────────────────────────────────────────────────
echo ""
echo "Cleaning up..."
$CLI stop -p "$PORT"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✨ Console capture demo complete!"
echo "   Each entry carries: level, text, args[], tab"
echo "   Filter by level or tab at capture start time."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
