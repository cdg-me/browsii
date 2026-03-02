package daemon

import (
	"encoding/json"
	"net/http"

	"github.com/go-rod/rod/lib/proto"
)

func (s *Server) registerConsoleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/console/capture/start", s.handleConsoleCaptureStart)
	mux.HandleFunc("/console/capture/stop", s.handleConsoleCaptureStop)
}

// /console/capture/start endpoint — begins buffering console events.
// Optional JSON body: {"tab": "<alias>", "level": "<comma-separated levels>"}
// Tab aliases: "" or "all" = all tabs, "active", "next", "last", "<N>".
// Level filter: "" = all levels, or e.g. "error,warn".
func (s *Server) handleConsoleCaptureStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tab   string `json:"tab"`
		Level string `json:"level"`
	}
	// Ignore decode errors — body is optional; defaults are all tabs, all levels.
	json.NewDecoder(r.Body).Decode(&req)

	// Resolve the tab alias to an integer index.
	var activeID proto.TargetTargetID
	if ap := s.activePage(); ap != nil {
		activeID = ap.TargetID
	}
	tabFilter := resolveTabAlias(req.Tab, s.pageOrder, activeID)

	s.mu.Lock()
	s.consoleCapturing = true
	s.consoleCapturedEntries = nil
	s.consoleTabFilter = tabFilter
	s.consoleLevelFilter = req.Level
	s.mu.Unlock()

	s.recordAction("console_capture_start", map[string]interface{}{"tab": req.Tab, "level": req.Level})
	w.WriteHeader(http.StatusOK)
}

// /console/capture/stop endpoint — stops buffering and returns captured entries.
func (s *Server) handleConsoleCaptureStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.consoleCapturing = false
	entries := s.consoleCapturedEntries
	s.consoleCapturedEntries = nil
	s.mu.Unlock()

	if entries == nil {
		entries = []map[string]interface{}{}
	}

	s.recordAction("console_capture_stop", nil)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}
