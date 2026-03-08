package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPress_SubmitsForm(t *testing.T) {
	// Create a page with a form that sets a flag on submit
	formServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
			<form id="myform" onsubmit="event.preventDefault(); document.getElementById('result').innerText = 'submitted';">
				<input type="text" id="input1" autofocus>
				<div id="result">waiting</div>
			</form>
		</body></html>`))
	}))
	defer formServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", formServer.URL)
	runCLI(t, bin, port, "click", "#input1")
	runCLI(t, bin, port, "press", "Enter")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('result').innerText")
	assert.Contains(t, jsOut, "submitted", "Pressing Enter should submit the form")
}

func TestPress_KeyCombo(t *testing.T) {
	// Create a page that listens for Escape key
	escServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
			<div id="result">waiting</div>
			<script>
				document.addEventListener('keydown', function(e) {
					if (e.key === 'Escape') {
						document.getElementById('result').innerText = 'escaped';
					}
				});
			</script>
		</body></html>`))
	}))
	defer escServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", escServer.URL)
	runCLI(t, bin, port, "press", "Escape")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('result').innerText")
	assert.Contains(t, jsOut, "escaped", "Escape key should trigger keydown listener")
}

func TestHover_TriggersEvent(t *testing.T) {
	// Create a page that changes text on hover
	hoverServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
			<div id="hoverTarget" style="width:100px;height:100px;background:blue;">Hover me</div>
			<div id="result">waiting</div>
			<script>
				document.getElementById('hoverTarget').addEventListener('mouseenter', function() {
					document.getElementById('result').innerText = 'hovered';
				});
			</script>
		</body></html>`))
	}))
	defer hoverServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", hoverServer.URL)
	runCLI(t, bin, port, "hover", "#hoverTarget")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('result').innerText")
	assert.Contains(t, jsOut, "hovered", "Hovering should trigger mouseenter event")
}
