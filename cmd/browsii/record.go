package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/cdg-me/browsii/internal/client"
	"github.com/spf13/cobra"
)

var replaySpeed float64

func resolveRecordingName(name string) string {
	if strings.HasSuffix(name, ".json") || strings.Contains(name, string(filepath.Separator)) {
		abs, err := filepath.Abs(name)
		if err == nil {
			return abs
		}
	}
	return name
}

func init() {
	recordCmd := &cobra.Command{
		Use:   "record",
		Short: "Record and replay browser sessions",
	}

	startCmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start recording browser actions",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := resolveRecordingName(args[0])
			payload := map[string]string{"name": name}
			_, err := client.SendCommand(port, "record/start", payload)
			if err != nil {
				log.Fatalf("Record start failed: %v", err)
			}
			fmt.Printf("Recording started: %s\n", args[0])
		},
	}

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop recording and save to disk",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.SendCommand(port, "record/stop", nil)
			if err != nil {
				log.Fatalf("Record stop failed: %v", err)
			}
			fmt.Printf("Recording saved: %s\n", string(resp))
		},
	}

	replayCmd := &cobra.Command{
		Use:   "replay <name>",
		Short: "Replay a recorded session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := resolveRecordingName(args[0])
			payload := map[string]interface{}{
				"name":  name,
				"speed": replaySpeed,
			}
			_, err := client.SendCommand(port, "record/replay", payload)
			if err != nil {
				log.Fatalf("Record replay failed: %v", err)
			}
			fmt.Printf("Replay of %q complete\n", args[0])
		},
	}
	replayCmd.Flags().Float64Var(&replaySpeed, "speed", 1.0, "Replay speed (0=instant, 1=real-time, 2=2x)")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all saved recordings",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.SendCommand(port, "record/list", nil)
			if err != nil {
				log.Fatalf("Record list failed: %v", err)
			}
			fmt.Print(string(resp))
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a saved recording",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := resolveRecordingName(args[0])
			payload := map[string]string{"name": name}
			_, err := client.SendCommand(port, "record/delete", payload)
			if err != nil {
				log.Fatalf("Record delete failed: %v", err)
			}
			fmt.Printf("Recording %q deleted\n", args[0])
		},
	}

	recordCmd.AddCommand(startCmd)
	recordCmd.AddCommand(stopCmd)
	recordCmd.AddCommand(replayCmd)
	recordCmd.AddCommand(listCmd)
	recordCmd.AddCommand(deleteCmd)

	rootCmd.AddCommand(recordCmd)
}
