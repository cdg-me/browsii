package daemon

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// activePage returns the currently active page, checking the active context first.
func (s *Server) activePage() *rod.Page {
	// Check if we're in a named context
	if s.activeCtx != "" && s.contexts != nil {
		if ctx, ok := s.contexts[s.activeCtx]; ok && ctx.page != nil {
			return ctx.page
		}
	}
	if s.activePg != nil {
		return s.activePg
	}
	pages, _ := s.browser.Pages()
	if len(pages) == 0 {
		return nil
	}
	return pages[0]
}

// trackPage appends a page to pageOrder if not already present and registers
// network and console listeners for it (idempotent — safe to call multiple times).
func (s *Server) trackPage(p *rod.Page) {
	for _, id := range s.pageOrder {
		if id == p.TargetID {
			return
		}
	}
	s.pageOrder = append(s.pageOrder, p.TargetID)
	s.attachNetworkListener(p)
	s.attachConsoleListener(p)
}

// untrackPage removes a page from pageOrder and all listener tracking maps.
func (s *Server) untrackPage(p *rod.Page) {
	for i, id := range s.pageOrder {
		if id == p.TargetID {
			s.pageOrder = append(s.pageOrder[:i], s.pageOrder[i+1:]...)
			delete(s.listenedPages, p.TargetID)
			delete(s.consoleListenedPages, p.TargetID)
			return
		}
	}
}

// orderedPages returns the browser's pages in stable creation order.
// Any pages not in pageOrder (e.g. opened externally) are appended at the end.
func (s *Server) orderedPages() []*rod.Page {
	pages, _ := s.browser.Pages()
	byID := make(map[proto.TargetTargetID]*rod.Page, len(pages))
	for _, p := range pages {
		byID[p.TargetID] = p
	}
	result := make([]*rod.Page, 0, len(pages))
	for _, id := range s.pageOrder {
		if p, ok := byID[id]; ok {
			result = append(result, p)
			delete(byID, id)
		}
	}
	// Append any pages Chrome knows about that aren't in our order list.
	for _, p := range pages {
		if _, remaining := byID[p.TargetID]; remaining {
			result = append(result, p)
		}
	}
	return result
}
