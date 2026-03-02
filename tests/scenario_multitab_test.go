package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiTabJSEval verifies that after opening a new tab via the daemon,
// JS eval runs on the newly opened tab (not the original one).
// This is the regression test for the "all tabs scrape the same page" bug.
func TestMultiTabJSEval(t *testing.T) {
	pageA := setupNamedServer("Page A")
	defer pageA.Close()
	pageB := setupNamedServer("Page B")
	defer pageB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate to Page A
	runCLI(t, bin, port, "navigate", pageA.URL)

	jsOut1 := runCLI(t, bin, port, "js", "() => document.getElementById('identity').innerText")
	assert.Contains(t, jsOut1, "I am Page A", "JS should run on Page A initially")

	// Open tab to Page B
	runCLI(t, bin, port, "tab", "new", pageB.URL)

	// JS should now run on Page B
	jsOut2 := runCLI(t, bin, port, "js", "() => document.getElementById('identity').innerText")
	assert.Contains(t, jsOut2, "I am Page B",
		"After 'tab new', JS eval should target the NEW tab (Page B)")

	// Find Page A's index and switch back
	listOut := runCLI(t, bin, port, "tab", "list")
	pageAIdx := findTabIndex(t, listOut, pageA.Listener.Addr().String())
	require.NotEqual(t, -1, pageAIdx, "Could not find Page A in tab list")

	runCLI(t, bin, port, "tab", "switch", itoa(pageAIdx))

	jsOut3 := runCLI(t, bin, port, "js", "() => document.getElementById('identity').innerText")
	assert.Contains(t, jsOut3, "I am Page A",
		"After switching to Page A's tab, JS eval should target Page A")
}
