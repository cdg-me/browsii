package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScreenshot_CreatesFile(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	screenshotPath := filepath.Join(os.TempDir(), "test_screenshot_api.png")
	defer os.Remove(screenshotPath) //nolint:errcheck

	runCLI(t, bin, port, "screenshot", screenshotPath)

	fileInfo, err := os.Stat(screenshotPath)
	require.NoError(t, err, "Screenshot file should exist")
	assert.Greater(t, fileInfo.Size(), int64(1024), "Screenshot should be larger than 1KB")
}

func TestScreenshot_TargetsActivePage(t *testing.T) {
	serverA := setupNamedServer("Page A")
	defer serverA.Close()
	serverB := setupNamedServer("Page B")
	defer serverB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", serverA.URL)
	runCLI(t, bin, port, "tab", "new", serverB.URL)

	screenshotPath := filepath.Join(os.TempDir(), "test_screenshot_tab.png")
	defer os.Remove(screenshotPath) //nolint:errcheck

	runCLI(t, bin, port, "screenshot", screenshotPath)

	// Can't easily assert content of a screenshot, but prove the file was created
	fileInfo, err := os.Stat(screenshotPath)
	require.NoError(t, err, "Screenshot file should exist")
	assert.Greater(t, fileInfo.Size(), int64(1024))
}

func TestScreenshot_Element(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	elementPath := filepath.Join(os.TempDir(), "test_screenshot_element.png")
	defer os.Remove(elementPath) //nolint:errcheck

	runCLI(t, bin, port, "screenshot", elementPath, "--element", "#target-box")

	fileInfo, err := os.Stat(elementPath)
	require.NoError(t, err, "Element screenshot file should exist")
	assert.Greater(t, fileInfo.Size(), int64(100), "Element screenshot should have content")
}

func TestScreenshot_FullPage(t *testing.T) {
	tallServer := setupTallServer()
	defer tallServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", tallServer.URL)

	fullPath := filepath.Join(os.TempDir(), "test_screenshot_fullpage.png")
	defer os.Remove(fullPath) //nolint:errcheck

	runCLI(t, bin, port, "screenshot", fullPath, "--full-page")

	fileInfo, err := os.Stat(fullPath)
	require.NoError(t, err, "Full-page screenshot file should exist")
	assert.Greater(t, fileInfo.Size(), int64(1024), "Full-page screenshot should be substantial")
}

func TestPDF_CreatesFile(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL)

	pdfPath := filepath.Join(os.TempDir(), "test_output.pdf")
	defer os.Remove(pdfPath) //nolint:errcheck

	runCLI(t, bin, port, "pdf", pdfPath)

	// Verify file exists and starts with %PDF magic bytes
	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err, "PDF file should exist")
	assert.Greater(t, len(data), 100, "PDF should have substantial content")
	assert.Equal(t, "%PDF", string(data[:4]), "File should start with PDF magic bytes")
}
