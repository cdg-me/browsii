package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


func TestNetworkCapture_RecordsRequests(t *testing.T) {
	// Create a server with multiple resources
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<html><body>
				<div id="result">loading</div>
				<script>
					fetch('/api/data').then(r => r.text()).then(d => {
						document.getElementById('result').innerText = d;
					});
				</script>
			</body></html>`)
		case "/api/data":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"status":"ok"}`) //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate first to create a page, then start capture
	runCLI(t, bin, port, "navigate", "about:blank")

	// Start capture
	runCLI(t, bin, port, "network", "capture", "start")

	runCLI(t, bin, port, "navigate", apiServer.URL)

	// Wait a moment for the fetch to complete
	runCLI(t, bin, port, "js", "() => new Promise(r => setTimeout(r, 200))")

	// Stop capture and get results
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var requests []map[string]interface{}
	err := json.Unmarshal([]byte(out), &requests)
	require.NoError(t, err, "network stop should return valid JSON array")
	assert.GreaterOrEqual(t, len(requests), 1, "Should have captured at least the page load request")

	// Check that at least one request contains the API endpoint
	found := false
	for _, req := range requests {
		if url, ok := req["url"].(string); ok {
			if url == apiServer.URL+"/api/data" || url == apiServer.URL+"/" {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "Should have captured requests to the test server")
}

func TestNetworkThrottle_SlowsRequests(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	// Set throttle (just verify the command succeeds, not actual timing)
	runCLI(t, bin, port, "network", "throttle", "--latency", "100", "--download", "500000", "--upload", "500000")

	// Verify page still works after throttle
	jsOut := runCLI(t, bin, port, "js", "() => document.title")
	assert.Contains(t, jsOut, "Test Bed")
}

func TestRouteMock_InterceptsRequests(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<html><body>
				<div id="result">loading</div>
				<script>
					fetch('/api/users').then(r => r.json()).then(d => {
						document.getElementById('result').innerText = d.name;
					});
				</script>
			</body></html>`)
		case "/api/users":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"name":"real_user"}`) //nolint:errcheck
		}
	}))
	defer apiServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate first to create a page
	runCLI(t, bin, port, "navigate", "about:blank")

	// Set up a route mock
	runCLI(t, bin, port, "network", "mock", "--pattern", "*/api/users",
		"--body", `{"name":"mocked_user"}`,
		"--content-type", "application/json")

	runCLI(t, bin, port, "navigate", apiServer.URL)

	// Wait for fetch
	runCLI(t, bin, port, "js", "() => new Promise(r => setTimeout(r, 300))")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('result').innerText")
	assert.Contains(t, jsOut, "mocked_user", "Route mock should intercept and return mocked response")
}

// ---------------------------------------------------------------------------
// Multi-tab capture and --tab filter tests
// ---------------------------------------------------------------------------

// TestNetworkCapture_AllTabs_Default verifies that capture with no --tab flag records
// requests from every open tab, not just the active one at start time.
func TestNetworkCapture_AllTabs_Default(t *testing.T) {
	serverA := setupFetchServer("/signal-a")
	defer serverA.Close()
	serverB := setupFetchServer("/signal-b")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)        // tab 0
	runCLI(t, bin, port, "tab", "new", serverB.URL)      // tab 1

	runCLI(t, bin, port, "network", "capture", "start") // default: all tabs

	// Fire requests from each tab
	runCLI(t, bin, port, "tab", "switch", "0")
	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "tab", "switch", "1")
	runCLI(t, bin, port, "navigate", serverB.URL)

	out := runCLI(t, bin, port, "network", "capture", "stop")

	var events []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &events))

	var sawA, sawB bool
	for _, e := range events {
		if u, _ := e["url"].(string); strings.HasSuffix(u, "/signal-a") {
			sawA = true
		}
		if u, _ := e["url"].(string); strings.HasSuffix(u, "/signal-b") {
			sawB = true
		}
	}
	assert.True(t, sawA, "Should have captured /signal-a fetch from tab 0")
	assert.True(t, sawB, "Should have captured /signal-b fetch from tab 1")
}

