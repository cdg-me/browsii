package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var findRegex string

func init() {
	findCmd := &cobra.Command{
		Use:   "find <text> | --regex <pattern>",
		Short: "Searches the rendered page text and returns matching lines",
		Long: `Searches the rendered page text (like grep over the page).

  browsii find "regulatory"
  browsii find --regex 'Total: \$\d+'
  browsii find --regex '/sign (in|up)/i'

Prints the match count and up to 3 matching lines with line numbers.
Exits 1 when nothing matches (grep convention). Searches text content;
for interactive controls use 'elements --filter'.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if findRegex != "" && len(args) == 1 {
				fmt.Fprintln(os.Stderr, "provide text or --regex, not both")
				os.Exit(1)
			}
			payload := map[string]any{}
			if findRegex != "" {
				payload["regex"] = findRegex
			} else if len(args) == 1 {
				payload["text"] = args[0]
			} else {
				fmt.Fprintln(os.Stderr, "provide text or --regex")
				os.Exit(1)
			}

			resp, err := client.SendCommand(port, "find", payload)
			if err != nil {
				failAction("Find", err)
			}
			var out struct {
				Count   int `json:"count"`
				Matches []struct {
					Line int    `json:"line"`
					Text string `json:"text"`
				} `json:"matches"`
			}
			if err := json.Unmarshal(resp, &out); err != nil {
				fmt.Fprintf(os.Stderr, "Find failed: unexpected response: %v\n", err)
				os.Exit(1)
			}
			if out.Count == 0 {
				fmt.Println("0 matches")
				os.Exit(1)
			}
			fmt.Printf("%d match(es)\n", out.Count)
			for _, m := range out.Matches {
				fmt.Printf("  %d: %s\n", m.Line, m.Text)
			}
		},
	}
	findCmd.Flags().StringVar(&findRegex, "regex", "", "Regular expression (optionally /pattern/flags form)")

	rootCmd.AddCommand(findCmd)

	var elementJSON bool
	elementCmd := &cobra.Command{
		Use:   "element <ref-or-selector>",
		Short: "Returns one element's full detail (attrs, form, rect, state)",
		Long: `Returns one element's full detail by ref or selector.

  browsii element 12
  browsii element "#checkout"
  browsii element "#checkout" --json

A stale ref is healed by fingerprint; the resolved selector is included
in the output.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			payload := interactionTarget(args[0])

			resp, err := client.SendCommand(port, "element", payload)
			if err != nil {
				failAction("Element", err)
			}
			if elementJSON {
				fmt.Println(string(resp))
				return
			}
			var e struct {
				Ref      int    `json:"ref"`
				Tag      string `json:"tag"`
				Role     string `json:"role"`
				Text     string `json:"text"`
				Name     string `json:"name"`
				Value    string `json:"value"`
				Checked  *bool  `json:"checked"`
				Visible  bool   `json:"visible"`
				Disabled bool   `json:"disabled"`
				Selector string `json:"selector"`
				Rect     *struct {
					X int `json:"x"`
					Y int `json:"y"`
					W int `json:"w"`
					H int `json:"h"`
				} `json:"rect"`
				Attrs map[string]string `json:"attrs"`
				Form  *struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"form"`
			}
			if err := json.Unmarshal(resp, &e); err != nil {
				fmt.Fprintf(os.Stderr, "Element failed: unexpected response: %v\n", err)
				os.Exit(1)
			}
			label := e.Name
			if label == "" {
				label = e.Text
			}
			fmt.Printf("[%d] %s %q (%s)\n", e.Ref, e.Role, label, e.Selector)
			if e.Value != "" {
				fmt.Printf("  value: %q\n", e.Value)
			}
			if e.Checked != nil {
				fmt.Printf("  checked: %v\n", *e.Checked)
			}
			fmt.Printf("  visible: %v disabled: %v\n", e.Visible, e.Disabled)
			if e.Rect != nil {
				fmt.Printf("  rect: x=%d y=%d w=%d h=%d\n", e.Rect.X, e.Rect.Y, e.Rect.W, e.Rect.H)
			}
			if e.Form != nil {
				fmt.Printf("  form: %s\n", e.Form.ID)
			}
			for k, v := range e.Attrs {
				fmt.Printf("  %s=%q\n", k, v)
			}
		},
	}
	elementCmd.Flags().BoolVar(&elementJSON, "json", false, "Print the raw JSON response")

	rootCmd.AddCommand(elementCmd)
}
