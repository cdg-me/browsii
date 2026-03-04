package client

import "encoding/json"

// NetworkCaptureStart begins recording network requests.
// Use NetworkCaptureOpts to configure which fields to capture, the output
// format, an optional output file, and an optional tab filter.
func (c *Client) NetworkCaptureStart(opts NetworkCaptureOpts) error {
	_, err := c.send("network/capture/start", map[string]any{
		"tab":     opts.Tab,
		"include": opts.Include,
		"format":  opts.Format,
		"output":  opts.Output,
	})
	return err
}

// NetworkCaptureStop stops recording and returns the captured data.
//
// The shape of the result depends on what was configured at start:
//   - format="" or "json", no output file → Result.Requests is populated
//   - format="ndjson" or "har", no output file → Result.Raw contains the bytes
//   - output file was set → Result.OutputPath and Result.Count are set
func (c *Client) NetworkCaptureStop() (*NetworkCaptureStopResult, error) {
	raw, err := c.send("network/capture/stop", nil)
	if err != nil {
		return nil, err
	}

	// Detect file-write confirmation: {"path":"...","count":N}
	var conf struct {
		Path  string `json:"path"`
		Count int    `json:"count"`
	}
	if json.Unmarshal(raw, &conf) == nil && conf.Path != "" {
		return &NetworkCaptureStopResult{
			OutputPath: conf.Path,
			Count:      conf.Count,
		}, nil
	}

	// Try typed JSON array (default format)
	var reqs []NetworkRequest
	if json.Unmarshal(raw, &reqs) == nil {
		return &NetworkCaptureStopResult{
			Requests: reqs,
			Count:    len(reqs),
		}, nil
	}

	// Raw output (ndjson, har without file)
	return &NetworkCaptureStopResult{Raw: raw}, nil
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
// tab filters by tab (same values as NetworkCaptureOpts.Tab).
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
