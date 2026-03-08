package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/cdg-me/browsii/internal/daemon"
	"github.com/spf13/cobra"
)

var mode string

func init() {
	// The actual blocking server loop (hidden from user)
	daemonCmd := &cobra.Command{
		Use:    "daemon",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			srv := daemon.NewServer(port, mode)
			err := srv.Start()
			if err != nil && err.Error() != "http: Server closed" {
				log.Fatalf("Daemon failed to start: %v", err)
			}
		},
	}
	daemonCmd.Flags().StringVarP(&mode, "mode", "m", "headful", "Mode")

	// The public command that spawns the daemon in the background
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Starts the background browser daemon",
		Long:  `Boots a persistent go-rod browser instance in the background.`,
		Run: func(cmd *cobra.Command, args []string) {

			// 1. Check if it's already running
			client := &http.Client{Timeout: 1 * time.Second}
			resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port)) //nolint:noctx
			if err == nil && resp.StatusCode == 200 {
				fmt.Printf("Daemon is already running on port %d\n", port)
				return
			}

			// 2. Spawn the daemon process
			executable, err := os.Executable()
			if err != nil {
				log.Fatalf("Failed to get executable path: %v", err)
			}

			bgCmd := exec.Command(executable, "daemon", "--port", fmt.Sprintf("%d", port), "--mode", mode) //nolint:noctx

			// Detach it from the current terminal
			if err := bgCmd.Start(); err != nil {
				log.Fatalf("Failed to start daemon: %v", err)
			}

			fmt.Printf("Daemon started in the background (PID: %d, Port: %d, Mode: %s)\n", bgCmd.Process.Pid, port, mode)

			// Polling wait to ensure it comes up before we return control
			for i := 0; i < 10; i++ {
				time.Sleep(500 * time.Millisecond)
				r, e := client.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port)) //nolint:noctx
				if e == nil && r.StatusCode == 200 {
					return
				}
			}
			fmt.Println("Warning: Daemon process started but API is not yet responding.")
		},
	}

	startCmd.Flags().StringVarP(&mode, "mode", "m", "headful", "Browser mode: headful (default), headless, user-headless, user-headful")

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(startCmd)
}