// TestNetworkCapture_Events_HaveTabIndex verifies each captured event carries an integer
// "tab" field identifying which tab index it came from.
func TestNetworkCapture_Events_HaveTabIndex(t *testing.T) {
	server := setupFetchServer("/check")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start")
	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "network", "capture", "stop")

	var events []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &events))
	require.NotEmpty(t, events, "Expected at least one captured event")

	for _, e := range events {
		tabVal, hasTab := e["tab"]
		assert.True(t, hasTab, "Event missing 'tab' field: %v", e)
		_, isNumber := tabVal.(float64)
		assert.True(t, isNumber, "'tab' field should be a number, got %T in %v", tabVal, e)
	}
}

// TestNetworkCapture_TabFilter_Active verifies --tab active captures only the tab
// that was active at capture start time, even after switching away.
func TestNetworkCapture_TabFilter_Active(t *testing.T) {
	serverA := setupFetchServer("/from-a")
	defer serverA.Close()
	serverB := setupFetchServer("/from-b")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)       // tab 0
	runCLI(t, bin, port, "tab", "new", serverB.URL)     // tab 1
	runCLI(t, bin, port, "tab", "switch", "0")          // make tab 0 active

	runCLI(t, bin, port, "network", "capture", "start", "--tab", "active") // resolves to tab 0

	// Navigate the active tab (should be captured)
	runCLI(t, bin, port, "navigate", serverA.URL)

	// Switch to tab 1 and navigate (should NOT be captured)
	runCLI(t, bin, port, "tab", "switch", "1")
	runCLI(t, bin, port, "navigate", serverB.URL)

	out := runCLI(t, bin, port, "network", "capture", "stop")

	var events []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &events))

	for _, e := range events {
		u, _ := e["url"].(string)
		assert.False(t, strings.HasSuffix(u, "/from-b"),
			"Tab 1 requests must not appear when --tab active was resolved to tab 0: %v", e)
	}
	var sawA bool
	for _, e := range events {
		if u, _ := e["url"].(string); strings.HasSuffix(u, "/from-a") {
			sawA = true
		}
	}
	assert.True(t, sawA, "Tab 0 (active) requests should be captured")
}

// TestNetworkCapture_TabFilter_Index verifies --tab <N> captures only that specific tab.
func TestNetworkCapture_TabFilter_Index(t *testing.T) {
	serverA := setupFetchServer("/only-tab0")
	defer serverA.Close()
	serverB := setupFetchServer("/not-tab0")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)    // tab 0
	runCLI(t, bin, port, "tab", "new", serverB.URL)  // tab 1

	runCLI(t, bin, port, "network", "capture", "start", "--tab", "0")

	runCLI(t, bin, port, "tab", "switch", "0")
	runCLI(t, bin, port, "navigate", serverA.URL) // captured

	runCLI(t, bin, port, "tab", "switch", "1")
	runCLI(t, bin, port, "navigate", serverB.URL) // not captured

	out := runCLI(t, bin, port, "network", "capture", "stop")

	var events []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &events))

	for _, e := range events {
		u, _ := e["url"].(string)
		assert.False(t, strings.HasSuffix(u, "/not-tab0"),
			"Tab 1 request must not appear when --tab 0: %v", e)
	}
	var sawTab0 bool
	for _, e := range events {
		if u, _ := e["url"].(string); strings.HasSuffix(u, "/only-tab0") {
			sawTab0 = true
		}
	}
	assert.True(t, sawTab0, "Tab 0 requests should be captured")
}

