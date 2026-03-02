#!/bin/bash
# ─────────────────────────────────────────────────────────────
# Session Management Demo — Multiple research sessions
# ─────────────────────────────────────────────────────────────
set -e

PORT=9500
CLI="./browsii"

echo "⚙️  Building CLI..."
go build -o "$CLI" cmd/browsii/*.go

echo "🚀 Starting daemon..."
$CLI start -p "$PORT" --mode headful
sleep 2

# ── Session 1: AI Research ──
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📚 Session 1: AI Research"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI navigate -p "$PORT" "https://en.wikipedia.org/wiki/Artificial_intelligence"
sleep 1
$CLI tab new -p "$PORT" "https://en.wikipedia.org/wiki/Large_language_model"
sleep 1
$CLI scroll -p "$PORT" --bottom
echo "📸 Scrolled to bottom of LLM article"

echo "💾 Saving session 'ai-research'..."
$CLI session save -p "$PORT" ai-research

# ── Session 2: Go Programming ──
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔧 Session 2: Go Programming"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

$CLI session new -p "$PORT" go-dev
$CLI navigate -p "$PORT" "https://go.dev/doc/"
sleep 1
$CLI tab new -p "$PORT" "https://pkg.go.dev/github.com/go-rod/rod"
sleep 1

echo "💾 Saving session 'go-dev'..."
$CLI session save -p "$PORT" go-dev

# ── List all sessions ──
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📋 All saved sessions:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
$CLI session list -p "$PORT"

# ── Resume AI Research ──
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔄 Resuming 'ai-research' session..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
$CLI session resume -p "$PORT" ai-research
sleep 2

echo "📋 Active tabs after resume:"
$CLI tab list -p "$PORT"

# ── Pause to admire ──
echo ""
echo "👀 AI Research session restored — take a look! (5 seconds)"
sleep 5

# ── Clean up sessions ──
echo ""
echo "🗑️  Cleaning up saved sessions..."
$CLI session delete -p "$PORT" ai-research
$CLI session delete -p "$PORT" go-dev
echo "📋 Sessions after cleanup:"
$CLI session list -p "$PORT"

# ── Shutdown ──
echo ""
echo "Cleaning up..."
$CLI stop -p "$PORT"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✨ Session demo complete!"
echo "   Created 2 sessions → listed → resumed → deleted"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
