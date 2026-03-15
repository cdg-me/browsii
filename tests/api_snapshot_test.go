package tests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cdg-me/browsii/client"
)

// TestSnapshot_RecordAndReplay is the primary snapshot scenario:
//
//  1. Start a local mock server serving a page with known content.
//  2. Navigate to it and capture the response as a HAR file.
//  3. Shut the mock server down — the URL is now unreachable.
//  4. Load the HAR snapshot.
//  5. Navigate to the same (now-dead) URL.
//  6. Scrape — the recorded content must be returned from the interceptor.
//
// This proves the full record-then-replay loop that LLMs use to test
// browser scripts against a fixed, offline copy of a page.
func TestSnapshot_RecordAndReplay(t *testing.T) {
	srv := setupNamedServer("Snapshot Works")

	port := nextPort()
	_, cleanup := startDaemon(t, port)
	defer cleanup()

	c, err := client.Attach(port)
	require.NoError(t, err)

	harPath := filepath.Join(t.TempDir(), "snap.har")

	// Record the page as HAR.
	require.NoError(t, c.NetworkCaptureStart(client.NetworkCaptureOpts{
		Include: []string{"response-headers", "response-body"},
		Format:  "har",
		Output:  harPath,
	}))
	require.NoError(t, c.Navigate(srv.URL+"/"))
	_, err = c.NetworkCaptureStop()
	require.NoError(t, err)

	// Server is gone — URL is now dead.
	srv.Close()

	// Load snapshot and navigate to the dead URL.
	require.NoError(t, c.SnapshotLoad(harPath))
	require.NoError(t, c.Navigate(srv.URL+"/"))

	text, err := c.Scrape(client.Text)
	require.NoError(t, err)
	assert.Contains(t, text, "Snapshot Works")
}

// TestSnapshot_UnrecordedURLsPassThrough verifies that requests for URLs not
// present in the snapshot are forwarded to the network as normal.
func TestSnapshot_UnrecordedURLsPassThrough(t *testing.T) {
	snapshotSrv := setupNamedServer("Snapshotted")
	liveSrv := setupNamedServer("Live Server")
	defer liveSrv.Close()

	port := nextPort()
	_, cleanup := startDaemon(t, port)
	defer cleanup()

	c, err := client.Attach(port)
	require.NoError(t, err)

	harPath := filepath.Join(t.TempDir(), "snap.har")

	// Record only the snapshot server.
	require.NoError(t, c.NetworkCaptureStart(client.NetworkCaptureOpts{
		Include: []string{"response-headers", "response-body"},
		Format:  "har",
		Output:  harPath,
	}))
	require.NoError(t, c.Navigate(snapshotSrv.URL+"/"))
	_, err = c.NetworkCaptureStop()
	require.NoError(t, err)

	snapshotSrv.Close() // snapshot server now dead

	require.NoError(t, c.SnapshotLoad(harPath))

	// Snapshotted URL — served from HAR even though server is down.
	require.NoError(t, c.Navigate(snapshotSrv.URL+"/"))
	text, err := c.Scrape(client.Text)
	require.NoError(t, err)
	assert.Contains(t, text, "Snapshotted")

	// Live URL — not in HAR, must reach the real server.
	require.NoError(t, c.Navigate(liveSrv.URL+"/"))
	text, err = c.Scrape(client.Text)
	require.NoError(t, err)
	assert.Contains(t, text, "Live Server")
}

