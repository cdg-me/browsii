package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// SendCommand sends a JSON payload to a specific endpoint on the daemon.
func SendCommand(port int, endpoint string, payload interface{}) ([]byte, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/%s", port, endpoint)

	var reqBody io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second} // Allow time for network requests/waiting
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon on port %d: %w", port, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("daemon returned error: %s", string(body))
	}

	return body, nil
}

// StreamEvent matches the daemon's JSON payload
type StreamEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// SubscribeToEvents opens an SSE connection to the daemon and streams events
// via the provided callback until the context is canceled.
func SubscribeToEvents(ctx context.Context, port int, onEvent func(StreamEvent)) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/events/stream", port)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to event stream: %w", err)
	}

	go func() {
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)

		for scanner.Scan() {
			line := scanner.Text()

			// SSE lines are formatted as "data: {json}"
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")

				var event StreamEvent
				if err := json.Unmarshal([]byte(data), &event); err == nil {
					// Handle the native backpressure overflow warning specifically
					if event.Type == "overflow_warning" {
						if msgMap, ok := event.Payload.(map[string]interface{}); ok {
							if msg, ok := msgMap["message"].(string); ok {
								// CRITICAL: Push overflow warnings directly to stderr so
								// the automation executor (human/LLM) sees the silent data loss.
								fmt.Fprintf(os.Stderr, "[browsii] WARNING: %s\n", msg)
							}
						}
					}

					// Bubble the event to the WASM runtime guest
					onEvent(event)
				}
			}
		}
	}()

	return nil
}
