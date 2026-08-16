package daemon

import (
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// Ring buffer sizes. Large enough to cover a full page load plus follow-up
// actions; small enough to be cheap on long-lived daemons.
const (
	netRingCap     = 500
	consoleRingCap = 300
)

// netRingEntry is one recent network request observed on a tracked page.
// Status is filled in when the response arrives; 0 means no response yet.
type netRingEntry struct {
	Seq    int64  `json:"seq"`
	URL    string `json:"url"`
	Method string `json:"method"`
	Tab    int    `json:"tab"`
	Status int    `json:"status"`
	TS     float64
}

// consoleRingEntry is one recent console error/warn on a tracked page.
type consoleRingEntry struct {
	Seq   int64  `json:"seq"`
	Level string `json:"level"`
	Text  string `json:"text"`
	Tab   int    `json:"tab"`
	TS    float64
}

// nextEventSeq returns the next monotonic event sequence number.
// Callers must hold s.mu.
func (s *Server) nextEventSeq() int64 {
	s.eventSeq++
	return int64(s.eventSeq)
}

// pushNetRing appends a request entry to the network ring (s.mu held).
func (s *Server) pushNetRing(e netRingEntry) *netRingEntry {
	entry := e
	entry.TS = float64(time.Now().UnixNano()) / 1e9
	s.netRing = append(s.netRing, entry)
	if len(s.netRing) > netRingCap {
		s.netRing = s.netRing[len(s.netRing)-netRingCap:]
	}
	return &s.netRing[len(s.netRing)-1]
}

// pushConsoleRing appends an error/warn entry to the console ring (s.mu held).
func (s *Server) pushConsoleRing(e consoleRingEntry) {
	e.TS = float64(time.Now().UnixNano()) / 1e9
	s.consoleRing = append(s.consoleRing, e)
	if len(s.consoleRing) > consoleRingCap {
		s.consoleRing = s.consoleRing[len(s.consoleRing)-consoleRingCap:]
	}
}

// currentEventSeq snapshots the ring sequence (use as the "since" anchor).
func (s *Server) currentEventSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(s.eventSeq)
}

// netRequestsSince returns ring entries with Seq greater than since,
// oldest first.
func (s *Server) netRequestsSince(since int64) []netRingEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []netRingEntry
	for _, e := range s.netRing {
		if e.Seq > since {
			out = append(out, e)
		}
	}
	return out
}

// consoleErrorsSince returns error-level console entries with Seq greater
// than since, oldest first.
func (s *Server) consoleErrorsSince(since int64) []consoleRingEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []consoleRingEntry
	for _, e := range s.consoleRing {
		if e.Seq > since && e.Level == "error" {
			out = append(out, e)
		}
	}
	return out
}

// formatNetEntry renders a ring entry as "METHOD path (status)" for
// compact reporting to agents. Long URLs (inline data:, long queries)
// are truncated so one entry cannot flood a failure listing.
func formatNetEntry(e netRingEntry) string {
	url := e.URL
	if i := strings.Index(url, "://"); i >= 0 {
		rest := url[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			url = rest[j:]
		}
	}
	if len(url) > 100 {
		url = url[:100] + "…"
	}
	status := "pending"
	if e.Status > 0 {
		status = strconv.Itoa(e.Status)
	}
	return e.Method + " " + url + " (" + status + ")"
}

// ringCorrelate associates a response status with its ring entry.
// Callers must hold s.mu.
func (s *Server) ringCorrelate(id proto.NetworkRequestID, status int) {
	if e, ok := s.ringInFlight[id]; ok {
		e.Status = status
		delete(s.ringInFlight, id)
	}
}

// ringDrop removes an in-flight ring entry (loading failed / cancelled).
// Callers must hold s.mu.
func (s *Server) ringDrop(id proto.NetworkRequestID) {
	delete(s.ringInFlight, id)
}
