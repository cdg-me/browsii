package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var (
	navWaitUntil  string
	navNoEvidence bool
)

func init() {
	navCmd := &cobra.Command{
		Use:   "navigate <url>",
		Short: "Navigates the active browser tab to a URL",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			url := args[0]
			payload := map[string]any{"url": url}
			if navWaitUntil != "" {
				payload["waitUntil"] = navWaitUntil
			}
			if navNoEvidence {
				payload["noEvidence"] = true
			}

			resp, err := client.SendCommand(port, "navigate", payload)
			if err != nil {
				log.Fatalf("Navigation failed: %v", err)
			}

			fmt.Printf("Successfully navigated to %s\n", url)
			printEvidenceFromBody(resp)
		},
	}

	navCmd.Flags().StringVar(&navWaitUntil, "wait-until", "", "Wait strategy: load (default), networkidle")
	navCmd.Flags().BoolVar(&navNoEvidence, "no-evidence", false, "Skip the post-action receipt (faster, less verifiable)")

	rootCmd.AddCommand(navCmd)
}
