package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-rod/rod/lib/proto"
)

func (s *Server) registerSessionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/session/save", s.handleSessionSave)
	mux.HandleFunc("/session/new", s.handleSessionNew)
	mux.HandleFunc("/session/resume", s.handleSessionResume)
	mux.HandleFunc("/session/list", s.handleSessionList)
	mux.HandleFunc("/session/delete", s.handleSessionDelete)
}

// /session/save endpoint — snapshots all tabs to disk
func (s *Server) handleSessionSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pages, _ := s.browser.Pages()
	var tabs []map[string]interface{}
	activeTab := 0

	for i, p := range pages {
		info, err := p.Info()
		if err != nil {
			continue
		}
		scroll := p.MustEval("() => ({ x: window.scrollX, y: window.scrollY })")
		tabs = append(tabs, map[string]interface{}{
			"url":     info.URL,
			"scrollX": scroll.Get("x").Int(),
			"scrollY": scroll.Get("y").Int(),
		})
		if s.activePg != nil && p.TargetID == s.activePg.TargetID {
			activeTab = i
		}
	}

	session := map[string]interface{}{
		"name":      req.Name,
		"activeTab": activeTab,
		"tabs":      tabs,
	}

	// Write to ~/.browsii/sessions/<name>.json
	homeDir, _ := os.UserHomeDir()
	sessDir := filepath.Join(homeDir, ".browsii", "sessions")
	os.MkdirAll(sessDir, 0755) //nolint:errcheck

	data, _ := json.MarshalIndent(session, "", "  ")
	if err := os.WriteFile(filepath.Join(sessDir, req.Name+".json"), data, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.recordAction("session_save", map[string]interface{}{"name": req.Name})
	w.WriteHeader(http.StatusOK)
}

// /session/new endpoint — closes all tabs and starts fresh
func (s *Server) handleSessionNew(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck

	// Open a blank page first (so browser always has at least one)
	blank := s.browser.MustPage("about:blank").MustWaitLoad()

	// Close all other pages
	pages, _ := s.browser.Pages()
	for _, p := range pages {
		if p.TargetID != blank.TargetID {
			p.MustClose()
		}
	}

	s.pageOrder = nil
	s.listenedPages = make(map[proto.TargetTargetID]struct{})
	s.consoleListenedPages = make(map[proto.TargetTargetID]struct{})
	s.trackPage(blank)
	s.activePg = blank
	s.recordAction("session_new", map[string]interface{}{"name": req.Name})
	w.WriteHeader(http.StatusOK)
}

// /session/resume endpoint — restores tabs from a saved session
func (s *Server) handleSessionResume(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	homeDir, _ := os.UserHomeDir()
	sessFile := filepath.Join(homeDir, ".browsii", "sessions", req.Name+".json")
	data, err := os.ReadFile(sessFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("session %q not found", req.Name), http.StatusNotFound)
		return
	}

	var session struct {
		ActiveTab int `json:"activeTab"`
		Tabs      []struct {
			URL     string `json:"url"`
			ScrollX int    `json:"scrollX"`
			ScrollY int    `json:"scrollY"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Restore tabs first, then close old pages
	oldPages, _ := s.browser.Pages()
	oldIDs := make(map[string]bool)
	for _, p := range oldPages {
		oldIDs[string(p.TargetID)] = true
	}

	s.pageOrder = nil
	s.listenedPages = make(map[proto.TargetTargetID]struct{})
	s.consoleListenedPages = make(map[proto.TargetTargetID]struct{})
	for i, tab := range session.Tabs {
		page := s.browser.MustPage(tab.URL).MustWaitLoad()
		s.trackPage(page)
		if tab.ScrollX != 0 || tab.ScrollY != 0 {
			page.MustEval(fmt.Sprintf("() => window.scrollTo(%d, %d)", tab.ScrollX, tab.ScrollY))
		}
		if i == session.ActiveTab {
			s.activePg = page
			page.MustActivate() // Changed from MustBringToFront to MustActivate
		}
	}

	// Now close the old pages
	for _, p := range oldPages {
		p.MustClose()
	}

	s.recordAction("session_resume", map[string]interface{}{"name": req.Name})
	w.WriteHeader(http.StatusOK)
}

// /session/list endpoint — returns available session names
func (s *Server) handleSessionList(w http.ResponseWriter, r *http.Request) {
	homeDir, _ := os.UserHomeDir()
	sessDir := filepath.Join(homeDir, ".browsii", "sessions")

	entries, err := os.ReadDir(sessDir)
	if err != nil {
		// No sessions directory yet — return empty
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]") //nolint:errcheck
		return
	}

	var sessions []map[string]string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		info, _ := e.Info()
		sessions = append(sessions, map[string]string{
			"name":     name,
			"modified": info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	if sessions == nil {
		sessions = []map[string]string{}
	}

	s.recordAction("session_list", nil)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions) //nolint:errcheck
}

// /session/delete endpoint — removes a saved session
func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	homeDir, _ := os.UserHomeDir()
	sessFile := filepath.Join(homeDir, ".browsii", "sessions", req.Name+".json")
	if err := os.Remove(sessFile); err != nil {
		http.Error(w, fmt.Sprintf("session %q not found", req.Name), http.StatusNotFound)
		return
	}

	s.recordAction("session_delete", map[string]interface{}{"name": req.Name})
	w.WriteHeader(http.StatusOK)
}
