package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupShadowAppServer serves a custom-element app whose entire interface
// lives inside an open shadow root: plain querySelectorAll cannot see it.
func setupShadowAppServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
			<html><head><title>Shadow Tasks</title></head><body>
			<h1>Task manager</h1>
			<task-list id="list"></task-list>
			<script>
			const store = { items: [] };
			class TaskList extends HTMLElement {
				constructor() {
					super();
					const root = this.attachShadow({mode: 'open'});
					root.innerHTML =
						'<form id="add"><input id="title" placeholder="New task"><button type="submit">Add</button></form>' +
						'<ul id="items"></ul><p id="count"></p>';
					root.getElementById('add').addEventListener('submit', e => {
						e.preventDefault();
						const v = root.getElementById('title').value.trim();
						if (!v) return;
						store.items.push(v);
						fetch('/api/sync', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({title: v})});
						render();
						root.getElementById('title').value = '';
					});
				}
				connectedCallback() { render(); }
			}
			customElements.define('task-list', TaskList);
			function render() {
				const root = document.getElementById('list').shadowRoot;
				root.getElementById('items').innerHTML =
					store.items.map(t => '<li>' + t + '</li>').join('');
				root.getElementById('count').textContent = store.items.length + ' item(s)';
			}
			</script></body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

// setupFrameServer serves a host page with a same-origin iframe form.
func setupFrameServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/inner.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input id="user" placeholder="Username">
			<button id="fbtn" onclick="parent.document.getElementById('frame-out').textContent = 'frame clicked: ' + document.getElementById('user').value">Frame action</button>
			</body></html>`) //nolint:errcheck
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<h1>Host page</h1>
			<iframe id="pane" src="/inner.html" style="width:400px;height:150px"></iframe>
			<div id="frame-out"></div>
			</body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestShadow_ElementsEnumerateInsideShadowRoot(t *testing.T) {
	server := setupShadowAppServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "elements")
	assert.Contains(t, out, `textbox "New task" (#list >>> #title)`)
	assert.Contains(t, out, `button "Add" (#list >>> form > button)`)
}

func TestShadow_InteractionsPierceTheBoundary(t *testing.T) {
	server := setupShadowAppServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "fill", "--field", `{"selector":"#list >>> #title","value":"First task"}`, "--no-evidence")
	assert.Contains(t, out, "Filled 1 field(s)")

	out = runCLI(t, bin, port, "click", "#list >>> form > button")
	assert.Contains(t, out, "Successfully clicked")
	assert.Contains(t, out, "POST /api/sync (201)")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('list').shadowRoot.getElementById('count').textContent")
	assert.Contains(t, jsOut, "1 item(s)")
}

func TestShadow_ExpectSeesShadowText(t *testing.T) {
	server := setupShadowAppServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	runCLI(t, bin, port, "fill", "--field", `{"selector":"#list >>> #title","value":"Visible task"}`, "--no-evidence")
	runCLI(t, bin, port, "click", "#list >>> form > button", "--no-evidence")

	out := runCLI(t, bin, port, "expect", "--text", "1 item(s)")
	assert.Contains(t, out, "OK:")

	out = runCLI(t, bin, port, "find", "item(s)")
	assert.Contains(t, out, "1 match")
}

func TestShadow_ElementDetailAndRefHealing(t *testing.T) {
	server := setupShadowAppServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "element", "#list >>> #title")
	assert.Contains(t, out, `textbox "New task"`)
	assert.Contains(t, out, "#list >>> #title")

	elems := runCLI(t, bin, port, "elements")
	ref := refOfSelector(t, elems, "#list >>> #title")
	out = runCLI(t, bin, port, "element", strconv.Itoa(ref))
	assert.Contains(t, out, `textbox "New task"`)
}

func TestShadow_RecordAndReplayAcrossBoundary(t *testing.T) {
	server := setupShadowAppServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")
	runCLI(t, bin, port, "record", "start", "shadow-flow")
	runCLI(t, bin, port, "fill", "--field", `{"selector":"#list >>> #title","value":"Recorded task"}`, "--no-evidence")
	runCLI(t, bin, port, "click", "#list >>> form > button", "--no-evidence")
	runCLI(t, bin, port, "expect", "--text", "1 item(s)")
	runCLI(t, bin, port, "record", "stop")

	runCLI(t, bin, port, "session", "new", "fresh")
	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")
	out, err := runCLIExpectFail(t, bin, port, "record", "replay", "shadow-flow", "--live")
	require.NoError(t, err, out)
	assert.Contains(t, out, "Replayed 3 steps")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('list').shadowRoot.getElementById('count').textContent")
	assert.Contains(t, jsOut, "1 item(s)")
}

func TestFrame_ElementsAndActionsPierceIframe(t *testing.T) {
	server := setupFrameServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "elements")
	assert.Contains(t, out, `textbox "Username" (#pane >>> #user)`)
	assert.Contains(t, out, `button "Frame action" (#pane >>> #fbtn)`)

	runCLI(t, bin, port, "fill", "--field", `{"selector":"#pane >>> #user","value":"frameuser"}`, "--no-evidence")
	runCLI(t, bin, port, "click", "#pane >>> #fbtn", "--no-evidence")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('frame-out').textContent")
	assert.Contains(t, jsOut, "frame clicked: frameuser")

	out = runCLI(t, bin, port, "expect", "--text", "frame clicked: frameuser")
	assert.Contains(t, out, "OK:")
}
