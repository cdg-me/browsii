//go:build wasip1

package main

import (
	sdk "browsii/sdk"
	"fmt"
)

// CapturedEntry mirrors the fields from each ConsoleEvent.
type CapturedEntry struct {
	Level string       `json:"level"`
	Text  string       `json:"text"`
	Args  []sdk.ConsoleArg `json:"args"`
	Tab   int          `json:"tab"`
}

type ConsoleResult struct {
	Entries []CapturedEntry `json:"entries"`
	Count   int             `json:"count"`
}

func main() {
	var result ConsoleResult

	// Register listener before navigating so we don't miss early console calls.
	sdk.OnConsoleEvent(func(e sdk.ConsoleEvent) {
		result.Entries = append(result.Entries, CapturedEntry{
			Level: e.Level,
			Text:  e.Text,
			Args:  e.Args,
			Tab:   e.Tab,
		})
		result.Count++
	})

	// Navigate to a page that fires all standard console levels plus a multi-arg call.
	target := "data:text/html,<script>" +
		"console.log('hello log');" +
		"console.warn('hello warn');" +
		"console.error('hello error');" +
		"console.info('hello info');" +
		"console.debug('hello debug');" +
		"console.log('multi', 'args', 42);" +
		"</script>"

	if err := sdk.Navigate(target); err != nil {
		sdk.SetResult(map[string]string{"error": fmt.Sprintf("navigate failed: %v", err)})
		return
	}

	// Yield the event loop to let SSE events arrive and flush into callbacks.
	sdk.WaitIdle(1000)

	if result.Count == 0 {
		sdk.SetResult(map[string]string{"warning": "no console events captured"})
		return
	}

	sdk.SetResult(result)
}
