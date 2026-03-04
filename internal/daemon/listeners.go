package daemon

import (
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// attachNetworkListener binds CDP network listeners to a specific page.
// It is idempotent: calling it multiple times for the same page is a no-op.
//
// Three event types are always registered:
//   - NetworkRequestWillBeSent  — creates a capture entry with base fields
//   - NetworkResponseReceived   — updates the entry with response data
//   - NetworkLoadingFinished    — updates the entry with transfer size
//
// Optional fields are only populated when the corresponding group is in
// s.captureInclude (set via --include on start). The SSE broadcast always
// carries only the base fields for backward compatibility.
func (s *Server) attachNetworkListener(page *rod.Page) {
	// Guard: only one listener per page, ever.
	if _, already := s.listenedPages[page.TargetID]; already {
		return
	}
	s.listenedPages[page.TargetID] = struct{}{}

	// CRITICAL: The Chromium CDP Network domain is disabled by default.
	// We MUST explicitly enable it, or EachEvent drops all payloads silently.
	_ = proto.NetworkEnable{}.Call(page)

	wait := page.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			s.mu.Lock()

			tabIdx := -1
			for i, id := range s.pageOrder {
				if id == page.TargetID {
					tabIdx = i
					break
				}
			}

			if s.capturing && (s.captureTabFilter == -1 || s.captureTabFilter == tabIdx) {
				entry := &capturedRequest{
					URL:    e.Request.URL,
					Method: e.Request.Method,
					Type:   string(e.Type),
					Tab:    tabIdx,
				}

				// request-timestamp
				if s.captureInclude["request-timestamp"] {
					entry.Timestamp = float64(e.WallTime)
				}
				// Store startedAt for HAR regardless of --include (cheap, always useful)
				wallSecs := int64(float64(e.WallTime))
				wallNanos := int64((float64(e.WallTime) - float64(wallSecs)) * 1e9)
				if wallSecs > 0 {
					entry.startedAt = time.Unix(wallSecs, wallNanos)
				} else {
					entry.startedAt = time.Now()
				}

				// request-initiator
				if s.captureInclude["request-initiator"] && e.Initiator != nil {
					entry.Initiator = &capturedInitiator{
						Type: string(e.Initiator.Type),
						URL:  e.Initiator.URL,
					}
				}

				// request-headers (NetworkHeaders values are gson.JSON)
				if s.captureInclude["request-headers"] {
					hdrs := make(map[string]string, len(e.Request.Headers))
					for k, v := range e.Request.Headers {
						hdrs[k] = v.Str()
					}
					entry.RequestHeaders = hdrs
				}

				// request-body
				if s.captureInclude["request-body"] && e.Request.PostData != "" {
					entry.PostData = e.Request.PostData
				}

				s.capturedReqs = append(s.capturedReqs, entry)
				s.inFlightReqs[e.RequestID] = entry
			}

			s.mu.Unlock()

			// Broadcast to SSE consumers (uses sseMu — safe to call after releasing mu).
			s.broadcastEvent(StreamEvent{
				Type: EventNetworkRequest,
				Payload: map[string]interface{}{
					"url":    e.Request.URL,
					"method": e.Request.Method,
					"type":   string(e.Type),
					"tab":    tabIdx,
				},
			})
		},

		func(e *proto.NetworkResponseReceived) {
			s.mu.Lock()
			defer s.mu.Unlock()

			if !s.capturing {
				return
			}
			entry := s.inFlightReqs[e.RequestID]
			if entry == nil {
				return
			}

			// response-headers: status, statusText, headers, mimeType
			if s.captureInclude["response-headers"] {
				entry.Status = e.Response.Status
				entry.StatusText = e.Response.StatusText
				entry.MimeType = e.Response.MIMEType
				hdrs := make(map[string]string, len(e.Response.Headers))
				for k, v := range e.Response.Headers {
					hdrs[k] = v.Str()
				}
				entry.ResponseHeaders = hdrs
			}

			// response-timing: timing breakdown from CDP ResourceTiming
			if s.captureInclude["response-timing"] && e.Response.Timing != nil {
				t := e.Response.Timing
				timing := &capturedTiming{Receive: -1} // Receive filled by LoadingFinished

				timing.DNS = phaseDuration(t.DNSStart, t.DNSEnd)
				// Connect excludes SSL handshake per HAR convention
				ssl := phaseDuration(t.SslStart, t.SslEnd)
				rawConnect := phaseDuration(t.ConnectStart, t.ConnectEnd)
				if rawConnect >= 0 && ssl >= 0 {
					timing.Connect = rawConnect - ssl
				} else {
					timing.Connect = rawConnect
				}
				timing.SSL = ssl
				timing.Send = phaseDuration(t.SendStart, t.SendEnd)
				// Wait = time from end of send to receipt of first response byte
				if t.ReceiveHeadersEnd >= 0 && t.SendEnd >= 0 {
					timing.Wait = t.ReceiveHeadersEnd - t.SendEnd
				} else {
					timing.Wait = -1
				}
				entry.Timing = timing
			}
		},

		func(e *proto.NetworkLoadingFinished) {
			s.mu.Lock()
			defer s.mu.Unlock()

			if !s.capturing {
				return
			}
			entry := s.inFlightReqs[e.RequestID]
			if entry == nil {
				return
			}

			// response-size: bytes transferred over the wire
			if s.captureInclude["response-size"] {
				size := int64(e.EncodedDataLength)
				entry.TransferSize = &size
			}

			delete(s.inFlightReqs, e.RequestID)
		},
	)

	go wait()
}

// phaseDuration returns end-start if both are non-negative, else -1.
func phaseDuration(start, end float64) float64 {
	if start >= 0 && end >= 0 {
		return end - start
	}
	return -1
}

// attachConsoleListener binds a CDP Runtime console listener to a page.
// It is idempotent: calling it multiple times for the same page is a no-op.
func (s *Server) attachConsoleListener(page *rod.Page) {
	if _, already := s.consoleListenedPages[page.TargetID]; already {
		return
	}
	s.consoleListenedPages[page.TargetID] = struct{}{}

	// The Runtime CDP domain must be enabled before consoleAPICalled fires.
	_ = proto.RuntimeEnable{}.Call(page)

	wait := page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		level := normalizeConsoleLevel(e.Type)
		text := formatConsoleArgs(e.Args)
		args := buildConsoleArgs(e.Args)

		s.mu.Lock()
		tabIdx := -1
		for i, id := range s.pageOrder {
			if id == page.TargetID {
				tabIdx = i
				break
			}
		}
		if s.consoleCapturing &&
			(s.consoleTabFilter == -1 || s.consoleTabFilter == tabIdx) &&
			matchesLevelFilter(level, s.consoleLevelFilter) {
			s.consoleCapturedEntries = append(s.consoleCapturedEntries, map[string]interface{}{
				"level": level,
				"text":  text,
				"args":  args,
				"tab":   tabIdx,
			})
		}
		s.mu.Unlock()

		s.broadcastEvent(StreamEvent{
			Type: EventConsole,
			Payload: map[string]interface{}{
				"level": level,
				"text":  text,
				"args":  args,
				"tab":   tabIdx,
			},
		})
	})

	go wait()
}
