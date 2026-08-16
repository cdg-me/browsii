package client

import (
	"encoding/json"
	"strconv"
)

// ExpectOpts configures an Expect assertion. Exactly one primary condition
// must be set; NoConsoleErrors may accompany any of them.
type ExpectOpts struct {
	// Text waits until the text is visible in the page body.
	Text string

	// TextGone waits until the text is no longer visible.
	TextGone string

	// URLPattern waits until location.href matches the glob (* wildcards).
	URLPattern string

	// Selector waits until the element is visible (or hidden with Hidden).
	Selector string

	// Hidden inverts Selector: wait until the element is hidden or gone.
	Hidden bool

	// Ref targets an element from a prior Elements call (with Value, or as
	// the visibility target instead of Selector).
	Ref int

	// Value waits until the target element's value equals it.
	Value string

	// Request waits until a request matching "METHOD glob" fired
	// (e.g. "POST */api/order*"). Observed without a capture session.
	Request string

	// NoConsoleErrors additionally requires zero error-level console
	// entries while waiting.
	NoConsoleErrors bool

	// TimeoutMs bounds the wait (default 5000).
	TimeoutMs int
}

// Expect waits for a page assertion to become true. On failure the daemon
// returns an actionable error (closest text, actual URL/value, or the
// requests that fired instead of the expected one).
func (c *Client) Expect(opts ExpectOpts) error {
	payload := map[string]any{}
	if opts.Text != "" {
		payload["text"] = opts.Text
	}
	if opts.TextGone != "" {
		payload["textGone"] = opts.TextGone
	}
	if opts.URLPattern != "" {
		payload["urlPattern"] = opts.URLPattern
	}
	if opts.Selector != "" {
		payload["selector"] = opts.Selector
	}
	if opts.Hidden {
		payload["hidden"] = true
	}
	if opts.Ref > 0 {
		payload["ref"] = opts.Ref
	}
	if opts.Value != "" {
		payload["value"] = opts.Value
	}
	if opts.Request != "" {
		payload["request"] = opts.Request
	}
	if opts.NoConsoleErrors {
		payload["noConsoleErrors"] = true
	}
	if opts.TimeoutMs > 0 {
		payload["timeoutMs"] = opts.TimeoutMs
	}
	_, err := c.send("expect", payload)
	return err
}

// ClickResult is the receipt attached to evidence-bearing actions:
// what the action actually caused.
type ClickResult struct {
	// Navigated reports that the page URL changed as a result of the action.
	Navigated bool `json:"navigated"`

	// URL is the page URL after the action (post-redirect).
	URL string `json:"url"`

	// Dialogs lists dialogs auto-handled as a result of the action.
	Dialogs []DialogEntry `json:"dialogs"`

	// Requests is the number of network requests the action triggered.
	Requests int `json:"requests"`

	// RequestSamples renders up to 5 of those requests as "METHOD path (status)".
	RequestSamples []string `json:"requestSamples"`

	// ConsoleErrors is the number of error-level console entries observed.
	ConsoleErrors int `json:"consoleErrors"`
}

// ClickWithEvidence clicks and returns the action receipt. target is either
// a CSS selector or a numeric element ref (from Elements) as a string.
func (c *Client) ClickWithEvidence(target string) (*ClickResult, error) {
	raw, err := c.send("click", interactionPayload(target, false))
	if err != nil {
		return nil, err
	}
	var res ClickResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ClickWithEvidenceRef clicks the ref'd element and returns the action receipt.
func (c *Client) ClickWithEvidenceRef(ref int) (*ClickResult, error) {
	raw, err := c.send("click", map[string]any{"ref": ref})
	if err != nil {
		return nil, err
	}
	var res ClickResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// interactionPayload builds a click request for a selector-or-ref target.
func interactionPayload(target string, noEvidence bool) map[string]any {
	payload := map[string]any{}
	if isNumericRef(target) {
		n, _ := strconv.Atoi(target)
		payload["ref"] = n
	} else {
		payload["selector"] = target
	}
	if noEvidence {
		payload["noEvidence"] = true
	}
	return payload
}

func isNumericRef(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
