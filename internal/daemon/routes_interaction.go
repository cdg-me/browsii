package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Server) registerInteractionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/press", s.handlePress)
	mux.HandleFunc("/hover", s.handleHover)
	mux.HandleFunc("/click", s.handleClick)
	mux.HandleFunc("/type", s.handleType)
}

func (s *Server) handlePress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	// Handle key combos like "Control+a" by using page.KeyActions()
	keys := parseKeyCombo(req.Key)
	ka := page.KeyActions()
	for _, k := range keys {
		ka = ka.Press(k)
	}
	ka.MustDo()
	s.recordAction("press", map[string]interface{}{"key": req.Key})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleHover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Selector string `json:"selector"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	page.MustElement(req.Selector).MustHover()
	s.recordAction("hover", map[string]interface{}{"selector": req.Selector})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleClick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Selector string `json:"selector"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	page.MustElement(req.Selector).MustClick()
	s.recordAction("click", map[string]interface{}{"selector": req.Selector})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleType(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Selector string `json:"selector"`
		Text     string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	// Wait for it to exist
	page.MustElement(req.Selector)

	// Safely clear and focus using JS. This avoids triggering chaotic framework
	// re-renders and node detachments that happen when simulating backspaces/deletes.
	js := fmt.Sprintf(`() => {
		const el = document.querySelector('%s');
		if (el) { el.value = ''; el.focus(); }
	}`, req.Selector)
	page.MustEval(js)

	// Now insert text as global keystrokes to whatever is focused
	page.MustInsertText(req.Text)

	s.recordAction("type", map[string]interface{}{"selector": req.Selector, "text": req.Text})
	w.WriteHeader(http.StatusOK)
}
