package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClick_ChangesElement(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "click", "#target-box")
	assert.Contains(t, out, "Successfully clicked")

	// Verify the click handler fired via JS
	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('target-box').innerText")
	assert.Contains(t, jsOut, "Clicked!")
}

func TestClick_ScrollsToDistantElement(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// Click an element at CSS position (2000, 2000) — requires auto-scroll
	out := runCLI(t, bin, port, "click", "#far-box")
	assert.Contains(t, out, "Successfully clicked")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('far-box').innerText")
	assert.Contains(t, jsOut, "Far Clicked!")

	// Confirm the page actually scrolled to reach it
	scrollOut := runCLI(t, bin, port, "js", "() => ({x: window.scrollX, y: window.scrollY})")
	assert.Contains(t, scrollOut, "x")
	// scrollX and scrollY should be well above 0 to reach pos (2000, 2000)
	assert.NotContains(t, scrollOut, `"x":0`, "scrollX should be non-zero after clicking far element")
	assert.NotContains(t, scrollOut, `"y":0`, "scrollY should be non-zero after clicking far element")
}

func TestType_SetsInputValue(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "type", "#inputBox", "hello automated agent")
	assert.Contains(t, out, "Successfully typed")

	// Verify the input value via JS
	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('inputBox').value")
	assert.Contains(t, jsOut, "hello automated agent")
}

func TestType_ClearsExistingText(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// Type something first
	runCLI(t, bin, port, "type", "#inputBox", "first value")
	// Type something else — should replace, not append
	runCLI(t, bin, port, "type", "#inputBox", "second value")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('inputBox').value")
	assert.Contains(t, jsOut, "second value")
	assert.NotContains(t, jsOut, "first value")
}
