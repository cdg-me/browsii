package main

import (
	"fmt"
	"log"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

func init() {
	hoverCmd := &cobra.Command{
		Use:   "hover <selector>",
		Short: "Hovers the mouse over an element",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			selector := args[0]
			payload := map[string]string{"selector": selector}

			_, err := client.SendCommand(port, "hover", payload)
			if err != nil {
				log.Fatalf("Hover failed: %v", err)
			}

			fmt.Printf("Successfully hovered over %s\n", selector)
		},
	}

	rootCmd.AddCommand(hoverCmd)
}
