package client

import "encoding/json"

// SessionSave saves the current browser state (cookies, localStorage, tabs)
// under the given name to ~/.browsii/sessions/.
func (c *Client) SessionSave(name string) error {
	_, err := c.send("session/save", map[string]any{"name": name})
	return err
}

// SessionNew closes all tabs and starts a fresh browser session.
// name is an optional label for the new session.
func (c *Client) SessionNew(name string) error {
	_, err := c.send("session/new", map[string]any{"name": name})
	return err
}

// SessionResume restores a previously saved session.
func (c *Client) SessionResume(name string) error {
	_, err := c.send("session/resume", map[string]any{"name": name})
	return err
}

// SessionList returns all saved sessions sorted by modification time.
func (c *Client) SessionList() ([]ListEntry, error) {
	raw, err := c.send("session/list", nil)
	if err != nil {
		return nil, err
	}
	var entries []ListEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// SessionDelete deletes the saved session with the given name.
func (c *Client) SessionDelete(name string) error {
	_, err := c.send("session/delete", map[string]any{"name": name})
	return err
}
