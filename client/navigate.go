package client

// Navigate navigates the active tab to url.
func (c *Client) Navigate(url string) error {
	_, err := c.send("navigate", map[string]any{"url": url})
	return err
}

// Reload reloads the current page and waits for it to load.
func (c *Client) Reload() error {
	_, err := c.send("reload", nil)
	return err
}

// Back navigates back in the browser history.
func (c *Client) Back() error {
	_, err := c.send("back", nil)
	return err
}

// Forward navigates forward in the browser history.
func (c *Client) Forward() error {
	_, err := c.send("forward", nil)
	return err
}

// Scroll scrolls the page. direction is one of "up", "down", "top", "bottom".
// pixels is ignored for "top" and "bottom".
func (c *Client) Scroll(direction string, pixels int) error {
	_, err := c.send("scroll", map[string]any{
		"direction": direction,
		"pixels":    pixels,
	})
	return err
}

// Upload sets the files for a file input element.
func (c *Client) Upload(selector string, files []string) error {
	_, err := c.send("upload", map[string]any{
		"selector": selector,
		"files":    files,
	})
	return err
}
