package client

import "encoding/json"

// NetworkCaptureStart begins recording network requests.
// tab filters which tab to capture: "all" or "" (all tabs), "active", "next",
// "last", or a numeric index string like "0".
func (c *Client) NetworkCaptureStart(tab string) error {
	_, err := c.send("network/capture/start", map[string]any{"tab": tab})
	return err
}

// NetworkCaptureStop stops recording and returns all captured requests.
func (c *Client) NetworkCaptureStop() ([]NetworkRequest, error) {
	raw, err := c.send("network/capture/stop", nil)
	if err != nil {
		return nil, err
	}
	var reqs []NetworkRequest
	if err := json.Unmarshal(raw, &reqs); err != nil {
		return nil, err
	}
	return reqs, nil
}

// NetworkThrottle applies network throttling to simulate slow connections.
// latency is in milliseconds; download and upload are in bytes/sec.
// Pass 0 for all three to disable throttling.
func (c *Client) NetworkThrottle(latency, download, upload int) error {
	_, err := c.send("network/throttle", map[string]any{
		"latency":  latency,
		"download": download,
		"upload":   upload,
	})
	return err
}

// NetworkMock intercepts requests matching pattern and responds with the
// provided body, contentType, and statusCode.
func (c *Client) NetworkMock(pattern, body, contentType string, statusCode int) error {
	_, err := c.send("network/mock", map[string]any{
		"pattern":     pattern,
		"body":        body,
		"contentType": contentType,
		"statusCode":  statusCode,
	})
	return err
}

// ConsoleCaptureStart begins recording browser console messages.
// tab filters by tab (same values as NetworkCaptureStart).
// level is a comma-separated allowlist of log levels, e.g. "error,warn".
// Pass "" for level to capture all levels.
func (c *Client) ConsoleCaptureStart(tab, level string) error {
	_, err := c.send("console/capture/start", map[string]any{
		"tab":   tab,
		"level": level,
	})
	return err
}

// ConsoleCaptureStop stops recording console messages and returns all captured entries.
func (c *Client) ConsoleCaptureStop() ([]ConsoleEntry, error) {
	raw, err := c.send("console/capture/stop", nil)
	if err != nil {
		return nil, err
	}
	var entries []ConsoleEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}
