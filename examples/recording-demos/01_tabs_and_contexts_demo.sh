#!/bin/bash
set -e

PORT=9601
CLI="../../browsii"
REC_FILE="./demo_tabs.json"

echo "⚙️ Building CLI..."
go build -o "$CLI" ../../cmd/browsii/*.go

echo "🚀 Starting daemon..."
$CLI start -p "$PORT" --mode headful
sleep 2

echo "▶️ Starting recording to $REC_FILE"
rm -f "$REC_FILE"
$CLI record start -p "$PORT" "$REC_FILE"

echo "   - Creating new context 'work'..."
$CLI context create -p "$PORT" --name "work"
sleep 1

echo "   - Opening tab to go.dev..."
$CLI tab new -p "$PORT" "https://go.dev"
sleep 2

echo "   - Opening tab to github.com..."
$CLI tab new -p "$PORT" "https://github.com"
sleep 2

echo "   - Switching back to tab 0..."
$CLI tab switch -p "$PORT" 0
sleep 2

echo "   - Closing current tab..."
$CLI tab close -p "$PORT"
sleep 1

echo "⏹️ Stopping recording..."
$CLI record stop -p "$PORT"

echo "🔄 Replaying $REC_FILE..."
$CLI record replay -p "$PORT" "$REC_FILE" --speed 1.5

echo "🗑️ Cleaning up..."
rm -f "$REC_FILE"
$CLI stop -p "$PORT"
echo "✅ Done!"
