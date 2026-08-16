package daemon

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (s *Server) registerInteractionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/press", s.handlePress)
	mux.HandleFunc("/hover", s.handleHover)
	mux.HandleFunc("/click", s.handleClick)
	mux.HandleFunc("/type", s.handleType)
}

// resolveElementTarget maps a request's ref-or-selector pair to a CSS
// selector. When ref > 0 the recorded element is fingerprint-verified: the
// selector must still resolve to an element with the same identity (tag,
// role, text, name, href, type). A mismatch triggers retargeting by
// identity — the element moved but survived — and failing that, an
// actionable error. This prevents refs from silently landing on unrelated
// elements when the DOM shifts between 'elements' and the action.
func (s *Server) resolveElementTarget(page *rod.Page, ref int, selector string) (string, *apiError) {
	if ref > 0 {
		e := s.lookupRefInStore(page.TargetID, ref)
		if e == nil {
			// Store missing (never enumerated, or navigated away) or ref out
			// of range: refresh once. There is no prior identity to verify
			// against, so the fresh positional element is the best answer.
			elems, err := s.enumerateElements(page)
			if err != nil || ref > len(elems) {
				return "", notFoundError(
					"element ref "+strconv.Itoa(ref)+" not found — the page may have changed since 'elements' was last run",
					"run 'elements' again to get fresh refs, then retry with the new ref",
					nil,
				)
			}
			fresh := elems[ref-1]
			return fresh.Selector, nil
		}
		return s.verifyRefIdentity(page, e)
	}
	if selector == "" {
		return "", &apiError{
			Status:  http.StatusBadRequest,
			Message: "selector or ref is required",
			Hint:    "pass {\"ref\": N} from 'elements' or a CSS selector",
		}
	}
	return selector, nil
}

// verifyRefIdentity confirms selector still resolves to an element with the
// recorded identity. When it does not, a fresh enumeration is searched for
// the identity: found → retarget to the element's current selector; not
// found → fail with the original identity and candidates so the agent can
// recover in one round trip.
func (s *Server) verifyRefIdentity(page *rod.Page, e *elementInfo) (string, *apiError) {
	want := fingerprintOf(*e)
	if live, err := liveFingerprint(page, e.Selector); err == nil && live == want {
		return e.Selector, nil // identity intact — page is quiet
	}
	elems, err := s.enumerateElements(page)
	if err != nil {
		// Enumeration failed (e.g. page mid-navigation); the recorded
		// selector is still the best available answer.
		return e.Selector, nil
	}
	for i := range elems {
		if fingerprintOf(elems[i]) == want {
			return elems[i].Selector, nil // element survived the move
		}
	}
	label := e.Text
	if label == "" {
		label = e.Name
	}
	return "", notFoundError(
		fmt.Sprintf("element ref %d no longer matches: was %s %q → %s", e.Ref, e.Role, label, e.Selector),
		"the page changed since 'elements' was last run; re-run it for fresh refs",
		findCandidates(elems, label+" "+e.Selector, 5),
	)
}

// elementNotFound builds the actionable not-found response: a fresh
// enumeration with fuzzy candidates for the failed selector.
func (s *Server) elementNotFound(page *rod.Page, selector string) *apiError {
	return notFoundError(
		"element not found: "+selector,
		"",
		s.candidatesFor(page, selector),
	)
}

// elementWait bounds how long interaction handlers wait for an element to
// appear before failing with an actionable error. A short grace period covers
// SPA re-renders after navigation; failing fast matters more, because the
// error response carries candidate elements for an immediate retry.
const elementWait = 2 * time.Second

// findElement locates sel and fails with an actionable error when missing or
// hidden (hidden elements cannot receive trusted mouse events).
func (s *Server) findElement(page *rod.Page, selector string) (*rod.Element, *apiError) {
	el, err := page.Timeout(elementWait).Element(selector)
	if err != nil || el == nil {
		return nil, s.elementNotFound(page, selector)
	}
	visible, err := el.Visible()
	if err == nil && !visible {
		return nil, notFoundError(
			"element exists but is not visible: "+selector,
			"the element is hidden (display:none, zero size, or visibility:hidden) so it cannot be clicked",
			s.candidatesFor(page, selector),
		)
	}
	return el, nil
}

func (s *Server) handlePress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key        string `json:"key"`
		NoEvidence bool   `json:"noEvidence"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	urlBefore, sinceSeq := s.actionAnchors(page)

	// Handle key combos like "Control+a" by using page.KeyActions()
	keys := parseKeyCombo(req.Key)
	ka := page.KeyActions()
	for _, k := range keys {
		ka = ka.Press(k)
	}
	ka.MustDo()
	s.recordAction("press", map[string]interface{}{"key": req.Key})

	// Pressing Enter can submit forms (navigation) and trigger dialogs.
	s.writeEvidence(w, page, urlBefore, sinceSeq, req.NoEvidence)
}

func (s *Server) handleHover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref      int    `json:"ref"`
		Selector string `json:"selector"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	selector, aerr := s.resolveElementTarget(page, req.Ref, req.Selector)
	if aerr != nil {
		writeAPIError(w, aerr)
		return
	}
	el, aerr := s.findElement(page, selector)
	if aerr != nil {
		writeAPIError(w, aerr)
		return
	}
	if err := el.Hover(); err != nil {
		http.Error(w, "hover failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.recordAction("hover", map[string]interface{}{"selector": selector})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleClick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref        int    `json:"ref"`
		Selector   string `json:"selector"`
		NoEvidence bool   `json:"noEvidence"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	selector, aerr := s.resolveElementTarget(page, req.Ref, req.Selector)
	if aerr != nil {
		writeAPIError(w, aerr)
		return
	}
	el, aerr := s.findElement(page, selector)
	if aerr != nil {
		writeAPIError(w, aerr)
		return
	}

	urlBefore, sinceSeq := s.actionAnchors(page)

	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		http.Error(w, "click failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.recordAction("click", map[string]interface{}{"selector": selector})

	// Receipt: what did the click actually cause?
	s.writeEvidence(w, page, urlBefore, sinceSeq, req.NoEvidence)
}

// actionAnchors snapshots the pre-action state evidence is measured against.
func (s *Server) actionAnchors(page *rod.Page) (urlBefore string, sinceSeq int64) {
	urlBefore, _ = s.pageURL(page)
	sinceSeq = s.currentEventSeq()
	return urlBefore, sinceSeq
}

func (s *Server) handleType(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref      int    `json:"ref"`
		Selector string `json:"selector"`
		Text     string `json:"text"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	selector, aerr := s.resolveElementTarget(page, req.Ref, req.Selector)
	if aerr != nil {
		writeAPIError(w, aerr)
		return
	}
	if _, aerr = s.findElement(page, selector); aerr != nil {
		writeAPIError(w, aerr)
		return
	}

	// Safely clear and focus using JS. This avoids triggering chaotic framework
	// re-renders and node detachments that happen when simulating backspaces/deletes.
	// The selector is passed as an eval argument (not interpolated) so selectors
	// containing quotes or backslashes cannot break out of the script.
	_, _ = page.Eval(`(sel) => {
		const el = document.querySelector(sel);
		if (el) { el.value = ''; el.focus(); }
	}`, selector)

	// Now insert text as global keystrokes to whatever is focused
	page.MustInsertText(req.Text)

	s.recordAction("type", map[string]interface{}{"selector": selector, "text": req.Text})
	w.WriteHeader(http.StatusOK)
}
