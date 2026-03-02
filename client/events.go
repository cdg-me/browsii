package client

import (
	"context"
	"encoding/json"

	iclient "github.com/cdg-me/browsii/internal/client"
)

// OnNetworkRequest subscribes to browser network request events, calling cb for
// each one. It blocks until ctx is cancelled. Run it in a goroutine if needed.
func (c *Client) OnNetworkRequest(ctx context.Context, cb func(NetworkRequest)) error {
	return iclient.SubscribeToEvents(ctx, c.port, func(e iclient.StreamEvent) {
		if e.Type != "network_request" {
			return
		}
		b, err := json.Marshal(e.Payload)
		if err != nil {
			return
		}
		var req NetworkRequest
		if err := json.Unmarshal(b, &req); err != nil {
			return
		}
		cb(req)
	})
}

// OnConsoleEvent subscribes to browser console events, calling cb for each one.
// It blocks until ctx is cancelled. Run it in a goroutine if needed.
func (c *Client) OnConsoleEvent(ctx context.Context, cb func(ConsoleEntry)) error {
	return iclient.SubscribeToEvents(ctx, c.port, func(e iclient.StreamEvent) {
		if e.Type != "console" {
			return
		}
		b, err := json.Marshal(e.Payload)
		if err != nil {
			return
		}
		var entry ConsoleEntry
		if err := json.Unmarshal(b, &entry); err != nil {
			return
		}
		cb(entry)
	})
}
