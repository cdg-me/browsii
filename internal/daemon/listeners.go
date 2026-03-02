package daemon

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// attachNetworkListener binds a CDP network trace to a specific page.
// It is idempotent: calling it multiple times for the same page is a no-op.
// All network events from the page are fanned out to:
//   - every connected SSE client (for WASM sdk.OnNetworkRequest callbacks), and
//   - the capture buffer (s.capturedReqs) when capturing is active and the tab
//     index matches s.captureTabFilter (-1 = all tabs).
func (s *Server) attachNetworkListener(page *rod.Page) {
	// Guard: only one listener per page, ever.
	if _, already := s.listenedPages[page.TargetID]; already {
		return
	}
	s.listenedPages[page.TargetID] = struct{}{}

	// CRITICAL: The Chromium CDP Network domain is disabled by default.
	// We MUST explicitly enable it, or EachEvent drops all payloads silently.
	_ = proto.NetworkEnable{}.Call(page)

	wait := page.EachEvent(func(e *proto.NetworkRequestWillBeSent) {
		// Resolve this page's current tab index and check capture filter under
		// the shared mutex so reads are consistent with concurrent writes.
		s.mu.Lock()
		tabIdx := -1
		for i, id := range s.pageOrder {
			if id == page.TargetID {
				tabIdx = i
				break
			}
		}
		if s.capturing && (s.captureTabFilter == -1 || s.captureTabFilter == tabIdx) {
			s.capturedReqs = append(s.capturedReqs, map[string]interface{}{
				"url":    e.Request.URL,
				"method": e.Request.Method,
				"type":   string(e.Type),
				"tab":    tabIdx,
			})
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
	})

	go wait()
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
