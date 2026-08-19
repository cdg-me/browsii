package client

import "encoding/json"

// FillField is one field of a Fill batch. Exactly one of Ref or Selector.
type FillField struct {
	Ref      int    `json:"ref,omitempty"`
	Selector string `json:"selector,omitempty"`
	Value    string `json:"value"`
}

// FillFailure reports one field that could not be set, with candidates.
type FillFailure struct {
	Field      string `json:"field"`
	Error      string `json:"error"`
	Hint       string `json:"hint"`
	Candidates []struct {
		Ref      int    `json:"ref"`
		Role     string `json:"role"`
		Text     string `json:"text"`
		Name     string `json:"name"`
		Note     string `json:"note"`
		Selector string `json:"selector"`
	} `json:"candidates"`
}

// FillResult is the /fill response (receipt fields included).
type FillResult struct {
	Filled   int           `json:"filled"`
	Failures []FillFailure `json:"failures"`

	Navigated      bool     `json:"navigated"`
	URL            string   `json:"url"`
	Requests       int      `json:"requests"`
	RequestSamples []string `json:"requestSamples"`
	ConsoleErrors  int      `json:"consoleErrors"`
}

// Fill sets multiple form fields in one call. With submit true, the form's
// submit button is clicked after all fields are set.
func (c *Client) Fill(fields []FillField, submit bool) (*FillResult, error) {
	raw, err := c.send("fill", map[string]any{"fields": fields, "submit": submit})
	if err != nil {
		return nil, err
	}
	var out FillResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Select picks an option in a select element. option matches the option's
// value first, then its label.
func (c *Client) Select(target, option string) ([]string, error) {
	payload := map[string]any{"value": option}
	if n, ok := refInt(target); ok {
		payload["ref"] = n
	} else {
		payload["selector"] = target
	}
	raw, err := c.send("select", payload)
	if err != nil {
		return nil, err
	}
	var out struct {
		Selected []string `json:"selected"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Selected, nil
}

// CheckResult reports the state transition of a check action.
type CheckResult struct {
	Was bool `json:"was"`
	Now bool `json:"now"`
}

// Check checks (or unchecks with checked=false) a checkbox or radio.
func (c *Client) Check(target string, checked bool) (*CheckResult, error) {
	payload := map[string]any{"checked": checked}
	if n, ok := refInt(target); ok {
		payload["ref"] = n
	} else {
		payload["selector"] = target
	}
	raw, err := c.send("check", payload)
	if err != nil {
		return nil, err
	}
	var out CheckResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// refInt parses an all-digit target as an element ref.
func refInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}
