package main

import (
	"fmt"
	"log"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

func init() {
	pressCmd := &cobra.Command{
		Use:   "press <key>",
		Short: "Presses a key or key combo (e.g. Enter, Control+a, Shift+Tab)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			key := args[0]
			payload := map[string]string{"key": key}

			_, err := client.SendCommand(port, "press", payload)
			if err != nil {
				log.Fatalf("Press failed: %v", err)
			}

			fmt.Printf("Successfully pressed %s\n", key)
		},
	}

	rootCmd.AddCommand(pressCmd)
}
