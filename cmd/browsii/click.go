package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	clickCmd := &cobra.Command{
		Use:   "click <selector>",
		Short: "Clicks an element on the active page",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			selector := args[0]
			payload := map[string]string{"selector": selector}

			_, err := client.SendCommand(port, "click", payload)
			if err != nil {
				log.Fatalf("Click failed: %v", err)
			}

			fmt.Printf("Successfully clicked '%s'\n", selector)
		},
	}

	rootCmd.AddCommand(clickCmd)
}
