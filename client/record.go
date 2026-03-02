package client

import "encoding/json"

// RecordStart begins recording user/automation actions under the given name.
func (c *Client) RecordStart(name string) error {
	_, err := c.send("record/start", map[string]any{"name": name})
	return err
}

// RecordStop stops recording and returns a summary of the saved recording.
func (c *Client) RecordStop() (RecordStopResult, error) {
	raw, err := c.send("record/stop", nil)
	if err != nil {
		return RecordStopResult{}, err
	}
	var result RecordStopResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return RecordStopResult{}, err
	}
	return result, nil
}

// RecordReplay replays a saved recording. speed controls playback rate:
// 0 = instant, 1.0 = real-time, 2.0 = double speed.
func (c *Client) RecordReplay(name string, speed float64) error {
	_, err := c.send("record/replay", map[string]any{
		"name":  name,
		"speed": speed,
	})
	return err
}

// RecordList returns all saved recordings.
func (c *Client) RecordList() ([]ListEntry, error) {
	raw, err := c.send("record/list", nil)
	if err != nil {
		return nil, err
	}
	var entries []ListEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// RecordDelete deletes the saved recording with the given name.
func (c *Client) RecordDelete(name string) error {
	_, err := c.send("record/delete", map[string]any{"name": name})
	return err
}
