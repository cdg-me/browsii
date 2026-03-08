package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

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
		Tab    string `json:"tab"`
		Level  string `json:"level"`
		Output string `json:"output"`
		Format string `json:"format"`
	}
	// Ignore decode errors — body is optional; defaults are all tabs, all levels, no output file.
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck

	// Resolve the tab alias to an integer index.
	var activeID proto.TargetTargetID
	if ap := s.activePage(); ap != nil {
		activeID = ap.TargetID
	}
	tabFilter := resolveTabAlias(req.Tab, s.pageOrder, activeID)
	pages := s.pagesForFilter(tabFilter)

	s.mu.Lock()
	s.consoleCapturing = true
	s.consoleCapturedEntries = nil
	s.consoleTabFilter = tabFilter
	s.consoleLevelFilter = req.Level
	s.consoleCaptureOutputPath = req.Output
	s.consoleCaptureOutputFormat = req.Format
	s.consoleCapturingPages = pages
	s.mu.Unlock()

	s.consoleDomain.acquirePages(pages)

	s.recordAction("console_capture_start", map[string]interface{}{"tab": req.Tab, "level": req.Level, "output": req.Output, "format": req.Format})
	w.WriteHeader(http.StatusOK)
}

// /console/capture/stop endpoint — stops buffering, writes to file if configured, returns entries.
func (s *Server) handleConsoleCaptureStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.consoleCapturing = false
	entries := s.consoleCapturedEntries
	s.consoleCapturedEntries = nil
	outputPath := s.consoleCaptureOutputPath
	outputFormat := s.consoleCaptureOutputFormat
	s.consoleCaptureOutputPath = ""
	s.consoleCaptureOutputFormat = ""
	pages := s.consoleCapturingPages
	s.consoleCapturingPages = nil
	s.mu.Unlock()

	s.consoleDomain.releasePages(pages)

	if entries == nil {
		entries = []map[string]interface{}{}
	}

	s.recordAction("console_capture_stop", nil)
	w.Header().Set("Content-Type", "application/json")

	raw, err := json.Marshal(entries)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out, err := formatConsoleEntries(raw, outputFormat)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if outputPath != "" {
		if err := os.WriteFile(outputPath, out, 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		buf, err := json.Marshal(map[string]interface{}{
			"path":  outputPath,
			"count": len(entries),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(buf) //nolint:errcheck
		return
	}

	// No output file: send formatted data in the response.
	if outputFormat == "" || outputFormat == "json" {
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "text/plain")
	}
	w.Write(out) //nolint:errcheck
}

// formatConsoleEntries converts a raw JSON array of console entries into the
// requested format: "json" (default), "ndjson", or "text".
func formatConsoleEntries(raw []byte, format string) ([]byte, error) {
	switch format {
	case "", "json":
		return raw, nil

	case "ndjson":
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		for _, e := range entries {
			buf.Write(e)
			buf.WriteByte('\n')
		}
		return buf.Bytes(), nil

	case "text":
		var entries []struct {
			Level string `json:"level"`
			Text  string `json:"text"`
			Tab   int    `json:"tab"`
		}
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		for _, e := range entries {
			fmt.Fprintf(&buf, "[%-5s] tab=%d: %s\n", e.Level, e.Tab, e.Text)
		}
		return buf.Bytes(), nil

	default:
		return nil, fmt.Errorf("unknown format %q: use json, ndjson, or text", format)
	}
}
