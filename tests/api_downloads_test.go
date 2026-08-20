package tests

import (
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

// setupDownloadServer serves a page with download links of varying sizes.
func setupDownloadServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/report.csv", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="report.csv"`)
		_, _ = fmt.Fprint(w, "a,b,c\n1,2,3\n") //nolint:errcheck
	})
	mux.HandleFunc("/big.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="big.bin"`)
		_, _ = w.Write(make([]byte, 300000)) //nolint:errcheck
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<a id="csv" href="/report.csv" download>Download report</a>
			<a id="big" href="/big.bin" download>Download big</a>
			</body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestDownloads_ClickReportsAndTracks(t *testing.T) {
	server := setupDownloadServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	out := runCLI(t, bin, port, "click", "#csv")
	assert.Contains(t, out, "Successfully clicked")
	assert.Contains(t, out, "↓ report.csv")
	assert.Contains(t, out, ".browsii/downloads")

	out = runCLI(t, bin, port, "downloads")
	assert.Contains(t, out, "report.csv")
	assert.Contains(t, out, fmt.Sprintf("port-%d", port))
	assert.Contains(t, out, "report.csv")
	assert.Contains(t, out, "12B")

	// The file exists at the reported path.
	home, _ := os.UserHomeDir()
	data, err := os.ReadFile(filepath.Join(home, ".browsii", "downloads", fmt.Sprintf("port-%d", port), "report.csv"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "1,2,3")
}

func TestDownloads_LargeFileCompletes(t *testing.T) {
	server := setupDownloadServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	runCLI(t, bin, port, "click", "#big", "--no-evidence")

	// Completion is asynchronous; poll the listing.
	listed := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out := runCLI(t, bin, port, "downloads")
		if strings.Contains(out, "big.bin") && strings.Contains(out, "293.0KB") {
			listed = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	assert.True(t, listed, "download should complete and be listed; last output: %s", func() string {
		return runCLI(t, bin, port, "downloads")
	}())
}

func TestDownloads_ClearForgets(t *testing.T) {
	server := setupDownloadServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")
	runCLI(t, bin, port, "click", "#csv", "--no-evidence")

	out := runCLI(t, bin, port, "downloads", "clear")
	assert.Contains(t, out, "Forgot 1")

	out = runCLI(t, bin, port, "downloads")
	assert.Contains(t, out, "No downloads recorded")

	// File on disk kept.
	home, _ := os.UserHomeDir()
	_, err := os.Stat(filepath.Join(home, ".browsii", "downloads", fmt.Sprintf("port-%d", port), "report.csv"))
	assert.NoError(t, err)
}

func TestExpect_CheckedAndCount(t *testing.T) {
	server := setupReactFormServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL, "--no-evidence")

	// Unchecked initially.
	out := runCLI(t, bin, port, "expect", "--selector", "#tos", "--unchecked")
	assert.Contains(t, out, "unchecked")

	out, err := runCLIExpectFail(t, bin, port, "expect", "--selector", "#tos", "--checked", "--timeout", "300")
	require.Error(t, err)
	assert.Contains(t, out, "element is unchecked")

	// Check flips it; the expectation now holds without waiting.
	runCLI(t, bin, port, "check", "#tos", "--no-evidence")
	out = runCLI(t, bin, port, "expect", "--selector", "#tos", "--checked")
	assert.Contains(t, out, "element checked")

	// Count: two textboxes on the fixture (name, email).
	out = runCLI(t, bin, port, "expect", "--selector", "input", "--count", "5")
	assert.Contains(t, out, "matches 5 element(s)")

	out, err = runCLIExpectFail(t, bin, port, "expect", "--selector", "input", "--count", "2", "--timeout", "300")
	require.Error(t, err)
	assert.Contains(t, out, "matches 5 element(s), want 2")

	// Zero is a valid assertion: element gone.
	out = runCLI(t, bin, port, "expect", "--selector", ".does-not-exist", "--count", "0")
	assert.Contains(t, out, "matches 0 element(s)")

	out, err = runCLIExpectFail(t, bin, port, "expect", "--selector", "input", "--count", "0", "--timeout", "300")
	require.Error(t, err)
	assert.Contains(t, out, "matches 5 element(s), want 0")
}
