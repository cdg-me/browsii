#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# Network Capture Demo
#
# Covers:
#   1. All-tabs capture (default)
#   2. --tab active / --tab next  (tab filters)
#   3. --include request-headers  (request detail)
#   4. --include response-headers (response status + headers)
#   5. --include request-*,response-* (full detail with wildcards)
#   6. --format ndjson            (newline-delimited JSON)
#   7. --format har --output      (HAR file)
# ─────────────────────────────────────────────────────────────────────────────
set -e

PORT=9510
CLI="./browsii"

echo "⚙️  Building CLI..."
go build -o "$CLI" cmd/browsii/*.go

echo "🚀 Starting daemon on port $PORT..."
$CLI start -p "$PORT" --mode headless &
sleep 1

# ── Demo 1: All-tabs capture (default) ───────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 1: all-tabs capture (default — no --tab flag)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI navigate -p "$PORT" "about:blank"
$CLI tab new  -p "$PORT"                          # open tab 1

$CLI network capture start -p "$PORT"             # captures all tabs

$CLI navigate   -p "$PORT" "https://example.com"  # tab 1 (active after tab new)
$CLI tab switch -p "$PORT" 0
$CLI navigate   -p "$PORT" "https://example.org"  # tab 0
sleep 1

echo "📡 Captured events (all tabs):"
$CLI network capture stop -p "$PORT" | python3 -m json.tool

# ── Demo 2: --tab active ──────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 2: --tab active (only the tab active at start time)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI tab switch -p "$PORT" 0
$CLI network capture start -p "$PORT" --tab active

$CLI navigate   -p "$PORT" "https://example.com"   # tab 0 — captured
$CLI tab switch -p "$PORT" 1
$CLI navigate   -p "$PORT" "https://example.org"   # tab 1 — not captured
sleep 1

echo "📡 Captured events (tab 0 only — tab 1 absent):"
$CLI network capture stop -p "$PORT" | python3 -m json.tool

# ── Demo 3: --tab next ────────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 3: --tab next (tab opened after capture starts)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI network capture start -p "$PORT" --tab next

$CLI tab new   -p "$PORT"                           # opens tab 2 — the "next" tab
$CLI navigate  -p "$PORT" "https://example.net"     # captured (tab 2)
$CLI tab switch -p "$PORT" 0
$CLI navigate  -p "$PORT" "https://example.com"     # not captured (tab 0)
sleep 1

echo "📡 Captured events (tab 2 / next only):"
$CLI network capture stop -p "$PORT" | python3 -m json.tool

# ── Demo 4: --include request-headers ────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 4: --include request-headers (outgoing headers per request)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI tab switch -p "$PORT" 0
$CLI network capture start -p "$PORT" --include request-headers
$CLI navigate   -p "$PORT" "https://example.com"
sleep 1

echo "📡 Each entry now has a 'requestHeaders' map:"
$CLI network capture stop -p "$PORT" | python3 -c "
import json, sys
data = json.load(sys.stdin)
for r in data[:2]:
    print(f\"  {r['method']} {r['url']}\")
    ua = r.get('requestHeaders', {}).get('User-Agent') or r.get('requestHeaders', {}).get('user-agent', '—')
    print(f\"    User-Agent: {ua[:60]}...\")
"

# ── Demo 5: --include response-headers ────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 5: --include response-headers (status + response headers + mimeType)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI network capture start -p "$PORT" --include response-headers
$CLI navigate   -p "$PORT" "https://example.com"
sleep 1  # allow NetworkResponseReceived events to arrive

echo "📡 Each entry now has 'status', 'statusText', 'mimeType', 'responseHeaders':"
$CLI network capture stop -p "$PORT" | python3 -c "
import json, sys
data = json.load(sys.stdin)
for r in data[:3]:
    status = r.get('status', '—')
    mime   = r.get('mimeType', '—')
    print(f\"  [{status}] {r['method']} {r['url']}\")
    print(f\"    mimeType: {mime}\")
"

# ── Demo 6: --include request-*,response-* (wildcards, all groups) ────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 6: --include 'request-*,response-*' (all groups via wildcards)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI network capture start -p "$PORT" --include "request-*,response-*"
$CLI navigate   -p "$PORT" "https://example.com"
sleep 1

echo "📡 Full detail — request headers, response status, timing, size:"
$CLI network capture stop -p "$PORT" | python3 -c "
import json, sys
data = json.load(sys.stdin)
for r in data[:2]:
    print(f\"  {r['method']} {r['url']}\")
    print(f\"    status={r.get('status','—')}  mime={r.get('mimeType','—')}\")
    if r.get('timing'):
        t = r['timing']
        print(f\"    timing: dns={t['dns']:.1f}ms connect={t['connect']:.1f}ms wait={t['wait']:.1f}ms\")
    if r.get('transferSize') is not None:
        print(f\"    transferSize={r['transferSize']} bytes\")
"

# ── Demo 7: --format ndjson ───────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 7: --format ndjson (newline-delimited JSON — easy to stream/pipe)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI network capture start -p "$PORT" --format ndjson
$CLI navigate   -p "$PORT" "https://example.com"
sleep 1

echo "📡 Stop output — one JSON object per line:"
$CLI network capture stop -p "$PORT" | head -5

# ── Demo 8: --format har --output ─────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Demo 8: --format har --output capture.har (HAR 1.2 file)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

HAR_FILE="/tmp/browsii_demo.har"

$CLI network capture start -p "$PORT" \
    --include "request-headers,response-headers,response-timing,response-size" \
    --format har \
    --output "$HAR_FILE"

$CLI navigate   -p "$PORT" "https://example.com"
sleep 1

echo "📡 Stop — daemon writes HAR file and returns confirmation:"
$CLI network capture stop -p "$PORT"

echo ""
echo "📄 HAR file preview ($HAR_FILE):"
python3 -c "
import json, sys
with open('$HAR_FILE') as f:
    har = json.load(f)
log = har['log']
print(f\"  HAR version : {log['version']}\")
print(f\"  Creator     : {log['creator']['name']}\")
print(f\"  Entries     : {len(log['entries'])}\")
for e in log['entries'][:3]:
    req  = e['request']
    resp = e['response']
    t    = e['timings']
    print(f\"  [{resp['status']}] {req['method']} {req['url']}\")
    wait = t.get('wait', -1)
    if wait >= 0:
        print(f\"       wait={wait:.1f}ms\")
"
rm -f "$HAR_FILE"

# ── Cleanup ───────────────────────────────────────────────────────────────────
echo ""
echo "Cleaning up..."
$CLI stop -p "$PORT"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✨ Network capture demo complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
