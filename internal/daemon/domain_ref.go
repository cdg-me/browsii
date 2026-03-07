package daemon

import (
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// domainRef is a per-page ref-counted CDP domain lifecycle controller.
// onEnable fires when a page's count transitions 0→1.
// onDisable fires when a page's count transitions 1→0.
type domainRef struct {
	mu        sync.Mutex
	refs      map[proto.TargetTargetID]int
	onEnable  func(*rod.Page)
	onDisable func(*rod.Page)
}

// acquirePages increments the ref count for each page and enables the domain
// on pages whose count transitions 0→1.
func (d *domainRef) acquirePages(pages []*rod.Page) {
	d.mu.Lock()
	var toEnable []*rod.Page
	for _, p := range pages {
		d.refs[p.TargetID]++
		if d.refs[p.TargetID] == 1 {
			toEnable = append(toEnable, p)
		}
	}
	d.mu.Unlock()
	for _, p := range toEnable {
		if d.onEnable != nil {
			d.onEnable(p)
		}
	}
}

// releasePages decrements the ref count for each page and disables the domain
// on pages whose count transitions 1→0. Releasing an unacquired page is a no-op.
func (d *domainRef) releasePages(pages []*rod.Page) {
	d.mu.Lock()
	var toDisable []*rod.Page
	for _, p := range pages {
		if d.refs[p.TargetID] <= 0 {
			continue
		}
		d.refs[p.TargetID]--
		if d.refs[p.TargetID] == 0 {
			delete(d.refs, p.TargetID)
			toDisable = append(toDisable, p)
		}
	}
	d.mu.Unlock()
	for _, p := range toDisable {
		if d.onDisable != nil {
			d.onDisable(p)
		}
	}
}

// activeForPage reports whether the domain is currently enabled for the given page.
func (d *domainRef) activeForPage(id proto.TargetTargetID) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.refs[id] > 0
}
