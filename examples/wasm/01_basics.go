//go:build ignore

package main

import (
	sdk "browsii/sdk"
	"fmt"
)

type Output struct {
	Success string `json:"success"`
	Status  string `json:"status,omitempty"`
}

func main() {
	// 1. Navigate to a test page
	if err := sdk.Navigate("https://example.com"); err != nil {
		sdk.SetResult(map[string]string{"error": fmt.Sprintf("Navigation failed: %v", err)})
		return
	}

	// 2. Wait for the primary H1 to be visible
	if err := sdk.WaitVisible("h1"); err != nil {
		sdk.SetResult(map[string]string{"error": fmt.Sprintf("WaitVisible failed: %v", err)})
		return
	}

	// 3. Return structured JSON matching the daemon's Exit Code taxonomy
	sdk.SetResult(Output{
		Success: "Successfully navigated and verified DOM visibility!",
		Status:  "DOM elements loaded correctly",
	})
}
