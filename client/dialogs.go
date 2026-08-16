package client

import "encoding/json"

// DialogEntry is a JavaScript dialog (alert/confirm/prompt/beforeunload)
// that the daemon auto-handled.
type DialogEntry struct {
	Type     string  `json:"type"`
	Message  string  `json:"message"`
	Accepted bool    `json:"accepted"`
	Tab      int     `json:"tab"`
	TS       float64 `json:"ts"`
}

// DialogState is the /dialogs response: current policy plus recent history.
type DialogState struct {
	Policy     string        `json:"policy"`
	PromptText string        `json:"prompt_text"`
	Recent     []DialogEntry `json:"recent"`
}

// Dialogs returns the dialog handling policy and recently auto-handled
// dialogs without changing anything.
func (c *Client) Dialogs() (*DialogState, error) {
	raw, err := c.send("dialogs", nil)
	if err != nil {
		return nil, err
	}
	var state DialogState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// SetDialogPolicy changes how unattended dialogs are resolved.
// policy must be "accept" or "dismiss"; promptText is typed into prompt()
// dialogs before accepting (empty leaves it unchanged). Pass clear=true to
// forget the recent-dialog history.
func (c *Client) SetDialogPolicy(policy, promptText string, clear bool) (*DialogState, error) {
	payload := map[string]any{}
	if policy != "" {
		payload["policy"] = policy
	}
	if promptText != "" {
		payload["prompt_text"] = promptText
	}
	if clear {
		payload["clear"] = true
	}
	raw, err := c.send("dialogs", payload)
	if err != nil {
		return nil, err
	}
	var state DialogState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
