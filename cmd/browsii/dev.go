package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

func init() {
	var devMode string
	var watch bool

	devCmd := &cobra.Command{
		Use:   "dev [flags] -- <command> [args...]",
		Short: "Run a command against the daemon, starting one if needed",
		Long: `Starts or attaches to a browser daemon, sets BROWSII_PORT, then runs your command.
The daemon is stopped on exit only if dev started it.

Use --watch to re-run the command whenever a .go file in the current directory changes.`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			os.Exit(runDev(port, devMode, watch, args))
		},
	}

	devCmd.Flags().StringVarP(&devMode, "mode", "m", "headful", "Browser mode: headful (default), headless, user-headless, user-headful")
	devCmd.Flags().BoolVarP(&watch, "watch", "w", false, "Re-run command on .go file changes")

	rootCmd.AddCommand(devCmd)
}

// runDev is the main entry point for the dev command. Returns an exit code.
func runDev(daemonPort int, devMode string, watch bool, args []string) int {
	owned := ensureDaemon(daemonPort, devMode)
	if owned {
		defer shutdownDaemon(daemonPort)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if !watch {
		return runSubprocess(daemonPort, args, sigCh)
	}

	runDevWatch(daemonPort, args, sigCh)
	return 0
}

// ensureDaemon attaches to an existing daemon or spawns one.
// Returns true if this process owns (and must stop) the daemon.
func ensureDaemon(daemonPort int, devMode string) bool {
	pingURL := fmt.Sprintf("http://127.0.0.1:%d/ping", daemonPort)
	httpClient := &http.Client{Timeout: time.Second}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", pingURL, nil)
	resp, err := httpClient.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close() //nolint:errcheck
		fmt.Printf("[browsii dev] attaching to existing daemon on :%d\n", daemonPort)
		return false
	}

	fmt.Printf("[browsii dev] starting daemon on :%d (mode: %s)...\n", daemonPort, devMode)
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[browsii dev] could not locate binary: %v\n", err)
		os.Exit(1)
	}

	bgCmd := exec.CommandContext(context.Background(), executable, "daemon", "--port", fmt.Sprintf("%d", daemonPort), "--mode", devMode)
	if err := bgCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[browsii dev] failed to start daemon: %v\n", err)
		os.Exit(1)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		pollReq, _ := http.NewRequestWithContext(context.Background(), "GET", pingURL, nil)
		r, e := httpClient.Do(pollReq)
		if e == nil && r.StatusCode == http.StatusOK {
			r.Body.Close() //nolint:errcheck
			fmt.Printf("[browsii dev] daemon ready (PID %d)\n", bgCmd.Process.Pid)
			return true
		}
	}

	fmt.Fprintln(os.Stderr, "[browsii dev] daemon did not become ready within 15s")
	os.Exit(1)
	return false // unreachable
}

// shutdownDaemon sends a graceful shutdown request to the daemon.
func shutdownDaemon(daemonPort int) {
	url := fmt.Sprintf("http://127.0.0.1:%d/shutdown", daemonPort)
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return
	}
	resp.Body.Close() //nolint:errcheck
	fmt.Println("[browsii dev] daemon stopped")
}

// runSubprocess runs args as a subprocess with BROWSII_PORT set.
// Signals from sigCh are forwarded to the subprocess.
// Returns the subprocess exit code.
func runSubprocess(daemonPort int, args []string, sigCh <-chan os.Signal) int {
	fmt.Printf("[browsii dev] running: %s\n", strings.Join(args, " "))

	cmd := exec.CommandContext(context.Background(), args[0], args[1:]...) //nolint:gosec
	cmd.Env = append(os.Environ(), fmt.Sprintf("BROWSII_PORT=%d", daemonPort))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[browsii dev] failed to start command: %v\n", err)
		return 1
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case sig := <-sigCh:
		fmt.Printf("\n[browsii dev] received %s\n", sig)
		cmd.Process.Signal(sig) //nolint:errcheck
		<-done
		return 130
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode()
			}
			return 1
		}
		return 0
	}
}

// runDevWatch sets up filesystem watching and delegates to runDevWatchLoop.
func runDevWatch(daemonPort int, args []string, sigCh <-chan os.Signal) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[browsii dev] failed to create watcher: %v\n", err)
		os.Exit(1)
	}
	defer watcher.Close() //nolint:errcheck

	cwd, _ := os.Getwd()
	addDirsToWatcher(watcher, cwd)
	fmt.Printf("[browsii dev] watching *.go in %s — Ctrl+C to stop\n", cwd)

	runDevWatchLoop(daemonPort, args, sigCh, watcher.Events, watcher.Errors)
}

// runDevWatchLoop is the core watch loop. It accepts injected event/error
// channels so tests can drive it without touching the real filesystem.
func runDevWatchLoop(daemonPort int, args []string, sigCh <-chan os.Signal, events <-chan fsnotify.Event, errs <-chan error) {
	var (
		mu       sync.Mutex
		proc     *exec.Cmd
		debounce *time.Timer
	)

	killCurrent := func() {
		mu.Lock()
		defer mu.Unlock()
		if proc != nil && proc.Process != nil {
			proc.Process.Kill() //nolint:errcheck
		}
	}

	startCurrent := func() {
		fmt.Printf("\n[browsii dev] running: %s\n", strings.Join(args, " "))
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...) //nolint:gosec
		cmd.Env = append(os.Environ(), fmt.Sprintf("BROWSII_PORT=%d", daemonPort))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		mu.Lock()
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "[browsii dev] failed to start: %v\n", err)
			mu.Unlock()
			return
		}
		proc = cmd
		mu.Unlock()

		go cmd.Wait() //nolint:errcheck
	}

	startCurrent()

	for {
		select {
		case sig := <-sigCh:
			fmt.Printf("\n[browsii dev] received %s, shutting down\n", sig)
			killCurrent()
			return

		case event, ok := <-events:
			if !ok {
				return
			}
			if filepath.Ext(event.Name) != ".go" {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			mu.Lock()
			if debounce != nil {
				debounce.Stop()
			}
			name := filepath.Base(event.Name)
			debounce = time.AfterFunc(200*time.Millisecond, func() {
				fmt.Printf("\n[browsii dev] %s changed, restarting...\n", name)
				killCurrent()
				startCurrent()
			})
			mu.Unlock()

		case err, ok := <-errs:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "[browsii dev] watcher error: %v\n", err)
		}
	}
}

// addDirsToWatcher walks root and adds every directory (excluding .git, vendor,
// node_modules) to the watcher so that nested .go file changes are detected.
func addDirsToWatcher(w *fsnotify.Watcher, root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		switch d.Name() {
		case ".git", "vendor", "node_modules":
			return filepath.SkipDir
		}
		w.Add(path) //nolint:errcheck
		return nil
	})
}
