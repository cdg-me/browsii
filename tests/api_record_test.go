package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecord_CapturesActions(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate first to seed a page
	runCLI(t, bin, port, "navigate", server.URL)

	// Start recording
	runCLI(t, bin, port, "record", "start", "capture-test")

	// Perform actions while recording
	runCLI(t, bin, port, "type", "#inputBox", "hello world")
	runCLI(t, bin, port, "scroll", "--down")

	// Stop recording
	out := runCLI(t, bin, port, "record", "stop")
	assert.Contains(t, out, "capture-test", "Stop should mention recording name")

	// Verify recording file
	homeDir, _ := os.UserHomeDir()
	recFile := filepath.Join(homeDir, ".browsii", "recordings", "capture-test.json")
	defer os.Remove(recFile) //nolint:errcheck

	data, err := os.ReadFile(recFile)
	require.NoError(t, err, "Recording file should exist")

	var recording struct {
		Name   string `json:"name"`
		Events []struct {
			T      int64  `json:"t"`
			Action string `json:"action"`
		} `json:"events"`
	}
	err = json.Unmarshal(data, &recording)
	require.NoError(t, err, "Recording should be valid JSON")

	assert.Equal(t, "capture-test", recording.Name)
	assert.GreaterOrEqual(t, len(recording.Events), 2, "Should have at least 2 recorded events")

	// Check action types are recorded
	actions := make([]string, 0)
	for _, e := range recording.Events {
		actions = append(actions, e.Action)
	}
	assert.Contains(t, actions, "type")
	assert.Contains(t, actions, "scroll")

	// Timing should be monotonic
	for i := 1; i < len(recording.Events); i++ {
		assert.GreaterOrEqual(t, recording.Events[i].T, recording.Events[i-1].T,
			"Event timestamps should be monotonically increasing")
	}
}

func TestRecord_ReplayRestoresState(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	homeDir, _ := os.UserHomeDir()
	recFile := filepath.Join(homeDir, ".browsii", "recordings", "replay-test.json")
	defer os.Remove(recFile) //nolint:errcheck

	// Navigate and start recording
	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "record", "start", "replay-test")

	// Type into the input
	runCLI(t, bin, port, "type", "#inputBox", "replay-value")

	// Stop
	runCLI(t, bin, port, "record", "stop")

	// Clear the input to prove replay works
	runCLI(t, bin, port, "js", "() => { document.querySelector('#inputBox').value = ''; }")

	// Verify it's cleared
	val := runCLI(t, bin, port, "js", "() => document.querySelector('#inputBox').value")
	assert.NotContains(t, val, "replay-value")

	// Replay the recording (speed 0 = instant)
	runCLI(t, bin, port, "record", "replay", "replay-test", "--speed", "0")

	// Verify the value was restored by replay
	val2 := runCLI(t, bin, port, "js", "() => document.querySelector('#inputBox').value")
	assert.Contains(t, val2, "replay-value", "Replay should have typed the value again")
}

func TestRecord_ListAndDelete(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", "about:blank")

	homeDir, _ := os.UserHomeDir()
	defer os.Remove(filepath.Join(homeDir, ".browsii", "recordings", "list-a.json")) //nolint:errcheck
	defer os.Remove(filepath.Join(homeDir, ".browsii", "recordings", "list-b.json")) //nolint:errcheck

	// Create two recordings
	runCLI(t, bin, port, "record", "start", "list-a")
	runCLI(t, bin, port, "record", "stop")
	runCLI(t, bin, port, "record", "start", "list-b")
	runCLI(t, bin, port, "record", "stop")

	// List should show both
	out := runCLI(t, bin, port, "record", "list")
	assert.Contains(t, out, "list-a")
	assert.Contains(t, out, "list-b")

	// Delete one
	runCLI(t, bin, port, "record", "delete", "list-a")

	// List should only show b
	out2 := runCLI(t, bin, port, "record", "list")
	assert.NotContains(t, out2, "list-a")
	assert.Contains(t, out2, "list-b")
}
