//go:build ignore

package main

import (
	sdk "browsii/sdk"
	"fmt"
)

type EventLog struct {
	TrackedURLs []string `json:"urls"`
	EventCount  int      `json:"count"`
}

func main() {
	var results EventLog

	// e.Tab holds the 0-based index of the tab that fired the request.
	// Use sdk.Navigate after registering the listener so early requests are not missed.
	sdk.OnNetworkRequest(func(e sdk.NetworkEvent) {
		if e.URL != "" {
			results.TrackedURLs = append(results.TrackedURLs, e.URL)
			results.EventCount++
		}
	})

	if err := sdk.Navigate("https://news.ycombinator.com"); err != nil {
		sdk.SetResult(map[string]string{"error": fmt.Sprintf("Navigation failed: %v", err)})
		return
	}

	// WaitIdle yields control back to the host so SSE events buffered during
	// navigation are dispatched into the guest before we read results.
	if err := sdk.WaitIdle(5000); err != nil {
		sdk.SetResult(map[string]string{"error": fmt.Sprintf("WaitIdle failed: %v", err)})
		return
	}

	// Return the aggregated data
	if results.EventCount == 0 {
		sdk.SetResult(map[string]string{"warning": "No network events intercepted"})
	} else {
		sdk.SetResult(results)
	}
}
