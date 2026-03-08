package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var scrapeFormat string

func init() {
	scrapeCmd := &cobra.Command{
		Use:   "scrape",
		Short: "Extracts the current page content in the specified format",
		Run: func(cmd *cobra.Command, args []string) {
			var payload interface{}
			if scrapeFormat != "" {
				payload = map[string]string{"format": scrapeFormat}
			}

			resp, err := client.SendCommand(port, "scrape", payload)
			if err != nil {
				log.Fatalf("Scrape failed: %v", err)
			}

			fmt.Println(string(resp))
		},
	}

	scrapeCmd.Flags().StringVar(&scrapeFormat, "format", "", "Output format: html (default), text, markdown")

	rootCmd.AddCommand(scrapeCmd)
}
