package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupExpectServer serves pages with delayed updates, fetch triggers, and
// console errors for expect/evidence assertions.
func setupExpectServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/order", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	})
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`)) //nolint:errcheck
	})
	mux.HandleFunc("/orders/123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body><h1>Order confirmed</h1></body></html>`) //nolint:errcheck
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
			<html><head><title>Expect Bed</title></head>
			<body>
				<div><button id="btn-slow" onclick="setTimeout(() => { const s = document.createElement('div'); s.id = 'saved-flag'; s.textContent = 'Saved ✓'; document.body.appendChild(s); }, 800)">Save</button></div>
				<div><button id="btn-nav" onclick="location.href = '/orders/123'">Go to order</button></div>
				<div><button id="btn-order" onclick="fetch('/api/order', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({item: 1})})">Place order</button></div>
				<div><button id="btn-quiet" onclick="document.getElementById('quiet-marker').textContent = 'done'">Quiet update</button></div>
				<div><button id="btn-error" onclick="setTimeout(() => { console.error('TypeError: x is not a function'); document.getElementById('quiet-marker').textContent = 'clicked'; }, 700)">Error during wait</button></div>
				<div><button id="btn-fast-error" onclick="setTimeout(() => console.error('RangeError: boom'), 150)">Fast error</button></div>
				<div><input id="email" value=""></div>
				<div id="spinner">Loading…</div>
				<div id="quiet-marker"></div>
				<script>
					fetch('/api/session');
				</script>
			</body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestExpect_TextAppearsAfterDelay(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "click", "#btn-slow", "--no-evidence")

	out := runCLI(t, bin, port, "expect", "--text", "Saved ✓")
	assert.Contains(t, out, "OK:")
	assert.Contains(t, out, `"Saved ✓" visible`)
}

func TestExpect_TextFuzzyDiffOnFailure(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out, err := runCLIExpectFail(t, bin, port, "expect", "--text", "Saved changes", "--timeout", "300")
	require.Error(t, err)
	assert.Contains(t, out, "expect failed")
	assert.Contains(t, out, "closest text")
	// "Save" (the button label) shares a token with the wanted text.
	assert.Contains(t, out, `"Save"`)
}

func TestExpect_UrlPatternAndFailure(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "click", "#btn-nav", "--no-evidence")

	out := runCLI(t, bin, port, "expect", "--url-pattern", "*/orders/*")
	assert.Contains(t, out, "url matches */orders/*")

	// Failure shows the actual URL.
	out, err := runCLIExpectFail(t, bin, port, "expect", "--url-pattern", "*/nothing/*", "--timeout", "200")
	require.Error(t, err)
	assert.Contains(t, out, "url is")
	assert.Contains(t, out, "/orders/123")
}

func TestExpect_SelectorVisibleAndHidden(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// Visible immediately.
	out := runCLI(t, bin, port, "expect", "--selector", "#spinner")
	assert.Contains(t, out, "element visible")

	// Hidden: spinner is removed 500ms after the quiet update.
	runCLI(t, bin, port, "js", "() => { const s = document.getElementById('spinner'); s.textContent = 'Loading…'; setTimeout(() => s.remove(), 500); return 'ok' }")
	out = runCLI(t, bin, port, "expect", "--selector", "#spinner", "--hidden", "--timeout", "3000")
	assert.Contains(t, out, "hidden")

	// Failure names the state.
	out, err := runCLIExpectFail(t, bin, port, "expect", "--selector", "#missing-thing")
	require.Error(t, err)
	assert.Contains(t, out, "element is missing")
}

func TestExpect_Value(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	runCLI(t, bin, port, "js", "() => { setTimeout(() => { document.getElementById('email').value = 'user@example.com'; }, 400); return 'ok' }")
	out := runCLI(t, bin, port, "expect", "--selector", "#email", "--value", "user@example.com", "--timeout", "3000")
	assert.Contains(t, out, `value = "user@example.com"`)

	out, err := runCLIExpectFail(t, bin, port, "expect", "--selector", "#email", "--value", "wrong", "--timeout", "200")
	require.Error(t, err)
	assert.Contains(t, out, `value of #email is "user@example.com"`)
}

func TestExpect_RequestFired(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "click", "#btn-order", "--no-evidence")

	out := runCLI(t, bin, port, "expect", "--request", "POST */api/order*", "--timeout", "3000")
	assert.Contains(t, out, "request matched POST */api/order*")
}

// TestExpect_RequestNotFiredListsWhatDid is the black-hole killer: a wrong
// expectation must show the requests that actually fired, not just fail.
func TestExpect_RequestNotFiredListsWhatDid(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out, err := runCLIExpectFail(t, bin, port, "expect", "--request", "POST */api/orders*", "--timeout", "500")
	require.Error(t, err, "wrong pattern must fail")
	assert.Contains(t, out, "requests that fired")
	assert.Contains(t, out, "/api/session")
}

func TestExpect_NoConsoleErrors(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// Clean wait passes.
	out := runCLI(t, bin, port, "expect", "--selector", "#quiet-marker", "--no-console-errors")
	assert.Contains(t, out, "OK:")

	// An error logged while waiting fails the combined assertion (the error
	// fires at 700ms, inside the expect wait window).
	runCLI(t, bin, port, "click", "#btn-error", "--no-evidence")
	out, err := runCLIExpectFail(t, bin, port, "expect", "--text", "clicked", "--no-console-errors", "--timeout", "4000")
	require.Error(t, err)
	assert.Contains(t, out, "console errors")
	assert.Contains(t, out, "TypeError: x is not a function")
}

func TestExpect_RejectsMultipleConditions(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	out, err := runCLIExpectFail(t, bin, port, "expect", "--text", "a", "--url-pattern", "*b*")
	require.Error(t, err)
	assert.Contains(t, out, "exactly one")
}
