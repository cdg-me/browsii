package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJS_ReturnsEvalResult(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "js", "() => document.getElementById('header').innerText")
	assert.Contains(t, out, "Browser CLI Test Bed")
}

func TestJS_ReturnsComputedValue(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "js", "() => 2 + 2")
	assert.Contains(t, out, "4")
}

func TestJS_ReturnsJSON(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "js", "() => ({title: document.title, hasInput: !!document.getElementById('inputBox')})")
	assert.Contains(t, out, "Test Bed")
	assert.Contains(t, out, "true")
}

// --- bare expression tests (no wrapping required from caller) ---

func TestJS_BareExpression_DOMQuery(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "js", "document.getElementById('header').innerText")
	assert.Contains(t, out, "Browser CLI Test Bed")
}

func TestJS_BareExpression_Arithmetic(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "js", "2 + 2")
	assert.Contains(t, out, "4")
}

func TestJS_BareExpression_ObjectLiteral(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "js", "({title: document.title, checked: true})")
	assert.Contains(t, out, "Test Bed")
	assert.Contains(t, out, "true")
}

func TestJS_FunctionKeyword_StillWorks(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "js", "function() { return 42; }")
	assert.Contains(t, out, "42")
}
