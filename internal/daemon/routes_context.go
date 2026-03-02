package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Server) registerContextRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/context/create", s.handleContextCreate)
	mux.HandleFunc("/context/switch", s.handleContextSwitch)
}

// /context/create endpoint — creates a new isolated browser context
func (s *Server) handleContextCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Name == "default" {
		http.Error(w, "name must be non-empty and not 'default'", http.StatusBadRequest)
		return
	}

	if s.contexts == nil {
		s.contexts = make(map[string]*contextState)
	}

	// Create an incognito browser context (isolated cookies, storage, etc.)
	incognito := s.browser.MustIncognito()
	page := incognito.MustPage("about:blank")

	s.contexts[req.Name] = &contextState{
		browser: incognito,
		page:    page,
	}
	s.activeCtx = req.Name

	s.recordAction("context_create", map[string]interface{}{"name": req.Name})
	w.WriteHeader(http.StatusOK)
}

// /context/switch endpoint — switches to a named context
func (s *Server) handleContextSwitch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "default" || req.Name == "" {
		s.activeCtx = ""
		w.WriteHeader(http.StatusOK)
		return
	}

	if s.contexts == nil || s.contexts[req.Name] == nil {
		http.Error(w, fmt.Sprintf("context %q not found", req.Name), http.StatusNotFound)
		return
	}

	s.activeCtx = req.Name
	s.recordAction("context_switch", map[string]interface{}{"name": req.Name})
	w.WriteHeader(http.StatusOK)
}
