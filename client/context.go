package client

// ContextCreate creates a new isolated incognito browser context with the given name.
// The name must not be "default".
func (c *Client) ContextCreate(name string) error {
	_, err := c.send("context/create", map[string]any{"name": name})
	return err
}

// ContextSwitch switches the active browser context.
// Pass "" or "default" to switch back to the main context.
func (c *Client) ContextSwitch(name string) error {
	_, err := c.send("context/switch", map[string]any{"name": name})
	return err
}
