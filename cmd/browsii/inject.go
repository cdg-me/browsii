package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var (
	injectTab string
	injectURL string
)

func init() {
	injectCmd := &cobra.Command{
		Use:   "inject",
		Short: "Inject scripts and resources into pages before they load",
	}

	// ── inject js ──────────────────────────────────────────────────────────────

	injectJSCmd := &cobra.Command{
		Use:   "js",
		Short: "Register JS to run before any other scripts on future page loads",
	}

	// inject js add

	injectJSAddCmd := &cobra.Command{
		Use:   "add [<script>]",
		Short: "Register a JS snippet (or URL) to inject before page scripts",
		Long: `Registers JavaScript to run before any other scripts on every future page
load for the targeted tab(s). The registration persists until 'inject js clear'
is called or the daemon stops.

Provide raw JS source as a positional argument, or a URL with --url.
Exactly one of the two must be given.

When --url is used the content is fetched eagerly at registration time: the
script will still run correctly even if the origin becomes unreachable later.

Examples:
  # Inject a global flag
  browsii inject js add "window.__testMode = true;"

  # Multi-statement block
  browsii inject js add "window.__a = 1; window.__b = 2;"

  # Load and inline an external script at registration time
  browsii inject js add --url https://cdn.example.com/polyfill.min.js

  # Inject only into the current active tab
  browsii inject js add --tab active "window.__local = true;"

  # Inject into the next tab that will be opened
  browsii inject js add --tab next "window.__nextOnly = true;"`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			script := ""
			if len(args) == 1 {
				script = args[0]
			}

			// Validate: exactly one of script/url.
			if script == "" && injectURL == "" {
				log.Fatal("inject js add: provide a <script> argument or --url, not neither")
			}
			if script != "" && injectURL != "" {
				log.Fatal("inject js add: provide a <script> argument or --url, not both")
			}

			payload := map[string]interface{}{
				"script": script,
				"url":    injectURL,
				"tab":    injectTab,
			}

			resp, err := client.SendCommand(port, "inject/js/add", payload)
			if err != nil {
				log.Fatalf("inject js add failed: %v", err)
			}

			// Print the assigned ID so callers can reference it.
			var result map[string]string
			if jsonErr := json.Unmarshal(resp, &result); jsonErr == nil {
				if id, ok := result["id"]; ok {
					fmt.Println(id)
					return
				}
			}
			fmt.Println(string(resp))
		},
	}
	injectJSAddCmd.Flags().StringVar(&injectTab, "tab", "",
		`Tab filter: "active", "next", "last", "<index>", or omit for all tabs`)
	injectJSAddCmd.Flags().StringVar(&injectURL, "url", "",
		"Fetch this URL server-side and inline its content as the injected script")

	// inject js list

	injectJSListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered inject-js entries",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]interface{}{"tab": injectTab}
			resp, err := client.SendCommand(port, "inject/js/list", payload)
			if err != nil {
				log.Fatalf("inject js list failed: %v", err)
			}
			fmt.Println(string(resp))
		},
	}
	injectJSListCmd.Flags().StringVar(&injectTab, "tab", "",
		`Tab filter: "active", "next", "last", "<index>", or omit for all tabs`)

	// inject js clear

	injectJSClearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Deregister inject-js scripts so they stop firing on future navigations",
		Long: `Removes registered inject-js scripts. The current document is not affected;
cleared scripts will simply not run on the next navigation.

With no --tab flag, all inject-js entries (global and per-tab) are cleared.
With --tab active (or a specific index), only that tab's per-tab entries are
cleared; global entries remain active.`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]interface{}{"tab": injectTab}
			_, err := client.SendCommand(port, "inject/js/clear", payload)
			if err != nil {
				log.Fatalf("inject js clear failed: %v", err)
			}
			fmt.Println("inject scripts cleared")
		},
	}
	injectJSClearCmd.Flags().StringVar(&injectTab, "tab", "",
		`Tab filter: "active", "next", "last", "<index>", or omit to clear all`)

	injectJSCmd.AddCommand(injectJSAddCmd, injectJSListCmd, injectJSClearCmd)
	injectCmd.AddCommand(injectJSCmd)
	rootCmd.AddCommand(injectCmd)
}
