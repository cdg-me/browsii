package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Server) registerNavigationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/reload", s.handleReload)
	mux.HandleFunc("/back", s.handleBack)
	mux.HandleFunc("/forward", s.handleForward)
	mux.HandleFunc("/scroll", s.handleScroll)
	mux.HandleFunc("/navigate", s.handleNavigate)
	mux.HandleFunc("/upload", s.handleUpload)
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	page.MustReload().MustWaitLoad()
	s.recordAction("reload", nil)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleBack(w http.ResponseWriter, r *http.Request) {
	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	page.MustNavigateBack().MustWaitLoad()
	s.recordAction("back", nil)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleForward(w http.ResponseWriter, r *http.Request) {
	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	page.MustNavigateForward().MustWaitLoad()
	s.recordAction("forward", nil)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleScroll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Direction string `json:"direction"` // down, up, top, bottom
		Pixels    int    `json:"pixels"`
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

	var jsCode string
	switch req.Direction {
	case "down":
		jsCode = fmt.Sprintf("() => window.scrollBy(0, %d)", req.Pixels)
	case "up":
		jsCode = fmt.Sprintf("() => window.scrollBy(0, -%d)", req.Pixels)
	case "top":
		jsCode = "() => window.scrollTo(0, 0)"
	case "bottom":
		jsCode = "() => window.scrollTo(0, document.body.scrollHeight)"
	default:
		http.Error(w, "invalid direction: use down, up, top, or bottom", http.StatusBadRequest)
		return
	}

	page.MustEval(jsCode)
	s.recordAction("scroll", map[string]interface{}{"direction": req.Direction, "pixels": req.Pixels})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleNavigate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL        string `json:"url"`
		WaitUntil  string `json:"waitUntil"` // load, networkidle
		NoEvidence bool   `json:"noEvidence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sinceSeq := s.currentEventSeq()
	page := s.activePage()
	if page == nil {
		// Create a new page in the appropriate context
		if s.activeCtx != "" && s.contexts != nil {
			if ctx, ok := s.contexts[s.activeCtx]; ok {
				page = ctx.browser.MustPage("")
				ctx.page = page
				s.attachDialogListener(page)
			}
		}
		if page == nil {
			page = s.browser.MustPage("")
			s.activePg = page
			s.trackPage(page) // also registers the network listener
		}
		page.MustNavigate(req.URL)
	} else {
		page.MustNavigate(req.URL)
	}

	// Wait strategy
	switch req.WaitUntil {
	case "networkidle":
		page.MustWaitNavigation()()
	default: // "load" or empty
		page.MustWaitLoad()
	}

	// Navigation invalidates element refs: the DOM they indexed is gone.
	s.invalidateElementRefs(page.TargetID)

	s.recordAction("navigate", map[string]interface{}{"url": req.URL, "waitUntil": req.WaitUntil})

	// Receipt: requests fired during load, console errors, and any
	// beforeunload dialog that was auto-handled (dismissing one cancels
	// the navigation, so the agent must see it).
	s.writeEvidence(w, page, "", sinceSeq, req.NoEvidence)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Selector string   `json:"selector"`
		Files    []string `json:"files"`
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

	el := page.MustElement(req.Selector)
	el.MustSetFiles(req.Files...)
	s.recordAction("upload", map[string]interface{}{"selector": req.Selector, "files": req.Files})
	w.WriteHeader(http.StatusOK)
}
