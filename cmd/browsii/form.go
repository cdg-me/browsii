package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var (
	fillJSON     string
	fillSubmit   bool
	fillNoEvid   bool
	selectMulti  bool
	selectNoEvid bool
	checkOff     bool
	checkNoEvid  bool
)

func init() {
	fillCmd := &cobra.Command{
		Use:   "fill --field <json> [--field …] [--submit]",
		Short: "Sets multiple form fields in one call",
		Long: `Sets multiple form fields in one call, then optionally submits the form.

Each --field takes JSON: {"ref":2,"value":"a@b.c"} or
{"selector":"#phone","value":"555-0100"}. Values replace existing content
and work with framework inputs (React, Vue). Per-field failures are
reported with candidates; other fields still apply. --submit clicks the
form's submit button after all fields are set (skipped when any field
failed).`,
		Run: func(cmd *cobra.Command, args []string) {
			var fields []map[string]any
			if fillJSON != "" {
				if err := json.Unmarshal([]byte(fillJSON), &fields); err != nil {
					log.Fatalf("invalid --json payload: %v", err)
				}
			}
			for _, f := range fillFieldSpecs {
				var m map[string]any
				if err := json.Unmarshal([]byte(f), &m); err != nil {
					log.Fatalf("invalid --field %q: %v", f, err)
				}
				fields = append(fields, m)
			}
			if len(fields) == 0 {
				log.Fatalf("no fields: pass --field '{\"selector\":\"#x\",\"value\":\"…\"}'")
			}
			payload := map[string]any{"fields": fields}
			if fillSubmit {
				payload["submit"] = true
			}
			if fillNoEvid {
				payload["noEvidence"] = true
			}

			resp, err := client.SendCommand(port, "fill", payload)
			if err != nil {
				log.Fatalf("Fill failed: %v", err)
			}
			var out struct {
				Filled   int `json:"filled"`
				Failures []struct {
					Field      string `json:"field"`
					Error      string `json:"error"`
					Hint       string `json:"hint"`
					Candidates []struct {
						Ref      int    `json:"ref"`
						Role     string `json:"role"`
						Text     string `json:"text"`
						Name     string `json:"name"`
						Note     string `json:"note"`
						Selector string `json:"selector"`
					} `json:"candidates"`
				} `json:"failures"`
			}
			if err := json.Unmarshal(resp, &out); err != nil {
				log.Fatalf("Fill failed: unexpected response: %v", err)
			}
			fmt.Printf("Filled %d field(s)\n", out.Filled)
			for _, f := range out.Failures {
				fmt.Printf("  failed: %s: %s\n", f.Field, f.Error)
				if f.Hint != "" {
					fmt.Printf("    hint: %s\n", f.Hint)
				}
				for _, c := range f.Candidates {
					label := c.Name
					if label == "" {
						label = c.Text
					}
					line := fmt.Sprintf("    [%d] %s", c.Ref, c.Role)
					if label != "" {
						line += fmt.Sprintf(" %q", label)
					}
					if c.Note != "" {
						line += " (" + c.Note + ")"
					}
					fmt.Printf("%s selector: %s\n", line, c.Selector)
				}
			}
			printEvidenceFromBody(resp)
		},
	}
	fillCmd.Flags().StringArrayVar(&fillFieldSpecs, "field", nil, `Field JSON, repeatable: {"ref":2,"value":"…"} or {"selector":"#x","value":"…"}`)
	fillCmd.Flags().StringVar(&fillJSON, "json", "", "Raw fields JSON array (alternative to --field)")
	fillCmd.Flags().BoolVar(&fillSubmit, "submit", false, "Submit the form after filling (skipped when any field failed)")
	fillCmd.Flags().BoolVar(&fillNoEvid, "no-evidence", false, "Skip the post-action receipt")

	selectCmd := &cobra.Command{
		Use:   "select <ref-or-selector> <option>",
		Short: "Picks an option in a select element",
		Long: `Picks an option in a <select> element. The option matches the option's
value first, then its label. Use --multiple with a comma-separated list
for multi-selects.`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			payload := interactionTarget(args[0])
			payload["value"] = args[1]
			if selectMulti {
				payload["multiple"] = true
			}
			if selectNoEvid {
				payload["noEvidence"] = true
			}

			resp, err := client.SendCommand(port, "select", payload)
			if err != nil {
				failAction("Select", err)
			}
			var out struct {
				Selected []string `json:"selected"`
			}
			if err := json.Unmarshal(resp, &out); err != nil {
				log.Fatalf("Select failed: unexpected response: %v", err)
			}
			fmt.Printf("Selected: %v\n", out.Selected)
			printEvidenceFromBody(resp)
		},
	}
	selectCmd.Flags().BoolVar(&selectMulti, "multiple", false, "Multi-select; option argument is comma-separated")
	selectCmd.Flags().BoolVar(&selectNoEvid, "no-evidence", false, "Skip the post-action receipt")

	checkCmd := &cobra.Command{
		Use:   "check <ref-or-selector> [--off]",
		Short: "Checks or unchecks a checkbox or radio",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			payload := interactionTarget(args[0])
			payload["checked"] = !checkOff
			if checkNoEvid {
				payload["noEvidence"] = true
			}

			resp, err := client.SendCommand(port, "check", payload)
			if err != nil {
				failAction("Check", err)
			}
			var out struct {
				Was bool `json:"was"`
				Now bool `json:"now"`
			}
			if err := json.Unmarshal(resp, &out); err != nil {
				log.Fatalf("Check failed: unexpected response: %v", err)
			}
			fmt.Printf("%v → %v\n", out.Was, out.Now)
			printEvidenceFromBody(resp)
		},
	}
	checkCmd.Flags().BoolVar(&checkOff, "off", false, "Uncheck instead of check")
	checkCmd.Flags().BoolVar(&checkNoEvid, "no-evidence", false, "Skip the post-action receipt")

	rootCmd.AddCommand(fillCmd)
	rootCmd.AddCommand(selectCmd)
	rootCmd.AddCommand(checkCmd)
}

var fillFieldSpecs []string
