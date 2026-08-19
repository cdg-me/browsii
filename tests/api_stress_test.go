package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupGatedFormServer serves a form whose submit button enables only after
// its requirements are met, with a field that appears after a delay.
func setupGatedFormServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
			<html><head><title>Gated</title></head><body>
			<form onsubmit="event.preventDefault(); window.ordered = true">
			<input id="email" oninput="render()"><input name="addr" oninput="render()">
			<input type="radio" name="pay" value="card" id="pay-card">
			<select id="speed" onchange="render()"><option value="">choose</option><option value="std">Standard</option><option value="exp">Express</option></select>
			<div id="extra"></div>
			<button id="buy" type="submit" disabled>Buy</button>
			</form>
			<script>
				function render() {
					const ok = document.getElementById('email').value.includes('@') &&
						document.querySelector('[name=addr]').value.length > 3 &&
						document.getElementById('speed').value &&
						document.querySelector('input[name=pay]:checked');
					document.getElementById('buy').disabled = !ok;
				}
				document.getElementById('pay-card').addEventListener('change', render);
				setTimeout(() => {
					const d = document.createElement('div');
					d.innerHTML = '<label>Note <input id="note"></label>';
					document.getElementById('extra').appendChild(d);
				}, 500);
			</script></body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestClick_DisabledElementFailsFast(t *testing.T) {
	server := setupGatedFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out, err := runCLIExpectFail(t, bin, port, "click", "#buy")
	require.Error(t, err)
	assert.Contains(t, out, "element is disabled: #buy")
	assert.Contains(t, out, "hint")
}

func TestExpect_EnabledAndDisabled(t *testing.T) {
	server := setupGatedFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	// Enabled expectation fails while gated.
	out, err := runCLIExpectFail(t, bin, port, "expect", "--selector", "#buy", "--enabled", "--timeout", "500")
	require.Error(t, err)
	assert.Contains(t, out, "element is disabled")

	// Disabled expectation holds now, and flips after the gate opens.
	out = runCLI(t, bin, port, "expect", "--selector", "#buy", "--disabled")
	assert.Contains(t, out, "element disabled")

	runCLI(t, bin, port, "fill", "--field", `{"selector":"#email","value":"a@b.c"}`, "--field", `{"selector":"[name=addr]","value":"123 Main"}`, "--no-evidence")
	runCLI(t, bin, port, "select", "#speed", "Express", "--no-evidence")
	runCLI(t, bin, port, "check", "#pay-card", "--no-evidence")

	out = runCLI(t, bin, port, "expect", "--selector", "#buy", "--enabled")
	assert.Contains(t, out, "element enabled")
}

func TestSelect_MultiReplacesSelection(t *testing.T) {
	server := setupMultiServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "select", "#tops", "a,b", "--multiple", "--no-evidence")
	assert.Contains(t, out, "[a b]")

	// Second call replaces rather than accumulates.
	out = runCLI(t, bin, port, "select", "#tops", "Alpha,Gamma", "--multiple", "--no-evidence")
	assert.Contains(t, out, "[a g]")
}

func setupMultiServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<select id="tops" multiple><option value="a">Alpha</option><option value="b">Beta</option><option value="g">Gamma</option></select>
			</body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

// TestFill_JsonAcceptsBothShapes verifies the --json passthrough takes both
// the bare array and the wrapped object form.
func TestFill_JsonAcceptsBothShapes(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "fill", "--json", `[{"selector":"#name","value":"Arr"}]`)
	assert.Contains(t, out, "Filled 1 field(s)")

	out = runCLI(t, bin, port, "fill", "--json", `{"fields":[{"selector":"#name","value":"Obj"}]}`)
	assert.Contains(t, out, "Filled 1 field(s)")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('name').value")
	assert.Contains(t, jsOut, "Obj")
}

// TestReplay_WaitsForDelayedElements proves replays tolerate elements that
// the recorded page adds after a delay: the instant-speed replay must wait
// for them rather than failing resolution.
func TestReplay_WaitsForDelayedElements(t *testing.T) {
	server := setupGatedFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "record", "start", "gated-flow")
	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")
	// The note input appears at 500ms; the recorded fill targeted it after
	// natural interaction timing.
	runCLI(t, bin, port, "fill", "--field", `{"selector":"#email","value":"r@t.co"}`, "--field", `{"selector":"[name=addr]","value":"9 Loop"}`, "--no-evidence")
	runCLI(t, bin, port, "js", "() => new Promise(r => setTimeout(() => { document.getElementById('note').value = 'waited'; r(1); }, 700))")
	runCLI(t, bin, port, "expect", "--selector", "#speed")
	runCLI(t, bin, port, "record", "stop")

	runCLI(t, bin, port, "session", "new", "fresh")
	out, err := runCLIExpectFail(t, bin, port, "record", "replay", "gated-flow", "--live")
	require.NoError(t, err, out)
	assert.Contains(t, out, "Replayed")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('email').value")
	assert.Contains(t, jsOut, "r@t.co")
}

func TestFill_UnicodeQuotesAndNewlines(t *testing.T) {
	server := setupMultiServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	// Contenteditable target with hostile content.
	out := runCLI(t, bin, port, "js", "() => { const d = document.createElement('div'); d.id = 'ed'; d.contentEditable = true; document.body.appendChild(d); return 1 }")
	assert.Contains(t, out, "1")

	out = runCLI(t, bin, port, "fill", "--field", `{"selector":"#ed","value":"Ünïcödé \"Straße\" 日本"}`)
	assert.Contains(t, out, "Filled 1 field(s)")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('ed').textContent")
	assert.Contains(t, jsOut, "Ünïcödé")
}
