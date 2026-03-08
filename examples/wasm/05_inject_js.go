//go:build ignore

package main

import (
	sdk "browsii/sdk"
	"fmt"
)

type Result struct {
	RegisteredIDs []string `json:"registeredIDs"`
	Errors        []string `json:"errors,omitempty"`
}

func main() {
	var result Result

	id, err := sdk.InjectJS("window.__sdkGlobal = true;")
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("inject global: %v", err))
		sdk.SetResult(result)
		return
	}
	result.RegisteredIDs = append(result.RegisteredIDs, id)

	if err := sdk.Navigate("https://example.com"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("navigate: %v", err))
		sdk.SetResult(result)
		return
	}

	if err := sdk.WaitVisible("h1"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("wait visible: %v", err))
		sdk.SetResult(result)
		return
	}

	if err := sdk.InjectJSClear(""); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
		sdk.SetResult(result)
		return
	}

	for _, script := range []string{
		`window.__sdkOrder = []; window.__sdkOrder.push("first");`,
		`window.__sdkOrder.push("second");`,
		`window.__sdkOrder.push("third");`,
	} {
		id, err := sdk.InjectJS(script)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("inject order script: %v", err))
			sdk.SetResult(result)
			return
		}
		result.RegisteredIDs = append(result.RegisteredIDs, id)
	}

	if err := sdk.Navigate("https://example.com"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("navigate order: %v", err))
		sdk.SetResult(result)
		return
	}

	if err := sdk.InjectJSClear(""); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("final clear: %v", err))
		sdk.SetResult(result)
		return
	}

	if err := sdk.Navigate("https://example.com"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("navigate after clear: %v", err))
		sdk.SetResult(result)
		return
	}

	sdk.SetResult(result)
}
