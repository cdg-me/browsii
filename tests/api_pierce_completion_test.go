package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupShadowLinkServer serves a shadow app with links and a file input
// inside the root, plus a paragraph of rendered text.
func setupShadowLinkServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
			<html><head><title>Shadow Links</title></head><body>
			<nav-host id="nav"></nav-host>
			<div id="shadow-text"></div>
			<script>
			class NavHost extends HTMLElement {
				constructor() {
					super();
					const root = this.attachShadow({mode: 'open'});
					root.innerHTML =
						'<a href="/shadow-one">Shadow one</a> ' +
						'<a href="/shadow-two">Shadow two</a> ' +
						'<p id="msg">rendered inside shadow</p> ' +
						'<input type="file" id="picker">';
				}
			}
			customElements.define('nav-host', NavHost);
			</script></body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestShadow_ScrapeTextAndLinksIncludeShadowContent(t *testing.T) {
	server := setupShadowLinkServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	text := runCLI(t, bin, port, "scrape", "--format", "text")
	assert.Contains(t, text, "rendered inside shadow", "scrape text must include shadow content")

	links := runCLI(t, bin, port, "get-links", "--pattern", "shadow-")
	assert.Contains(t, links, "/shadow-one")
	assert.Contains(t, links, "/shadow-two")
}

func TestShadow_ScreenshotElementPierces(t *testing.T) {
	server := setupShadowLinkServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := t.TempDir()
	png := filepath.Join(out, "el.png")
	runCLI(t, bin, port, "screenshot", png, "--element", "#nav >>> #msg")
	data, err := os.ReadFile(png)
	require.NoError(t, err)
	assert.Greater(t, len(data), 500, "element screenshot should produce a real image")
}

func TestShadow_UploadPierces(t *testing.T) {
	server := setupShadowLinkServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	tmp := filepath.Join(t.TempDir(), "f.txt")
	require.NoError(t, os.WriteFile(tmp, []byte("hi"), 0644))

	runCLI(t, bin, port, "upload", "#nav >>> #picker", tmp)

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('nav').shadowRoot.getElementById('picker').files.length")
	assert.Contains(t, jsOut, "1")
}

// TestExport_PiercingSelector is covered by the buildPlaywrightSpec unit
// goldens in internal/daemon (plainSpecLocator chain mapping).
