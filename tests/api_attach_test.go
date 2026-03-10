package tests

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cdg-me/browsii/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
