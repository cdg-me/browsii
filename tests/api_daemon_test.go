package tests

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonLifecycle(t *testing.T) {
	bin := binPath(t)
	port := nextPort()

	// Start the daemon
	startCmd := exec.Command(bin, "start", "--port", fmt.Sprintf("%d", port), "--mode", "headless") //nolint:noctx
	startCmd.Stdout = os.Stdout
	startCmd.Stderr = os.Stderr

	err := startCmd.Run()
	require.NoError(t, err, "Failed to execute start wrapper")

	defer func() {
		exec.Command(bin, "stop", "--port", fmt.Sprintf("%d", port)).Run() //nolint:errcheck,noctx
	}()

	// Poll /ping until alive
	apiURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 1 * time.Second}

	alive := false
	for i := 0; i < 15; i++ {
		resp, pingErr := client.Get(apiURL + "/ping") //nolint:noctx
		if pingErr == nil && resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close() //nolint:errcheck
			if strings.TrimSpace(string(body)) == "pong" {
				alive = true
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.True(t, alive, "Daemon failed to start and respond to /ping within timeout")

	// Stop the daemon
	stopCmd := exec.Command(bin, "stop", "--port", fmt.Sprintf("%d", port)) //nolint:noctx
	stopOut, err := stopCmd.CombinedOutput()
	require.NoError(t, err, "Stop command failed: %s", string(stopOut))
	assert.Contains(t, string(stopOut), "Daemon gracefully shut down.")

	// Verify dead
	time.Sleep(1 * time.Second)
	_, pingErr := client.Get(apiURL + "/ping") //nolint:noctx
	assert.Error(t, pingErr, "Daemon should be dead, but /ping still succeeded")
}

// TestBrowserLaunchAndScreenshot is a low-level go-rod validation test.
// It doesn't use the daemon — it directly tests that go-rod can launch and interact.
func TestBrowserLaunchAndScreenshot(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	browser := rod.New().ControlURL(newLauncher().Headless(true).MustLaunch()).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(server.URL)
	page.MustWaitLoad()

	text := page.MustElement("#header").MustText()
	assert.Equal(t, "Browser CLI Test Bed", text)

	screenshotPath := filepath.Join(os.TempDir(), "testbed_screenshot.png")
	page.MustScreenshot(screenshotPath)

	fileInfo, err := os.Stat(screenshotPath)
	require.NoError(t, err, "Screenshot file should exist")
	assert.Greater(t, fileInfo.Size(), int64(1024))

	os.Remove(screenshotPath) //nolint:errcheck
}