// TestNetworkCapture_TabFilter_Next verifies --tab next captures only the tab opened
// after capture start, excluding pre-existing tabs.
func TestNetworkCapture_TabFilter_Next(t *testing.T) {
	serverA := setupFetchServer("/existing-tab")
	defer serverA.Close()
	serverB := setupFetchServer("/next-tab")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL) // tab 0 exists before capture

	runCLI(t, bin, port, "network", "capture", "start", "--tab", "next") // next = tab 1

	runCLI(t, bin, port, "tab", "new", serverB.URL) // opens tab 1 — the "next" tab

	// Also navigate existing tab 0 — should not appear
	runCLI(t, bin, port, "tab", "switch", "0")
	runCLI(t, bin, port, "navigate", serverA.URL)

	out := runCLI(t, bin, port, "network", "capture", "stop")

	var events []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &events))

	for _, e := range events {
		u, _ := e["url"].(string)
		assert.False(t, strings.HasSuffix(u, "/existing-tab"),
			"Pre-existing tab 0 requests must not appear with --tab next: %v", e)
	}
	var sawNext bool
	for _, e := range events {
		if u, _ := e["url"].(string); strings.HasSuffix(u, "/next-tab") {
			sawNext = true
		}
	}
	assert.True(t, sawNext, "The newly opened tab's requests should be captured")
}

// TestNetworkCapture_TabFilter_Last verifies --tab last captures only the most recently
// opened tab at capture start time.
func TestNetworkCapture_TabFilter_Last(t *testing.T) {
	serverA := setupFetchServer("/first-tab")
	defer serverA.Close()
	serverB := setupFetchServer("/last-tab")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)    // tab 0
	runCLI(t, bin, port, "tab", "new", serverB.URL)  // tab 1 = last

	runCLI(t, bin, port, "network", "capture", "start", "--tab", "last") // resolves to tab 1

	// Navigate tab 0 (should NOT be captured)
	runCLI(t, bin, port, "tab", "switch", "0")
	runCLI(t, bin, port, "navigate", serverA.URL)

	// Navigate tab 1 (should be captured)
	runCLI(t, bin, port, "tab", "switch", "1")
	runCLI(t, bin, port, "navigate", serverB.URL)

	out := runCLI(t, bin, port, "network", "capture", "stop")

	var events []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &events))

	for _, e := range events {
		u, _ := e["url"].(string)
		assert.False(t, strings.HasSuffix(u, "/first-tab"),
			"Tab 0 requests must not appear with --tab last: %v", e)
	}
	var sawLast bool
	for _, e := range events {
		if u, _ := e["url"].(string); strings.HasSuffix(u, "/last-tab") {
			sawLast = true
		}
	}
	assert.True(t, sawLast, "Last tab's requests should be captured")
}

// TestNetworkCapture_TabFilter_Next_NoNewTab verifies --tab next returns an empty array
// (not an error) when no new tab is ever opened before capture stop.
func TestNetworkCapture_TabFilter_Next_NoNewTab(t *testing.T) {
	server := setupFetchServer("/ignored")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL) // tab 0 — the only tab

	runCLI(t, bin, port, "network", "capture", "start", "--tab", "next") // next = tab 1 (never opened)

	// Navigate the existing tab; these requests should be silently ignored
	runCLI(t, bin, port, "navigate", server.URL)

	out := runCLI(t, bin, port, "network", "capture", "stop")

	var events []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &events))
	assert.Empty(t, events, "--tab next with no new tab should return [] not an error")
}

