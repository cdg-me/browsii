package main

import (
	"fmt"
	"log"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

func init() {
	typeCmd := &cobra.Command{
		Use:   "type <selector> <text>",
		Short: "Types text into an input element",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			selector := args[0]
			text := args[1]
			payload := map[string]string{
				"selector": selector,
				"text":     text,
			}

			_, err := client.SendCommand(port, "type", payload)
			if err != nil {
				log.Fatalf("Type failed: %v", err)
			}

			fmt.Printf("Successfully typed into '%s'\n", selector)
		},
	}

	rootCmd.AddCommand(typeCmd)
}
