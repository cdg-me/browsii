package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTwoPageServer serves distinct pages at /a and /b.
func setupTwoPageServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		who := "A"
		if r.URL.Path == "/b" {
			who = "B"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!DOCTYPE html><html><body>
			<h1 id="who">page %s</h1>
			<input id="field-%[1]s" value="">
			<button id="btn-%[1]s" onclick="window.done = 'clicked-%[1]s'">go %[1]s</button>
			</body></html>`, who) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestTabScope_OperationsWithoutSwitching(t *testing.T) {
	server := setupTwoPageServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL+"/a", "--no-evidence")
	runCLI(t, bin, port, "tab", "new", server.URL+"/b")

	// Tab 1 is active. Everything below operates on tab 0 via --tab.

	jsOut := runCLI(t, bin, port, "--tab", "0", "js", "() => document.getElementById('who').textContent")
	assert.Contains(t, jsOut, "page A")

	out := runCLI(t, bin, port, "--tab", "0", "fill", "--field", `{"selector":"#field-A","value":"scoped"}`, "--no-evidence")
	assert.Contains(t, out, "Filled 1 field(s)")

	out = runCLI(t, bin, port, "--tab", "0", "click", "#btn-A", "--no-evidence")
	assert.Contains(t, out, "Successfully clicked")

	jsOut = runCLI(t, bin, port, "--tab", "0", "js", "() => JSON.stringify({done: window.done, field: document.getElementById('field-A').value})")
	assert.Contains(t, jsOut, "clicked-A")
	assert.Contains(t, jsOut, "scoped")

	// The active tab (1) is untouched.
	jsOut = runCLI(t, bin, port, "js", "() => document.getElementById('who').textContent")
	assert.Contains(t, jsOut, "page B")
	jsOut = runCLI(t, bin, port, "js", "() => window.done || 'none'")
	assert.Contains(t, jsOut, "none")
}

func TestTabScope_ConcurrentAgentsDoNotInterfere(t *testing.T) {
	server := setupTwoPageServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL+"/a", "--no-evidence")
	runCLI(t, bin, port, "tab", "new", server.URL+"/b")

	// Two scoped "agents" in interleaved operations.
	runCLI(t, bin, port, "--tab", "0", "fill", "--field", `{"selector":"#field-A","value":"agent1"}`, "--no-evidence")
	runCLI(t, bin, port, "--tab", "1", "fill", "--field", `{"selector":"#field-B","value":"agent2"}`, "--no-evidence")
	runCLI(t, bin, port, "--tab", "0", "click", "#btn-A", "--no-evidence")
	runCLI(t, bin, port, "--tab", "1", "click", "#btn-B", "--no-evidence")

	jsOut := runCLI(t, bin, port, "--tab", "0", "js", "() => JSON.stringify({done: window.done, field: document.getElementById('field-A').value})")
	assert.Contains(t, jsOut, "clicked-A")
	assert.Contains(t, jsOut, "agent1")

	jsOut = runCLI(t, bin, port, "--tab", "1", "js", "() => JSON.stringify({done: window.done, field: document.getElementById('field-B').value})")
	assert.Contains(t, jsOut, "clicked-B")
	assert.Contains(t, jsOut, "agent2")
}

func TestTabScope_ExpectAndElementScoped(t *testing.T) {
	server := setupTwoPageServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL+"/a", "--no-evidence")
	runCLI(t, bin, port, "tab", "new", server.URL+"/b")

	out := runCLI(t, bin, port, "--tab", "0", "expect", "--text", "page A")
	assert.Contains(t, out, "OK:")

	out = runCLI(t, bin, port, "--tab", "0", "element", "#btn-A")
	assert.Contains(t, out, `go A`)

	// Elements enumerate per scoped tab.
	out = runCLI(t, bin, port, "--tab", "1", "elements", "--filter", "go B")
	assert.Contains(t, out, "go B")
}

func TestTabScope_InvalidTabErrors(t *testing.T) {
	server := setupTwoPageServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL+"/a", "--no-evidence")

	out, err := runCLIExpectFail(t, bin, port, "--tab", "5", "js", "() => 1")
	require.Error(t, err)
	assert.Contains(t, out, "tab 5 does not exist")
	assert.Contains(t, out, "tab list")
}

func TestTabScope_CloseScopedTab(t *testing.T) {
	server := setupTwoPageServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL+"/a", "--no-evidence")
	runCLI(t, bin, port, "tab", "new", server.URL+"/b", "--background")

	out := runCLI(t, bin, port, "--tab", "0", "tab", "close")
	assert.Contains(t, out, "Successfully closed tab 0")

	out = runCLI(t, bin, port, "tab", "list")
	assert.Contains(t, out, "/b")
	assert.NotContains(t, out, "/a\n")
}
