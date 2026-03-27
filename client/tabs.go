package client

import "encoding/json"

// TabNew opens a new tab. If url is empty, a blank tab is opened.
func (c *Client) TabNew(url string) error {
	_, err := c.send("tab/new", map[string]any{"url": url})
	return err
}

// TabNewBackground opens a new tab in the background without activating it.
// If url is empty, a blank tab is opened.
func (c *Client) TabNewBackground(url string) error {
	_, err := c.send("tab/new", map[string]any{"url": url, "background": true})
	return err
}

// TabList returns all currently open tabs ordered by their tab index.
func (c *Client) TabList() ([]Tab, error) {
	raw, err := c.send("tab/list", nil)
	if err != nil {
		return nil, err
	}
	var tabs []Tab
	if err := json.Unmarshal(raw, &tabs); err != nil {
		return nil, err
	}
	return tabs, nil
}

// TabClose closes the active tab.
func (c *Client) TabClose() error {
	_, err := c.send("tab/close", nil)
	return err
}

// TabSwitch switches to the tab at the given zero-based index.
func (c *Client) TabSwitch(index int) error {
	_, err := c.send("tab/switch", map[string]any{"index": index})
	return err
}
