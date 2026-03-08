package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMouseMove_UpdatesPosition(t *testing.T) {
	// Create a page with mousemove listener that records coordinates
	moveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<div id="coords">0,0</div>
			<script>
				document.addEventListener('mousemove', function(e) {
					document.getElementById('coords').innerText = e.clientX + ',' + e.clientY;
				});
			</script>
		</body></html>`)
	}))
	defer moveServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", moveServer.URL)
	runCLI(t, bin, port, "mouse", "move", "150", "200")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('coords').innerText")
	assert.Contains(t, jsOut, "150,200", "Mouse should have moved to (150, 200)")
}

func TestMouseDrag_RecordsPath(t *testing.T) {
	// Create a canvas-like page that records mousedown, mousemove, mouseup events
	dragServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<div id="canvas" style="width:500px;height:500px;background:#eee;"></div>
			<div id="events">0</div>
			<div id="sequence"></div>
			<script>
				var eventCount = 0;
				var seq = [];
				var canvas = document.getElementById('canvas');
				canvas.addEventListener('mousedown', function(e) {
					eventCount++; seq.push('down');
					document.getElementById('events').innerText = eventCount;
					document.getElementById('sequence').innerText = seq.join(',');
				});
				canvas.addEventListener('mousemove', function(e) {
					if (seq.includes('down') && !seq.includes('up')) { eventCount++; }
					document.getElementById('events').innerText = eventCount;
				});
				canvas.addEventListener('mouseup', function(e) {
					eventCount++; seq.push('up');
					document.getElementById('events').innerText = eventCount;
					document.getElementById('sequence').innerText = seq.join(',');
				});
			</script>
		</body></html>`)
	}))
	defer dragServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", dragServer.URL)

	// Drag from (50,50) to (200,200) with 5 steps
	runCLI(t, bin, port, "mouse", "drag", "50", "50", "200", "200", "--steps", "5")

	seqOut := runCLI(t, bin, port, "js", "() => document.getElementById('sequence').innerText")
	assert.Contains(t, seqOut, "down", "Should have mousedown event")
	assert.Contains(t, seqOut, "up", "Should have mouseup event")

	eventsOut := runCLI(t, bin, port, "js", "() => parseInt(document.getElementById('events').innerText)")
	// Should have at least mousedown + some mousemoves + mouseup = 5+ events
	assert.NotContains(t, eventsOut, "0", "Should have recorded multiple events during drag")
}

func TestMouseRightClick_TriggersContextMenu(t *testing.T) {
	rightClickServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<div id="target" style="width:100px;height:100px;background:blue;">Right-click me</div>
			<div id="result">waiting</div>
			<script>
				document.getElementById('target').addEventListener('contextmenu', function(e) {
					e.preventDefault();
					document.getElementById('result').innerText = 'right-clicked';
				});
			</script>
		</body></html>`)
	}))
	defer rightClickServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", rightClickServer.URL)
	runCLI(t, bin, port, "mouse", "right-click", "#target")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('result').innerText")
	assert.Contains(t, jsOut, "right-clicked", "Right-click should fire contextmenu event")
}

func TestMouseDoubleClick_TriggersEvent(t *testing.T) {
	dblClickServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<div id="target" style="width:100px;height:100px;background:green;">Double-click me</div>
			<div id="result">waiting</div>
			<script>
				document.getElementById('target').addEventListener('dblclick', function(e) {
					document.getElementById('result').innerText = 'double-clicked';
				});
			</script>
		</body></html>`)
	}))
	defer dblClickServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", dblClickServer.URL)
	runCLI(t, bin, port, "mouse", "double-click", "#target")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('result').innerText")
	assert.Contains(t, jsOut, "double-clicked", "Double-click should fire dblclick event")
}
