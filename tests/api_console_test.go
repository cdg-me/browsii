package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findConsoleEntry returns the first entry whose "level" matches and whose
// "text" contains the given substring, or nil if not found.
func findConsoleEntry(entries []map[string]interface{}, level, textContains string) map[string]interface{} {
	for _, e := range entries {
		if e["level"] == level && strings.Contains(e["text"].(string), textContains) {
			return e
		}
	}
	return nil
}

func parseConsoleEntries(t *testing.T, out string) []map[string]interface{} {
	t.Helper()
	var entries []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &entries))
	return entries
}

// TestConsoleCapture_BasicLog verifies a single console.log call is captured
// with level="log" and the correct text.
func TestConsoleCapture_BasicLog(t *testing.T) {
	server := setupConsoleServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "console", "capture", "start")
	runCLI(t, bin, port, "navigate", server.URL)
	time.Sleep(500 * time.Millisecond)

	out := runCLI(t, bin, port, "console", "capture", "stop")
	entries := parseConsoleEntries(t, out)

	entry := findConsoleEntry(entries, "log", "hello log")
	assert.NotNil(t, entry, "expected a log entry containing 'hello log'")
}

// TestConsoleCapture_AllLevels verifies all five standard console levels are
// captured with their normalized names (warning → warn).
func TestConsoleCapture_AllLevels(t *testing.T) {
	server := setupConsoleServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "console", "capture", "start")
	runCLI(t, bin, port, "navigate", server.URL)
	time.Sleep(500 * time.Millisecond)

	out := runCLI(t, bin, port, "console", "capture", "stop")
	entries := parseConsoleEntries(t, out)

	for _, want := range []struct{ level, text string }{
		{"log", "hello log"},
		{"warn", "hello warn"},   // CDP "warning" normalized to "warn"
		{"error", "hello error"},
		{"info", "hello info"},
		{"debug", "hello debug"},
	} {
		assert.NotNil(t, findConsoleEntry(entries, want.level, want.text),
			"missing entry: level=%s text=%s", want.level, want.text)
	}
}

// TestConsoleCapture_MultipleArgs verifies that console.log("alpha", "beta", 42)
// produces a text of "alpha beta 42" and an args array with three elements.
func TestConsoleCapture_MultipleArgs(t *testing.T) {
	server := setupConsoleServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "console", "capture", "start")
	runCLI(t, bin, port, "navigate", server.URL+"/multi")
	time.Sleep(500 * time.Millisecond)

	out := runCLI(t, bin, port, "console", "capture", "stop")
	entries := parseConsoleEntries(t, out)

	entry := findConsoleEntry(entries, "log", "alpha")
	require.NotNil(t, entry, "expected a log entry containing 'alpha'")

	assert.Contains(t, entry["text"], "beta")
	assert.Contains(t, entry["text"], "42")

	args, ok := entry["args"].([]interface{})
	require.True(t, ok, "args should be an array")
	assert.Equal(t, 3, len(args), "expected 3 args")

	first := args[0].(map[string]interface{})
	assert.Equal(t, "string", first["type"])
	assert.Equal(t, "alpha", first["value"])

	third := args[2].(map[string]interface{})
	assert.Equal(t, "number", third["type"])
}

// TestConsoleCapture_Events_HaveTabIndex verifies each captured entry carries
// an integer "tab" field.
func TestConsoleCapture_Events_HaveTabIndex(t *testing.T) {
	server := setupConsoleServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "console", "capture", "start")
	runCLI(t, bin, port, "navigate", server.URL)
	time.Sleep(500 * time.Millisecond)

	out := runCLI(t, bin, port, "console", "capture", "stop")
	entries := parseConsoleEntries(t, out)

	require.NotEmpty(t, entries)
	for _, e := range entries {
		_, hasTab := e["tab"]
		assert.True(t, hasTab, "entry missing 'tab' field: %v", e)
		// JSON numbers decode as float64
		tab, ok := e["tab"].(float64)
		assert.True(t, ok, "tab should be a number")
		assert.Equal(t, float64(0), tab)
	}
}

// TestConsoleCapture_TabFilter_Active verifies that --tab active only captures
// entries from the active tab, not from a second tab navigated afterwards.
func TestConsoleCapture_TabFilter_Active(t *testing.T) {
	server := setupConsoleServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// tab 0 is active; open tab 1
	runCLI(t, bin, port, "navigate", server.URL) // seed tab 0 URL
	runCLI(t, bin, port, "tab", "new")           // tab 1, now active

	// switch back to tab 0 and start capture locked to it
	runCLI(t, bin, port, "tab", "switch", "0")
	runCLI(t, bin, port, "console", "capture", "start", "--tab", "active")

	// navigate tab 0 → fires console calls
	runCLI(t, bin, port, "navigate", server.URL)
	// navigate tab 1 → also fires console calls, but should NOT be captured
	runCLI(t, bin, port, "tab", "switch", "1")
	runCLI(t, bin, port, "navigate", server.URL)
	time.Sleep(500 * time.Millisecond)

	out := runCLI(t, bin, port, "console", "capture", "stop")
	entries := parseConsoleEntries(t, out)

	require.NotEmpty(t, entries)
	for _, e := range entries {
		tab, _ := e["tab"].(float64)
		assert.Equal(t, float64(0), tab, "expected only tab 0 entries, got tab %v", tab)
	}
}

// TestConsoleCapture_LevelFilter verifies that --level error,warn excludes
// log/info/debug entries.
func TestConsoleCapture_LevelFilter(t *testing.T) {
	server := setupConsoleServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "console", "capture", "start", "--level", "error,warn")
	runCLI(t, bin, port, "navigate", server.URL)
	time.Sleep(500 * time.Millisecond)

	out := runCLI(t, bin, port, "console", "capture", "stop")
	entries := parseConsoleEntries(t, out)

	require.NotEmpty(t, entries)
	for _, e := range entries {
		level := e["level"].(string)
		assert.Contains(t, []string{"error", "warn"}, level,
			"unexpected level %q in filtered capture", level)
	}
}
