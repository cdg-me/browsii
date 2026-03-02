#!/bin/bash
set -e

echo "================================================="
echo " Browser CLI: Persistent Profile & Auth Demo"
echo "================================================="

echo "Building CLI..."
go build -o browsii cmd/browsii/*.go

# 1. Clear the profile completely for a clean state
echo ""
echo "[1/3] Wiping the existing ~/.browsii/profile..."
rm -rf ~/.browsii/profile

# 2. Open the foreground manual setup window
echo ""
echo "[2/3] Booting manual profile setup."
echo "--------------------------------------------------------"
echo "INSTRUCTIONS FOR THIS DEMO:"
echo " 1. The browser will open."
echo " 2. Navigate to https://en.wikipedia.org"
echo " 3. Click 'Create account' or 'Log in' in the top right."
echo " 4. (Or just change the language/theme settings)."
echo " 5. Close the browser window completely (Cmd+Q)."
echo "--------------------------------------------------------"

# Run it directly in the foreground, blocking the terminal
./browsii profile setup "https://en.wikipedia.org"

# 3. Resume the automated agent daemon using the saved profile
echo ""
echo "[3/3] Simulating an LLM Agent run using the saved profile..."
echo "Starting daemon natively (user-headful) on port 9005..."
./browsii start --mode user-headful --port 9005
sleep 3

# Navigate back to Wikipedia
echo "Navigating automated daemon back to Wikipedia..."
./browsii navigate "https://en.wikipedia.org" --port 9005
sleep 3

# Take a screenshot to prove the LLM is logged in
echo "Taking screenshot of the LLM's viewport to prove persistent auth state..."
SS_FILE="wikipedia_auth_proof.png"
./browsii screenshot "$SS_FILE" --port 9005

echo ""
echo "--- Cookies Loaded by the LLM Daemon ---"
./browsii cookies --port 9005
echo "----------------------------------------"

echo "Shutting down automated daemon..."
./browsii stop --port 9005

echo ""
echo "Done! Check $SS_FILE. If you logged in during step 2, you will be logged in within the screenshot."
