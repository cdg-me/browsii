package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stops the background browser daemon",
		Long:  `Sends a shutdown signal to the local API server, safely closing the browser instance.`,
		Run: func(cmd *cobra.Command, args []string) {
			url := fmt.Sprintf("http://127.0.0.1:%d/shutdown", port)
			client := &http.Client{Timeout: 2 * time.Second}

			resp, err := client.Get(url)
			if err != nil {
				fmt.Printf("Error stopping daemon on port %d: %v\n", port, err)
				fmt.Println("Is it running?")
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				fmt.Println("Daemon gracefully shut down.")
			} else {
				fmt.Printf("Daemon returned unexpected status: %d\n", resp.StatusCode)
			}
		},
	}

	rootCmd.AddCommand(stopCmd)
}
