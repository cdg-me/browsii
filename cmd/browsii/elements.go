package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var (
	elementsAll    bool
	elementsFilter string
	elementsJSON   bool
)

// elementCLI mirrors one entry of the daemon's /elements response.
type elementCLI struct {
	Ref      int    `json:"ref"`
	Tag      string `json:"tag"`
	Role     string `json:"role"`
	Text     string `json:"text"`
	Name     string `json:"name"`
	Href     string `json:"href"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	Selector string `json:"selector"`
	Visible  bool   `json:"visible"`
	Disabled bool   `json:"disabled"`
}

func elementLine(e elementCLI) string {
	label := e.Name
	if label == "" {
		label = e.Text
	}
	var parts []string
	if label != "" {
		parts = append(parts, strconv.Quote(label))
	}
	if e.Value != "" {
		parts = append(parts, "= "+strconv.Quote(e.Value))
	}
	if !e.Visible {
		parts = append(parts, "hidden")
	}
	if e.Disabled {
		parts = append(parts, "disabled")
	}
	if e.Href != "" {
		parts = append(parts, "-> "+e.Href)
	}
	line := fmt.Sprintf("[%d] %s", e.Ref, e.Role)
	if len(parts) > 0 {
		line += " " + strings.Join(parts, " ")
	}
	return line + " (" + e.Selector + ")"
}

func init() {
	elementsCmd := &cobra.Command{
		Use:   "elements",
		Short: "Lists interactive elements on the active page with clickable refs",
		Long: `Lists the interactive elements (links, buttons, inputs, selects, roles)
of the active page, each with a stable ref number:

  [1] link "Home" -> /home (#logo)
  [2] textbox "Email address" (#email)
  [3] button "Sign in" (#signin)

Use the ref directly in click/hover/type:  browsii click 2
Refs stay valid until the page changes; a stale ref fails with a hint to
re-run 'elements'. Hidden elements are omitted unless --all is set.`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]any{}
			if elementsAll {
				payload["all"] = true
			}
			if elementsFilter != "" {
				payload["filter"] = elementsFilter
			}

			resp, err := client.SendCommand(port, "elements", payload)
			if err != nil {
				failAction("Elements", err)
			}

			if elementsJSON {
				fmt.Println(strings.TrimRight(string(resp), "\n"))
				return
			}

			var parsed struct {
				Count     int          `json:"count"`
				Truncated bool         `json:"truncated"`
				Elements  []elementCLI `json:"elements"`
			}
			if err := json.Unmarshal(resp, &parsed); err != nil {
				log.Fatalf("Elements failed: unexpected response: %v", err)
			}
			for _, e := range parsed.Elements {
				fmt.Println(elementLine(e))
			}
			if parsed.Truncated {
				fmt.Fprintln(os.Stderr, "warning: element list truncated; use --filter to narrow it down")
			}
		},
	}

	elementsCmd.Flags().BoolVar(&elementsAll, "all", false, "Include hidden elements")
	elementsCmd.Flags().StringVar(&elementsFilter, "filter", "", "Only include elements matching this substring (text, name, role, tag, selector, href)")
	elementsCmd.Flags().BoolVar(&elementsJSON, "json", false, "Print the raw JSON response instead of summary lines")

	rootCmd.AddCommand(elementsCmd)
}
