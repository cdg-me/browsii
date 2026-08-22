package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTabScope_ConcurrentClicksAcrossTabs fires simultaneous scoped clicks
// on two tabs and verifies every one lands on the right element of the
// right tab. Without per-page serialization, concurrent lookups race in
// rod's shared JS context and clicks are lost or mis-targeted.
func TestTabScope_ConcurrentClicksAcrossTabs(t *testing.T) {
	server := setupTwoPageServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL+"/a", "--no-evidence")
	runCLI(t, bin, port, "tab", "new", server.URL+"/b", "--background")

	// Track clicks per tab with distinct handlers per button.
	runCLI(t, bin, port, "--tab", "0", "js", `() => { window.hits = []; document.querySelectorAll('button').forEach(b => b.addEventListener('click', () => window.hits.push(b.id))); return 1 }`)
	runCLI(t, bin, port, "--tab", "1", "js", `() => { window.hits = []; document.querySelectorAll('button').forEach(b => b.addEventListener('click', () => window.hits.push(b.id))); return 1 }`)

	// Four simultaneous scoped clicks: two per tab, distinct targets.
	var wg sync.WaitGroup
	for _, spec := range []struct {
		tab int
		sel string
	}{
		{0, "#btn-A"}, {0, "#field-A"}, {1, "#btn-B"}, {1, "#field-B"},
	} {
		wg.Add(1)
		go func(tab int, sel string) {
			defer wg.Done()
			cmd := exec.CommandContext(context.Background(), bin, "--tab", strconv.Itoa(tab), "click", sel, "--no-evidence", "--port", strconv.Itoa(port)) //nolint:gosec
			out, err := cmd.CombinedOutput()
			assert.NoError(t, err, "click tab=%d sel=%s: %s", tab, sel, out)
		}(spec.tab, spec.sel)
	}
	wg.Wait()

	hitsA := runCLI(t, bin, port, "--tab", "0", "js", "() => JSON.stringify(window.hits)")
	hitsB := runCLI(t, bin, port, "--tab", "1", "js", "() => JSON.stringify(window.hits)")

	assert.Contains(t, hitsA, "btn-A")
	assert.NotContains(t, hitsA, "btn-B", "tab 0 must never receive tab 1 clicks")
	assert.Contains(t, hitsB, "btn-B")
	assert.NotContains(t, hitsB, "btn-A", "tab 1 must never receive tab 0 clicks")
}

// TestTabScope_ElementsScopedPerTab verifies enumeration respects the tab
// header (a regression: /elements ignored it while every other route
// honored it).
func TestTabScope_ElementsScopedPerTab(t *testing.T) {
	server := setupTwoPageServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL+"/a", "--no-evidence")
	runCLI(t, bin, port, "tab", "new", server.URL+"/b", "--background")

	outA := runCLI(t, bin, port, "--tab", "0", "elements", "--filter", "go A")
	assert.Contains(t, outA, `go A`)
	assert.NotContains(t, outA, "go B")

	outB := runCLI(t, bin, port, "--tab", "1", "elements", "--filter", "go B")
	require.Contains(t, outB, `go B`)
	assert.NotContains(t, outB, "go A")
}

// TestTabScope_ShopConcurrency is the realistic multi-agent shape: two
// scoped agents interleaving cart operations on separate shops.
func TestTabScope_ShopConcurrency(t *testing.T) {
	mux := http.NewServeMux()
	for _, cat := range []string{"electronics", "books"} {
		items := map[string][][2]interface{}{
			"electronics": {{"laptop", 999}, {"phone", 699}},
			"books":       {{"novel", 18}, {"rust", 32}},
		}[cat]
		body := `<!DOCTYPE html><html><body>`
		for i, it := range items {
			body += fmt.Sprintf(`<div class="item"><span>%s</span><button id="add-%d" onclick="window.cart = (window.cart||0) + %d">add</button></div>`, it[0], i, it[1])
		}
		body += `</body></html>`
		cat := cat
		mux.HandleFunc("/"+cat, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, body) //nolint:errcheck
		})
	}
	server := httptest.NewServer(mux)
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", server.URL+"/electronics", "--no-evidence")
	runCLI(t, bin, port, "tab", "new", server.URL+"/books", "--background")

	// Both agents add both items concurrently.
	var wg sync.WaitGroup
	for _, spec := range []struct {
		tab int
		sel string
	}{
		{0, "#add-0"}, {0, "#add-1"}, {1, "#add-0"}, {1, "#add-1"},
	} {
		wg.Add(1)
		go func(tab int, sel string) {
			defer wg.Done()
			cmd := exec.CommandContext(context.Background(), bin, "--tab", strconv.Itoa(tab), "click", sel, "--no-evidence", "--port", strconv.Itoa(port)) //nolint:gosec
			out, err := cmd.CombinedOutput()
			assert.NoError(t, err, "tab=%d sel=%s: %s", tab, sel, out)
		}(spec.tab, spec.sel)
	}
	wg.Wait()

	cartA := runCLI(t, bin, port, "--tab", "0", "js", "() => window.cart")
	cartB := runCLI(t, bin, port, "--tab", "1", "js", "() => window.cart")

	assert.Contains(t, cartA, "1698") // laptop + phone
	assert.Contains(t, cartB, "50")   // novel + rust
}
