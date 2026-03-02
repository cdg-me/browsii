//go:build ignore

package main

import (
	sdk "browsii/sdk"
	"fmt"
)

// CapturedRequest mirrors the fields we care about from each NetworkEvent.
type CapturedRequest struct {
	Tab    int    `json:"tab"`
	Method string `json:"method"`
	URL    string `json:"url"`
	Type   string `json:"type"`
}

type Result struct {
	Requests []CapturedRequest `json:"requests"`
	Count    int               `json:"count"`
}

func main() {
	var result Result

	// Register listener before any navigation so we don't miss early requests.
	// Each event carries a "tab" index — tab 0 is always the first page.
	sdk.OnNetworkRequest(func(e sdk.NetworkEvent) {
		result.Requests = append(result.Requests, CapturedRequest{
			Tab:    e.Tab,
			Method: e.Method,
			URL:    e.URL,
			Type:   e.Type,
		})
		result.Count++
	})

	// Navigate tab 0 to a page with sub-resources.
	if err := sdk.Navigate("https://example.com"); err != nil {
		sdk.SetResult(map[string]string{"error": fmt.Sprintf("navigate failed: %v", err)})
		return
	}

	// Give background requests time to land.
	if err := sdk.WaitIdle(2000); err != nil {
		sdk.SetResult(map[string]string{"error": fmt.Sprintf("wait_idle failed: %v", err)})
		return
	}

	if result.Count == 0 {
		sdk.SetResult(map[string]string{"warning": "no network events captured"})
		return
	}

	sdk.SetResult(result)
}
