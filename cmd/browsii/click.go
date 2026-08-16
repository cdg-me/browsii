package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	clickCmd := &cobra.Command{
		Use:   "click <ref-or-selector>",
		Short: "Clicks an element on the active page",
		Long: `Clicks an element on the active page.

Accepts either a CSS selector or a numeric element ref from 'browsii elements':
  browsii click #submit-btn
  browsii click 12

When the element is not found, the error lists similar elements with refs
that can be used directly in a retry.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			target := args[0]
			payload := interactionTarget(target)

			resp, err := client.SendCommand(port, "click", payload)
			if err != nil {
				failAction("Click", err)
			}

			fmt.Printf("Successfully clicked '%s'\n", target)
			printDialogsFromBody(resp)
		},
	}

	rootCmd.AddCommand(clickCmd)
}
