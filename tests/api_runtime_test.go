package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWasm_NetworkEvents(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test.json" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"status":"ok"}`) //nolint:errcheck
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<script>
				setTimeout(() => fetch('/test.json'), 500);
			</script>
		</body></html>`)
	}))
	defer apiServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Seed tab to prevent race conditions on very first launch
	runCLI(t, bin, port, "navigate", "about:blank")
	exec.Command(bin, "install-runtimes").Run() //nolint:errcheck,noctx

	scriptContent := fmt.Sprintf(`//go:build wasip1
package main

import (
	"browsii/sdk"
	"strings"
)

type Output struct {
	Count int      `+"`json:\"count\"`"+`
	URLs  []string `+"`json:\"urls\"`"+`
}

var capturedURLs []string

func onNetworkRequest(event sdk.NetworkEvent) {
	if strings.Contains(event.URL, "test.json") {
		capturedURLs = append(capturedURLs, event.URL)
	}
}

func main() {
	sdk.OnNetworkRequest(onNetworkRequest)

	if err := sdk.Navigate("%s"); err != nil {
		sdk.SetResult(map[string]string{"error": err.Error()})
		return
	}

	// Yield event loop to allow the async fetch inside setTimeout to fire and map across SSE
	sdk.WaitIdle(1500)

	sdk.SetResult(Output{
		Count: len(capturedURLs),
		URLs:  capturedURLs,
	})
}`, apiServer.URL)

	scriptPath := filepath.Join(t.TempDir(), "events_test.go")
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0644)
	require.NoError(t, err)

	out := runCLI(t, bin, port, "run", scriptPath)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &result)
	require.NoError(t, err, "WASM execution should output valid JSON. Got: %s", out)

	count, ok := result["count"].(float64)
	require.True(t, ok, "Expected 'count' number in JSON output")
	assert.GreaterOrEqual(t, count, float64(1), "Should have intercepted at least 1 /test.json payload")

	urlsRaw, ok := result["urls"].([]interface{})
	require.True(t, ok, "Expected 'urls' array in JSON output")
	assert.GreaterOrEqual(t, len(urlsRaw), 1)
	assert.True(t, strings.HasSuffix(urlsRaw[0].(string), "/test.json"))
}

func TestWasm_BasicNavigation(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h1 id="target">Found Me</h1></body></html>`) //nolint:errcheck
	}))
	defer apiServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	exec.Command(bin, "install-runtimes").Run() //nolint:errcheck,noctx

	scriptContent := fmt.Sprintf(`//go:build wasip1
package main

import "browsii/sdk"

func main() {
	if err := sdk.Navigate("%s"); err != nil {
		sdk.SetResult(map[string]string{"error": err.Error()})
		return
	}

	if err := sdk.WaitVisible("#target"); err != nil {
		sdk.SetResult(map[string]string{"error": err.Error()})
		return
	}

	sdk.SetResult(map[string]string{"success": "true"})
}`, apiServer.URL)

	scriptPath := filepath.Join(t.TempDir(), "nav_test.go")
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0644)
	require.NoError(t, err)

	out := runCLI(t, bin, port, "run", scriptPath)

	var result map[string]string
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &result)
	require.NoError(t, err)
	assert.Equal(t, "true", result["success"])
}

// TestWasm_NetworkEvents_ImmediateFetch verifies that network events fired synchronously
// during page load (no setTimeout) are delivered to the WASM callback even when the
// script calls no WaitIdle after Navigate.
//
// Current behavior (red): events are only flushed at WaitIdle yield points, so count = 0.
// Expected behavior (green): host flushes buffered events before _navigate returns → count ≥ 1.
func TestWasm_NetworkEvents_ImmediateFetch(t *testing.T) {
	server := setupFetchServer("/immediate-signal")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", "about:blank")
	exec.Command(bin, "install-runtimes").Run() //nolint:errcheck,noctx

	scriptContent := fmt.Sprintf(`//go:build wasip1
package main

import (
	"browsii/sdk"
	"strings"
)

type ImmediateOutput struct {
	Count int      `+"`json:\"count\"`"+`
	URLs  []string `+"`json:\"urls\"`"+`
}

var immediateURLs []string

func main() {
	sdk.OnNetworkRequest(func(e sdk.NetworkEvent) {
		if strings.Contains(e.URL, "/immediate-signal") {
			immediateURLs = append(immediateURLs, e.URL)
		}
	})

	if err := sdk.Navigate("%s"); err != nil {
		sdk.SetResult(map[string]string{"error": err.Error()})
		return
	}

	// Intentionally NO WaitIdle — host must flush buffered events before _navigate returns.
	sdk.SetResult(ImmediateOutput{Count: len(immediateURLs), URLs: immediateURLs})
}`, server.URL)

	scriptPath := filepath.Join(t.TempDir(), "immediate_fetch_test.go")
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0644)
	require.NoError(t, err)

	out := runCLI(t, bin, port, "run", scriptPath)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &result)
	require.NoError(t, err, "WASM execution should output valid JSON. Got: %s", out)

	count, ok := result["count"].(float64)
	require.True(t, ok, "Expected 'count' in JSON output")
	assert.GreaterOrEqual(t, count, float64(1),
		"fetch() fires synchronously on page load; host must flush events after _navigate (no WaitIdle needed)")
}

