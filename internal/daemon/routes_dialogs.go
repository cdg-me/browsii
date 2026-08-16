package daemon

import (
	"encoding/json"
	"net/http"
)

// handleDialogs manages dialog auto-handling policy and history.
//
// GET /dialogs returns the current policy and recent auto-handled dialogs.
// POST /dialogs {"policy":"accept"|"dismiss","prompt_text":"...","clear":bool}
// updates the policy and/or clears the history, then returns the same shape.
func (s *Server) handleDialogs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Policy     string `json:"policy"`
		PromptText string `json:"prompt_text"`
		Clear      bool   `json:"clear"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	if req.Policy != "" && req.Policy != "accept" && req.Policy != "dismiss" {
		http.Error(w, "invalid policy: use accept or dismiss", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	changed := false
	if req.Policy != "" && req.Policy != s.dialogPolicy {
		s.dialogPolicy = req.Policy
		changed = true
	}
	if req.PromptText != "" {
		s.dialogPromptText = req.PromptText
		changed = true
	}
	if req.Clear {
		s.dialogLog = nil
	}
	policy := s.dialogPolicy
	prompt := s.dialogPromptText
	s.mu.Unlock()

	resp := map[string]interface{}{
		"policy":      policy,
		"prompt_text": prompt,
		"recent":      s.peekDialogs(),
	}
	if changed {
		resp["message"] = "dialog policy updated"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) registerDialogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/dialogs", s.handleDialogs)
}
