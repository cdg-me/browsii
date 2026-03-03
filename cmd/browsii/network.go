package main

import (
	"fmt"
	"log"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

var (
	captureTab       string
	captureOutput    string // set on start, consumed on stop
	throttleLatency  int
	throttleDownload int
	throttleUpload   int
	mockPattern      string
	mockBody         string
	mockContentType  string
	mockStatusCode   int
)

func init() {
	networkCmd := &cobra.Command{
		Use:   "network",
		Short: "Network capture, throttling, and mocking",
	}

	// ── network capture start/stop ──

	captureCmd := &cobra.Command{
		Use:   "capture",
		Short: "Network request capture (start, stop)",
	}

	captureStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Starts capturing network requests",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]interface{}{
				"tab":    captureTab,
				"output": captureOutput,
			}
			_, err := client.SendCommand(port, "network/capture/start", payload)
			if err != nil {
				log.Fatalf("Network capture start failed: %v", err)
			}
			fmt.Println("Network capture started")
		},
	}
	captureStartCmd.Flags().StringVar(&captureTab, "tab", "", `Tab filter: "active", "next", "last", "<index>", or omit for all tabs`)
	captureStartCmd.Flags().StringVarP(&captureOutput, "output", "o", "", "Write captured requests to this file on stop")

	captureStopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stops capture and prints results (or writes to file if --output was set on start)",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.SendCommand(port, "network/capture/stop", nil)
			if err != nil {
				log.Fatalf("Network capture stop failed: %v", err)
			}
			fmt.Print(string(resp))
		},
	}

	captureCmd.AddCommand(captureStartCmd)
	captureCmd.AddCommand(captureStopCmd)

	// ── network throttle ──

	throttleCmd := &cobra.Command{
		Use:   "throttle",
		Short: "Sets network throttling conditions",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]int{
				"latency":  throttleLatency,
				"download": throttleDownload,
				"upload":   throttleUpload,
			}
			_, err := client.SendCommand(port, "network/throttle", payload)
			if err != nil {
				log.Fatalf("Network throttle failed: %v", err)
			}
			fmt.Printf("Network conditions set: latency=%dms, download=%d B/s, upload=%d B/s\n",
				throttleLatency, throttleDownload, throttleUpload)
		},
	}
	throttleCmd.Flags().IntVar(&throttleLatency, "latency", 0, "Added latency in milliseconds")
	throttleCmd.Flags().IntVar(&throttleDownload, "download", -1, "Download throughput in bytes/sec (-1 for unlimited)")
	throttleCmd.Flags().IntVar(&throttleUpload, "upload", -1, "Upload throughput in bytes/sec (-1 for unlimited)")

	// ── network mock ──

	mockCmd := &cobra.Command{
		Use:   "mock",
		Short: "Intercepts matching requests and returns a custom response",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			payload := map[string]interface{}{
				"pattern":     mockPattern,
				"body":        mockBody,
				"contentType": mockContentType,
				"statusCode":  mockStatusCode,
			}
			_, err := client.SendCommand(port, "network/mock", payload)
			if err != nil {
				log.Fatalf("Network mock failed: %v", err)
			}
			fmt.Printf("Network mock set for pattern: %s\n", mockPattern)
		},
	}
	mockCmd.Flags().StringVar(&mockPattern, "pattern", "", "URL pattern to match (glob syntax)")
	mockCmd.Flags().StringVar(&mockBody, "body", "", "Response body to return")
	mockCmd.Flags().StringVar(&mockContentType, "content-type", "", "Response Content-Type header")
	mockCmd.Flags().IntVar(&mockStatusCode, "status", 200, "HTTP status code to return")

	networkCmd.AddCommand(captureCmd)
	networkCmd.AddCommand(throttleCmd)
	networkCmd.AddCommand(mockCmd)

	rootCmd.AddCommand(networkCmd)
}