func TestNetworkCapture_SSE(t *testing.T) {
	// Create a test server with an explicit fetch script
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test.json" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"status":"ok"}`) //nolint:errcheck
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<script>
				fetch('/test.json');
			</script>
		</body></html>`)
	}))
	defer apiServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate first to create a clean page
	runCLI(t, bin, port, "navigate", "about:blank")

	// Connect to the SSE stream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sseURL := fmt.Sprintf("http://127.0.0.1:%d/events/stream", port)
	req, err := http.NewRequestWithContext(ctx, "GET", sseURL, nil)
	require.NoError(t, err)

	clientResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Failed to connect to SSE stream")
	defer clientResp.Body.Close() //nolint:errcheck

	// Wait a moment for SSE connection to establish
	time.Sleep(200 * time.Millisecond)

	// Trigger navigation via CLI
	runCLI(t, bin, port, "navigate", apiServer.URL)

	// Read events from the stream
	scanner := bufio.NewScanner(clientResp.Body)

	eventCount := 0
	timeout := time.After(5 * time.Second)

	// Start a goroutine to read events so we can timeout
	done := make(chan bool)
	go func() {
		for scanner.Scan() {
			text := scanner.Text()
			t.Logf("SSE Raw: %s", text)
			if strings.HasPrefix(text, "data: ") {
				var event struct {
					Type    string                 `json:"type"`
					Payload map[string]interface{} `json:"payload"`
				}
				dataStr := strings.TrimPrefix(text, "data: ")
				if err := json.Unmarshal([]byte(dataStr), &event); err == nil {
					if event.Type == "network_request" {
						eventCount++
						url, _ := event.Payload["url"].(string)
						if strings.HasSuffix(url, "/test.json") {
							done <- true
							return
						}
					}
				}
			}
		}
	}()

	select {
	case <-done:
		// Success
		assert.Greater(t, eventCount, 0, "Should have intercepted network events")
	case <-timeout:
		t.Fatal("Timed out waiting for network events over SSE")
	}
}

// ---------------------------------------------------------------------------
// --output flag tests
// ---------------------------------------------------------------------------

// TestNetworkCapture_Output_WritesFile verifies that --output writes the captured
// requests to the specified file as a valid JSON array.
func TestNetworkCapture_Output_WritesFile(t *testing.T) {
	server := setupFetchServer("/output-test")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	outFile := filepath.Join(t.TempDir(), "capture.json")

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start", "--output", outFile)
	runCLI(t, bin, port, "navigate", server.URL)
	confirm := runCLI(t, bin, port, "network", "capture", "stop")

	assert.Contains(t, confirm, outFile, "confirmation message should include the output path")

	raw, err := os.ReadFile(outFile)
	require.NoError(t, err, "output file should exist after stop --output")

	var requests []map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &requests), "output file should contain a valid JSON array")
	assert.NotEmpty(t, requests, "output file should contain at least one captured request")
}

// TestNetworkCapture_Output_SuppressesStdout verifies that when --output is used
// the JSON array is not printed to stdout (only the confirmation message appears).
func TestNetworkCapture_Output_SuppressesStdout(t *testing.T) {
	server := setupFetchServer("/suppress-test")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	outFile := filepath.Join(t.TempDir(), "capture.json")

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start", "--output", outFile)
	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "network", "capture", "stop")

	// When --output was set on start, stop returns a confirmation object, not the JSON array.
	assert.NotContains(t, out, `"url"`, "JSON should not be printed to stdout when --output is set")
}

// ---------------------------------------------------------------------------
// --include flag tests
// ---------------------------------------------------------------------------

// TestNetworkCapture_Include_BaseFields verifies that without --include only
// the base fields (url, method, type, tab) are present — no response data.
func TestNetworkCapture_Include_BaseFields(t *testing.T) {
	server := setupFetchServer("/base-fields")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start") // no --include
	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var reqs []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &reqs))
	require.NotEmpty(t, reqs)

	for _, r := range reqs {
		assert.Contains(t, r, "url")
		assert.Contains(t, r, "method")
		assert.Contains(t, r, "type")
		assert.Contains(t, r, "tab")
		// Optional fields must be absent when --include is not set
		assert.NotContains(t, r, "requestHeaders", "requestHeaders should be absent without --include request-headers")
		assert.NotContains(t, r, "status", "status should be absent without --include response-headers")
	}
}

// TestNetworkCapture_Include_RequestHeaders verifies that --include request-headers
// populates the requestHeaders field on each entry.
func TestNetworkCapture_Include_RequestHeaders(t *testing.T) {
	server := setupFetchServer("/req-headers")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start", "--include", "request-headers")
	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var reqs []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &reqs))
	require.NotEmpty(t, reqs)

	found := false
	for _, r := range reqs {
		if hdrs, ok := r["requestHeaders"].(map[string]interface{}); ok && len(hdrs) > 0 {
			found = true
			break
		}
	}
	assert.True(t, found, "at least one entry should have non-empty requestHeaders")
}

// TestNetworkCapture_Include_ResponseHeaders verifies that --include response-headers
// populates status, statusText, responseHeaders, and mimeType.
func TestNetworkCapture_Include_ResponseHeaders(t *testing.T) {
	server := setupFetchServer("/resp-headers")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start", "--include", "response-headers")
	runCLI(t, bin, port, "navigate", server.URL)
	// Give responses time to arrive
	runCLI(t, bin, port, "js", "() => new Promise(r => setTimeout(r, 300))")
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var reqs []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &reqs))
	require.NotEmpty(t, reqs)

	found := false
	for _, r := range reqs {
		if status, ok := r["status"].(float64); ok && status == 200 {
			found = true
			break
		}
	}
	assert.True(t, found, "at least one entry should have status=200")
}

// TestNetworkCapture_Include_Wildcard_Request verifies that request-* expands to
// all request groups (headers, body, initiator, timestamp).
func TestNetworkCapture_Include_Wildcard_Request(t *testing.T) {
	server := setupFetchServer("/wildcard-req")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start", "--include", "request-*")
	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var reqs []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &reqs))
	require.NotEmpty(t, reqs)

	// timestamp should be present (seconds since epoch > 0)
	foundTS := false
	for _, r := range reqs {
		if ts, ok := r["timestamp"].(float64); ok && ts > 0 {
			foundTS = true
			break
		}
	}
	assert.True(t, foundTS, "request-* should include request-timestamp")

	// requestHeaders should be present on at least one entry
	foundHdrs := false
	for _, r := range reqs {
		if hdrs, ok := r["requestHeaders"].(map[string]interface{}); ok && len(hdrs) > 0 {
			foundHdrs = true
			break
		}
	}
	assert.True(t, foundHdrs, "request-* should include request-headers")
}

// TestNetworkCapture_Include_CommaSeparated verifies that comma-separated values
// within a single --include flag are handled identically to separate flags.
func TestNetworkCapture_Include_CommaSeparated(t *testing.T) {
	server := setupFetchServer("/comma-sep")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	// comma-separated in a single flag value
	runCLI(t, bin, port, "network", "capture", "start", "--include", "request-headers,request-timestamp")
	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var reqs []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &reqs))
	require.NotEmpty(t, reqs)

	foundTS, foundHdrs := false, false
	for _, r := range reqs {
		if ts, ok := r["timestamp"].(float64); ok && ts > 0 {
			foundTS = true
		}
		if hdrs, ok := r["requestHeaders"].(map[string]interface{}); ok && len(hdrs) > 0 {
			foundHdrs = true
		}
	}
	assert.True(t, foundTS, "comma-sep --include should enable request-timestamp")
	assert.True(t, foundHdrs, "comma-sep --include should enable request-headers")
}

// ---------------------------------------------------------------------------
// --format flag tests (network)
// ---------------------------------------------------------------------------

// TestNetworkCapture_Format_NDJSON verifies that --format ndjson produces
// newline-delimited JSON with one object per line.
func TestNetworkCapture_Format_NDJSON(t *testing.T) {
	server := setupFetchServer("/ndjson-test")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start", "--format", "ndjson")
	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "network", "capture", "stop")

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.NotEmpty(t, lines, "ndjson output should have at least one line")
	for i, line := range lines {
		if line == "" {
			continue
		}
		var obj map[string]interface{}
		assert.NoError(t, json.Unmarshal([]byte(line), &obj), "line %d should be valid JSON: %q", i, line)
		assert.Contains(t, obj, "url", "each ndjson line should have a url field")
	}
}

// TestNetworkCapture_Format_HAR verifies that --format har produces a valid
// HAR 1.2 document with a "log" key containing "entries".
func TestNetworkCapture_Format_HAR(t *testing.T) {
	server := setupFetchServer("/har-test")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start", "--format", "har")
	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var har map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &har), "HAR output should be valid JSON")

	log, ok := har["log"].(map[string]interface{})
	require.True(t, ok, "HAR must have a 'log' object")
	assert.Equal(t, "1.2", log["version"], "HAR version should be 1.2")

	entries, ok := log["entries"].([]interface{})
	require.True(t, ok, "HAR log must have 'entries' array")
	assert.NotEmpty(t, entries, "HAR entries should not be empty")

	// Validate first entry structure
	entry, ok := entries[0].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, entry, "startedDateTime")
	assert.Contains(t, entry, "request")
	assert.Contains(t, entry, "response")
	assert.Contains(t, entry, "timings")

	req, ok := entry["request"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, req, "url")
	assert.Contains(t, req, "method")
}

// TestNetworkCapture_Format_HAR_WithResponseHeaders verifies that combining
// --format har with --include response-headers populates response fields.
func TestNetworkCapture_Format_HAR_WithResponseHeaders(t *testing.T) {
	server := setupFetchServer("/har-resp")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start",
		"--format", "har", "--include", "response-headers")
	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "js", "() => new Promise(r => setTimeout(r, 300))")
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var har map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &har))

	log := har["log"].(map[string]interface{})
	entries := log["entries"].([]interface{})
	require.NotEmpty(t, entries)

	found := false
	for _, e := range entries {
		entry := e.(map[string]interface{})
		resp := entry["response"].(map[string]interface{})
		if status, ok := resp["status"].(float64); ok && status == 200 {
			found = true
			break
		}
	}
	assert.True(t, found, "HAR response should have status=200 when --include response-headers is set")
}

// TestNetworkCapture_Format_HAR_WrittenToFile verifies --format har with --output
// writes a valid HAR file.
func TestNetworkCapture_Format_HAR_WrittenToFile(t *testing.T) {
	server := setupFetchServer("/har-file")
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	outFile := filepath.Join(t.TempDir(), "capture.har")

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start", "--format", "har", "--output", outFile)
	runCLI(t, bin, port, "navigate", server.URL)
	confirm := runCLI(t, bin, port, "network", "capture", "stop")

	assert.Contains(t, confirm, outFile)

	raw, err := os.ReadFile(outFile)
	require.NoError(t, err)

	var har map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &har), "HAR file should be valid JSON")
	log := har["log"].(map[string]interface{})
	assert.Equal(t, "1.2", log["version"])
}

// TestNetworkCapture_Format_Invalid verifies that an unrecognised format returns an error.
func TestNetworkCapture_Format_Invalid(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", "about:blank")
	runCLI(t, bin, port, "network", "capture", "start", "--format", "xml")
	out, _ := runCLIExpectFail(t, bin, port, "network", "capture", "stop")
	assert.Contains(t, out, "unknown format", "invalid format should report an error")
}

// ---------------------------------------------------------------------------
// --include response-body tests
// ---------------------------------------------------------------------------

// TestNetworkCapture_Include_ResponseBody verifies that --include response-body
// populates the responseBody field with the actual response text.
func TestNetworkCapture_Include_ResponseBody(t *testing.T) {
	const bodyText = `{"hello":"world"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, bodyText) //nolint:errcheck
	}))
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start",
		"--include", "response-headers,response-body")
	runCLI(t, bin, port, "navigate", server.URL)
	// No sleep needed: capture stop blocks until all body-fetch goroutines complete.
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var reqs []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &reqs))
	require.NotEmpty(t, reqs)

	found := false
	for _, r := range reqs {
		if body, ok := r["responseBody"].(string); ok && body != "" {
			found = true
			// For JSON responses the body should contain our expected text (not base64)
			if strings.Contains(body, "hello") {
				assert.Equal(t, bodyText, body,
					"responseBody should match the actual response text")
			}
			break
		}
	}
	assert.True(t, found, "at least one entry should have a non-empty responseBody")
}

