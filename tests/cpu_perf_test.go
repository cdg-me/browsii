//go:build (linux || darwin) && cpu_perf

package tests

// Two-phase CPU benchmark for the CDP domain always-on bug.
//
// Design rationale:
//   Before/after is measured on the same Chrome instance in one test run,
//   eliminating hardware variance. The JS workload runs continuously through
//   both phases so any difference is attributable solely to domain state.
//
//   Phase 1: start network+console capture → NetworkEnable + RuntimeEnable are
//            active on the page. V8 runs in debug/instrumented mode with
//            reduced JIT optimization. Network stack in intercept mode.
//
//   Phase 2: stop both captures → NetworkDisable + RuntimeDisable called (via
//            ref count reaching 0). V8 returns to full JIT speed.
//
//   Sanity check: Phase 1 CPU must exceed 5% to confirm the workload is
//   actually running and measurable. A broken workload can't produce a
//   false-positive "improvement" from 0.6% to 0.4%.
//
// Run:
//
//	go build -o browsii ./cmd/browsii
//	go test -tags cpu_perf -run TestCPU_DomainEnableDisable -v -timeout 90s ./tests/...

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// setupJSWorkloadServer serves a page that continuously drives both overhead
// paths that the CDP domain fix targets:
//
//   - RuntimeEnable overhead: V8 in debug/instrumented mode cannot fully
//     JIT-optimize closures or dynamic dispatch. The workload creates 2000
//     objects via a closure every 16ms, keeping the renderer busy with
//     code that V8 cannot fully inline away in instrumented mode.
//
//   - NetworkEnable overhead: every fetch goes through Chrome's intercept
//     path regardless of whether any interceptors are registered.
//     10 req/s keeps this path consistently exercised.
func setupJSWorkloadServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "pong")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body><script>
// 2000 objects, closure-based transform, sort every 16ms (~60fps).
// Closures + dynamic dispatch prevent V8 from optimizing away the work
// in instrumented mode; the debug overhead is on top of the baseline cost.
let items = Array.from({length: 2000}, (_, i) => ({id: i, v: Math.random(), s: String(i)}));

function makeTransform(seed) {
  return (o) => ({id: o.id, v: o.v * seed + Math.sin(o.id), s: o.id.toString(36)});
}

setInterval(() => {
  const fn = makeTransform(Math.random() * 10 + 1);
  items = items.map(fn);
  items.sort((a, b) => a.v - b.v);
  // Prevent dead-code elimination
  if (items[0].v < -1e9) console.log('x');
}, 16);

// 10 req/s keeps NetworkEnable intercept path warm.
setInterval(() => fetch('/ping').catch(() => {}), 100);
</script></body></html>`)
	})
	return httptest.NewServer(mux)
}

// postJSON fires a POST to the daemon and discards the response body.
func postJSON(t *testing.T, port int, path string, body interface{}) {
	t.Helper()
	var r io.Reader = http.NoBody
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	resp, err := http.Post( //nolint:noctx
		fmt.Sprintf("http://127.0.0.1:%d%s", port, path),
		"application/json", r,
	)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	resp.Body.Close()
}

func daemonPID(t *testing.T, port int) int {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pid", port)) //nolint:noctx
	if err != nil {
		t.Fatalf("GET /debug/pid: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		PID int `json:"pid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /debug/pid: %v", err)
	}
	return body.PID
}

// TestCPU_DomainEnableDisable measures Chrome CPU in two phases on the same
// running instance:
//
//	Phase 1 — domains ON  (network + console capture active)
//	Phase 2 — domains OFF (captures stopped; ref count hits 0, domains disabled)
//
// Sanity check asserts Phase 1 > 5% so a silent workload failure can't
// produce a misleading result.
func TestCPU_DomainEnableDisable(t *testing.T) {
	ts := setupJSWorkloadServer()
	defer ts.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", ts.URL)

	// Let the JS workload JIT-warm before any measurement.
	t.Log("warming up JS workload for 4s...")
	time.Sleep(4 * time.Second)

	pid := daemonPID(t, port)
	pids := chromePIDs(t, pid)
	if len(pids) == 0 {
		t.Fatalf("no child processes found under daemon PID %d — is Chrome running?", pid)
	}
	t.Logf("daemon PID %d → subprocess PIDs: %v", pid, pids)

	// ── Phase 1: domains ON ────────────────────────────────────────────────
	postJSON(t, port, "/network/capture/start", nil)
	postJSON(t, port, "/console/capture/start", nil)

	t.Log("settling with domains ON (2s)...")
	time.Sleep(2 * time.Second)

	t.Log("sampling Phase 1 (domains ON, 5s window)...")
	cpu1 := sumCPUPercent(t, pids, 5*time.Second)
	t.Logf("=== Phase 1 (domains ON):  %.1f%% ===", cpu1)

	if cpu1 < 5.0 {
		t.Fatalf("Phase 1 CPU %.1f%% < 5%% — workload is not exercising V8 "+
			"(page may not have loaded or setInterval is not running)", cpu1)
	}

	// ── Phase 2: domains OFF ───────────────────────────────────────────────
	// ref count goes 1→0 on each stop, triggering NetworkDisable + RuntimeDisable.
	postJSON(t, port, "/network/capture/stop", nil)
	postJSON(t, port, "/console/capture/stop", nil)

	t.Log("settling with domains OFF (3s)...")
	time.Sleep(3 * time.Second)

	t.Log("sampling Phase 2 (domains OFF, 5s window)...")
	cpu2 := sumCPUPercent(t, pids, 5*time.Second)
	t.Logf("=== Phase 2 (domains OFF): %.1f%% ===", cpu2)

	delta := cpu1 - cpu2
	t.Logf("CPU delta: %.1fpp  (%.1f%% → %.1f%%)", delta, cpu1, cpu2)

	if delta < 1.0 {
		t.Errorf("expected ≥1.0pp CPU reduction after disabling domains, got %.1fpp", delta)
	}
}

