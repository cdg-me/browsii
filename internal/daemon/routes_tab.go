package daemon

import (
	"encoding/json"
	"net/http"
)

func (s *Server) registerTabRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/tab/new", s.handleTabNew)
	mux.HandleFunc("/tab/list", s.handleTabList)
	mux.HandleFunc("/tab/close", s.handleTabClose)
	mux.HandleFunc("/tab/switch", s.handleTabSwitch)
}

func (s *Server) handleTabNew(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.activePg = s.browser.MustPage("")
	s.trackPage(s.activePg) // also registers the network listener
	s.activePg.MustNavigate(req.URL).MustWaitLoad()

	s.recordAction("tab_new", map[string]interface{}{"url": req.URL})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleTabList(w http.ResponseWriter, r *http.Request) {
	pages := s.orderedPages()
	var result []map[string]interface{}

	for i, p := range pages {
		info, err := p.Info()
		if err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"index": i,
			"id":    p.TargetID, // TargetID is a stable string identifier
			"url":   info.URL,
			"title": info.Title,
		})
	}

	s.recordAction("tab_list", nil)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result) //nolint:errcheck
}

func (s *Server) handleTabClose(w http.ResponseWriter, r *http.Request) {
	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages to close", http.StatusBadRequest)
		return
	}

	s.untrackPage(page)
	page.MustClose()
	s.activePg = nil // Reset so activePage() falls back to pages[0]
	s.recordAction("tab_close", nil)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleTabSwitch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Index int `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pages := s.orderedPages()
	if req.Index < 0 || req.Index >= len(pages) {
		http.Error(w, "invalid tab index", http.StatusBadRequest)
		return
	}

	s.activePg = pages[req.Index]
	s.activePg.MustActivate()
	s.recordAction("tab_switch", map[string]interface{}{"index": req.Index})
	w.WriteHeader(http.StatusOK)
}
