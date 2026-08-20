package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

func init() {
	downloadsCmd := &cobra.Command{
		Use:   "downloads",
		Short: "Lists browser downloads managed by the daemon",
		Long: `Lists browser downloads managed by the daemon.

Downloads are saved to ~/.browsii/downloads/<port>/ and tracked from
begin to completion; click receipts report any download they trigger.`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.SendCommand(port, "downloads", nil)
			if err != nil {
				log.Fatalf("Downloads failed: %v", err)
			}
			var out struct {
				Dir       string `json:"dir"`
				Downloads []struct {
					Filename      string  `json:"filename"`
					URL           string  `json:"url"`
					State         string  `json:"state"`
					ReceivedBytes float64 `json:"receivedBytes"`
					TotalBytes    float64 `json:"totalBytes"`
					Path          string  `json:"path"`
				} `json:"downloads"`
			}
			if err := json.Unmarshal(resp, &out); err != nil {
				log.Fatalf("Downloads failed: unexpected response: %v", err)
			}
			fmt.Printf("Directory: %s\n", out.Dir)
			if len(out.Downloads) == 0 {
				fmt.Println("No downloads recorded.")
				return
			}
			for _, d := range out.Downloads {
				line := fmt.Sprintf("  %-20s %s", d.Filename, d.State)
				switch d.State {
				case "inProgress":
					line = fmt.Sprintf("  %-20s %d/%d bytes", d.Filename, int64(d.ReceivedBytes), int64(d.TotalBytes))
				case "completed":
					line = fmt.Sprintf("  %-20s %s → %s", d.Filename, humanBytes(d.TotalBytes), d.Path)
				}
				fmt.Println(line)
			}
		},
	}

	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Forget tracked downloads (files on disk are kept)",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.SendCommand(port, "downloads/clear", nil)
			if err != nil {
				log.Fatalf("Downloads clear failed: %v", err)
			}
			var out struct {
				Cleared int `json:"cleared"`
			}
			if err := json.Unmarshal(resp, &out); err != nil {
				log.Fatalf("Downloads clear failed: unexpected response: %v", err)
			}
			fmt.Printf("Forgot %d tracked download(s)\n", out.Cleared)
		},
	}
	downloadsCmd.AddCommand(clearCmd)

	rootCmd.AddCommand(downloadsCmd)
}

func humanBytes(b float64) string {
	const kb, mb = 1024, 1024 * 1024
	switch {
	case b >= mb:
		return fmt.Sprintf("%.1fMB", b/mb)
	case b >= kb:
		return fmt.Sprintf("%.1fKB", b/kb)
	default:
		return fmt.Sprintf("%dB", int64(b))
	}
}