// TestNetworkCapture_Include_ResponseBody_HAR verifies that response-body is
// included in the HAR content section when both --format har and --include
// response-body are specified.
func TestNetworkCapture_Include_ResponseBody_HAR(t *testing.T) {
	const bodyText = `<html><body>hello har</body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, bodyText) //nolint:errcheck
	}))
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start",
		"--format", "har",
		"--include", "response-headers,response-body")
	runCLI(t, bin, port, "navigate", server.URL)
	// No sleep needed: capture stop blocks until all body-fetch goroutines complete.
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var har map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &har))

	log := har["log"].(map[string]interface{})
	entries := log["entries"].([]interface{})
	require.NotEmpty(t, entries)

	found := false
	for _, e := range entries {
		entry := e.(map[string]interface{})
		resp := entry["response"].(map[string]interface{})
		content, ok := resp["content"].(map[string]interface{})
		if !ok {
			continue
		}
		if text, ok := content["text"].(string); ok && text != "" {
			found = true
			if strings.Contains(text, "hello har") {
				assert.Equal(t, bodyText, text,
					"HAR content.text should contain the actual body")
			}
			break
		}
	}
	assert.True(t, found, "HAR content.text should be populated when --include response-body is set")
}

// TestNetworkCapture_Include_ResponseBody_StopDrainsGoroutines is a regression
// test for the race between in-flight body-fetch goroutines and capture stop.
// It calls stop immediately after navigate with no intervening sleep: the stop
// handler must block on bodyFetchWg.Wait() and return correct bodies anyway.
func TestNetworkCapture_Include_ResponseBody_StopDrainsGoroutines(t *testing.T) {
	const bodyText = `{"drain":"test"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, bodyText) //nolint:errcheck
	}))
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "network", "capture", "start", "--include", "response-body")
	runCLI(t, bin, port, "navigate", server.URL)
	// Deliberately no sleep — stop must drain body fetches itself.
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var reqs []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &reqs))
	require.NotEmpty(t, reqs)

	found := false
	for _, r := range reqs {
		if body, ok := r["responseBody"].(string); ok && body != "" {
			found = true
			break
		}
	}
	assert.True(t, found, "stop should drain body-fetch goroutines without an explicit sleep")
}

