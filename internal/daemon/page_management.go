package daemon

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func enableNetworkDomain(p *rod.Page)  { _ = proto.NetworkEnable{}.Call(p) }
func disableNetworkDomain(p *rod.Page) { _ = proto.NetworkDisable{}.Call(p) }
func enableConsoleDomain(p *rod.Page)  { _ = proto.RuntimeEnable{}.Call(p) }
func disableConsoleDomain(p *rod.Page) { _ = proto.RuntimeDisable{}.Call(p) }

// activePage returns the currently active page, checking the active context first.
func (s *Server) activePage() *rod.Page {
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

// pagesForFilter returns the pages matching filter.
// filter == -1 returns all pages; filter >= 0 returns the single page at that index.
func (s *Server) pagesForFilter(filter int) []*rod.Page {
	pages := s.orderedPages()
	if filter < 0 {
		return pages
	}
	if filter < len(pages) {
		return []*rod.Page{pages[filter]}
	}
	return nil
}

// applyDomainsToNewPage enables CDP domains for p if any active all-tabs capture
// needs it, and registers p with the capturing page list for later release.
func (s *Server) applyDomainsToNewPage(p *rod.Page) {
	s.mu.Lock()
	addNet := s.capturing && s.captureTabFilter == -1
	addCon := s.consoleCapturing && s.consoleTabFilter == -1
	if addNet {
		s.networkCapturingPages = append(s.networkCapturingPages, p)
	}
	if addCon {
		s.consoleCapturingPages = append(s.consoleCapturingPages, p)
	}
	s.mu.Unlock()

	if addNet {
		s.networkDomain.acquirePages([]*rod.Page{p})
	}
	if addCon {
		s.consoleDomain.acquirePages([]*rod.Page{p})
	}
}

// trackPage appends a page to pageOrder if not already present, registers
// event listeners, and enables any CDP domains that currently have consumers
// so a tab opened mid-capture receives events immediately.
func (s *Server) trackPage(p *rod.Page) {
	for _, id := range s.pageOrder {
		if id == p.TargetID {
			return
		}
	}
	s.pageOrder = append(s.pageOrder, p.TargetID)
	s.attachNetworkListener(p)
	s.attachConsoleListener(p)
	s.applyDomainsToNewPage(p)
	s.applyInjectScriptsToNewPage(p)
}

// applyInjectScriptsToNewPage registers all global inject-js entries on p so
// that tabs opened after inject js add still receive the scripts.
func (s *Server) applyInjectScriptsToNewPage(p *rod.Page) {
	s.mu.Lock()
	entries := make([]injectJSEntry, len(s.injectJSGlobal))
	copy(entries, s.injectJSGlobal)
	s.mu.Unlock()

	for _, entry := range entries {
		sid, err := registerInjectScript(p, entry.Script)
		if err != nil {
			continue
		}
		s.mu.Lock()
		if s.injectJSCDPIDs[entry.ID] == nil {
			s.injectJSCDPIDs[entry.ID] = make(map[proto.TargetTargetID]proto.PageScriptIdentifier)
		}
		s.injectJSCDPIDs[entry.ID][p.TargetID] = sid
		s.mu.Unlock()
	}
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
	for _, p := range pages {
		if _, remaining := byID[p.TargetID]; remaining {
			result = append(result, p)
		}
	}
	return result
}
