package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"time"

	"github.com/go-rod/rod/lib/proto"
)

// downloadsCap bounds the tracked-download table.
const downloadsCap = 50

// downloadEntry is one tracked browser download.
type downloadEntry struct {
	GUID          string  `json:"guid"`
	Filename      string  `json:"filename"`
	URL           string  `json:"url"`
	State         string  `json:"state"` // inProgress | completed | canceled
	ReceivedBytes float64 `json:"receivedBytes"`
	TotalBytes    float64 `json:"totalBytes"`
	Path          string  `json:"path,omitempty"`
	TS            float64 `json:"ts"`

	startedAt time.Time
}

// downloadsDir is where the daemon routes browser downloads.
func (s *Server) downloadsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".browsii", "downloads", fmt.Sprintf("port-%d", s.port))
}

// startDownloadManagement configures the browser to save downloads into the
// daemon-managed directory and tracks their lifecycle. Without this,
// downloads land in ~/Downloads invisibly — a click that triggers one
// reports nothing.
func (s *Server) startDownloadManagement() error {
	dir := s.downloadsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// The behavior must be set before the event subscription is armed:
	// subscribing first empirically delivers no events with this
	// rod/Chromium combination.
	if err := (proto.BrowserSetDownloadBehavior{
		Behavior:         proto.BrowserSetDownloadBehaviorBehaviorAllowAndName,
		BrowserContextID: s.browser.BrowserContextID,
		DownloadPath:     dir,
	}).Call(s.browser); err != nil {
		return err
	}

	s.downloadWait = s.browser.EachEvent(
		func(e *proto.PageDownloadWillBegin) {
			s.mu.Lock()
			s.trackDownloadLocked(downloadEntry{
				GUID:     e.GUID,
				Filename: e.SuggestedFilename,
				URL:      e.URL,
				State:    "inProgress",
				TS:       float64(time.Now().UnixNano()) / 1e9,
			})
			s.mu.Unlock()
		},
		func(e *proto.PageDownloadProgress) {
			s.mu.Lock()
			defer s.mu.Unlock()
			entry, ok := s.downloadByGUIDLocked(e.GUID)
			if !ok {
				return
			}
			entry.ReceivedBytes = e.ReceivedBytes
			entry.TotalBytes = e.TotalBytes
			switch e.State {
			case proto.PageDownloadProgressStateCompleted:
				entry.State = "completed"
				entry.Path = s.resolveDownloadPathLocked(*entry)
			case proto.PageDownloadProgressStateCanceled:
				entry.State = "canceled"
			}
		},
	)
	go func() { s.downloadWait() }()
	return nil
}

// resolveDownloadPathLocked finds the completed file on disk: AllowAndName
// names it by GUID; a file matching the suggested name (browser may dedupe
// with " (1)" suffixes) is preferred when present.
func (s *Server) resolveDownloadPathLocked(entry downloadEntry) string {
	dir := s.downloadsDir()
	if _, err := os.Stat(filepath.Join(dir, entry.Filename)); err == nil {
		return filepath.Join(dir, entry.Filename)
	}
	guidPath := filepath.Join(dir, entry.GUID)
	if _, err := os.Stat(guidPath); err == nil {
		// Rename to the suggested name so the artifact is identifiable.
		target := filepath.Join(dir, entry.Filename)
		if os.Rename(guidPath, target) == nil {
			return target
		}
		return guidPath
	}
	return ""
}

// trackDownloadLocked inserts or replaces an entry, newest first, capped.
// Callers must hold s.mu.
func (s *Server) trackDownloadLocked(e downloadEntry) {
	for i, existing := range s.downloads {
		if existing.GUID == e.GUID {
			s.downloads[i] = e
			return
		}
	}
	s.downloads = append([]downloadEntry{e}, s.downloads...)
	if len(s.downloads) > downloadsCap {
		s.downloads = s.downloads[:downloadsCap]
	}
}

func (s *Server) downloadByGUIDLocked(guid string) (*downloadEntry, bool) {
	for i := range s.downloads {
		if s.downloads[i].GUID == guid {
			return &s.downloads[i], true
		}
	}
	return nil, false
}

// downloadsSnapshot returns a copy of the tracked table, newest first.
func (s *Server) downloadsSnapshot() []downloadEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]downloadEntry, len(s.downloads))
	copy(out, s.downloads)
	sort.SliceStable(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	return out
}

// downloadsSince returns entries started after the anchor time, for
// receipts. ts is unix seconds.
func (s *Server) downloadsSince(ts float64) []downloadEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []downloadEntry
	for _, d := range s.downloads {
		if d.TS >= ts {
			out = append(out, d)
		}
	}
	return out
}

// formatDownload renders an entry for receipts and listings.
func formatDownload(d downloadEntry) string {
	state := d.State
	if d.State == "inProgress" {
		state = fmt.Sprintf("%d/%d bytes", int64(d.ReceivedBytes), int64(d.TotalBytes))
	}
	return fmt.Sprintf("%s (%s)", d.Filename, state)
}

func (s *Server) registerDownloadRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/downloads", s.handleDownloads)
	mux.HandleFunc("/downloads/clear", s.handleDownloadsClear)
}

// handleDownloads lists tracked downloads.
func (s *Server) handleDownloads(w http.ResponseWriter, r *http.Request) {
	entries := s.downloadsSnapshot()
	if entries == nil {
		entries = []downloadEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"dir":       s.downloadsDir(),
		"downloads": entries,
	})
}

// handleDownloadsClear forgets tracked entries (files on disk are kept).
func (s *Server) handleDownloadsClear(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	cleared := len(s.downloads)
	s.downloads = nil
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"cleared": cleared})
}
