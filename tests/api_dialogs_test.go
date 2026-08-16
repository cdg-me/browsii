package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDialogServer serves a page whose buttons trigger the three dialog types.
func setupDialogServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
			<html><head><title>Dialogs</title></head>
			<body>
				<button id="btn-confirm" onclick="window.result = confirm('Are you sure?')">Confirm me</button>
				<button id="btn-alert" onclick="window.alerted = true; alert('an alert fired')">Alert me</button>
				<button id="btn-prompt" onclick="window.prompted = prompt('name?')">Prompt me</button>
			</body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestDialogs_DefaultPolicyDismissesConfirm(t *testing.T) {
	server := setupDialogServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// Default policy is dismiss: confirm() returns false.
	out := runCLI(t, bin, port, "click", "#btn-confirm")
	assert.Contains(t, out, "Successfully clicked")
	assert.Contains(t, out, `Dialog dismissed (confirm): "Are you sure?"`)

	jsOut := runCLI(t, bin, port, "js", "() => window.result")
	assert.Contains(t, jsOut, "false")
}

func TestDialogs_AcceptPolicyConfirms(t *testing.T) {
	server := setupDialogServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "dialogs", "--policy", "accept")

	out := runCLI(t, bin, port, "click", "#btn-confirm")
	assert.Contains(t, out, `Dialog accepted (confirm): "Are you sure?"`)

	jsOut := runCLI(t, bin, port, "js", "() => window.result")
	assert.Contains(t, jsOut, "true")
}

func TestDialogs_PromptTextTypedWhenAccepting(t *testing.T) {
	server := setupDialogServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "dialogs", "--policy", "accept", "--prompt-text", "Alice")

	runCLI(t, bin, port, "click", "#btn-prompt")

	jsOut := runCLI(t, bin, port, "js", "() => window.prompted")
	assert.Contains(t, jsOut, "Alice")
}

func TestDialogs_ListingShowsHistoryAndClears(t *testing.T) {
	server := setupDialogServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// Trigger an alert that lands after the js call's inline reporting window,
	// so it stays in the log for the 'dialogs' listing.
	runCLI(t, bin, port, "js", "() => { setTimeout(() => alert('listed alert'), 400); return 'ok' }")

	listed := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if out := runCLI(t, bin, port, "dialogs"); strings.Contains(out, "listed alert") {
			assert.Contains(t, out, "Dialog policy: dismiss")
			assert.Contains(t, out, `alert dismissed: "listed alert"`)
			listed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.True(t, listed, "delayed alert never appeared in the dialogs log")

	// Clear forgets the history.
	out := runCLI(t, bin, port, "dialogs", "--clear")
	assert.Contains(t, out, "No dialogs recorded.")
}

func TestDialogs_AlertDuringEvalDoesNotStall(t *testing.T) {
	server := setupDialogServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// alert() inside an eval would stall the page forever without the
	// auto-handler; the eval must return its value normally.
	out := runCLI(t, bin, port, "js", "() => { alert('from js'); return 42 }")
	assert.Contains(t, out, "42")
	assert.Contains(t, out, `{"dialogs":[{"type":"alert"`)

	// The page is still usable afterwards.
	jsOut := runCLI(t, bin, port, "js", "() => document.title")
	assert.Contains(t, jsOut, "Dialogs")
}

func TestDialogs_InvalidPolicyRejected(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	out, err := runCLIExpectFail(t, bin, port, "dialogs", "--policy", "explode")
	assert.Error(t, err)
	assert.Contains(t, out, "invalid policy")
}
