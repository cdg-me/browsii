#!/usr/bin/env bash
# snapshot-demo.sh — record a multi-page HN flow then replay it fully offline
#
# What this tests:
#   1. Record: front page + top 3 story pages captured as a single HAR
#   2. Load snapshot (no more network for recorded URLs)
#   3. Navigate front page → scrape headlines
#   4. Navigate into each recorded story → scrape content
#   5. Navigate to an unrecorded URL → passes through to live network
#
# Usage: ./scripts/snapshot-demo.sh [port]
set -uo pipefail

PORT=${1:-9850}
BINARY=${BINARY:-./browsii}
HAR=$(mktemp /tmp/browsii-demo-XXXX.har)
BASE="https://news.ycombinator.com"

cleanup() {
  "$BINARY" stop --port "$PORT" 2>/dev/null || true
  rm -f "$HAR"
}
trap cleanup EXIT

echo "==> starting daemon (port $PORT, headless)"
"$BINARY" start --port "$PORT" --mode headless

# ── Record ────────────────────────────────────────────────────────────────────
echo ""
echo "── RECORD ──────────────────────────────────────────────────────────────"
"$BINARY" network capture start \
  --port "$PORT" \
  --format har \
  --include response-headers,response-body \
  --output "$HAR"

echo "  navigating front page..."
"$BINARY" navigate "$BASE/" --port "$PORT"

# Pull first 3 /item? links from front page so we record those story pages too
STORY_LINKS=$("$BINARY" get-links --port "$PORT" \
  | python3 -c "
import json, sys
links = json.load(sys.stdin)
seen = []
for l in links:
    if '/item?id=' in l and l not in seen:
        seen.append(l)
    if len(seen) == 3:
        break
print('\n'.join(seen))
")

echo "  recording story pages:"
while IFS= read -r link; do
  [[ -z "$link" ]] && continue
  echo "    $link"
  "$BINARY" navigate "$link" --port "$PORT"
done <<< "$STORY_LINKS"

"$BINARY" network capture stop --port "$PORT"

ENTRIES=$(python3 -c "import json; d=json.load(open('$HAR')); print(len(d['log']['entries']))")
echo "  recorded $ENTRIES network entries → $HAR"

# ── Replay ────────────────────────────────────────────────────────────────────
echo ""
echo "── REPLAY (snapshot loaded, no live network for recorded URLs) ──────────"
"$BINARY" snapshot load "$HAR" --port "$PORT"

echo ""
echo "  front page from snapshot:"
"$BINARY" navigate "$BASE/" --port "$PORT"
"$BINARY" scrape --format text --port "$PORT" | head -20 || true

echo ""
echo "  story pages (each served from snapshot, no network):"
while IFS= read -r link; do
  [[ -z "$link" ]] && continue
  "$BINARY" navigate "$link" --port "$PORT"
  echo "  ── $link"
  "$BINARY" scrape --format text --port "$PORT" | head -8 || true
  echo ""
done <<< "$STORY_LINKS"

echo "  unrecorded URL ($BASE/ask) — passes through to live network:"
"$BINARY" navigate "$BASE/ask" --port "$PORT" \
  && "$BINARY" scrape --format text --port "$PORT" | head -5 \
  || echo "  (navigation failed — expected if truly offline)"

echo ""
echo "==> clearing snapshot and stopping daemon"
"$BINARY" snapshot clear --port "$PORT"
