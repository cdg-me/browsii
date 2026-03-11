//go:build !windows

package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// waitForLines polls path until it contains at least want newline-terminated
// lines or the deadline passes.
func waitForLines(t *testing.T, path string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(path)
		if strings.Count(string(data), "\n") >= want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	data, _ := os.ReadFile(path)
	t.Fatalf("timed out after %s waiting for %d lines in sentinel; got %d:\n%s",
		timeout, want, strings.Count(string(data), "\n"), data)
}

// startDevWatch launches "browsii dev --port <port> --mode headless --watch -- sh -c 'echo x >> <sentinel>'"
// in watchDir and returns the running command. The caller is responsible for
// stopping it (via SIGINT or Process.Kill).
func startDevWatch(t *testing.T, bin string, port int, watchDir, sentinel string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext( //nolint:gosec
		context.Background(), bin,
		"dev",
		"--port", fmt.Sprintf("%d", port),
		"--mode", "headless",
		"--watch",
		"--",
		"sh", "-c", "echo x >> "+sentinel,
	)
	cmd.Dir = watchDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	return cmd
}

// TestDevWatch_RestartOnGoFileChange is the primary scenario:
//  1. browsii dev --watch starts the subprocess (sentinel gets 1 line).
//  2. A .go file is written into the watched directory.
//  3. After the debounce the subprocess restarts (sentinel gets 2 lines).
func TestDevWatch_RestartOnGoFileChange(t *testing.T) {
	bin := binPath(t)
	port := nextPort()

	watchDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(watchDir, "placeholder.go"), []byte("package tmp\n"), 0644))

	// Sentinel is outside watchDir so writing to it does not trigger another restart.
	sentinel := filepath.Join(t.TempDir(), "sentinel")

	cmd := startDevWatch(t, bin, port, watchDir, sentinel)
	t.Cleanup(func() {
		cmd.Process.Signal(syscall.SIGINT) //nolint:errcheck
		cmd.Wait()                         //nolint:errcheck
	})

	// Wait for the daemon to boot and the subprocess to run for the first time.
	// The 20 s budget covers Chrome headless startup (~10-15 s in CI).
	waitForLines(t, sentinel, 1, 20*time.Second)

	// Trigger a restart by writing a new .go file into the watched directory.
	require.NoError(t, os.WriteFile(filepath.Join(watchDir, "change.go"), []byte("// change\n"), 0644))

	// Debounce is 200 ms; allow 5 s for the restart to complete.
	waitForLines(t, sentinel, 2, 5*time.Second)
}

// TestDevWatch_SignalStopsCleanly verifies that SIGINT causes browsii dev to
// shut down gracefully and exit within a reasonable window.
func TestDevWatch_SignalStopsCleanly(t *testing.T) {
	bin := binPath(t)
	port := nextPort()

	watchDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(watchDir, "placeholder.go"), []byte("package tmp\n"), 0644))

	sentinel := filepath.Join(t.TempDir(), "sentinel")

	cmd := startDevWatch(t, bin, port, watchDir, sentinel)

	// Wait for initial run before sending the signal.
	waitForLines(t, sentinel, 1, 20*time.Second)

	require.NoError(t, cmd.Process.Signal(syscall.SIGINT))

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	select {
	case <-exited:
		// Any exit code is acceptable — the subprocess may exit with 130 (signal).
	case <-time.After(10 * time.Second):
		cmd.Process.Kill() //nolint:errcheck
		t.Fatal("browsii dev did not exit within 10 s of SIGINT")
	}
}
