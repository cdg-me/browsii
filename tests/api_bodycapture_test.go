package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// setupPostServer serves a page that POSTs JSON and a large text payload via
// fetch, so body-capture defaults can be asserted.
func setupPostServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/order", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
			<html><body>
				<button id="submit-small" onclick="fetch('/api/order', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({item:'widget', qty:3})})">Order</button>
				<button id="submit-large" onclick="fetch('/api/order', {method:'POST', headers:{'Content-Type':'text/plain'}, body: 'x'.repeat(9000)})">Big payload</button>
			</body></html>`)
	})
	return httptest.NewServer(mux)
}

// TestNetworkCapture_RequestBodyCapturedByDefault verifies small textual
// POST bodies appear in capture output with no --include flags.
func TestNetworkCapture_RequestBodyCapturedByDefault(t *testing.T) {
	server := setupPostServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start")
	runCLI(t, bin, port, "click", "#submit-small")
	out := runCLI(t, bin, port, "network", "capture", "stop")

	assert.Contains(t, out, "/api/order")
	assert.Contains(t, out, `"postData"`)
	assert.Contains(t, out, `widget`,
		"small JSON POST body must be captured without --include request-body")
}

// TestNetworkCapture_OversizedBodyOmittedByDefault verifies the size cap:
// >4KB bodies are excluded by default but still captured with the explicit
// include group.
func TestNetworkCapture_OversizedBodyOmittedByDefault(t *testing.T) {
	server := setupPostServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// Default: oversized body omitted.
	runCLI(t, bin, port, "network", "capture", "start")
	runCLI(t, bin, port, "click", "#submit-large")
	out := runCLI(t, bin, port, "network", "capture", "stop")
	assert.Contains(t, out, "/api/order")
	assert.NotContains(t, out, "postData", "oversized body must be omitted by default")

	// Opt-in: body present.
	runCLI(t, bin, port, "network", "capture", "start", "--include", "request-body")
	runCLI(t, bin, port, "click", "#submit-large")
	out = runCLI(t, bin, port, "network", "capture", "stop")
	assert.Contains(t, out, "postData")
	assert.True(t, strings.Contains(out, "postData") && len(out) > 9000,
		"opt-in capture must include the 9KB body")
}
