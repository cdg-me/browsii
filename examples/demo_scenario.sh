#!/bin/bash
set -e

echo "============================================="
echo " Browser CLI: Multi-tab Demo Scenario"
echo "============================================="

# Ensure the CLI is built
echo "Building CLI..."
go build -o browsii cmd/browsii/*.go

# Start the daemon
echo "Starting daemon in headful mode..."
./browsii start --mode headful --port 8000
sleep 2

echo "Defining URLs to open..."
URLS=(
    "https://news.ycombinator.com"
    "https://en.wikipedia.org/wiki/Main_Page"
    "https://github.com/trending"
    "https://lobste.rs"
    "https://go.dev"
)

# Open 5 tabs
echo "Opening ${#URLS[@]} tabs..."
for url in "${URLS[@]}"; do
    echo "  -> Opening $url"
    ./browsii tab new "$url" --port 8000
    sleep 1
done

echo "Listing active tabs..."
./browsii tab list --port 8000
sleep 2

# Switch between them
echo "Switching between tabs iteratively..."
for i in {0..4}; do
    echo "  -> Activating tab index $i"
    ./browsii tab switch "$i" --port 8000
    sleep 1.5
done

# Cleanup
echo "Demonstration complete. Shutting down daemon..."
./browsii stop --port 8000
echo "Done."
