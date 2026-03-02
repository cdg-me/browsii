package main

import (
	"fmt"
	"log"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

var getLinksPattern string

func init() {
	getLinksCmd := &cobra.Command{
		Use:   "get-links",
		Short: "Extracts all hyperlinked URLs from the page as a JSON array",
		Run: func(cmd *cobra.Command, args []string) {
			var payload interface{}
			if getLinksPattern != "" {
				payload = map[string]string{"pattern": getLinksPattern}
			}

			resp, err := client.SendCommand(port, "links", payload)
			if err != nil {
				log.Fatalf("Get links failed: %v", err)
			}

			fmt.Print(string(resp))
		},
	}

	getLinksCmd.Flags().StringVar(&getLinksPattern, "pattern", "", "Filter links by URL pattern (substring match)")

	rootCmd.AddCommand(getLinksCmd)
}
