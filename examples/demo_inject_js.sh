#!/bin/bash
# ─────────────────────────────────────────────────────────────
# Inject JS Demo — register scripts that run before any page code
# ─────────────────────────────────────────────────────────────
set -e

PORT=9540
CLI="./browsii"

echo "⚙️  Building CLI..."
go build -o "$CLI" cmd/browsii/*.go

echo "🚀 Starting daemon on port $PORT..."
$CLI start -p "$PORT" --mode headless
sleep 2

# ── Demo 1: Inject a global before any page script runs ───────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 1: Inject a global flag before the page loads"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI inject js add -p "$PORT" "window.__testMode = true; window.__env = 'staging';"

$CLI navigate -p "$PORT" "https://example.com"

echo "🔍 Reading injected globals:"
$CLI js -p "$PORT" "() => ({ testMode: window.__testMode, env: window.__env })"

# ── Demo 2: Override a browser API before the page can ────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 2: Intercept fetch before the page touches it"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI inject js clear -p "$PORT"

$CLI inject js add -p "$PORT" "
  window.__intercepted = [];
  const _fetch = window.fetch;
  window.fetch = function(...args) {
    window.__intercepted.push(args[0]);
    return _fetch.apply(this, args);
  };
"

$CLI navigate -p "$PORT" "https://example.com"

echo "🔍 Intercepted fetch calls:"
$CLI js -p "$PORT" "() => window.__intercepted"

# ── Demo 3: Stack multiple injections, verify order ───────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 3: Multiple adds — all fire in registration order"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI inject js clear -p "$PORT"

ID1=$($CLI inject js add -p "$PORT" "window.__order = []; window.__order.push('first');")
ID2=$($CLI inject js add -p "$PORT" "window.__order.push('second');")
ID3=$($CLI inject js add -p "$PORT" "window.__order.push('third');")
echo "  Registered: $ID1  $ID2  $ID3"

$CLI navigate -p "$PORT" "https://example.com"

echo "🔍 Execution order:"
$CLI js -p "$PORT" "() => window.__order"

# ── Demo 4: Tab-specific injection ────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 4: Tab-specific inject (--tab active)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI inject js clear -p "$PORT"
$CLI navigate -p "$PORT" "about:blank"

$CLI inject js add -p "$PORT" --tab active "window.__tabSpecific = 'only-tab-0';"

$CLI tab new -p "$PORT" "https://example.com"

echo "🔍 Tab 1 (should be null — injection was tab-0 only):"
$CLI js -p "$PORT" "() => window.__tabSpecific"

$CLI tab switch -p "$PORT" 0
$CLI navigate -p "$PORT" "https://example.com"

echo "🔍 Tab 0 (should be 'only-tab-0'):"
$CLI js -p "$PORT" "() => window.__tabSpecific"

# ── Demo 5: Global inject applies to tabs opened later ────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 5: Global inject — new tabs pick it up automatically"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI inject js clear -p "$PORT"
$CLI navigate -p "$PORT" "about:blank"

$CLI inject js add -p "$PORT" "window.__global = 'present';"

$CLI tab new -p "$PORT" "https://example.com"

echo "🔍 New tab global (should be 'present'):"
$CLI js -p "$PORT" "() => window.__global"

# ── Demo 6: --url fetches eagerly at registration time ────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 6: --url — content is inlined at add time"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI inject js clear -p "$PORT"

$CLI inject js add -p "$PORT" --url "https://cdn.jsdelivr.net/npm/dayjs@1/dayjs.min.js"

$CLI navigate -p "$PORT" "https://example.com"

echo "🔍 dayjs available before page (should return today's year):"
$CLI js -p "$PORT" "() => dayjs().year()"

# ── Demo 7: list then clear ───────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 7: List registered entries, then clear"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI inject js add -p "$PORT" "window.__a = 1;"
$CLI inject js add -p "$PORT" "window.__b = 2;"

echo "📋 Registered inject-js entries:"
$CLI inject js list -p "$PORT" | python3 -m json.tool

$CLI inject js clear -p "$PORT"

echo "📋 After clear (should be empty):"
$CLI inject js list -p "$PORT"

$CLI navigate -p "$PORT" "https://example.com"

echo "🔍 Globals after clear (should both be null):"
$CLI js -p "$PORT" "() => ({ a: window.__a, b: window.__b })"

# ── Cleanup ───────────────────────────────────────────────────
echo ""
echo "Cleaning up..."
$CLI stop -p "$PORT"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✨ Inject JS demo complete!"
echo ""
echo "   Key properties demonstrated:"
echo "   • Scripts run BEFORE any page-owned scripts"
echo "   • Multiple adds stack, fire in registration order"
echo "   • Global (no --tab) applies to future tabs too"
echo "   • --tab active scopes to one tab only"
echo "   • --url fetches content eagerly at add time"
echo "   • clear stops future injections; current page unaffected"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
