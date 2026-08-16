package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var pressNoEvidence bool

func init() {
	pressCmd := &cobra.Command{
		Use:   "press <key>",
		Short: "Presses a key or key combo (e.g. Enter, Control+a, Shift+Tab)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			key := args[0]
			payload := map[string]any{"key": key}
			if pressNoEvidence {
				payload["noEvidence"] = true
			}

			resp, err := client.SendCommand(port, "press", payload)
			if err != nil {
				log.Fatalf("Press failed: %v", err)
			}

			fmt.Printf("Successfully pressed %s\n", key)
			printEvidenceFromBody(resp)
		},
	}

	pressCmd.Flags().BoolVar(&pressNoEvidence, "no-evidence", false, "Skip the post-action receipt (faster, less verifiable)")

	rootCmd.AddCommand(pressCmd)
}
