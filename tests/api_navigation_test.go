package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNavigate_SetsPageURL(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	out := runCLI(t, bin, port, "navigate", server.URL)
	assert.Contains(t, out, "Successfully navigated")

	// Verify via JS that the page is actually at the right URL
	jsOut := runCLI(t, bin, port, "js", "() => window.location.href")
	assert.Contains(t, jsOut, server.URL)
}

func TestScrape_ReturnsHTML(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "scrape")
	assert.Contains(t, out, "Browser CLI Test Bed")
	assert.Contains(t, out, "Click Me")
	assert.Contains(t, out, "inputBox")
}

func TestScrape_FormatText(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "scrape", "--format", "text")
	assert.Contains(t, out, "Browser CLI Test Bed")
	assert.Contains(t, out, "Click Me")
	// Text format should NOT contain HTML tags
	assert.NotContains(t, out, "<html")
	assert.NotContains(t, out, "<div")
}

func TestScrape_FormatMarkdown(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "scrape", "--format", "markdown")
	// Markdown should contain heading syntax
	assert.Contains(t, out, "# Browser CLI Test Bed")
	// Should NOT contain HTML tags
	assert.NotContains(t, out, "<html")
}

func TestReload_ResetsPageState(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// Set a JS variable that only exists in memory
	runCLI(t, bin, port, "js", "() => { window.__testFlag = 'before_reload'; return window.__testFlag; }")

	// Verify it's set
	out1 := runCLI(t, bin, port, "js", "() => window.__testFlag || 'undefined'")
	assert.Contains(t, out1, "before_reload")

	// Reload the page
	runCLI(t, bin, port, "reload")

	// The variable should be gone after reload
	out2 := runCLI(t, bin, port, "js", "() => window.__testFlag || 'undefined'")
	assert.Contains(t, out2, "undefined", "JS state should be wiped after reload")
	assert.NotContains(t, out2, "before_reload", "Old JS state should not survive reload")
}

func TestBackForward_NavigatesHistory(t *testing.T) {
	pageA := setupNamedServer("Page A")
	defer pageA.Close()
	pageB := setupNamedServer("Page B")
	defer pageB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate to A, then B
	runCLI(t, bin, port, "navigate", pageA.URL)
	jsOut1 := runCLI(t, bin, port, "js", "() => document.title")
	assert.Contains(t, jsOut1, "Page A")

	runCLI(t, bin, port, "navigate", pageB.URL)
	jsOut2 := runCLI(t, bin, port, "js", "() => document.title")
	assert.Contains(t, jsOut2, "Page B")

	// Go back — should be on Page A
	runCLI(t, bin, port, "back")
	jsOut3 := runCLI(t, bin, port, "js", "() => document.title")
	assert.Contains(t, jsOut3, "Page A", "After 'back', should be on Page A")

	// Go forward — should be on Page B
	runCLI(t, bin, port, "forward")
	jsOut4 := runCLI(t, bin, port, "js", "() => document.title")
	assert.Contains(t, jsOut4, "Page B", "After 'forward', should be on Page B")
}

func TestScroll_Down(t *testing.T) {
	// Create a tall page to scroll
	tallServer := setupTallServer()
	defer tallServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", tallServer.URL)

	// Scroll down 500 pixels
	runCLI(t, bin, port, "scroll", "--down", "--pixels", "500")

	jsOut := runCLI(t, bin, port, "js", "() => window.scrollY")
	assert.Contains(t, jsOut, "500", "scrollY should be 500 after scrolling down 500px")
}

func TestScroll_Bottom(t *testing.T) {
	tallServer := setupTallServer()
	defer tallServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", tallServer.URL)

	// Scroll to bottom
	runCLI(t, bin, port, "scroll", "--bottom")

	jsOut := runCLI(t, bin, port, "js", "() => window.scrollY > 0")
	assert.Contains(t, jsOut, "true", "scrollY should be > 0 after scrolling to bottom")

	// Scroll back to top
	runCLI(t, bin, port, "scroll", "--top")

	jsOut2 := runCLI(t, bin, port, "js", "() => window.scrollY")
	assert.Contains(t, jsOut2, "0", "scrollY should be 0 after scrolling to top")
}
