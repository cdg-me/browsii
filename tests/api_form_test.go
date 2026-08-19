package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupReactFormServer serves a form whose inputs are driven by a minimal
// React-style value tracker: state updates only through input event
// listeners that read e.target.value. Proves fill's native-setter sequence
// triggers framework handlers.
func setupReactFormServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
			<html><head><title>React-ish</title></head><body>
			<div id="out"></div>
			<input id="name"><input id="email">
			<select id="size"><option value="s">Small</option><option value="m">Medium</option></select>
			<input type="checkbox" id="tos">
			<input type="radio" name="plan" value="pro" id="plan-pro">
			<input type="radio" name="plan" value="free" id="plan-free">
			<script>
				// Minimal controlled-input simulation: state changes only via
				// input events, rendered back to #out — like React re-render.
				const state = {name: '', email: ''};
				function render() {
					document.getElementById('out').textContent =
						'name=' + state.name + ';email=' + state.email;
				}
				for (const id of ['name', 'email']) {
					const el = document.getElementById(id);
					el.addEventListener('input', e => { state[id] = e.target.value; render(); });
				}
				render();
			</script>
			</body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestFill_SetsFieldsAndFiresInput(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "fill",
		"--field", `{"selector":"#name","value":"Ada"}`,
		"--field", `{"selector":"#email","value":"a@x.com"}`)
	assert.Contains(t, out, "Filled 2 field(s)")

	// The framework handler observed the changes.
	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('out').textContent")
	assert.Contains(t, jsOut, "name=Ada")
	assert.Contains(t, jsOut, "email=a@x.com")
}

func TestFill_PartialFailureReportsCandidates(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "fill",
		"--field", `{"selector":"#nama","value":"Ada"}`,
		"--field", `{"selector":"#email","value":"a@x.com"}`)
	assert.Contains(t, out, "Filled 1 field(s)")
	assert.Contains(t, out, "failed")
	assert.Contains(t, out, "#nama")

	// The valid field still applied.
	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('out').textContent")
	assert.Contains(t, jsOut, "email=a@x.com")
}

func TestFill_RejectsCheckboxWithHint(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "fill", "--field", `{"selector":"#tos","value":"x"}`)
	assert.Contains(t, out, "failed")
	assert.Contains(t, out, "checkboxes/radios need check")
}

func TestFill_ByRefWorks(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	elems := runCLI(t, bin, port, "elements")
	ref := refOfSelector(t, elems, "#name")

	out := runCLI(t, bin, port, "fill", "--field", fmt.Sprintf(`{"ref":%d,"value":"ByRef"}`, ref))
	assert.Contains(t, out, "Filled 1 field(s)")
}

func TestFill_SubmitSkippedOnFailure(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "fill",
		"--field", `{"selector":"#name","value":"Ada"}`,
		"--field", `{"selector":"#missing","value":"x"}`,
		"--submit")
	assert.Contains(t, out, "Filled 1 field(s)")
	assert.Contains(t, out, "failed")
}

func TestSelect_ValueThenLabelPrecedence(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	// By value.
	out := runCLI(t, bin, port, "select", "#size", "m", "--no-evidence")
	assert.Contains(t, out, "Selected: [m]")

	// By label (no option has value "Medium").
	out = runCLI(t, bin, port, "select", "#size", "Medium", "--no-evidence")
	assert.Contains(t, out, "Selected: [m]")

	// Miss lists the options.
	out, err := runCLIExpectFail(t, bin, port, "select", "#size", "Large")
	require.Error(t, err)
	assert.Contains(t, out, "no matching option")
	assert.Contains(t, out, `"Medium" (value m)`)
}

func TestSelect_FiresChangeEvents(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	runCLI(t, bin, port, "js", "() => { window.events = []; const s = document.getElementById('size'); s.addEventListener('change', () => window.events.push('change')); s.addEventListener('input', () => window.events.push('input')); return 'ok' }")
	runCLI(t, bin, port, "select", "#size", "s", "--no-evidence")

	jsOut := runCLI(t, bin, port, "js", "() => JSON.stringify(window.events)")
	assert.Contains(t, jsOut, `input`)
	assert.Contains(t, jsOut, `change`)
}

func TestCheck_CheckboxRadioStateTransitions(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "check", "#tos")
	assert.Contains(t, out, "false → true")

	// Idempotent.
	out = runCLI(t, bin, port, "check", "#tos")
	assert.Contains(t, out, "true → true")

	out = runCLI(t, bin, port, "check", "#tos", "--off")
	assert.Contains(t, out, "true → false")

	// Radio: checking one unchecks the sibling.
	runCLI(t, bin, port, "check", "#plan-pro", "--no-evidence")
	out = runCLI(t, bin, port, "check", "#plan-free", "--no-evidence")
	assert.Contains(t, out, "false → true")
	jsOut := runCLI(t, bin, port, "js", "() => JSON.stringify({pro: document.getElementById('plan-pro').checked, free: document.getElementById('plan-free').checked})")
	assert.Contains(t, jsOut, `pro`)
	assert.Contains(t, jsOut, `false`)
	assert.Contains(t, jsOut, `free`)
	assert.Contains(t, jsOut, `true`)
}

func TestCheck_RejectsNonCheckable(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out, err := runCLIExpectFail(t, bin, port, "check", "#name")
	require.Error(t, err)
	assert.Contains(t, out, "not a checkbox or radio")
}

func TestFill_RecordsAndReplays(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "record", "start", "form-flow")
	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")
	runCLI(t, bin, port, "fill", "--field", `{"selector":"#name","value":"Recorded"}`, "--no-evidence")
	runCLI(t, bin, port, "select", "#size", "Medium", "--no-evidence")
	runCLI(t, bin, port, "check", "#tos", "--no-evidence")
	runCLI(t, bin, port, "record", "stop")

	runCLI(t, bin, port, "session", "new", "fresh")
	out, err := runCLIExpectFail(t, bin, port, "record", "replay", "form-flow", "--live")
	require.NoError(t, err, out)
	assert.Contains(t, out, "Replayed")

	jsOut := runCLI(t, bin, port, "js", "() => JSON.stringify({name: document.getElementById('name').value, size: document.getElementById('size').value, tos: document.getElementById('tos').checked})")
	assert.Contains(t, jsOut, "Recorded")
	assert.Contains(t, jsOut, "m")
	assert.Contains(t, jsOut, "true")
}
