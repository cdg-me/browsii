#!/bin/bash
set -e

PORT=9604
CLI="../../browsii"
REC_FILE="./demo_content.json"

echo "⚙️ Building CLI..."
go build -o "$CLI" ../../cmd/browsii/*.go

echo "🚀 Starting daemon..."
$CLI start -p "$PORT" --mode headful
sleep 2

echo "▶️ Starting recording to $REC_FILE"
rm -f "$REC_FILE"
$CLI record start -p "$PORT" "$REC_FILE"

echo "   - Navigating to pkg.go.dev/net/http..."
$CLI navigate -p "$PORT" "https://pkg.go.dev/net/http"
sleep 2

echo "   - Scraping page..."
$CLI scrape -p "$PORT" --format markdown > /dev/null
sleep 1

echo "   - Extracting links..."
$CLI get-links -p "$PORT" > /dev/null
sleep 1

echo "   - Taking screenshot..."
$CLI screenshot -p "$PORT" "test_screen.png"
sleep 1

echo "   - Generating PDF..."
$CLI pdf -p "$PORT" "test_doc.pdf"
sleep 1

echo "⏹️ Stopping recording..."
$CLI record stop -p "$PORT"

echo "🔄 Replaying $REC_FILE at 1x speed..."
rm -f test_screen.png test_doc.pdf
$CLI record replay -p "$PORT" "$REC_FILE" --speed 1.0

echo "🔍 Verifying generated artifacts exist..."
if [ -f "test_screen.png" ]; then echo "Screenshot generated via replay!"; fi
if [ -f "test_doc.pdf" ]; then echo "PDF generated via replay!"; fi

echo "🗑️ Cleaning up..."
rm -f "$REC_FILE" test_screen.png test_doc.pdf
$CLI stop -p "$PORT"
echo "✅ Done!"
