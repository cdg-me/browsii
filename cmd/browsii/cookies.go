package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

var cookiesCmd = &cobra.Command{
	Use:   "cookies",
	Short: "Prints the active tab's cookies as JSON",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := client.SendCommand(port, "cookies", nil)
		if err != nil {
			log.Fatalf("Failed to fetch cookies: %v", err)
		}

		var cookies []map[string]interface{}
		if err := json.Unmarshal(resp, &cookies); err != nil {
			log.Fatalf("Failed to parse cookies JSON: %v", err)
		}

		// Print formatted JSON
		prettyData, err := json.MarshalIndent(cookies, "", "  ")
		if err != nil {
			log.Fatalf("Failed to pretty print cookies: %v", err)
		}

		fmt.Println(string(prettyData))
	},
}

func init() {
	rootCmd.AddCommand(cookiesCmd)
}
