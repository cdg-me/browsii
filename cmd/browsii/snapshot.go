package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	snapshotCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Load and clear network snapshots for offline testing",
	}

	loadCmd := &cobra.Command{
		Use:   "load <file.har>",
		Short: "Load a HAR snapshot and intercept matching requests on the active page",
		Long: `Reads a HAR file and registers a network interceptor on the active page.
Requests whose URL appears in the HAR are served from the recorded response.
All other requests pass through to the network unchanged.

Record a HAR with:
  browsii network capture start --include response-headers,response-body --format har --output snap.har
  browsii navigate <url>
  browsii network capture stop`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.SendCommand(port, "snapshot/load", map[string]string{"path": args[0]})
			if err != nil {
				log.Fatalf("snapshot load failed: %v", err)
			}
			fmt.Print(string(resp))
		},
	}

	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Stop the active snapshot router and restore normal network behaviour",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			_, err := client.SendCommand(port, "snapshot/clear", nil)
			if err != nil {
				log.Fatalf("snapshot clear failed: %v", err)
			}
			fmt.Println("Snapshot cleared")
		},
	}

	snapshotCmd.AddCommand(loadCmd)
	snapshotCmd.AddCommand(clearCmd)
	rootCmd.AddCommand(snapshotCmd)
}
