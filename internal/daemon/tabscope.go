package daemon

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// tabHeader overrides the active-tab resolution for a single request, so
// concurrent agents can drive different tabs of one daemon without fighting
// over the global active tab.
const tabHeader = "X-Browsii-Tab"

// pageFromRequest resolves the page a request operates on: the tab header
// when present (must name a live tab), otherwise the active page.
func (s *Server) pageFromRequest(r *http.Request) (*rod.Page, error) {
	raw := r.Header.Get(tabHeader)
	if raw == "" {
		return s.activePage(), nil
	}
	idx, err := strconv.Atoi(raw)
	if err != nil || idx < 0 {
		return nil, fmt.Errorf("invalid %s %q: expected a zero-based tab index", tabHeader, raw)
	}
	pages := s.orderedPages()
	if idx >= len(pages) {
		return nil, fmt.Errorf("tab %d does not exist: %d tab(s) open (see 'tab list')", idx, len(pages))
	}
	return pages[idx], nil
}

// writePageError reports a page-resolution failure. ok is false when the
// response was written.
func writePageError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return true
	}
	http.Error(w, err.Error(), http.StatusNotFound)
	return false
}

// bgInputMu serializes background-tab input actions per page. Concurrent
// element lookups + shape evals on one background page race inside rod's
// shared JS context, producing clicks that report success but never fire.
var bgInputMu sync.Map // TargetID -> *sync.Mutex

func bgMutexFor(id proto.TargetTargetID) *sync.Mutex {
	m, _ := bgInputMu.LoadOrStore(id, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// clickElement clicks el, working on background tabs. rod's Click and even
// Mouse.MoveTo wait on rendering feedback that background tabs throttle
// (MoveTo stalls exactly 5s), so for pages that are not the active tab the
// press/release is dispatched directly at the element's center point.
func (s *Server) clickElement(el *rod.Element) error {
	if s.isActiveElementPage(el) {
		return el.Click(proto.InputMouseButtonLeft, 1)
	}
	pt, err := el.Interactable()
	if err != nil {
		return err
	}
	page := el.Page()
	for _, typ := range []proto.InputDispatchMouseEventType{
		proto.InputDispatchMouseEventTypeMousePressed,
		proto.InputDispatchMouseEventTypeMouseReleased,
	} {
		if err := (proto.InputDispatchMouseEvent{
			Type:       typ,
			X:          pt.X,
			Y:          pt.Y,
			Button:     proto.InputMouseButtonLeft,
			ClickCount: 1,
		}).Call(page); err != nil {
			return err
		}
	}
	return nil
}

// hoverElement hovers el. rod's Hover moves the mouse and waits for render
// feedback, which background tabs defer indefinitely, so on non-active tabs
// DOM-level mouseover/mousemove events are dispatched instead — page
// handlers fire, though :hover CSS is not evaluated by the compositor on a
// frozen tab.
func (s *Server) hoverElement(el *rod.Element) error {
	if s.isActiveElementPage(el) {
		return el.Hover()
	}
	_, err := el.Eval(`() => {
		for (const type of ['mouseover', 'mousemove']) {
			this.dispatchEvent(new MouseEvent(type, {bubbles: true, cancelable: true}));
		}
		return true;
	}`)
	return err
}

// isActiveElementPage reports whether el belongs to the active page (where
// rod's stabilizing actions work).
func (s *Server) isActiveElementPage(el *rod.Element) bool {
	active := s.activePage()
	return active != nil && el.Page().TargetID == active.TargetID
}

// pageIsActive reports whether page is the daemon's active page.
func (s *Server) pageIsActive(page *rod.Page) bool {
	active := s.activePage()
	return active != nil && page.TargetID == active.TargetID
}
