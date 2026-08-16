package client

import "encoding/json"

// ElementRect is the viewport-space bounding box of an element.
type ElementRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Element describes one interactive element returned by Elements.
type Element struct {
	Ref      int          `json:"ref"`
	Tag      string       `json:"tag"`
	Role     string       `json:"role"`
	Text     string       `json:"text,omitempty"`
	Name     string       `json:"name,omitempty"`
	Href     string       `json:"href,omitempty"`
	Type     string       `json:"type,omitempty"`
	Value    string       `json:"value,omitempty"`
	Checked  *bool        `json:"checked,omitempty"`
	Selector string       `json:"selector"`
	Visible  bool         `json:"visible"`
	Disabled bool         `json:"disabled,omitempty"`
	Rect     *ElementRect `json:"rect,omitempty"`
}

// ElementList is the /elements response.
type ElementList struct {
	Count     int       `json:"count"`
	Truncated bool      `json:"truncated"`
	Elements  []Element `json:"elements"`
}

// ElementOpts filters an Elements call.
type ElementOpts struct {
	// All includes hidden elements (default: visible only).
	All bool

	// Filter restricts results to elements whose text, name, role, tag,
	// selector, or href contains the substring (case-insensitive).
	Filter string
}

// Elements enumerates the interactive elements of the active page. The refs
// in the result can be passed to ClickRef/HoverRef/TypeRef. Refs stay valid
// until the page changes or the next Elements call.
func (c *Client) Elements(opts ElementOpts) (*ElementList, error) {
	payload := map[string]any{}
	if opts.All {
		payload["all"] = true
	}
	if opts.Filter != "" {
		payload["filter"] = opts.Filter
	}
	raw, err := c.send("elements", payload)
	if err != nil {
		return nil, err
	}
	var list ElementList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// ClickRef clicks the element with the given ref from a prior Elements call.
func (c *Client) ClickRef(ref int) error {
	_, err := c.send("click", map[string]any{"ref": ref})
	return err
}

// HoverRef hovers the element with the given ref from a prior Elements call.
func (c *Client) HoverRef(ref int) error {
	_, err := c.send("hover", map[string]any{"ref": ref})
	return err
}

// TypeRef clears the ref'd element's value and types text into it.
func (c *Client) TypeRef(ref int, text string) error {
	_, err := c.send("type", map[string]any{"ref": ref, "text": text})
	return err
}
