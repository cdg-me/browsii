package tests

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTabNew_CreatesTab(t *testing.T) {
	serverA := setupNamedServer("Page A")
	defer serverA.Close()
	serverB := setupNamedServer("Page B")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "tab", "new", serverB.URL)

	// Tab list should show 2 tabs
	listOut := runCLI(t, bin, port, "tab", "list")
	assert.Contains(t, listOut, "Page A")
	assert.Contains(t, listOut, "Page B")
}

func TestTabNew_AutoSwitchesActivePage(t *testing.T) {
	serverA := setupNamedServer("Page A")
	defer serverA.Close()
	serverB := setupNamedServer("Page B")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)

	// Verify we're on Page A
	jsOut1 := runCLI(t, bin, port, "js", "() => document.getElementById('identity').innerText")
	assert.Contains(t, jsOut1, "I am Page A")

	// Open Page B — should auto-switch
	runCLI(t, bin, port, "tab", "new", serverB.URL)

	jsOut2 := runCLI(t, bin, port, "js", "() => document.getElementById('identity').innerText")
	assert.Contains(t, jsOut2, "I am Page B",
		"After tab new, JS eval should target the new tab")
}

func TestTabList_ReturnsAllTabs(t *testing.T) {
	serverA := setupNamedServer("Alpha")
	defer serverA.Close()
	serverB := setupNamedServer("Beta")
	defer serverB.Close()
	serverC := setupNamedServer("Gamma")
	defer serverC.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "tab", "new", serverB.URL)
	runCLI(t, bin, port, "tab", "new", serverC.URL)

	listOut := runCLI(t, bin, port, "tab", "list")
	assert.Contains(t, listOut, "Alpha")
	assert.Contains(t, listOut, "Beta")
	assert.Contains(t, listOut, "Gamma")
}

func TestTabSwitch_ChangesActivePage(t *testing.T) {
	serverA := setupNamedServer("Page A")
	defer serverA.Close()
	serverB := setupNamedServer("Page B")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "tab", "new", serverB.URL)

	// Currently on Page B (auto-switched). Find Page A's index via tab list.
	listOut := runCLI(t, bin, port, "tab", "list")

	// Parse the tab list to find Page A's index
	// The CLI prints human-readable format, try to find the index from raw API
	// Use JS to get the current URL first to confirm we're on B
	jsOutB := runCLI(t, bin, port, "js", "() => document.getElementById('identity').innerText")
	assert.Contains(t, jsOutB, "I am Page B")

	// Find Page A index by trying each tab
	// We know there are 2 tabs. If current is B, the other is A.
	pageAIdx := findTabIndex(t, listOut, serverA.Listener.Addr().String())
	require.NotEqual(t, -1, pageAIdx, "Could not find Page A in tab list")

	runCLI(t, bin, port, "tab", "switch", itoa(pageAIdx))

	jsOutA := runCLI(t, bin, port, "js", "() => document.getElementById('identity').innerText")
	assert.Contains(t, jsOutA, "I am Page A",
		"After tab switch, JS should target Page A")
}

// findTabIndex parses tab list output to find a tab whose URL contains the given substring.
// Returns -1 if not found.
func findTabIndex(t *testing.T, listOutput string, urlSubstring string) int {
	t.Helper()
	// Try JSON parse first (raw API response)
	var tabs []struct {
		Index int    `json:"index"`
		URL   string `json:"url"`
	}
	if err := json.Unmarshal([]byte(listOutput), &tabs); err == nil {
		for _, tab := range tabs {
			if contains(tab.URL, urlSubstring) {
				return tab.Index
			}
		}
	}
	// Parse human-readable CLI format: "  [N] Title (URL)"
	return tabIndexFor(parseTabList(listOutput), urlSubstring)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchString(s, substr)))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

// parseTabList parses "  [N] Title (URL)" lines from tab list output.
// Returns a slice of (index, url) pairs in the order they appear.
func parseTabList(output string) []struct{ Index int; URL string } {
	var result []struct{ Index int; URL string }
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		closeBracket := strings.Index(line, "]")
		if closeBracket == -1 {
			continue
		}
		idx, err := strconv.Atoi(line[1:closeBracket])
		if err != nil {
			continue
		}
		openParen := strings.LastIndex(line, "(")
		closeParen := strings.LastIndex(line, ")")
		if openParen == -1 || closeParen <= openParen {
			continue
		}
		url := line[openParen+1 : closeParen]
		result = append(result, struct{ Index int; URL string }{idx, url})
	}
	return result
}

// tabIndexFor returns the index of the tab whose URL contains addr, or -1.
func tabIndexFor(tabs []struct{ Index int; URL string }, addr string) int {
	for _, tab := range tabs {
		if strings.Contains(tab.URL, addr) {
			return tab.Index
		}
	}
	return -1
}

// TestTabList_NewTabAppendsToEnd asserts that tabs are listed in creation order:
// the first-navigated tab is index 0, each subsequent tab new increments the index.
func TestTabList_NewTabAppendsToEnd(t *testing.T) {
	serverA := setupNamedServer("Page A")
	defer serverA.Close()
	serverB := setupNamedServer("Page B")
	defer serverB.Close()
	serverC := setupNamedServer("Page C")
	defer serverC.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "tab", "new", serverB.URL)
	runCLI(t, bin, port, "tab", "new", serverC.URL)

	listOut := runCLI(t, bin, port, "tab", "list")
	tabs := parseTabList(listOut)

	require.Len(t, tabs, 3, "Expected 3 tabs in list output:\n%s", listOut)

	idxA := tabIndexFor(tabs, serverA.Listener.Addr().String())
	idxB := tabIndexFor(tabs, serverB.Listener.Addr().String())
	idxC := tabIndexFor(tabs, serverC.Listener.Addr().String())

	require.NotEqual(t, -1, idxA, "Page A not found in tab list")
	require.NotEqual(t, -1, idxB, "Page B not found in tab list")
	require.NotEqual(t, -1, idxC, "Page C not found in tab list")

	assert.Equal(t, 0, idxA, "Page A (first tab) should be index 0")
	assert.Equal(t, 1, idxB, "Page B (second tab) should be index 1")
	assert.Equal(t, 2, idxC, "Page C (third tab) should be index 2")
}

func TestTabClose_RemovesTab(t *testing.T) {
	serverA := setupNamedServer("Alpha")
	defer serverA.Close()
	serverB := setupNamedServer("Beta")
	defer serverB.Close()
	serverC := setupNamedServer("Gamma")
	defer serverC.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "tab", "new", serverB.URL)
	runCLI(t, bin, port, "tab", "new", serverC.URL)

	// Verify 3 tabs
	listOut1 := runCLI(t, bin, port, "tab", "list")
	assert.Contains(t, listOut1, "Alpha")
	assert.Contains(t, listOut1, "Beta")
	assert.Contains(t, listOut1, "Gamma")

	// Close the active tab (Gamma, the most recently opened)
	runCLI(t, bin, port, "tab", "close")

	// Should have 2 tabs, Gamma gone
	listOut2 := runCLI(t, bin, port, "tab", "list")
	assert.Contains(t, listOut2, "Alpha")
	assert.Contains(t, listOut2, "Beta")
	assert.NotContains(t, listOut2, "Gamma", "Closed tab should be removed from list")
}
