package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSSEBroadcastAndBackpressure verifies the SSE contract: a slow client
// never blocks the daemon, overflowing events are dropped (oldest first)
// with a visible overflow_warning, and the stream recovers to deliver later
// events once the client drains.
//
// The consumer is held until the burst completes so the overflow cascade is
// deterministic regardless of scheduling; every read is deadline-guarded so
// a regression fails the test instead of hanging it.
func TestSSEBroadcastAndBackpressure(t *testing.T) {
	s := NewServer(0, "headless")

	releaseConsumer := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/events/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		clientChan := make(chan StreamEvent, 2)
		s.sseMu.Lock()
		s.sseClients[clientChan] = struct{}{}
		s.sseMu.Unlock()

		defer func() {
			s.sseMu.Lock()
			delete(s.sseClients, clientChan)
			close(clientChan)
			s.sseMu.Unlock()
		}()

		flusher, _ := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		// Park until the test has fired the burst; registration already
		// happened, so broadcasts overflow the buffer deterministically.
		select {
		case <-r.Context().Done():
			return
		case <-releaseConsumer:
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case event := <-clientChan:
				data, _ := json.Marshal(event)
				w.Write([]byte("data: " + string(data) + "\n\n")) //nolint:errcheck
				flusher.Flush()
			}
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/events/stream", nil)
	clientResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to SSE: %v", err)
	}
	defer clientResp.Body.Close() //nolint:errcheck

	lines := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(clientResp.Body)
		for scanner.Scan() {
			if text := scanner.Text(); strings.HasPrefix(text, "data: ") {
				lines <- text
			}
		}
		close(lines)
	}()

	// collect fails the test on timeout instead of blocking forever.
	collect := func(n int) []string {
		t.Helper()
		deadline := time.After(5 * time.Second)
		var got []string
		for len(got) < n {
			select {
			case line, ok := <-lines:
				if !ok {
					t.Fatalf("stream closed after %d lines, wanted %d: %v", len(got), n, got)
				}
				got = append(got, line)
			case <-deadline:
				t.Fatalf("timed out collecting %d lines, got %v", n, got)
			}
		}
		return got
	}

	// Burst while the consumer is parked: buffer fills, e1/e2 are dropped
	// oldest-first, warnings are enqueued. Final buffer: [warn, warn].
	s.broadcastEvent(StreamEvent{Type: EventNetworkRequest, Payload: "event 1"})
	s.broadcastEvent(StreamEvent{Type: EventNetworkRequest, Payload: "event 2"})
	s.broadcastEvent(StreamEvent{Type: EventNetworkRequest, Payload: "event 3"})
	s.broadcastEvent(StreamEvent{Type: EventNetworkRequest, Payload: "event 4"})

	close(releaseConsumer)

	// The consumer delivers the two warnings; receiving both proves the
	// buffer is empty and the handler is parked in its select.
	overflow := collect(2)
	if !strings.Contains(overflow[0], "overflow_warning") || !strings.Contains(overflow[1], "overflow_warning") {
		t.Fatalf("expected two overflow warnings first, got: %v", overflow)
	}

	// Recovery: with the buffer drained, the next event is delivered.
	s.broadcastEvent(StreamEvent{Type: EventNetworkRequest, Payload: "event 5"})
	recovered := collect(1)
	if !strings.Contains(recovered[0], "event 5") {
		t.Errorf("expected event 5 delivered after drain, got: %v", recovered)
	}
	if strings.Contains(strings.Join(append(overflow, recovered...), "\n"), "event 1") {
		t.Errorf("dropped events must not be delivered, got: %v %v", overflow, recovered)
	}
}
