package daemon

// broadcastEvent sends an event to all connected SSE clients.
// It uses a non-blocking send; if a client's channel is full, the event is dropped
// and an overflow warning is enqueued for that specific client instead.
func (s *Server) broadcastEvent(event StreamEvent) {
	s.sseMu.RLock()
	defer s.sseMu.RUnlock()

	for ch := range s.sseClients {
		select {
		case ch <- event:
		default:
			// The channel buffer is full (client is blocking/slow).
			// We MUST NOT block the daemon. Drop the oldest event by draining one,
			// then push an OverflowWarning.
			select {
			case <-ch: // drop oldest
			default:
			}

			warning := StreamEvent{
				Type: EventOverflow,
				Payload: map[string]string{
					"message": "Daemon SSE buffer overflow: events were dropped because the client is consuming too slowly.",
				},
			}

			// Try to push the warning, but don't block if even that fails
			select {
			case ch <- warning:
			default:
			}
		}
	}
}
