package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupArticleServer serves an article wrapped in heavy navigation clutter.
func setupArticleServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var body string
		body += `<!DOCTYPE html><html><head><title>Regulation in Tech</title></head><body>`
		body += `<nav><a href="/home">Home</a> <a href="/about">About</a> <a href="/login">Login</a> <a href="/signup">Signup</a> <a href="/careers">Careers</a> <a href="/blog">Blog</a> <a href="/docs">Docs</a></nav>`
		body += `<aside><div>Related reading</div><a href="/x">Also read this</a></aside>`
		body += `<article><h1>Regulation in Tech</h1><p class="byline">By Jane Doe</p>`
		for i := 0; i < 12; i++ {
			body += fmt.Sprintf(`<p>Paragraph %d discusses regulatory risk and compliance frameworks in European markets. The regulatory burden has grown steadily.</p>`, i)
		}
		body += `</article>`
		body += `<footer><a href="/tos">Terms</a> <a href="/privacy">Privacy</a> <a href="/contact">Contact</a></footer>`
		body += `</body></html>`
		_, _ = fmt.Fprint(w, body) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestReadable_ExtractsArticleWithoutClutter(t *testing.T) {
	server := setupArticleServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	htmlLen := len(runCLI(t, bin, port, "scrape"))
	textOut := runCLI(t, bin, port, "scrape", "--format", "readable")

	assert.Contains(t, textOut, "Regulation in Tech")
	assert.Contains(t, textOut, "By Jane Doe")
	assert.Contains(t, textOut, "Paragraph 5")
	assert.NotContains(t, textOut, "Signup", "nav clutter must be stripped")
	assert.NotContains(t, textOut, "Terms", "footer clutter must be stripped")
	assert.NotContains(t, textOut, "Related reading", "aside clutter must be stripped")
	assert.Less(t, len(textOut), htmlLen, "readable output must be smaller than html")
}

func TestReadable_NonArticleFailsCleanly(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out, err := runCLIExpectFail(t, bin, port, "scrape", "--format", "readable")
	require.Error(t, err)
	assert.Contains(t, out, "not readerable")
	assert.Contains(t, out, "hint")
}

func TestFind_TextAndRegex(t *testing.T) {
	server := setupArticleServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "find", "regulatory")
	assert.Contains(t, out, "12 match")
	assert.Contains(t, out, "Paragraph 0")

	out = runCLI(t, bin, port, "find", "--regex", "/Regulation in (Tech|EU)/")
	assert.Contains(t, out, "1 match")
	assert.Contains(t, out, "Regulation in Tech")
}

func TestFind_ZeroMatchesExitsOne(t *testing.T) {
	server := setupArticleServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out, err := runCLIExpectFail(t, bin, port, "find", "zzz-nothing-matches")
	require.Error(t, err)
	assert.Contains(t, out, "0 match")
}

func TestFind_RejectsBothModes(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	out, err := runCLIExpectFail(t, bin, port, "find", "x", "--regex", "y")
	require.Error(t, err)
	assert.Contains(t, out, "not both")
}

func TestElement_FullDetail(t *testing.T) {
	server := setupElementServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "element", "#save-btn")
	assert.Contains(t, out, `button "Save"`)
	assert.Contains(t, out, "visible: true")

	out = runCLI(t, bin, port, "element", "#email", "--json")
	assert.Contains(t, out, `"attrs"`)
	assert.Contains(t, out, `"placeholder"`)
	assert.Contains(t, out, `"form"`)

	// Stale ref heals.
	elems := runCLI(t, bin, port, "elements")
	ref := refOfSelector(t, elems, "#save-btn")
	out = runCLI(t, bin, port, "element", fmt.Sprint(ref))
	assert.Contains(t, out, `"Save"`)

	out, err := runCLIExpectFail(t, bin, port, "element", "#nope")
	require.Error(t, err)
	assert.Contains(t, out, "element not found")
}
