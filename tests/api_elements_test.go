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

// setupElementServer serves a page with a form and varied interactive
// elements for element-map and candidate tests.
func setupElementServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
			<html><head><title>Element Map</title></head>
			<body>
				<a href="/home" id="logo">Home</a>
				<button id="save-btn">Save</button>
				<button class="cta">Submit order</button>
				<button class="cta" disabled>Submit payment</button>
				<input type="text" id="email" name="email" placeholder="Email address">
				<input type="checkbox" id="tos">
				<select id="color"><option>red</option><option>blue</option></select>
				<div id="hidden-wrap" style="display:none"><button id="secret">Secret action</button></div>
				<span id="clickable-span" tabindex="0" onclick="this.textContent='span clicked'">Span target</span>
				<script>
					document.getElementById('save-btn').addEventListener('click', function() {
						this.textContent = 'Saved!';
					});
				</script>
			</body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

// refOfSelector finds the ref assigned to selector in 'elements' JSON output.
func refOfSelector(t *testing.T, out, selector string) int {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "("+selector+")") {
			bracket := strings.Index(line, "]")
			require.Greater(t, bracket, 0, "malformed elements line: %q", line)
			ref, err := strconv.Atoi(strings.TrimPrefix(line[:bracket], "["))
			require.NoError(t, err, "malformed ref in line: %q", line)
			return ref
		}
	}
	t.Fatalf("selector %q not found in elements output:\n%s", selector, out)
	return 0
}

func TestElements_ListsInteractiveElements(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "elements")

	// Core interactive elements appear with refs, roles, and selectors.
	assert.Contains(t, out, `[1] link "Home" -> /home (#logo)`)
	assert.Contains(t, out, `button "Save" (#save-btn)`)
	assert.Contains(t, out, `textbox "Email address" (#email)`)
	assert.Contains(t, out, `checkbox`)
	assert.Contains(t, out, `combobox`)

	// Hidden elements are excluded by default.
	assert.NotContains(t, out, "Secret action")
}

func TestElements_AllIncludesHidden(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "elements", "--all")
	assert.Contains(t, out, `hidden (#secret)`)
}

func TestElements_FilterNarrowsResults(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "elements", "--filter", "submit")
	assert.Contains(t, out, "Submit order")
	assert.Contains(t, out, "Submit payment")
	assert.NotContains(t, out, `"Save"`)
	assert.NotContains(t, out, "Home")
}

func TestClick_ByRefWorks(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "elements")
	ref := refOfSelector(t, out, "#save-btn")

	out = runCLI(t, bin, port, "click", strconv.Itoa(ref))
	assert.Contains(t, out, fmt.Sprintf("Successfully clicked '%d'", ref))

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('save-btn').textContent")
	assert.Contains(t, jsOut, "Saved!")
}

func TestClick_NonbuttonElementByRef(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "elements")
	ref := refOfSelector(t, out, "#clickable-span")

	runCLI(t, bin, port, "click", strconv.Itoa(ref))

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('clickable-span').textContent")
	assert.Contains(t, jsOut, "span clicked")
}

func TestClick_StaleRefFailsWithHint(t *testing.T) {
	server := setupElementServer()
	defer server.Close()
	empty := setupNamedServer("Empty Page")
	defer empty.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "elements")
	ref := refOfSelector(t, out, "#save-btn")

	// Navigating to a page without interactive elements invalidates refs.
	// (Re-navigation to an identical DOM would self-heal the ref via
	// re-enumeration, so the target page must actually differ.)
	runCLI(t, bin, port, "navigate", empty.URL)

	out, err := runCLIExpectFail(t, bin, port, "click", strconv.Itoa(ref))
	require.Error(t, err, "clicking a stale ref must fail")
	assert.Contains(t, out, "not found")
	assert.Contains(t, out, "run 'elements' again")
}

func TestClick_NotFoundListsCandidates(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// ".submit" matches nothing as a selector, but two buttons say "Submit …".
	out, err := runCLIExpectFail(t, bin, port, "click", ".submit")
	require.Error(t, err, "clicking a nonexistent selector must fail")
	assert.Contains(t, out, "element not found: .submit")
	assert.Contains(t, out, "Submit order")
	assert.Contains(t, out, "Submit payment")
	assert.Contains(t, out, "(disabled)")
	assert.Contains(t, out, "selector:")
}

func TestClick_HiddenElementFailsWithExplanation(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out, err := runCLIExpectFail(t, bin, port, "click", "#secret")
	require.Error(t, err, "clicking a hidden element must fail")
	assert.Contains(t, out, "not visible")
}

func TestType_ByRefWorks(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "elements")
	ref := refOfSelector(t, out, "#email")

	runCLI(t, bin, port, "type", strconv.Itoa(ref), "agent@example.com")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('email').value")
	assert.Contains(t, jsOut, "agent@example.com")
}

func TestType_SelectorWithQuotesIsSafe(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// A selector containing a quote must not break out of the focus script
	// (regression: the selector used to be interpolated into single-quoted JS).
	runCLI(t, bin, port, "type", `input[name="email"]`, "quoted")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('email').value")
	assert.Contains(t, jsOut, "quoted")
}

func TestElements_JSONOutput(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "elements", "--json")
	assert.Contains(t, out, `"count":`)
	assert.Contains(t, out, `"elements":`)
	assert.Contains(t, out, `"ref":1`)
	assert.Contains(t, out, `"selector":"#logo"`)
}
