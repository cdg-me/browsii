package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLinks_ReturnsAllLinks(t *testing.T) {
	linkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<a href="https://example.com">Example</a>
			<a href="https://google.com">Google</a>
			<a href="https://github.com">GitHub</a>
			<a href="/relative-page">Relative</a>
			<a href="https://docs.example.com/api">API Docs</a>
		</body></html>`)
	}))
	defer linkServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", linkServer.URL)

	out := runCLI(t, bin, port, "get-links")

	// Should return JSON array
	var links []string
	err := json.Unmarshal([]byte(out), &links)
	require.NoError(t, err, "get-links should return valid JSON array")
	assert.GreaterOrEqual(t, len(links), 5, "Should find at least 5 links")
	assert.Contains(t, out, "example.com")
	assert.Contains(t, out, "google.com")
	assert.Contains(t, out, "github.com")
}

func TestGetLinks_FiltersWithPattern(t *testing.T) {
	linkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<a href="https://example.com/page1">Page 1</a>
			<a href="https://example.com/page2">Page 2</a>
			<a href="https://google.com/search">Google</a>
			<a href="https://example.com/api/v1">API</a>
		</body></html>`)
	}))
	defer linkServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", linkServer.URL)

	out := runCLI(t, bin, port, "get-links", "--pattern", "example.com")

	var links []string
	err := json.Unmarshal([]byte(out), &links)
	require.NoError(t, err, "get-links should return valid JSON array")
	assert.Equal(t, 3, len(links), "Should find 3 links matching 'example.com'")
	assert.NotContains(t, out, "google.com", "Google link should be filtered out")
}
