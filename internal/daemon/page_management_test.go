package daemon

import (
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func newTestServer(capturing bool, netFilter int, consCapturing bool, conFilter int) *Server {
	s := &Server{
		capturing:        capturing,
		captureTabFilter: netFilter,
		consoleCapturing: consCapturing,
		consoleTabFilter: conFilter,
	}
	s.networkDomain = domainRef{refs: make(map[proto.TargetTargetID]int)}
	s.consoleDomain = domainRef{refs: make(map[proto.TargetTargetID]int)}
	return s
}

func TestApplyDomainsToNewPage_NoCapture(t *testing.T) {
	var enabled []proto.TargetTargetID
	s := newTestServer(false, -1, false, -1)
	s.networkDomain.onEnable = func(p *rod.Page) { enabled = append(enabled, p.TargetID) }

	s.applyDomainsToNewPage(fakePage("tab-0"))

	if len(enabled) != 0 {
		t.Errorf("expected no domains enabled when no capture active, got %v", enabled)
	}
}

func TestApplyDomainsToNewPage_AllTabsCapture(t *testing.T) {
	var netEnabled, conEnabled []proto.TargetTargetID
	s := newTestServer(true, -1, true, -1)
	s.networkDomain.onEnable = func(p *rod.Page) { netEnabled = append(netEnabled, p.TargetID) }
	s.consoleDomain.onEnable = func(p *rod.Page) { conEnabled = append(conEnabled, p.TargetID) }

	p := fakePage("tab-0")
	s.applyDomainsToNewPage(p)

	if len(netEnabled) != 1 || netEnabled[0] != "tab-0" {
		t.Errorf("expected network enabled for tab-0, got %v", netEnabled)
	}
	if len(conEnabled) != 1 || conEnabled[0] != "tab-0" {
		t.Errorf("expected console enabled for tab-0, got %v", conEnabled)
	}
	if len(s.networkCapturingPages) != 1 {
		t.Errorf("expected 1 page in networkCapturingPages, got %d", len(s.networkCapturingPages))
	}
	if len(s.consoleCapturingPages) != 1 {
		t.Errorf("expected 1 page in consoleCapturingPages, got %d", len(s.consoleCapturingPages))
	}
}

func TestApplyDomainsToNewPage_ScopedNetworkCapture(t *testing.T) {
	var netEnabled []proto.TargetTargetID
	s := newTestServer(true, 0, false, -1) // network scoped to tab 0, no console
	s.networkDomain.onEnable = func(p *rod.Page) { netEnabled = append(netEnabled, p.TargetID) }

	s.applyDomainsToNewPage(fakePage("new-tab"))

	if len(netEnabled) != 0 {
		t.Errorf("expected no network enable for scoped capture, got %v", netEnabled)
	}
	if len(s.networkCapturingPages) != 0 {
		t.Errorf("expected 0 pages in networkCapturingPages, got %d", len(s.networkCapturingPages))
	}
}

func TestApplyDomainsToNewPage_ScopedConsoleCapture(t *testing.T) {
	var conEnabled []proto.TargetTargetID
	s := newTestServer(false, -1, true, 1) // console scoped to tab 1
	s.consoleDomain.onEnable = func(p *rod.Page) { conEnabled = append(conEnabled, p.TargetID) }

	s.applyDomainsToNewPage(fakePage("new-tab"))

	if len(conEnabled) != 0 {
		t.Errorf("expected no console enable for scoped capture, got %v", conEnabled)
	}
	if len(s.consoleCapturingPages) != 0 {
		t.Errorf("expected 0 pages in consoleCapturingPages, got %d", len(s.consoleCapturingPages))
	}
}

func TestApplyDomainsToNewPage_MixedFilters(t *testing.T) {
	var netEnabled, conEnabled []proto.TargetTargetID
	// network = all tabs, console = scoped to tab 0
	s := newTestServer(true, -1, true, 0)
	s.networkDomain.onEnable = func(p *rod.Page) { netEnabled = append(netEnabled, p.TargetID) }
	s.consoleDomain.onEnable = func(p *rod.Page) { conEnabled = append(conEnabled, p.TargetID) }

	s.applyDomainsToNewPage(fakePage("new-tab"))

	if len(netEnabled) != 1 {
		t.Errorf("expected network enabled for new-tab (all-tabs capture), got %v", netEnabled)
	}
	if len(conEnabled) != 0 {
		t.Errorf("expected no console enable (scoped capture), got %v", conEnabled)
	}
}
