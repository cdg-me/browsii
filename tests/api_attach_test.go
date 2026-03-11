package tests

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cdg-me/browsii/client"
)

// TestAttach_CanIssueCommandsToRunningDaemon verifies that an attached client
// can drive a real daemon — i.e., the HTTP plumbing through to the browser works.
func TestAttach_CanIssueCommandsToRunningDaemon(t *testing.T) {
	srv := setupMockServer()
	defer srv.Close()

	port := nextPort()
	_, cleanup := startDaemon(t, port)
	defer cleanup()

	c, err := client.Attach(port)
	require.NoError(t, err)

	require.NoError(t, c.Navigate(srv.URL))
}

// TestStart_BROWSII_PORT_AttachesAndCanNavigate verifies the env-var dev mode
// path end-to-end: Start() resolves to Attach(), commands reach the browser,
// and Stop() leaves the daemon alive.
func TestStart_BROWSII_PORT_AttachesAndCanNavigate(t *testing.T) {
	srv := setupMockServer()
	defer srv.Close()

	port := nextPort()
	_, cleanup := startDaemon(t, port)
	defer cleanup()

	t.Setenv("BROWSII_PORT", strconv.Itoa(port))

	c, err := client.Start(client.Options{})
	require.NoError(t, err)
	assert.Equal(t, port, c.Port())

	require.NoError(t, c.Navigate(srv.URL))

	c.Stop()

	httpClient := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("http://127.0.0.1:%d/ping", port), nil)
	resp, err := httpClient.Do(req)
	require.NoError(t, err, "daemon must still respond after Stop via BROWSII_PORT client")
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestAttach_StopDoesNotKillDaemon verifies that Stop on an attached client
// leaves the daemon running.
func TestAttach_StopDoesNotKillDaemon(t *testing.T) {
	port := nextPort()
	_, cleanup := startDaemon(t, port)
	defer cleanup()

	c, err := client.Attach(port)
	require.NoError(t, err)

	c.Stop()

	httpClient := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("http://127.0.0.1:%d/ping", port), nil)
	resp, err := httpClient.Do(req)
	require.NoError(t, err, "daemon must still respond after Stop on attached client")
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
