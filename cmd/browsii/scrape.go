package main

import (
	"fmt"
	"os"

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
				if body, ok := parseDaemonError(err); ok {
					fmt.Fprintln(os.Stderr, body.Error)
					if body.Hint != "" {
						fmt.Fprintf(os.Stderr, "hint: %s\n", body.Hint)
					}
					os.Exit(1)
				}
				fmt.Fprintf(os.Stderr, "Scrape failed: %v\n", err)
				os.Exit(1)
			}

			fmt.Println(string(resp))
		},
	}

	scrapeCmd.Flags().StringVar(&scrapeFormat, "format", "", "Output format: html (default), text, markdown, readable")

	rootCmd.AddCommand(scrapeCmd)
}
