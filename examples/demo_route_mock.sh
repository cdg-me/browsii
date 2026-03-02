#!/bin/bash
# ─────────────────────────────────────────────────────────────
# Route Mock Demo — Intercept HN and inject fake data
# ─────────────────────────────────────────────────────────────
set -e

PORT=9500
CLI="./browsii"
DATE=$(date +%Y-%m-%d_%H%M%S)
RESULTS_DIR="script-results/route-mock-${DATE}"

mkdir -p "$RESULTS_DIR"

echo "⚙️  Building CLI..."
go build -o "$CLI" cmd/browsii/*.go

# Start daemon (visible Chrome window)
echo "🚀 Starting daemon on port $PORT..."
$CLI start -p "$PORT" --mode headful
sleep 2

# Navigate to seed a page
echo "📄 Opening blank page..."
$CLI navigate -p "$PORT" "about:blank"

# Start network capture
echo "📡 Starting network capture..."
$CLI network capture start -p "$PORT"

# Install route mock
echo "🔀 Installing route mock for news.ycombinator.com..."
$CLI network mock -p "$PORT" \
  --pattern "*/news.ycombinator.com/*" \
  --content-type "text/html" \
  --body '<html>
<head><title>Hacker News (MOCKED)</title>
<style>
  body { font-family: Verdana, sans-serif; background: #f6f6ef; margin: 0; }
  .header { background: #ff6600; padding: 8px 12px; color: white; font-weight: bold; font-size: 16px; }
  .stories { padding: 10px 15px; }
  .story { margin: 10px 0; }
  .story a { color: #000; text-decoration: none; font-size: 14px; }
  .story a:hover { text-decoration: underline; }
  .meta { color: #828282; font-size: 10px; margin-top: 2px; }
  .rank { color: #828282; margin-right: 5px; }
  .banner { background: #2d2d2d; color: #0f0; font-family: monospace; padding: 12px;
            text-align: center; font-size: 13px; letter-spacing: 1px; }
</style>
</head>
<body>
  <div class="banner">⚡ THIS PAGE WAS INTERCEPTED BY browsii route mock ⚡</div>
  <div class="header">Hacker News — MOCKED RESPONSE</div>
  <div class="stories">
    <div class="story"><span class="rank">1.</span> <a href="#">browsii: 46 tests, zero regressions 🎯</a><div class="meta">420 points by you | 69 comments</div></div>
    <div class="story"><span class="rank">2.</span> <a href="#">Show HN: I built a CLI that mocks any website API</a><div class="meta">337 points by go-rod-fan | 42 comments</div></div>
    <div class="story"><span class="rank">3.</span> <a href="#">TDD in Go: How we shipped 20 features in one session</a><div class="meta">256 points by tdd_enjoyer | 31 comments</div></div>
    <div class="story"><span class="rank">4.</span> <a href="#">Route mocking vs. network capture — when to use each</a><div class="meta">198 points by cdp_wizard | 28 comments</div></div>
    <div class="story"><span class="rank">5.</span> <a href="#">Ask HN: What CLI tools do you use for browser automation?</a><div class="meta">145 points by curious_dev | 87 comments</div></div>
  </div>
</body>
</html>'

# Navigate to HN (intercepted by route mock)
echo "🌐 Navigating to https://news.ycombinator.com/ ..."
$CLI navigate -p "$PORT" "https://news.ycombinator.com/"

# Let the user see the page
echo ""
echo "👀 Page is open — take a look! Waiting 5 seconds..."
sleep 5

# Save outputs
echo "📸 Saving screenshot..."
$CLI screenshot -p "$PORT" "${RESULTS_DIR}/screenshot.png"

echo "📡 Stopping network capture..."
$CLI network capture stop -p "$PORT" > "${RESULTS_DIR}/network_capture.json"

echo "📋 Saving scraped text..."
$CLI scrape -p "$PORT" --format text > "${RESULTS_DIR}/scraped_text.txt"

# Clean shutdown
echo ""
echo "Cleaning up..."
$CLI stop -p "$PORT"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✨ Route mock demo complete!"
echo ""
echo "   Results saved to: ${RESULTS_DIR}/"
ls -lh "${RESULTS_DIR}/"
echo ""
echo "   • screenshot.png        — visual proof of the mocked page"
echo "   • network_capture.json  — all intercepted network requests"
echo "   • scraped_text.txt      — page content as plain text"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
