package client

import "encoding/json"

// FindMatch is one matching line with its line number.
type FindMatch struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// FindResult is the /find response.
type FindResult struct {
	Count   int         `json:"count"`
	Matches []FindMatch `json:"matches"`
}

// Find searches the rendered page text. Exactly one of text or regex must
// be set; regex accepts an optional /pattern/flags form. Returns the true
// match count plus up to three matching lines.
func (c *Client) Find(text, regex string) (*FindResult, error) {
	payload := map[string]any{}
	if text != "" {
		payload["text"] = text
	}
	if regex != "" {
		payload["regex"] = regex
	}
	raw, err := c.send("find", payload)
	if err != nil {
		return nil, err
	}
	var out FindResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ElementDetail is one element's full descriptor: the enumeration fields
// plus all attributes and the owning form.
type ElementDetail struct {
	Element
	Attrs map[string]string `json:"attrs,omitempty"`
	Form  *FormRef          `json:"form,omitempty"`
}

// FormRef identifies the form owning an element.
type FormRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// ElementDetailOf returns one element's full detail. target is a numeric
// ref (as a string) or a CSS selector. Stale refs are healed by
// fingerprint; the resolved selector is included.
func (c *Client) ElementDetailOf(target string) (*ElementDetail, error) {
	payload := map[string]any{}
	if n, ok := refInt(target); ok {
		payload["ref"] = n
	} else {
		payload["selector"] = target
	}
	raw, err := c.send("element", payload)
	if err != nil {
		return nil, err
	}
	var out ElementDetail
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Readable returns the page's article text via Readability. It errors with
// "page is not readerable" when no article-like content exists.
func (c *Client) Readable() (string, error) {
	raw, err := c.send("scrape", map[string]any{"format": "readable"})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
