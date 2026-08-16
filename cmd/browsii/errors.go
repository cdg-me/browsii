package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// daemonErrorPrefix is added by internal/client.SendCommand when the daemon
// returns a non-2xx response; the remainder is the raw error body.
const daemonErrorPrefix = "daemon returned error: "

// apiErrorBody mirrors the daemon's actionable error payload.
type apiErrorBody struct {
	Error      string                `json:"error"`
	Hint       string                `json:"hint,omitempty"`
	Candidates []elementCandidateCLI `json:"candidates,omitempty"`
}

// elementCandidateCLI is one candidate element from an actionable error.
type elementCandidateCLI struct {
	Ref      int    `json:"ref"`
	Tag      string `json:"tag"`
	Role     string `json:"role"`
	Text     string `json:"text"`
	Name     string `json:"name"`
	Note     string `json:"note"`
	Selector string `json:"selector"`
}

// parseDaemonError extracts the JSON error body from a SendCommand error, if any.
func parseDaemonError(err error) (apiErrorBody, bool) {
	if err == nil {
		return apiErrorBody{}, false
	}
	msg := err.Error()
	idx := strings.Index(msg, daemonErrorPrefix)
	if idx < 0 {
		return apiErrorBody{}, false
	}
	var body apiErrorBody
	if json.Unmarshal([]byte(msg[idx+len(daemonErrorPrefix):]), &body) != nil {
		return apiErrorBody{}, false
	}
	return body, body.Error != ""
}

// failAction prints an interaction failure for an agent: the message, the
// recovery hint, and matching candidate elements (one per line, with refs
// that can be used directly in a retry). Exits 1.
func failAction(verb string, err error) {
	if body, ok := parseDaemonError(err); ok {
		var b strings.Builder
		fmt.Fprintf(&b, "%s failed: %s\n", verb, body.Error)
		if body.Hint != "" {
			fmt.Fprintf(&b, "hint: %s\n", body.Hint)
		}
		for _, c := range body.Candidates {
			label := c.Name
			if label == "" {
				label = c.Text
			}
			line := fmt.Sprintf("  [%d] %s", c.Ref, c.Role)
			if label != "" {
				line += fmt.Sprintf(" %q", label)
			}
			if c.Note != "" {
				line += " (" + c.Note + ")"
			}
			line += " selector: " + c.Selector
			fmt.Fprintln(&b, line)
		}
		fmt.Fprint(os.Stderr, b.String())
		os.Exit(1)
	}
	log.Fatalf("%s failed: %v", verb, err)
}

// isRefArg reports whether an interaction target argument is all digits,
// i.e. an element ref from 'browsii elements' rather than a CSS selector.
func isRefArg(s string) bool {
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

// interactionTarget builds the click/hover/type request payload for a
// selector-or-ref argument: bare numbers are treated as element refs.
func interactionTarget(target string) map[string]any {
	if isRefArg(target) {
		if n, err := strconv.Atoi(target); err == nil {
			return map[string]any{"ref": n}
		}
	}
	return map[string]any{"selector": target}
}

// dialogsBody mirrors the {"dialogs":[...]} payload appended to action
// responses when a dialog was auto-handled.
type dialogsBody struct {
	Dialogs []struct {
		Type     string `json:"type"`
		Message  string `json:"message"`
		Accepted bool   `json:"accepted"`
	} `json:"dialogs"`
}

// printDialogsFromBody prints any auto-handled dialogs embedded in an action
// response body. Bodies without a dialogs field (including empty bodies and
// the plain-text responses of older handlers) print nothing.
func printDialogsFromBody(body []byte) {
	if len(body) == 0 {
		return
	}
	var d dialogsBody
	if json.Unmarshal(body, &d) != nil || len(d.Dialogs) == 0 {
		return
	}
	for _, dg := range d.Dialogs {
		disposition := "dismissed"
		if dg.Accepted {
			disposition = "accepted"
		}
		fmt.Printf("Dialog %s (%s): %q\n", disposition, dg.Type, dg.Message)
	}
}

// printEvidenceFromBody renders the action receipt for the agent: navigation,
// requests, console errors, and any dialogs (from either the evidence body or
// the legacy dialogs-only body).
func printEvidenceFromBody(body []byte) {
	if len(body) == 0 {
		return
	}
	var ev struct {
		Navigated      bool     `json:"navigated"`
		URL            string   `json:"url"`
		RequestSamples []string `json:"requestSamples"`
		ConsoleErrors  int      `json:"consoleErrors"`
		Requests       int      `json:"requests"`
		Dialogs        []struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			Accepted bool   `json:"accepted"`
		} `json:"dialogs"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return
	}
	if ev.Navigated && ev.URL != "" {
		fmt.Printf("→ navigated to %s\n", ev.URL)
	}
	for _, dg := range ev.Dialogs {
		disposition := "dismissed"
		if dg.Accepted {
			disposition = "accepted"
		}
		fmt.Printf("Dialog %s (%s): %q\n", disposition, dg.Type, dg.Message)
	}
	if ev.Requests > 0 {
		shown := ev.RequestSamples
		if len(shown) == 0 {
			fmt.Printf("⟳ %d request(s) fired\n", ev.Requests)
		} else {
			fmt.Printf("⟳ %d request(s): %s\n", ev.Requests, strings.Join(shown, ", "))
		}
	}
	if ev.ConsoleErrors > 0 {
		fmt.Printf("⚠ %d console error(s) — inspect with: browsii console capture start\n", ev.ConsoleErrors)
	}
}
