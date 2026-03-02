package client

// Click clicks the first element matching selector.
func (c *Client) Click(selector string) error {
	_, err := c.send("click", map[string]any{"selector": selector})
	return err
}

// Type clears selector's value and types text into it.
func (c *Client) Type(selector, text string) error {
	_, err := c.send("type", map[string]any{
		"selector": selector,
		"text":     text,
	})
	return err
}

// Press presses a key or key combination, e.g. "Enter", "Control+a".
func (c *Client) Press(key string) error {
	_, err := c.send("press", map[string]any{"key": key})
	return err
}

// Hover moves the mouse over the first element matching selector.
func (c *Client) Hover(selector string) error {
	_, err := c.send("hover", map[string]any{"selector": selector})
	return err
}

// MouseMove moves the mouse cursor to the given viewport coordinates.
func (c *Client) MouseMove(x, y float64) error {
	_, err := c.send("mouse/move", map[string]any{"x": x, "y": y})
	return err
}

// MouseDrag drags from (x1,y1) to (x2,y2) in steps intermediate positions.
func (c *Client) MouseDrag(x1, y1, x2, y2 float64, steps int) error {
	_, err := c.send("mouse/drag", map[string]any{
		"x1": x1, "y1": y1,
		"x2": x2, "y2": y2,
		"steps": steps,
	})
	return err
}

// MouseRightClick right-clicks the first element matching selector.
func (c *Client) MouseRightClick(selector string) error {
	_, err := c.send("mouse/rightclick", map[string]any{"selector": selector})
	return err
}

// MouseDoubleClick double-clicks the first element matching selector.
func (c *Client) MouseDoubleClick(selector string) error {
	_, err := c.send("mouse/doubleclick", map[string]any{"selector": selector})
	return err
}
