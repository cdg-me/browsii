package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cdg-me/browsii/internal/client"
)

var (
	replaySpeed      float64
	replayLive       bool
	replaySession    string
	recordCaptureHar bool
)

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
		Long: `Starts recording browser actions.

Every click, hover, and type is stored with the target element's
fingerprint, and every expect call becomes a checkpoint that replay
enforces. Use --capture-har to also record network traffic, which lets
replays run offline.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := resolveRecordingName(args[0])
			payload := map[string]any{"name": name}
			if recordCaptureHar {
				payload["captureHar"] = true
			}
			_, err := client.SendCommand(port, "record/start", payload)
			if err != nil {
				log.Fatalf("Record start failed: %v", err)
			}
			fmt.Printf("Recording started: %s\n", args[0])
			if recordCaptureHar {
				fmt.Println("Network capture active; replay will run offline against the recorded HAR.")
			}
		},
	}
	startCmd.Flags().BoolVar(&recordCaptureHar, "capture-har", false, "Record network traffic to a HAR file alongside the recording")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop recording and save to disk",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := client.SendCommand(port, "record/stop", nil)
			if err != nil {
				log.Fatalf("Record stop failed: %v", err)
			}
			var saved struct {
				Name   string `json:"name"`
				Events int    `json:"events"`
				HAR    string `json:"har"`
			}
			if err := json.Unmarshal(resp, &saved); err != nil {
				log.Fatalf("Record stop failed: unexpected response: %v", err)
			}
			if saved.HAR != "" {
				fmt.Printf("Recording saved: %s (%d events, HAR: %s)\n", saved.Name, saved.Events, saved.HAR)
			} else {
				fmt.Printf("Recording saved: %s (%d events)\n", saved.Name, saved.Events)
			}
		},
	}

	replayCmd := &cobra.Command{
		Use:   "replay <name>",
		Short: "Replay a recorded session",
		Long: `Replays a recorded session.

Element targets are matched by their recorded fingerprint: when the
selector no longer resolves to the same element, the element is relocated
and the substitution is reported under "healed". Recorded expects are
enforced as checkpoints.

When the recording has a HAR file, replay serves all recorded responses
locally and needs no network. Use --live to hit the real network instead,
--session <name> to restore a saved session (cookies, tabs) first.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := resolveRecordingName(args[0])
			payload := map[string]any{
				"name":  name,
				"speed": replaySpeed,
			}
			if replayLive {
				payload["live"] = true
			}
			if replaySession != "" {
				payload["session"] = replaySession
			}
			resp, err := client.SendCommand(port, "record/replay", payload)
			if err != nil {
				// The daemon returns the report JSON with a 417 on failure.
				printReplayFailure(err)
			}
			printReplayReport(resp)
		},
	}
	replayCmd.Flags().Float64Var(&replaySpeed, "speed", 0, "Replay speed (0=instant, 1=recorded timing, 2=twice as fast)")
	replayCmd.Flags().BoolVar(&replayLive, "live", false, "Hit the real network; ignore any recorded HAR")
	replayCmd.Flags().StringVar(&replaySession, "session", "", "Restore this saved session before replaying")

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

	exportCmd := &cobra.Command{
		Use:   "export <name>",
		Short: "Write a Playwright TypeScript spec for the recording",
		Long: `Writes a Playwright spec (test) that reproduces the recording:
fingerprinted elements become role-based locators, expects become
assertions, and a recorded HAR becomes routeFromHAR so the test runs
offline. Run it with: npx playwright test <file>.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := resolveRecordingName(args[0])
			payload := map[string]string{"name": name}
			if exportOut != "" {
				payload["out"] = exportOut
			}
			resp, err := client.SendCommand(port, "record/export", payload)
			if err != nil {
				log.Fatalf("Record export failed: %v", err)
			}
			var out struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(resp, &out); err != nil || out.Path == "" {
				log.Fatalf("Record export failed: unexpected response")
			}
			fmt.Printf("Wrote %s\n", out.Path)
		},
	}
	exportCmd.Flags().StringVar(&exportOut, "out", "", "Output path (default: alongside the recording)")

	recordCmd.AddCommand(startCmd)
	recordCmd.AddCommand(stopCmd)
	recordCmd.AddCommand(replayCmd)
	recordCmd.AddCommand(listCmd)
	recordCmd.AddCommand(deleteCmd)
	recordCmd.AddCommand(exportCmd)

	rootCmd.AddCommand(recordCmd)
}

var exportOut string

type replayReportCLI struct {
	Name        string `json:"name"`
	Steps       int    `json:"steps"`
	Checkpoints struct {
		Total  int `json:"total"`
		Passed int `json:"passed"`
	} `json:"checkpoints"`
	Healed []struct {
		Step int    `json:"step"`
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"healed"`
	DurationMs int64  `json:"durationMs"`
	FailedStep int    `json:"failedStep"`
	Error      string `json:"error"`
}

func printReplayReport(resp []byte) {
	var report replayReportCLI
	if err := json.Unmarshal(resp, &report); err != nil {
		fmt.Println("Replay complete")
		return
	}
	if report.Error != "" {
		fmt.Printf("Replay failed at step %d of %d: %s\n", report.FailedStep, report.Steps, report.Error)
		fmt.Printf("checkpoints: %d/%d passed before failure\n", report.Checkpoints.Passed, report.Checkpoints.Total)
		os.Exit(1)
	}
	fmt.Printf("Replayed %d steps, %d/%d checkpoints passed in %dms\n",
		report.Steps, report.Checkpoints.Passed, report.Checkpoints.Total, report.DurationMs)
	for _, h := range report.Healed {
		fmt.Printf("  healed: step %d  %s → %s\n", h.Step, h.From, h.To)
	}
}

// printReplayFailure extracts the daemon report from the error string and
// renders the failed replay for the operator.
func printReplayFailure(err error) {
	msg := err.Error()
	const marker = "daemon returned error: "
	idx := strings.Index(msg, marker)
	if idx < 0 {
		log.Fatalf("Replay failed: %v", err)
	}
	var report replayReportCLI
	if json.Unmarshal([]byte(msg[idx+len(marker):]), &report) != nil || report.Error == "" {
		log.Fatalf("Replay failed: %v", err)
	}
	fmt.Fprintf(os.Stderr, "Replay failed at step %d of %d: %s\n", report.FailedStep, report.Steps, report.Error)
	fmt.Fprintf(os.Stderr, "checkpoints: %d/%d passed before failure\n", report.Checkpoints.Passed, report.Checkpoints.Total)
	for _, h := range report.Healed {
		fmt.Fprintf(os.Stderr, "  healed: step %d  %s → %s\n", h.Step, h.From, h.To)
	}
	os.Exit(1)
}
