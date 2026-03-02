package main

import (
	"fmt"
	"log"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

var (
	consoleCaptureTab   string
	consoleCaptureLevel string
)

func init() {
	consoleCmd := &cobra.Command{
		Use:   "console",
		Short: "Console event capture",
	}

	captureConsoleCmd := &cobra.Command{
		Use:   "capture",
		Short: "Console event capture (start, stop)",
	}

	captureConsoleStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Starts buffering console events",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]interface{}{
				"tab":   consoleCaptureTab,
				"level": consoleCaptureLevel,
			}
			_, err := client.SendCommand(port, "console/capture/start", payload)
			if err != nil {
				log.Fatalf("Console capture start failed: %v", err)
			}
			fmt.Println("Console capture started")
		},
	}
	captureConsoleStartCmd.Flags().StringVar(&consoleCaptureTab, "tab", "", `Tab filter: "active", "next", "last", "<index>", or omit for all tabs`)
	captureConsoleStartCmd.Flags().StringVar(&consoleCaptureLevel, "level", "", `Level filter: comma-separated list e.g. "error,warn", or omit for all levels`)

	captureConsoleStopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stops capture and returns recorded console entries as JSON",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.SendCommand(port, "console/capture/stop", nil)
			if err != nil {
				log.Fatalf("Console capture stop failed: %v", err)
			}
			fmt.Print(string(resp))
		},
	}

	captureConsoleCmd.AddCommand(captureConsoleStartCmd)
	captureConsoleCmd.AddCommand(captureConsoleStopCmd)
	consoleCmd.AddCommand(captureConsoleCmd)
	rootCmd.AddCommand(consoleCmd)
}
