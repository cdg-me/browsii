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

func TestSSEBroadcastAndBackpressure(t *testing.T) {
	// 1. Setup a test Server with a small capacity channel for testing
	s := NewServer(0, "headless")

	// 2. Start a test HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/events/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		// Use a tiny buffer of 2 to easily trigger overflow
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

		ctx := r.Context()

		for {
			select {
			case <-ctx.Done():
				return
			case event := <-clientChan:
				time.Sleep(50 * time.Millisecond) // Simulate a slow client to allow the buffer to fill
				data, _ := json.Marshal(event)
				w.Write([]byte("data: " + string(data) + "\n\n")) //nolint:errcheck
				flusher.Flush()
			}
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 3. Connect a background client bound to a cancelable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/events/stream", nil)
	clientResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to connect to SSE: %v", err)
	}
	defer clientResp.Body.Close() //nolint:errcheck

	// 4. Force an overflow by sending 4 events into a capacity 2 channel
	s.broadcastEvent(StreamEvent{Type: EventNetworkRequest, Payload: "event 1"})
	s.broadcastEvent(StreamEvent{Type: EventNetworkRequest, Payload: "event 2"})
	s.broadcastEvent(StreamEvent{Type: EventNetworkRequest, Payload: "event 3"}) // Triggers overflow
	s.broadcastEvent(StreamEvent{Type: EventNetworkRequest, Payload: "event 4"})

	// 5. Read events from the stream (which is the capacity of the channel plus the initial instant read)
	scanner := bufio.NewScanner(clientResp.Body)
	var lines []string

	for len(lines) < 3 && scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "data: ") {
			lines = append(lines, text)
		}
	}

	output := strings.Join(lines, "\n")

	// Verification
	// The buffer should contain an overflow warning because the client was slow/blocked.
	if !strings.Contains(output, "overflow_warning") {
		t.Errorf("Expected overflow warning in output, got: %s", output)
	}

	if !strings.Contains(output, "event 3") && !strings.Contains(output, "event 4") {
		t.Errorf("Expected newer events to be preserved, got: %s", output)
	}
}
