package client

import "encoding/json"

// InjectJSAdd registers script to run before any other scripts on future
// document loads. tab uses the standard tab filter ("", "all", "active",
// "next", "last", or a numeric index string). Returns the stable entry ID.
func (c *Client) InjectJSAdd(script, tab string) (string, error) {
	return c.injectJSAdd(script, "", tab)
}

// InjectJSAddURL fetches url server-side at registration time, inlines the
// content, and registers it identically to InjectJSAdd. Returns the entry ID.
func (c *Client) InjectJSAddURL(url, tab string) (string, error) {
	return c.injectJSAdd("", url, tab)
}

func (c *Client) injectJSAdd(script, url, tab string) (string, error) {
	raw, err := c.send("inject/js/add", map[string]any{
		"script": script,
		"url":    url,
		"tab":    tab,
	})
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	return result.ID, nil
}

// InjectJSList returns registered inject-js entries. tab filters by scope;
// pass "" to return all entries across all tabs.
func (c *Client) InjectJSList(tab string) ([]InjectJSEntry, error) {
	raw, err := c.send("inject/js/list", map[string]any{"tab": tab})
	if err != nil {
		return nil, err
	}
	var entries []InjectJSEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// InjectJSClear deregisters inject-js scripts, stopping them from firing on
// future navigations. Pass "" to clear all scopes, or a tab filter to clear
// only that tab's per-tab entries (global entries are unaffected by a
// tab-scoped clear).
func (c *Client) InjectJSClear(tab string) error {
	_, err := c.send("inject/js/clear", map[string]any{"tab": tab})
	return err
}