// TestSnapshot_ClearRestoresNetwork verifies that SnapshotClear removes the
// interceptor so live requests resume normal (failing) behaviour.
func TestSnapshot_ClearRestoresNetwork(t *testing.T) {
	srv := setupNamedServer("Original")

	port := nextPort()
	_, cleanup := startDaemon(t, port)
	defer cleanup()

	c, err := client.Attach(port)
	require.NoError(t, err)

	harPath := filepath.Join(t.TempDir(), "snap.har")

	require.NoError(t, c.NetworkCaptureStart(client.NetworkCaptureOpts{
		Include: []string{"response-headers", "response-body"},
		Format:  "har",
		Output:  harPath,
	}))
	require.NoError(t, c.Navigate(srv.URL+"/"))
	_, err = c.NetworkCaptureStop()
	require.NoError(t, err)

	srv.Close()

	require.NoError(t, c.SnapshotLoad(harPath))
	require.NoError(t, c.SnapshotClear())

	// After clearing, the URL is truly dead — Navigate must error.
	err = c.Navigate(srv.URL + "/")
	assert.Error(t, err, "expected Navigate to fail after snapshot cleared and server shut down")
}

// TestSnapshot_AppliesToAllTabs verifies that the snapshot router operates at
// the browser level: a HAR loaded while tab 0 is active also intercepts
// requests made by a brand-new tab opened afterwards. This is the property that
// makes LLM test flows repeatable — the snapshot applies to every tab the LLM
// opens, not just the one that was current at load time.
func TestSnapshot_AppliesToAllTabs(t *testing.T) {
	srvA := setupNamedServer("Tab A Content")
	srvB := setupNamedServer("Tab B Content")

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	c, err := client.Attach(port)
	require.NoError(t, err)

	harPath := filepath.Join(t.TempDir(), "multitab.har")

	// Record both servers from tab 0.
	require.NoError(t, c.NetworkCaptureStart(client.NetworkCaptureOpts{
		Include: []string{"response-headers", "response-body"},
		Format:  "har",
		Output:  harPath,
	}))
	require.NoError(t, c.Navigate(srvA.URL+"/"))
	require.NoError(t, c.Navigate(srvB.URL+"/"))
	_, err = c.NetworkCaptureStop()
	require.NoError(t, err)

	srvA.Close()
	srvB.Close()

	// Load snapshot — snapshot router is now browser-level.
	require.NoError(t, c.SnapshotLoad(harPath))

	// Tab 0: srvA served from snapshot.
	require.NoError(t, c.Navigate(srvA.URL+"/"))
	text, err := c.Scrape(client.Text)
	require.NoError(t, err)
	assert.Contains(t, text, "Tab A Content")

	// Open a new tab (tab 1) — snapshot must still apply.
	runCLI(t, bin, port, "tab", "new")

	require.NoError(t, c.Navigate(srvB.URL+"/"))
	text, err = c.Scrape(client.Text)
	require.NoError(t, err)
	assert.Contains(t, text, "Tab B Content", "snapshot must cover new tabs opened after load")
}

// TestSnapshot_MultipleURLsAllServedOffline records two different servers into
// one HAR, shuts both down, loads the snapshot, and verifies both URLs are
// served from the interceptor.
func TestSnapshot_MultipleURLsAllServedOffline(t *testing.T) {
	srvA := setupNamedServer("Page A")
	srvB := setupNamedServer("Page B")

	port := nextPort()
	_, cleanup := startDaemon(t, port)
	defer cleanup()

	c, err := client.Attach(port)
	require.NoError(t, err)

	harPath := filepath.Join(t.TempDir(), "multi.har")

	require.NoError(t, c.NetworkCaptureStart(client.NetworkCaptureOpts{
		Include: []string{"response-headers", "response-body"},
		Format:  "har",
		Output:  harPath,
	}))
	require.NoError(t, c.Navigate(srvA.URL+"/"))
	require.NoError(t, c.Navigate(srvB.URL+"/"))
	_, err = c.NetworkCaptureStop()
	require.NoError(t, err)

	srvA.Close()
	srvB.Close()

	require.NoError(t, c.SnapshotLoad(harPath))

	require.NoError(t, c.Navigate(srvA.URL+"/"))
	textA, err := c.Scrape(client.Text)
	require.NoError(t, err)
	assert.Contains(t, textA, "Page A")

	require.NoError(t, c.Navigate(srvB.URL+"/"))
	textB, err := c.Scrape(client.Text)
	require.NoError(t, err)
	assert.Contains(t, textB, "Page B")
}
