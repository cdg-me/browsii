package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var navWaitUntil string

func init() {
	navCmd := &cobra.Command{
		Use:   "navigate <url>",
		Short: "Navigates the active browser tab to a URL",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			url := args[0]
			payload := map[string]string{"url": url}
			if navWaitUntil != "" {
				payload["waitUntil"] = navWaitUntil
			}

			_, err := client.SendCommand(port, "navigate", payload)
			if err != nil {
				log.Fatalf("Navigation failed: %v", err)
			}

			fmt.Printf("Successfully navigated to %s\n", url)
		},
	}

	navCmd.Flags().StringVar(&navWaitUntil, "wait-until", "", "Wait strategy: load (default), networkidle")

	rootCmd.AddCommand(navCmd)
}
