//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// waitForLinesUnit polls path until it contains at least want newline-terminated
// lines or the deadline passes.
func waitForLinesUnit(t *testing.T, path string, want int, timeout time.Duration) {
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
	t.Fatalf("timed out after %s waiting for %d lines in sentinel; got %d lines:\n%s",
		timeout, want, strings.Count(string(data), "\n"), data)
}

// sendSig sends sig on ch and waits for done to close, failing if timeout elapses.
func sendSigAndWait(t *testing.T, ch chan<- os.Signal, sig os.Signal, done <-chan struct{}, timeout time.Duration) {
	t.Helper()
	ch <- sig
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("runDevWatchLoop did not return within %s after %s", timeout, sig)
	}
}

// startLoop launches runDevWatchLoop in a goroutine and returns its channels.
// The caller must send a signal on sigCh to stop the loop.
func startLoop(t *testing.T, sentinel string, events chan fsnotify.Event, errs chan error) (sigCh chan os.Signal, done <-chan struct{}) {
	t.Helper()
	sig := make(chan os.Signal, 1)
	d := make(chan struct{})
	go func() {
		defer close(d)
		runDevWatchLoop(0,
			[]string{"sh", "-c", "echo x >> " + sentinel},
			sig, events, errs)
	}()
	return sig, d
}

// TestDevWatchLoop_InitialRunFires verifies the subprocess is launched once on startup.
func TestDevWatchLoop_InitialRunFires(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "sentinel")
	events := make(chan fsnotify.Event)
	errs := make(chan error)

	sigCh, done := startLoop(t, sentinel, events, errs)
	waitForLinesUnit(t, sentinel, 1, 5*time.Second)
	sendSigAndWait(t, sigCh, syscall.SIGINT, done, 3*time.Second)
}

// TestDevWatchLoop_GoFileWriteTriggersRestart verifies that a Write event on a
// .go file kills the running subprocess and starts a new one after the debounce.
func TestDevWatchLoop_GoFileWriteTriggersRestart(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "sentinel")
	events := make(chan fsnotify.Event, 1)
	errs := make(chan error)

	sigCh, done := startLoop(t, sentinel, events, errs)
	waitForLinesUnit(t, sentinel, 1, 5*time.Second)

	events <- fsnotify.Event{Name: "main.go", Op: fsnotify.Write}
	waitForLinesUnit(t, sentinel, 2, 5*time.Second)

	sendSigAndWait(t, sigCh, syscall.SIGINT, done, 3*time.Second)
}

// TestDevWatchLoop_CreateEventTriggersRestart verifies Create operations also
// trigger a restart (not only Write).
func TestDevWatchLoop_CreateEventTriggersRestart(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "sentinel")
	events := make(chan fsnotify.Event, 1)
	errs := make(chan error)

	sigCh, done := startLoop(t, sentinel, events, errs)
	waitForLinesUnit(t, sentinel, 1, 5*time.Second)

	events <- fsnotify.Event{Name: "new_file.go", Op: fsnotify.Create}
	waitForLinesUnit(t, sentinel, 2, 5*time.Second)

	sendSigAndWait(t, sigCh, syscall.SIGINT, done, 3*time.Second)
}

// TestDevWatchLoop_NonGoFileChangeIgnored verifies that events for non-.go files
// do not trigger a restart.
func TestDevWatchLoop_NonGoFileChangeIgnored(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "sentinel")
	events := make(chan fsnotify.Event, 1)
	errs := make(chan error)

	sigCh, done := startLoop(t, sentinel, events, errs)
	waitForLinesUnit(t, sentinel, 1, 5*time.Second)

	events <- fsnotify.Event{Name: "README.md", Op: fsnotify.Write}

	// Wait past the debounce window — no restart should fire.
	time.Sleep(500 * time.Millisecond)

	data, _ := os.ReadFile(sentinel)
	if got := strings.Count(string(data), "\n"); got != 1 {
		t.Fatalf("expected 1 line after non-.go event, got %d", got)
	}

	sendSigAndWait(t, sigCh, syscall.SIGINT, done, 3*time.Second)
}

// TestDevWatchLoop_SignalStops verifies that sending SIGINT causes the loop to
// return cleanly.
func TestDevWatchLoop_SignalStops(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "sentinel")
	events := make(chan fsnotify.Event)
	errs := make(chan error)

	sigCh, done := startLoop(t, sentinel, events, errs)
	waitForLinesUnit(t, sentinel, 1, 5*time.Second)
	sendSigAndWait(t, sigCh, syscall.SIGINT, done, 3*time.Second)
}

// TestDevWatchLoop_ClosedEventChannelStops verifies that closing the events
// channel causes the loop to return (mirrors real watcher teardown).
func TestDevWatchLoop_ClosedEventChannelStops(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "sentinel")
	events := make(chan fsnotify.Event)
	errs := make(chan error)

	done := make(chan struct{})
	go func() {
		defer close(done)
		runDevWatchLoop(0,
			[]string{"sh", "-c", "echo x >> " + sentinel},
			make(chan os.Signal, 1), events, errs)
	}()

	waitForLinesUnit(t, sentinel, 1, 5*time.Second)
	close(events)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runDevWatchLoop did not return after events channel was closed")
	}
}
