package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var clickNoEvidence bool

func init() {
	clickCmd := &cobra.Command{
		Use:   "click <ref-or-selector>",
		Short: "Clicks an element on the active page",
		Long: `Clicks an element on the active page.

Accepts either a CSS selector or a numeric element ref from 'browsii elements':
  browsii click #submit-btn
  browsii click 12

When the element is not found, the error lists similar elements with refs
that can be used directly in a retry.

The response includes a receipt of what the click caused: navigation,
requests fired, dialogs auto-handled, and console errors. Use --no-evidence
to skip the settle window when speed matters more than verification.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			target := args[0]
			payload := interactionTarget(target)
			if clickNoEvidence {
				payload["noEvidence"] = true
			}

			resp, err := client.SendCommand(port, "click", payload)
			if err != nil {
				failAction("Click", err)
			}

			fmt.Printf("Successfully clicked '%s'\n", target)
			printEvidenceFromBody(resp)
		},
	}

	clickCmd.Flags().BoolVar(&clickNoEvidence, "no-evidence", false, "Skip the post-action receipt (faster, less verifiable)")

	rootCmd.AddCommand(clickCmd)
}
