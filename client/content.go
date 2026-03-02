package client

import "encoding/json"

// Scrape returns the current page content in the requested format.
func (c *Client) Scrape(format ScrapeFormat) (string, error) {
	raw, err := c.send("scrape", map[string]any{"format": string(format)})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// Links returns all href values on the page. pattern is an optional substring
// filter; pass "" to return all links.
func (c *Client) Links(pattern string) ([]string, error) {
	raw, err := c.send("links", map[string]any{"pattern": pattern})
	if err != nil {
		return nil, err
	}
	var links []string
	if err := json.Unmarshal(raw, &links); err != nil {
		return nil, err
	}
	return links, nil
}

// Screenshot saves a screenshot to filename. element is an optional CSS selector
// to capture only that element. Set fullPage to capture the full scrollable page.
func (c *Client) Screenshot(filename, element string, fullPage bool) error {
	_, err := c.send("screenshot", map[string]any{
		"filename": filename,
		"element":  element,
		"fullPage": fullPage,
	})
	return err
}

// PDF saves the current page as a PDF to filename.
func (c *Client) PDF(filename string) error {
	_, err := c.send("pdf", map[string]any{"filename": filename})
	return err
}

// JS executes script in the page context and returns the JSON-encoded result.
func (c *Client) JS(script string) (json.RawMessage, error) {
	raw, err := c.send("js", map[string]any{"script": script})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// Cookies returns all cookies for the current page.
func (c *Client) Cookies() ([]map[string]any, error) {
	raw, err := c.send("cookies", nil)
	if err != nil {
		return nil, err
	}
	var cookies []map[string]any
	if err := json.Unmarshal(raw, &cookies); err != nil {
		return nil, err
	}
	return cookies, nil
}
