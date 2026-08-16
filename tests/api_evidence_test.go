package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidence_ClickReportsNavigation(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "click", "#btn-nav")
	assert.Contains(t, out, "Successfully clicked")
	assert.Contains(t, out, "→ navigated to")
	assert.Contains(t, out, "/orders/123")
}

func TestEvidence_ClickReportsRequests(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "click", "#btn-order")
	assert.Contains(t, out, "⟳ 1 request(s): POST /api/order (201)")
}

func TestEvidence_ClickReportsConsoleErrors(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	// btn-fast-error logs within the 400ms evidence settle window.
	out := runCLI(t, bin, port, "click", "#btn-fast-error")
	assert.Contains(t, out, "⚠ 1 console error(s)")
}

func TestEvidence_NoEvidenceFlagSkipsReceipt(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "click", "#btn-order", "--no-evidence")
	assert.Contains(t, out, "Successfully clicked")
	assert.NotContains(t, out, "⟳")
	assert.NotContains(t, out, "→ navigated")
}

func TestEvidence_NavigateReportsLoadActivity(t *testing.T) {
	server := setupExpectServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	out := runCLI(t, bin, port, "navigate", server.URL)
	assert.Contains(t, out, "Successfully navigated")
	assert.Contains(t, out, "⟳")
	assert.Contains(t, out, "/api/session")
}

// TestEvidence_ClickSubmitFullReceipt is the end-to-end verify loop: act,
// get the receipt, then independently assert with expect.
func TestEvidence_ClickSubmitFullReceipt(t *testing.T) {
	server := setupPostServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "click", "#submit-small")
	require.Contains(t, out, "Successfully clicked")
	assert.Contains(t, out, "⟳ 1 request(s): POST /api/order (201)")

	out = runCLI(t, bin, port, "expect", "--request", "POST */api/order*")
	assert.Contains(t, out, "OK: request matched")
}
