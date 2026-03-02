package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_SaveAndResume(t *testing.T) {
	serverA := setupNamedServer("Page Alpha")
	defer serverA.Close()
	serverB := setupNamedServer("Page Beta")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Set up a session with two tabs
	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "tab", "new", serverB.URL)

	// Save the session
	runCLI(t, bin, port, "session", "save", "test-session")

	// Verify session file was created
	homeDir, _ := os.UserHomeDir()
	sessionFile := filepath.Join(homeDir, ".browsii", "sessions", "test-session.json")
	defer os.Remove(sessionFile)

	data, err := os.ReadFile(sessionFile)
	require.NoError(t, err, "Session file should exist")

	var session map[string]interface{}
	err = json.Unmarshal(data, &session)
	require.NoError(t, err, "Session should be valid JSON")

	tabs, ok := session["tabs"].([]interface{})
	require.True(t, ok, "Session should have tabs array")
	assert.GreaterOrEqual(t, len(tabs), 2, "Should have at least 2 tabs saved")
	assert.Contains(t, session, "activeTab", "Session should have activeTab field")
}

func TestSession_NewClearsTabs(t *testing.T) {
	serverA := setupNamedServer("Old Page")
	defer serverA.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate to a page and open another tab
	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "tab", "new", serverA.URL)

	// Start a new session — should close all tabs
	runCLI(t, bin, port, "session", "new", "fresh")

	// Tab list should show only one blank tab
	out := runCLI(t, bin, port, "tab", "list")
	assert.Contains(t, out, "about:blank", "New session should have an about:blank tab")
	assert.NotContains(t, out, serverA.URL, "New session should not have old URLs")
}

func TestSession_ResumeRestoresState(t *testing.T) {
	serverA := setupNamedServer("Restore A")
	defer serverA.Close()
	serverB := setupNamedServer("Restore B")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Build a session
	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "tab", "new", serverB.URL)

	// Save
	runCLI(t, bin, port, "session", "save", "resume-test")

	homeDir, _ := os.UserHomeDir()
	sessionFile := filepath.Join(homeDir, ".browsii", "sessions", "resume-test.json")
	defer os.Remove(sessionFile)

	// Start fresh
	runCLI(t, bin, port, "session", "new", "blank")

	// Resume the saved session
	runCLI(t, bin, port, "session", "resume", "resume-test")

	// Verify tabs were restored
	out := runCLI(t, bin, port, "tab", "list")
	assert.Contains(t, out, serverA.URL, "Resumed session should contain server A URL")
	assert.Contains(t, out, serverB.URL, "Resumed session should contain server B URL")
}

func TestSession_ListShowsSessions(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", "about:blank")

	// Save two sessions
	runCLI(t, bin, port, "session", "save", "list-test-a")
	runCLI(t, bin, port, "session", "save", "list-test-b")

	homeDir, _ := os.UserHomeDir()
	defer os.Remove(filepath.Join(homeDir, ".browsii", "sessions", "list-test-a.json"))
	defer os.Remove(filepath.Join(homeDir, ".browsii", "sessions", "list-test-b.json"))

	// List sessions
	out := runCLI(t, bin, port, "session", "list")
	assert.Contains(t, out, "list-test-a", "Should list session A")
	assert.Contains(t, out, "list-test-b", "Should list session B")
}

func TestSession_Delete(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", "about:blank")

	// Save a session
	runCLI(t, bin, port, "session", "save", "delete-test")

	homeDir, _ := os.UserHomeDir()
	sessionFile := filepath.Join(homeDir, ".browsii", "sessions", "delete-test.json")

	// Verify it exists
	_, err := os.Stat(sessionFile)
	require.NoError(t, err, "Session file should exist before delete")

	// Delete it
	runCLI(t, bin, port, "session", "delete", "delete-test")

	// Verify it's gone
	_, err = os.Stat(sessionFile)
	assert.True(t, os.IsNotExist(err), "Session file should be gone after delete")

	// List should not contain it
	out := runCLI(t, bin, port, "session", "list")
	assert.NotContains(t, out, "delete-test", "Deleted session should not appear in list")
}
