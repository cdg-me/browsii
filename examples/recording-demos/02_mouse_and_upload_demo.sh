#!/bin/bash
set -e

PORT=9602
CLI="../../browsii"
REC_FILE="./demo_mouse.json"

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

echo "   - Moving mouse..."
$CLI mouse move -p "$PORT" 100 200
sleep 1
$CLI mouse move -p "$PORT" 300 400
sleep 1

echo "   - Dragging mouse..."
$CLI mouse drag -p "$PORT" 300 400 500 500 --steps 20
sleep 1

echo "   - Double clicking header..."
$CLI mouse double-click -p "$PORT" "h1"
sleep 2

echo "   - Right clicking link..."
$CLI mouse right-click -p "$PORT" "a"
sleep 2

echo "⏹️ Stopping recording..."
$CLI record stop -p "$PORT"

echo "🔄 Replaying $REC_FILE at 1x speed..."
$CLI record replay -p "$PORT" "$REC_FILE" --speed 1.0

echo "🗑️ Cleaning up..."
rm -f "$REC_FILE"
$CLI stop -p "$PORT"
echo "✅ Done!"
