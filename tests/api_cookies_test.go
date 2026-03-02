package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCookies_ReturnsCookies(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// Set a cookie via JS
	runCLI(t, bin, port, "js", `() => document.cookie = "testcookie=abc123; path=/"`)

	out := runCLI(t, bin, port, "cookies")
	assert.Contains(t, out, "testcookie")
	assert.Contains(t, out, "abc123")
}

func TestCookies_RespectsActivePage(t *testing.T) {
	serverA := setupNamedServer("Page A")
	defer serverA.Close()
	serverB := setupNamedServer("Page B")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate to A, set a cookie
	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "js", `() => document.cookie = "from_a=yes; path=/"`)

	// Open B, set a different cookie
	runCLI(t, bin, port, "tab", "new", serverB.URL)
	runCLI(t, bin, port, "js", `() => document.cookie = "from_b=yes; path=/"`)

	// Cookies should be from page B (the active page)
	out := runCLI(t, bin, port, "cookies")
	assert.Contains(t, out, "from_b")
}
