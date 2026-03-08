package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (s *Server) registerInjectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/inject/js/add", s.handleInjectJSAdd)
	mux.HandleFunc("/inject/js/list", s.handleInjectJSList)
	mux.HandleFunc("/inject/js/clear", s.handleInjectJSClear)
}

// handleInjectJSAdd registers a JS snippet to run before any other scripts on
// future document loads for the targeted tab(s).
//
// Request body (exactly one of script/url must be set):
//
//	script  — raw JS source to inject
//	url     — URL whose content is fetched server-side at registration time and inlined
//	tab     — tab filter ("", "all", "active", "next", "last", "<N>"); default = all tabs
//
// Response: {"id":"inject-N"}
func (s *Server) handleInjectJSAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Script string `json:"script"`
		URL    string `json:"url"`
		Tab    string `json:"tab"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	// Validate: exactly one of script/url must be provided.
	hasScript := req.Script != ""
	hasURL := req.URL != ""
	if !hasScript && !hasURL {
		http.Error(w, "exactly one of 'script' or 'url' must be provided", http.StatusBadRequest)
		return
	}
	if hasScript && hasURL {
		http.Error(w, "only one of 'script' or 'url' may be provided, not both", http.StatusBadRequest)
		return
	}

	// For URL type: fetch eagerly at registration time so the content is
	// guaranteed to be available at future navigate calls even if the origin
	// becomes unreachable.
	isURL := hasURL
	sourceURL := req.URL
	script := req.Script
	if isURL {
		fetched, err := fetchScriptURL(req.URL)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch script URL: %v", err), http.StatusBadGateway)
			return
		}
		script = fetched
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Assign a stable ID.
	s.injectJSCounter++
	id := fmt.Sprintf("inject-%d", s.injectJSCounter)

	entry := injectJSEntry{
		ID:     id,
		Script: script,
		IsURL:  isURL,
		URL:    sourceURL,
		Tab:    req.Tab,
	}

	// Resolve which existing pages to register on.
	var activeID proto.TargetTargetID
	if ap := s.activePage(); ap != nil {
		activeID = ap.TargetID
	}
	tabFilter := resolveTabAlias(req.Tab, s.pageOrder, activeID)
	pages := s.pagesForFilter(tabFilter)

	// Register on each matching page and record CDP IDs for later removal.
	cdpIDs := make(map[proto.TargetTargetID]proto.PageScriptIdentifier)
	for _, p := range pages {
		sid, err := registerInjectScript(p, script)
		if err != nil {
			http.Error(w, fmt.Sprintf("CDP registration failed: %v", err), http.StatusInternalServerError)
			return
		}
		cdpIDs[p.TargetID] = sid
	}
	s.injectJSCDPIDs[id] = cdpIDs

	// Store the entry in the appropriate bucket so new pages pick it up.
	if tabFilter < 0 {
		// Global: applyInjectScriptsToNewPage will handle future tabs.
		s.injectJSGlobal = append(s.injectJSGlobal, entry)
	} else {
		// Per-tab: keyed to each matched TargetID.
		for _, p := range pages {
			s.injectJSByPage[p.TargetID] = append(s.injectJSByPage[p.TargetID], entry)
		}
	}

	s.recordAction("inject_js_add", map[string]interface{}{
		"script": script,
		"isURL":  isURL,
		"url":    sourceURL,
		"tab":    req.Tab,
		"id":     id,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id}) //nolint:errcheck
}

// handleInjectJSList returns all registered inject-js entries.
//
// Optional request body:
//
//	tab — filter as in add; omit to return all entries across all scopes
//
// Response: JSON array of injectJSEntry objects.
func (s *Server) handleInjectJSList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tab string `json:"tab"`
	}
	decodeBody(w, r, &req) //nolint:errcheck // body is optional

	s.mu.Lock()
	defer s.mu.Unlock()

	var entries []injectJSEntry

	if req.Tab == "" || req.Tab == "all" {
		// Return everything: global entries + all per-tab entries.
		entries = append(entries, s.injectJSGlobal...)
		for _, tabEntries := range s.injectJSByPage {
			entries = append(entries, tabEntries...)
		}
	} else {
		// Resolve to a specific page and return that page's global + per-tab.
		var activeID proto.TargetTargetID
		if ap := s.activePage(); ap != nil {
			activeID = ap.TargetID
		}
		tabFilter := resolveTabAlias(req.Tab, s.pageOrder, activeID)
		pages := s.pagesForFilter(tabFilter)

		// Always include globals.
		entries = append(entries, s.injectJSGlobal...)
		for _, p := range pages {
			entries = append(entries, s.injectJSByPage[p.TargetID]...)
		}
	}

	if entries == nil {
		entries = []injectJSEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries) //nolint:errcheck
}

// handleInjectJSClear deregisters inject-js scripts, stopping them from firing
// on future navigations. Has no effect on the currently loaded document.
//
// Optional request body:
//
//	tab — scope of the clear; omit/"all" = clear everything
//
// Response: 200 OK with no body.
func (s *Server) handleInjectJSClear(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tab string `json:"tab"`
	}
	decodeBody(w, r, &req) //nolint:errcheck // body is optional

	s.mu.Lock()
	defer s.mu.Unlock()

	clearAll := req.Tab == "" || req.Tab == "all"

	if clearAll {
		// Remove all global entries from every page they were registered on.
		for _, entry := range s.injectJSGlobal {
			s.removeCDPScript(entry.ID)
		}
		// Remove all per-tab entries.
		for _, tabEntries := range s.injectJSByPage {
			for _, entry := range tabEntries {
				s.removeCDPScript(entry.ID)
			}
		}
		s.injectJSGlobal = nil
		s.injectJSByPage = make(map[proto.TargetTargetID][]injectJSEntry)
	} else {
		// Resolve specific pages and clear only their per-tab entries.
		// Global entries are intentionally NOT cleared by a tab-scoped clear.
		var activeID proto.TargetTargetID
		if ap := s.activePage(); ap != nil {
			activeID = ap.TargetID
		}
		tabFilter := resolveTabAlias(req.Tab, s.pageOrder, activeID)
		pages := s.pagesForFilter(tabFilter)

		for _, p := range pages {
			for _, entry := range s.injectJSByPage[p.TargetID] {
				s.removeCDPScript(entry.ID)
			}
			delete(s.injectJSByPage, p.TargetID)
		}
	}

	s.recordAction("inject_js_clear", map[string]interface{}{"tab": req.Tab})
	w.WriteHeader(http.StatusOK)
}

// --- helpers -----------------------------------------------------------------

// registerInjectScript calls CDP addScriptToEvaluateOnNewDocument on p and
// returns the CDP ScriptIdentifier assigned to it.
func registerInjectScript(p *rod.Page, script string) (proto.PageScriptIdentifier, error) {
	res, err := proto.PageAddScriptToEvaluateOnNewDocument{Source: script}.Call(p)
	if err != nil {
		return "", err
	}
	return res.Identifier, nil
}

// removeCDPScript calls CDP removeScriptToEvaluateOnNewDocument for every page
// that has a CDP ID mapped to id. Must be called with s.mu held.
func (s *Server) removeCDPScript(id string) {
	pageIDs, ok := s.injectJSCDPIDs[id]
	if !ok {
		return
	}
	pages, _ := s.browser.Pages()
	byID := make(map[proto.TargetTargetID]*rod.Page, len(pages))
	for _, p := range pages {
		byID[p.TargetID] = p
	}
	for targetID, sid := range pageIDs {
		if p, ok := byID[targetID]; ok {
			_ = proto.PageRemoveScriptToEvaluateOnNewDocument{Identifier: sid}.Call(p)
		}
	}
	delete(s.injectJSCDPIDs, id)
}

// fetchScriptURL performs a server-side HTTP GET and returns the response body
// as a string. Returns an error if the request fails or returns non-2xx.
func fetchScriptURL(url string) (string, error) {
	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
