package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	hoverCmd := &cobra.Command{
		Use:   "hover <ref-or-selector>",
		Short: "Hovers the mouse over an element",
		Long: `Hovers the mouse over an element.

Accepts either a CSS selector or a numeric element ref from 'browsii elements'.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			target := args[0]
			payload := interactionTarget(target)

			_, err := client.SendCommand(port, "hover", payload)
			if err != nil {
				failAction("Hover", err)
			}

			fmt.Printf("Successfully hovered over %s\n", target)
		},
	}

	rootCmd.AddCommand(hoverCmd)
}
