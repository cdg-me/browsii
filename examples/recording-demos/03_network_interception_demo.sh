#!/bin/bash
set -e

PORT=9603
CLI="../../browsii"
REC_FILE="./demo_network.json"

echo "⚙️ Building CLI..."
go build -o "$CLI" ../../cmd/browsii/*.go

echo "🚀 Starting daemon..."
$CLI start -p "$PORT" --mode headful
sleep 2

echo "▶️ Starting recording to $REC_FILE"
rm -f "$REC_FILE"
$CLI record start -p "$PORT" "$REC_FILE"

echo "   - Navigating to example.com..."
$CLI navigate -p "$PORT" "https://example.com"
sleep 1

echo "   - Mocking API Response (network mock)..."
$CLI network mock -p "$PORT" --pattern "https://example.com/api/test" --body '{"mocked": true, "message": "Network Intercepted via CLI Session Reply!"}' --content-type "application/json" --status 200
sleep 1

echo "   - Throttling connection (network throttle)..."
$CLI network throttle -p "$PORT" --latency 500 --download 50000 --upload 50000
sleep 1

echo "   - Injecting JS to trigger the mock endpoint..."
$CLI js -p "$PORT" "() => fetch('https://example.com/api/test').then(r=>r.json()).then(d=>document.body.innerText=JSON.stringify(d))"
sleep 3

echo "⏹️ Stopping recording..."
$CLI record stop -p "$PORT"

echo "🔄 Replaying $REC_FILE at 1x speed..."
$CLI navigate -p "$PORT" "https://example.com"
$CLI record replay -p "$PORT" "$REC_FILE" --speed 1.0

echo "🗑️ Cleaning up..."
rm -f "$REC_FILE"
$CLI stop -p "$PORT"
echo "✅ Done!"