// TestWasm_NetworkEvents_NoDuplicates verifies that navigating the same page N times
// results in exactly N network callbacks, not N*(N+1)/2 due to duplicate listeners.
//
// Current behavior (red): attachNetworkListener is called on every navigate, so the second
// navigate delivers 2 copies of each event, the third delivers 3 → total = 1+2+3 = 6.
// Expected behavior (green): single listener per page, registered once → exactly 3 events.
func TestWasm_NetworkEvents_NoDuplicates(t *testing.T) {
	server := setupFetchServer("/marker")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", "about:blank")
	exec.Command(bin, "install-runtimes").Run() //nolint:errcheck,noctx

	scriptContent := fmt.Sprintf(`//go:build wasip1
package main

import (
	"browsii/sdk"
	"strings"
)

type DupOutput struct {
	Count int      `+"`json:\"count\"`"+`
	URLs  []string `+"`json:\"urls\"`"+`
}

var markerURLs []string

func main() {
	sdk.OnNetworkRequest(func(e sdk.NetworkEvent) {
		if strings.Contains(e.URL, "/marker") {
			markerURLs = append(markerURLs, e.URL)
		}
	})

	url := "%s"

	if err := sdk.Navigate(url); err != nil {
		sdk.SetResult(map[string]string{"error": err.Error()})
		return
	}
	sdk.WaitIdle(300)

	if err := sdk.Navigate(url); err != nil {
		sdk.SetResult(map[string]string{"error": err.Error()})
		return
	}
	sdk.WaitIdle(300)

	if err := sdk.Navigate(url); err != nil {
		sdk.SetResult(map[string]string{"error": err.Error()})
		return
	}
	sdk.WaitIdle(300)

	sdk.SetResult(DupOutput{Count: len(markerURLs), URLs: markerURLs})
}`, server.URL)

	scriptPath := filepath.Join(t.TempDir(), "no_duplicates_test.go")
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0644)
	require.NoError(t, err)

	out := runCLI(t, bin, port, "run", scriptPath)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &result)
	require.NoError(t, err, "WASM execution should output valid JSON. Got: %s", out)

	count, ok := result["count"].(float64)
	require.True(t, ok, "Expected 'count' in JSON output")
	assert.Equal(t, float64(3), count,
		"3 navigates × 1 fetch each = exactly 3 events (duplicate listeners would give 1+2+3=6)")
}

// TestWasm_ConsoleEvents verifies that sdk.OnConsoleEvent receives callbacks
// when the page fires console calls, with correct level and text fields.
func TestWasm_ConsoleEvents(t *testing.T) {
	server := setupConsoleServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	exec.Command(bin, "install-runtimes").Run() //nolint:errcheck,noctx

	scriptContent := fmt.Sprintf(`//go:build wasip1
package main

import (
	sdk "browsii/sdk"
)

type ConsoleResult struct {
	Count      int    `+"`"+`json:"count"`+"`"+`
	FirstLevel string `+"`"+`json:"first_level"`+"`"+`
	FirstText  string `+"`"+`json:"first_text"`+"`"+`
}

func main() {
	var result ConsoleResult
	sdk.OnConsoleEvent(func(e sdk.ConsoleEvent) {
		result.Count++
		if result.FirstLevel == "" {
			result.FirstLevel = e.Level
			result.FirstText = e.Text
		}
	})
	if err := sdk.Navigate(%q); err != nil {
		sdk.SetResult(map[string]string{"error": err.Error()})
		return
	}
	sdk.WaitIdle(1000)
	sdk.SetResult(result)
}`, server.URL)

	scriptPath := filepath.Join(t.TempDir(), "console_test.go")
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0644)
	require.NoError(t, err)

	out := runCLI(t, bin, port, "run", scriptPath)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &result)
	require.NoError(t, err, "WASM execution should output valid JSON. Got: %s", out)

	count, ok := result["count"].(float64)
	require.True(t, ok, "Expected 'count' in JSON output")
	assert.GreaterOrEqual(t, count, float64(5), "expected at least 5 console events (log/warn/error/info/debug)")

	firstLevel, _ := result["first_level"].(string)
	assert.NotEmpty(t, firstLevel, "expected a non-empty first_level")

	firstText, _ := result["first_text"].(string)
	assert.NotEmpty(t, firstText, "expected a non-empty first_text")
}
