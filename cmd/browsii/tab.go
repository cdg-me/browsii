package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

func init() {
	tabCmd := &cobra.Command{
		Use:   "tab",
		Short: "Manage browser tabs (new, list, switch)",
	}

	newCmd := &cobra.Command{
		Use:   "new [url]",
		Short: "Opens a new tab and navigates to the URL (default: about:blank)",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			url := "about:blank"
			if len(args) == 1 {
				url = args[0]
			}
			payload := map[string]string{"url": url}

			_, err := client.SendCommand(port, "tab/new", payload)
			if err != nil {
				log.Fatalf("Failed to open new tab: %v", err)
			}
			fmt.Printf("Successfully opened new tab: %s\n", url)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Lists all active tabs",
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.SendCommand(port, "tab/list", nil)
			if err != nil {
				log.Fatalf("Failed to list tabs: %v", err)
			}

			var tabs []map[string]interface{}
			if err := json.Unmarshal(resp, &tabs); err != nil {
				log.Fatalf("Failed to parse tab list: %v", err)
			}

			fmt.Println("Active Tabs:")
			for _, tab := range tabs {
				index := int(tab["index"].(float64))
				title := tab["title"].(string)
				url := tab["url"].(string)
				fmt.Printf("  [%d] %s (%s)\n", index, title, url)
			}
		},
	}

	switchCmd := &cobra.Command{
		Use:   "switch <index>",
		Short: "Switches focus to the specified tab index",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			index, err := strconv.Atoi(args[0])
			if err != nil {
				log.Fatalf("Index must be an integer: %v", err)
			}

			payload := map[string]int{"index": index}
			_, err = client.SendCommand(port, "tab/switch", payload)
			if err != nil {
				log.Fatalf("Failed to switch tab: %v", err)
			}
			fmt.Printf("Successfully activated tab %d\n", index)
		},
	}

	closeCmd := &cobra.Command{
		Use:   "close",
		Short: "Closes the active tab",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			_, err := client.SendCommand(port, "tab/close", nil)
			if err != nil {
				log.Fatalf("Failed to close tab: %v", err)
			}
			fmt.Println("Successfully closed active tab")
		},
	}

	tabCmd.AddCommand(newCmd)
	tabCmd.AddCommand(listCmd)
	tabCmd.AddCommand(switchCmd)
	tabCmd.AddCommand(closeCmd)

	rootCmd.AddCommand(tabCmd)
}
