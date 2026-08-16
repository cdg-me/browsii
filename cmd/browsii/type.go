package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	typeCmd := &cobra.Command{
		Use:   "type <ref-or-selector> <text>",
		Short: "Types text into an input element",
		Long: `Types text into an input element, replacing any existing value.

Accepts either a CSS selector or a numeric element ref from 'browsii elements':
  browsii type #email "user@example.com"
  browsii type 3 "user@example.com"`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			target := args[0]
			text := args[1]
			payload := interactionTarget(target)
			payload["text"] = text

			_, err := client.SendCommand(port, "type", payload)
			if err != nil {
				failAction("Type", err)
			}

			fmt.Printf("Successfully typed into '%s'\n", target)
		},
	}

	rootCmd.AddCommand(typeCmd)
}
