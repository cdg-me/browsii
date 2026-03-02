package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContext_CreateIsolatesCookies(t *testing.T) {
	serverA := setupNamedServer("Main Page")
	defer serverA.Close()
	serverB := setupNamedServer("Incognito Page")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate in the default context and set a cookie
	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "js", "() => { document.cookie = 'session=main_ctx'; }")

	// Verify cookie is set
	jsOut1 := runCLI(t, bin, port, "js", "() => document.cookie")
	assert.Contains(t, jsOut1, "session=main_ctx")

	// Create a new incognito context and switch to it
	runCLI(t, bin, port, "context", "create", "--name", "incognito1")
	runCLI(t, bin, port, "navigate", serverB.URL)

	// Cookie should NOT exist in incognito context
	jsOut2 := runCLI(t, bin, port, "js", "() => document.cookie")
	assert.NotContains(t, jsOut2, "session=main_ctx",
		"Incognito context should not share cookies with default context")

	// Switch back to default context
	runCLI(t, bin, port, "context", "switch", "default")

	// Cookie should still exist in default context
	jsOut3 := runCLI(t, bin, port, "js", "() => document.cookie")
	assert.Contains(t, jsOut3, "session=main_ctx",
		"Default context should still have its cookies")
}
