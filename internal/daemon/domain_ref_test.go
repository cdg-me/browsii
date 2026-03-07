package daemon

import (
	"sync"
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func fakePage(id string) *rod.Page {
	p := &rod.Page{}
	p.TargetID = proto.TargetTargetID(id)
	return p
}

// ── single-page acquire / release ─────────────────────────────────────────────

func TestDomainRef_AcquireEnablesOnFirstOnly(t *testing.T) {
	calls := 0
	d := domainRef{
		refs:     make(map[proto.TargetTargetID]int),
		onEnable: func(_ *rod.Page) { calls++ },
	}
	p := fakePage("a")

	d.acquirePages([]*rod.Page{p})
	if calls != 1 {
		t.Fatalf("expected 1 enable call after first acquire, got %d", calls)
	}
	d.acquirePages([]*rod.Page{p})
	if calls != 1 {
		t.Fatalf("expected no second enable call, got %d", calls)
	}
}

func TestDomainRef_ReleaseDisablesOnLastOnly(t *testing.T) {
	calls := 0
	d := domainRef{
		refs:      make(map[proto.TargetTargetID]int),
		onEnable:  func(_ *rod.Page) {},
		onDisable: func(_ *rod.Page) { calls++ },
	}
	p := fakePage("a")

	d.acquirePages([]*rod.Page{p})
	d.acquirePages([]*rod.Page{p})
	d.releasePages([]*rod.Page{p})
	if calls != 0 {
		t.Fatalf("expected no disable call with refs still active, got %d", calls)
	}
	d.releasePages([]*rod.Page{p})
	if calls != 1 {
		t.Fatalf("expected 1 disable call after last release, got %d", calls)
	}
}

func TestDomainRef_ActiveForPage(t *testing.T) {
	d := domainRef{
		refs:      make(map[proto.TargetTargetID]int),
		onEnable:  func(_ *rod.Page) {},
		onDisable: func(_ *rod.Page) {},
	}
	p := fakePage("a")

	if d.activeForPage(p.TargetID) {
		t.Fatal("expected inactive before any acquire")
	}
	d.acquirePages([]*rod.Page{p})
	if !d.activeForPage(p.TargetID) {
		t.Fatal("expected active after acquire")
	}
	d.releasePages([]*rod.Page{p})
	if d.activeForPage(p.TargetID) {
		t.Fatal("expected inactive after release")
	}
}

func TestDomainRef_NilCallbacksNoPanic(t *testing.T) {
	d := domainRef{refs: make(map[proto.TargetTargetID]int)}
	p := fakePage("a")
	d.acquirePages([]*rod.Page{p})
	d.releasePages([]*rod.Page{p})
}

// ── per-page independence ──────────────────────────────────────────────────────

func TestDomainRef_AcquirePagesEnablesOnlyGivenPages(t *testing.T) {
	var enabled []proto.TargetTargetID
	d := domainRef{
		refs:     make(map[proto.TargetTargetID]int),
		onEnable: func(p *rod.Page) { enabled = append(enabled, p.TargetID) },
	}
	p0 := fakePage("p0")
	p1 := fakePage("p1")

	d.acquirePages([]*rod.Page{p0})

	if len(enabled) != 1 || enabled[0] != "p0" {
		t.Fatalf("expected only p0 enabled, got %v", enabled)
	}
	if d.activeForPage(p1.TargetID) {
		t.Fatal("p1 should not be active")
	}
}

func TestDomainRef_PerPageRefCounting(t *testing.T) {
	var disabled []proto.TargetTargetID
	d := domainRef{
		refs:      make(map[proto.TargetTargetID]int),
		onEnable:  func(_ *rod.Page) {},
		onDisable: func(p *rod.Page) { disabled = append(disabled, p.TargetID) },
	}
	p0 := fakePage("p0")
	p1 := fakePage("p1")

	// Acquire both, release p0, only p0 should disable
	d.acquirePages([]*rod.Page{p0, p1})
	d.releasePages([]*rod.Page{p0})

	if len(disabled) != 1 || disabled[0] != "p0" {
		t.Fatalf("expected only p0 disabled, got %v", disabled)
	}
	if !d.activeForPage(p1.TargetID) {
		t.Fatal("p1 should still be active")
	}
}

func TestDomainRef_PerPageThresholds(t *testing.T) {
	enableCalls := map[proto.TargetTargetID]int{}
	disableCalls := map[proto.TargetTargetID]int{}
	d := domainRef{
		refs:      make(map[proto.TargetTargetID]int),
		onEnable:  func(p *rod.Page) { enableCalls[p.TargetID]++ },
		onDisable: func(p *rod.Page) { disableCalls[p.TargetID]++ },
	}
	p := fakePage("a")

	d.acquirePages([]*rod.Page{p})
	d.acquirePages([]*rod.Page{p})
	if enableCalls[p.TargetID] != 1 {
		t.Fatalf("expected 1 enable, got %d", enableCalls[p.TargetID])
	}

	d.releasePages([]*rod.Page{p})
	if disableCalls[p.TargetID] != 0 {
		t.Fatalf("expected 0 disable with 1 ref remaining, got %d", disableCalls[p.TargetID])
	}
	d.releasePages([]*rod.Page{p})
	if disableCalls[p.TargetID] != 1 {
		t.Fatalf("expected 1 disable, got %d", disableCalls[p.TargetID])
	}
}

func TestDomainRef_ReleaseUnacquiredPageIsNoop(t *testing.T) {
	calls := 0
	d := domainRef{
		refs:      make(map[proto.TargetTargetID]int),
		onDisable: func(_ *rod.Page) { calls++ },
	}
	d.releasePages([]*rod.Page{fakePage("never-acquired")})
	if calls != 0 {
		t.Fatalf("expected no disable call for unacquired page, got %d", calls)
	}
}

// ── concurrency ───────────────────────────────────────────────────────────────

func TestDomainRef_ConcurrentAcquireRelease(t *testing.T) {
	var mu sync.Mutex
	enableCount := 0
	disableCount := 0

	d := domainRef{
		refs:      make(map[proto.TargetTargetID]int),
		onEnable:  func(_ *rod.Page) { mu.Lock(); enableCount++; mu.Unlock() },
		onDisable: func(_ *rod.Page) { mu.Lock(); disableCount++; mu.Unlock() },
	}
	p := fakePage("shared")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.acquirePages([]*rod.Page{p})
			d.releasePages([]*rod.Page{p})
		}()
	}
	wg.Wait()

	if d.activeForPage(p.TargetID) {
		t.Error("expected inactive after all goroutines released")
	}
	if enableCount != disableCount {
		t.Errorf("enable count %d != disable count %d", enableCount, disableCount)
	}
}

func TestDomainRef_ConcurrentPerPage(t *testing.T) {
	d := domainRef{
		refs:      make(map[proto.TargetTargetID]int),
		onEnable:  func(_ *rod.Page) {},
		onDisable: func(_ *rod.Page) {},
	}
	pages := []*rod.Page{fakePage("x"), fakePage("y"), fakePage("z")}

	var wg sync.WaitGroup
	for _, p := range pages {
		for i := 0; i < 50; i++ {
			wg.Add(1)
			pp := p
			go func() {
				defer wg.Done()
				d.acquirePages([]*rod.Page{pp})
				d.releasePages([]*rod.Page{pp})
			}()
		}
	}
	wg.Wait()

	for _, p := range pages {
		if d.activeForPage(p.TargetID) {
			t.Errorf("page %s should be inactive after all releases", p.TargetID)
		}
	}
}
