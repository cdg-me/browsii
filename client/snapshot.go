package client

// SnapshotLoad reads a HAR file at path and installs a network interceptor on
// the active page. Requests whose URL appears in the HAR are served from the
// recorded response; all other requests pass through normally.
//
// Calling SnapshotLoad again replaces the previous snapshot.
// Call SnapshotClear to restore normal network behaviour.
//
// Record a HAR with:
//
//	c.NetworkCaptureStart(NetworkCaptureOpts{
//	    Include: []string{"response-headers", "response-body"},
//	    Format:  "har",
//	    Output:  "testdata/snap.har",
//	})
//	c.Navigate("https://example.com")
//	c.NetworkCaptureStop()
func (c *Client) SnapshotLoad(path string) error {
	_, err := c.send("snapshot/load", map[string]string{"path": path})
	return err
}

// SnapshotClear stops the active snapshot router and restores normal network
// behaviour. Safe to call when no snapshot is loaded.
func (c *Client) SnapshotClear() error {
	_, err := c.send("snapshot/clear", nil)
	return err
}
