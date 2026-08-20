package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var (
	expectText      string
	expectTextGone  string
	expectURLPat    string
	expectSelector  string
	expectHidden    bool
	expectEnabled   bool
	expectChecked   bool
	expectUnchecked bool
	expectCount     int // sentinel -1 = unset
	expectDisabled  bool
	expectRef       int
	expectValue     string
	expectRequest   string
	expectNoConsole bool
	expectTimeout   int
)

func init() {
	expectCmd := &cobra.Command{
		Use:   "expect",
		Short: "Waits for a page assertion to become true, or fails with an actionable diff",
		Long: `Verifies that what you expected to happen actually happened. Waits (default 5s)
for the condition, so it doubles as the wait primitive for SPA updates.

Exactly one primary condition; --no-console-errors may accompany any of them:

  browsii expect --text "Saved"                    # text visible anywhere in body
  browsii expect --text-gone "Loading…"            # text disappeared
  browsii expect --url-pattern "*/orders/*"        # URL glob match
  browsii expect --selector ".results"             # element visible
  browsii expect --selector ".spinner" --hidden    # element hidden/gone
  browsii expect --ref 3 --value "user@x.com"      # input value equals
  browsii expect --request "POST */api/order*"     # request fired (observed without capture)
  browsii expect --no-console-errors               # no error-level console entries

Failures explain themselves: closest text on the page, the actual URL/value,
or the requests that fired instead of the expected one.`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]any{}
			if expectText != "" {
				payload["text"] = expectText
			}
			if expectTextGone != "" {
				payload["textGone"] = expectTextGone
			}
			if expectURLPat != "" {
				payload["urlPattern"] = expectURLPat
			}
			if expectSelector != "" {
				payload["selector"] = expectSelector
			}
			if expectHidden {
				payload["hidden"] = true
			}
			if expectEnabled {
				payload["enabled"] = true
			}
			if expectDisabled {
				payload["enabled"] = false
			}
			if expectChecked {
				payload["checked"] = true
			}
			if expectUnchecked {
				payload["checked"] = false
			}
			if expectCount >= 0 {
				payload["count"] = expectCount
				payload["selector"] = expectSelector
			}
			if expectRef > 0 {
				payload["ref"] = expectRef
			}
			if expectValue != "" {
				payload["value"] = expectValue
			}
			if expectRequest != "" {
				payload["request"] = expectRequest
			}
			if expectNoConsole {
				payload["noConsoleErrors"] = true
			}
			if expectTimeout > 0 {
				payload["timeoutMs"] = expectTimeout
			}

			resp, err := client.SendCommand(port, "expect", payload)
			if err != nil {
				if body, ok := parseDaemonError(err); ok {
					fmt.Fprintln(os.Stderr, body.Error)
					if body.Hint != "" {
						fmt.Fprintf(os.Stderr, "hint: %s\n", body.Hint)
					}
					os.Exit(1)
				}
				log.Fatalf("Expect failed: %v", err)
			}

			fmt.Printf("OK: %s\n", mustDetail(resp))
		},
	}

	expectCmd.Flags().StringVar(&expectText, "text", "", "Wait until this text is visible in the page body")
	expectCmd.Flags().StringVar(&expectTextGone, "text-gone", "", "Wait until this text is no longer visible")
	expectCmd.Flags().StringVar(&expectURLPat, "url-pattern", "", "Wait until location.href matches this glob (* wildcards)")
	expectCmd.Flags().StringVar(&expectSelector, "selector", "", "Wait until this element is visible (or hidden with --hidden, enabled/disabled with --enabled/--disabled)")
	expectCmd.Flags().BoolVar(&expectEnabled, "enabled", false, "With --selector: wait until the element is enabled")
	expectCmd.Flags().BoolVar(&expectChecked, "checked", false, "With --selector: wait until the checkbox/radio is checked")
	expectCmd.Flags().BoolVar(&expectUnchecked, "unchecked", false, "With --selector: wait until the checkbox/radio is unchecked")
	expectCmd.Flags().IntVar(&expectCount, "count", -1, "With --selector: wait until exactly N elements match (0 = none remain)")
	expectCmd.Flags().BoolVar(&expectDisabled, "disabled", false, "With --selector: wait until the element is disabled")
	expectCmd.Flags().BoolVar(&expectHidden, "hidden", false, "With --selector: wait until the element is hidden or gone")
	expectCmd.Flags().IntVar(&expectRef, "ref", 0, "Element ref from 'elements' (for --value, or visibility)")
	expectCmd.Flags().StringVar(&expectValue, "value", "", "Wait until the element's value equals this (needs --ref or --selector)")
	expectCmd.Flags().StringVar(&expectRequest, "request", "", "Wait until a request matching METHOD glob fired (e.g. \"POST */api/order*\")")
	expectCmd.Flags().BoolVar(&expectNoConsole, "no-console-errors", false, "Also require zero error-level console entries during the wait")
	expectCmd.Flags().IntVar(&expectTimeout, "timeout", 0, "Timeout in milliseconds (default 5000)")

	rootCmd.AddCommand(expectCmd)
}

// mustDetail extracts the "detail" field from a successful expect response.
func mustDetail(resp []byte) string {
	var parsed struct {
		OK     bool   `json:"ok"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil || parsed.Detail == "" {
		return "condition met"
	}
	return parsed.Detail
}