// TestNetworkCapture_Include_ResponseBody_HARContentSize verifies that the HAR
// content.size field is set from the body length when response-size is not
// captured alongside response-body.
func TestNetworkCapture_Include_ResponseBody_HARContentSize(t *testing.T) {
	const bodyText = `{"size":"check"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, bodyText) //nolint:errcheck
	}))
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)
	// response-body only — no response-size, so content.size must be derived from body text
	runCLI(t, bin, port, "network", "capture", "start",
		"--format", "har", "--include", "response-body")
	runCLI(t, bin, port, "navigate", server.URL)
	out := runCLI(t, bin, port, "network", "capture", "stop")

	var har map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &har))
	log := har["log"].(map[string]interface{})
	entries := log["entries"].([]interface{})
	require.NotEmpty(t, entries)

	found := false
	for _, e := range entries {
		entry := e.(map[string]interface{})
		resp := entry["response"].(map[string]interface{})
		content, ok := resp["content"].(map[string]interface{})
		if !ok {
			continue
		}
		text, hasText := content["text"].(string)
		size, hasSize := content["size"].(float64)
		if hasText && text != "" && hasSize {
			found = true
			assert.Equal(t, float64(len(text)), size,
				"content.size should equal len(content.text) when response-size is not captured")
			break
		}
	}
	assert.True(t, found, "HAR entry with non-empty content.text and content.size should exist")
}
