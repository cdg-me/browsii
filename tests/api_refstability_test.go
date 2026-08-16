package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRefStabilityServer serves buttons whose selectors are structural
// (no ids/names) so DOM reordering changes what they resolve to.
func setupRefStabilityServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
			<html><head><title>Ref Stability</title></head>
			<body>
				<button onclick="window.lastClicked='alpha'">Alpha</button>
				<button onclick="window.lastClicked='beta'">Beta</button>
				<button onclick="window.lastClicked='gamma'">Gamma</button>
			</body></html>`)
	})
	return httptest.NewServer(mux)
}

// setupFeedServer serves an HN-like table of story links whose selectors are
// positional (nth-of-type), so live re-ranking shifts them.
func setupFeedServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
			<html><head><title>Feed</title></head>
			<body><table><tbody>
				<tr><td><a href="/story-1">First story</a></td></tr>
				<tr><td><a href="/story-2">Second story</a></td></tr>
				<tr><td><a href="/story-3">Third story</a></td></tr>
			</tbody></table></body></html>`)
	})
	return httptest.NewServer(mux)
}

// refOfText finds the ref whose element line displays exactly label.
func refOfText(t *testing.T, out, label string) int {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, fmt.Sprintf("%q", label)) {
			bracket := strings.Index(line, "]")
			require.Greater(t, bracket, 0, "malformed elements line: %q", line)
			ref, err := strconv.Atoi(strings.TrimPrefix(line[:bracket], "["))
			require.NoError(t, err)
			return ref
		}
	}
	t.Fatalf("element with label %q not found in:\n%s", label, out)
	return 0
}

// TestClick_RefSurvivesReorder proves fingerprint retargeting: buttons with
// identical structure but distinct identities are reordered between
// 'elements' and 'click'. The ref must still act on its original element.
func TestClick_RefSurvivesReorder(t *testing.T) {
	server := setupRefStabilityServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "elements")
	betaRef := refOfText(t, out, "Beta")
	require.NotZero(t, betaRef)

	// Reorder: move Gamma before Beta. Beta's structural selector changes
	// (nth-of-type(2) → nth-of-type(3)); without verification the ref would
	// now resolve to Gamma.
	runCLI(t, bin, port, "js", `() => {
		const btns = document.querySelectorAll('button');
		btns[2].parentElement.insertBefore(btns[2], btns[1]);
	}`)

	runCLI(t, bin, port, "click", strconv.Itoa(betaRef))

	jsOut := runCLI(t, bin, port, "js", "() => window.lastClicked")
	assert.Contains(t, jsOut, "beta", "reordered click must still hit Beta")
}

// TestClick_RefFailsWhenElementReplaced proves the failure mode is loud:
// when the element is removed (not moved), the ref must error with the
// original identity instead of clicking its replacement.
func TestClick_RefFailsWhenElementReplaced(t *testing.T) {
	server := setupRefStabilityServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "elements")
	betaRef := refOfText(t, out, "Beta")

	// Replace Beta's button with a different-labeled one at the same position.
	runCLI(t, bin, port, "js", `() => {
		const btns = document.querySelectorAll('button');
		const repl = document.createElement('button');
		repl.textContent = 'Delta';
		repl.onclick = () => { window.lastClicked = 'delta'; };
		btns[1].replaceWith(repl);
	}`)

	out, err := runCLIExpectFail(t, bin, port, "click", strconv.Itoa(betaRef))
	require.Error(t, err, "clicking a replaced ref must fail")
	assert.Contains(t, out, "no longer matches")
	assert.Contains(t, out, `"Beta"`)
	assert.Contains(t, out, "re-run it for fresh refs")

	// And critically: nothing was clicked.
	jsOut := runCLI(t, bin, port, "js", "() => window.lastClicked || 'none'")
	assert.Contains(t, jsOut, "none")
}

// TestClick_RefRetargetsOnLiveFeed is the HN incident as a regression test:
// a feed re-ranks (new row prepended) between 'elements' and 'click'; the
// ref must navigate to the story it was captured from.
func TestClick_RefRetargetsOnLiveFeed(t *testing.T) {
	server := setupFeedServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "elements")
	secondRef := refOfText(t, out, "Second story")

	// Live re-rank: a new story lands at the top, shifting every row down.
	runCLI(t, bin, port, "js", `() => {
		const tb = document.querySelector('tbody');
		const tr = document.createElement('tr');
		const td = document.createElement('td');
		const a = document.createElement('a');
		a.href = '/story-0';
		a.textContent = 'Zeroth story';
		td.appendChild(a);
		tr.appendChild(td);
		tb.insertBefore(tr, tb.firstChild);
	}`)

	runCLI(t, bin, port, "click", strconv.Itoa(secondRef))

	jsOut := runCLI(t, bin, port, "js", "() => location.pathname")
	assert.Contains(t, jsOut, "/story-2", "click must follow the original story's link, not the row that took its place")
}
