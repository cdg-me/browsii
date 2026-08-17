package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordFlow runs the canonical shop flow under a recording: click product
// two's Add to cart by element ref, then verify the total with expect.
// Recording is stopped and the saved JSON is returned.
func recordFlow(t *testing.T, bin string, port int, serverURL string, name string) map[string]any {
	t.Helper()
	runCLI(t, bin, port, "record", "start", name)
	runCLI(t, bin, port, "navigate", serverURL)

	out := runCLI(t, bin, port, "elements", "--json")
	var list struct {
		Elements []struct {
			Ref      int    `json:"ref"`
			Role     string `json:"role"`
			Text     string `json:"text"`
			Selector string `json:"selector"`
		} `json:"elements"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &list))

	gadgetRef := 0
	for _, e := range list.Elements {
		if e.Selector == "#p2-add" {
			gadgetRef = e.Ref
			break
		}
	}
	require.NotZero(t, gadgetRef, "#p2-add not found in elements output")

	runCLI(t, bin, port, "click", strconv.Itoa(gadgetRef))
	runCLI(t, bin, port, "expect", "--text", "Total: $30")
	runCLI(t, bin, port, "record", "stop")

	home, _ := os.UserHomeDir()
	data, err := os.ReadFile(filepath.Join(home, ".browsii", "recordings", name+".json"))
	require.NoError(t, err)
	var rec map[string]any
	require.NoError(t, json.Unmarshal(data, &rec))
	return rec
}

func replayCLI(t *testing.T, bin string, port int, name string, extra ...string) (string, error) {
	t.Helper()
	args := append([]string{"record", "replay", name}, extra...)
	return runCLIExpectFail(t, bin, port, args...)
}

func TestReplay_RecordAndReplayDeterministic(t *testing.T) {
	server := setupShopServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	rec := recordFlow(t, bin, port, server.URL, "shop-basic")
	events := rec["events"].([]any)
	assert.GreaterOrEqual(t, len(events), 3, "navigate + click + expect minimum")

	// The click event carries a fingerprint, and the expect was recorded.
	hasFP, hasExpect := false, false
	for _, e := range events {
		ev := e.(map[string]any)
		if ev["action"] == "click" && ev["fp"] != nil {
			hasFP = true
		}
		if ev["action"] == "expect" {
			hasExpect = true
		}
	}
	assert.True(t, hasFP, "click events must record fingerprints")
	assert.True(t, hasExpect, "expect calls must be recorded as events")

	// Fresh page, then replay twice.
	for i := 0; i < 2; i++ {
		runCLI(t, bin, port, "session", "new", "fresh")
		out, err := replayCLI(t, bin, port, "shop-basic")
		require.NoError(t, err, "replay %d failed: %s", i+1, out)
		assert.Contains(t, out, "Replayed")
		assert.Contains(t, out, "checkpoints passed")

		jsOut := runCLI(t, bin, port, "js", "() => window.cartCount")
		assert.Contains(t, jsOut, "1", "replay must perform the recorded click")
	}
}

func TestReplay_FastByDefault(t *testing.T) {
	server := setupShopServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	recordFlow(t, bin, port, server.URL, "shop-fast")

	runCLI(t, bin, port, "session", "new", "fresh")
	start := time.Now()
	out, err := replayCLI(t, bin, port, "shop-fast")
	require.NoError(t, err, out)
	require.Less(t, time.Since(start), 10*time.Second, "instant replay must not wait recorded gaps")
}

func TestReplay_HealsThroughUIDrift(t *testing.T) {
	server := setupShopServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	recordFlow(t, bin, port, server.URL+"?variant=canonical", "shop-drift")
	retargetRecording(t, "shop-drift", server.URL+"?variant=drifted")

	runCLI(t, bin, port, "session", "new", "fresh")
	out, err := replayCLI(t, bin, port, "shop-drift", "--live")
	require.NoError(t, err, "replay must pass against drifted UI: %s", out)
	assert.Contains(t, out, "healed: step", "drifted selector must be reported as healed")

	// The click hit a product: drifted order is Gizmo, Widget, Gadget, all
	// fingerprint-identical, so cartCount proves the click landed somewhere
	// real rather than nothing happening.
	jsOut := runCLI(t, bin, port, "js", "() => window.cartCount")
	assert.Contains(t, jsOut, "1")
}

// retargetRecording rewrites the recorded navigate URL so replay targets
// toURL instead of the one the flow was recorded on.
func retargetRecording(t *testing.T, name, toURL string) {
	t.Helper()
	home, _ := os.UserHomeDir()
	recPath := filepath.Join(home, ".browsii", "recordings", name+".json")
	data, err := os.ReadFile(recPath)
	require.NoError(t, err)
	var rec struct {
		Name   string           `json:"name"`
		URL    string           `json:"url"`
		Events []map[string]any `json:"events"`
	}
	require.NoError(t, json.Unmarshal(data, &rec))
	rec.URL = toURL
	for _, ev := range rec.Events {
		if ev["action"] == "navigate" {
			params := ev["params"].(map[string]any)
			params["url"] = toURL
		}
	}
	fixed, err := json.Marshal(rec)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recPath, fixed, 0644))
}

func TestReplay_JsAndTypeRegressions(t *testing.T) {
	server := setupShopServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "record", "start", "shop-js")
	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "js", "document.title")
	runCLI(t, bin, port, "record", "stop")

	runCLI(t, bin, port, "session", "new", "fresh")
	out, err := replayCLI(t, bin, port, "shop-js")
	require.NoError(t, err, out)
}

func TestReplay_SemanticChangeFailsAtTheRightStep(t *testing.T) {
	server := setupShopServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Record a flow that ends by clicking checkout.
	runCLI(t, bin, port, "record", "start", "shop-broken")
	runCLI(t, bin, port, "navigate", server.URL+"?variant=canonical")
	runCLI(t, bin, port, "click", "#p1-add")
	runCLI(t, bin, port, "click", "#checkout")
	runCLI(t, bin, port, "record", "stop")

	retargetRecording(t, "shop-broken", server.URL+"?variant=broken")

	runCLI(t, bin, port, "session", "new", "fresh")
	out, err := replayCLI(t, bin, port, "shop-broken", "--live")
	require.Error(t, err, "replay against broken UI must fail")
	assert.Contains(t, out, "no longer matches")
	assert.Contains(t, out, `"Checkout"`)
	assert.Contains(t, out, "step 3")

	// Earlier steps executed.
	jsOut := runCLI(t, bin, port, "js", "() => window.cartCount")
	assert.Contains(t, jsOut, "1")
}

func TestReplay_CheckpointCatchesWrongOutcome(t *testing.T) {
	server := setupShopServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	recordFlow(t, bin, port, server.URL+"?variant=canonical", "shop-checkpoint")

	// Corrupt the recorded checkpoint: expect a total the server never emits.
	home, _ := os.UserHomeDir()
	recPath := filepath.Join(home, ".browsii", "recordings", "shop-checkpoint.json")
	data, err := os.ReadFile(recPath)
	require.NoError(t, err)
	var rec struct {
		Name   string           `json:"name"`
		URL    string           `json:"url"`
		Events []map[string]any `json:"events"`
	}
	require.NoError(t, json.Unmarshal(data, &rec))
	for _, ev := range rec.Events {
		if ev["action"] == "expect" {
			params := ev["params"].(map[string]any)
			params["text"] = "Total: $999"
		}
	}
	fixed, err := json.Marshal(rec)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recPath, fixed, 0644))

	runCLI(t, bin, port, "session", "new", "fresh")
	out, err := replayCLI(t, bin, port, "shop-checkpoint", "--live")
	require.Error(t, err, "wrong outcome must fail the replay")
	assert.Contains(t, out, "checkpoint failed")
	assert.Contains(t, out, "Total: $999")

	// The action itself succeeded — only the checkpoint failed.
	jsOut := runCLI(t, bin, port, "js", "() => window.cartCount")
	assert.Contains(t, jsOut, "1")
}

func TestReplay_OfflineWithHAR(t *testing.T) {
	server := setupShopServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	name := "shop-har"
	runCLI(t, bin, port, "record", "start", name, "--capture-har")
	runCLI(t, bin, port, "navigate", server.URL)
	runCLI(t, bin, port, "click", "#p1-add")
	runCLI(t, bin, port, "expect", "--text", "Total: $30")
	runCLI(t, bin, port, "record", "stop")

	home, _ := os.UserHomeDir()
	recData, err := os.ReadFile(filepath.Join(home, ".browsii", "recordings", name+".json"))
	require.NoError(t, err)
	var rec struct {
		HAR string `json:"har"`
	}
	require.NoError(t, json.Unmarshal(recData, &rec))
	require.NotEmpty(t, rec.HAR, "recording must reference its HAR")
	_, err = os.Stat(rec.HAR)
	require.NoError(t, err, "HAR file must exist")

	// Kill the server, then replay: everything must be served from the HAR.
	server.Close()
	runCLI(t, bin, port, "session", "new", "fresh")
	out, err := replayCLI(t, bin, port, name)
	require.NoError(t, err, "replay must succeed offline: %s", out)
	assert.Contains(t, out, "checkpoints passed")

	// --live against a dead server fails with a clear error.
	runCLI(t, bin, port, "session", "new", "fresh")
	out, err = replayCLI(t, bin, port, name, "--live")
	require.Error(t, err, "--live must hit the network")
	assert.True(t, strings.Contains(out, "failed") || strings.Contains(out, "error"), out)
}

func TestReplay_SessionPinning(t *testing.T) {
	mux := setupMemberServer()
	server := mux
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Save the logged-in session.
	runCLI(t, bin, port, "navigate", server.URL+"/login")
	runCLI(t, bin, port, "session", "save", "member")

	// Record the member flow.
	runCLI(t, bin, port, "record", "start", "shop-member")
	runCLI(t, bin, port, "navigate", server.URL+"/prices")
	runCLI(t, bin, port, "expect", "--text", "member price")
	runCLI(t, bin, port, "record", "stop")

	// Fresh daemon state loses the cookie; --session restores it.
	runCLI(t, bin, port, "session", "new", "fresh")
	out, err := replayCLI(t, bin, port, "shop-member", "--session", "member", "--live")
	require.NoError(t, err, "replay with session must pass: %s", out)
	assert.Contains(t, out, "checkpoints passed")
}

// setupMemberServer gates content behind a cookie.
func setupMemberServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Set-Cookie", "member=1; Path=/")
		fmt.Fprint(w, `<html><body>logged in</body></html>`) //nolint:errcheck
	})
	mux.HandleFunc("/prices", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if c, err := r.Cookie("member"); err == nil && c.Value == "1" {
			fmt.Fprint(w, `<html><body><p>member price: $8</p></body></html>`) //nolint:errcheck
			return
		}
		fmt.Fprint(w, `<html><body><p>guest price: $10</p></body></html>`) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}

func TestReplay_ReportShape(t *testing.T) {
	server := setupShopServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	recordFlow(t, bin, port, server.URL, "shop-report")

	runCLI(t, bin, port, "session", "new", "fresh")
	out, err := replayCLI(t, bin, port, "shop-report")
	require.NoError(t, err)

	// The human line mirrors the JSON report fields (steps, checkpoints,
	// duration); the full shape is asserted in the daemon unit tests.
	assert.Contains(t, out, "Replayed")
	assert.Contains(t, out, "1/1 checkpoints passed")
}

func TestExport_WritesPlayableSpec(t *testing.T) {
	server := setupShopServer()
	defer server.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	rec := recordFlow(t, bin, port, server.URL, "shop-export")
	_ = rec

	out := runCLI(t, bin, port, "record", "export", "shop-export")
	assert.Contains(t, out, "Wrote")

	home, _ := os.UserHomeDir()
	specPath := filepath.Join(home, ".browsii", "recordings", "shop-export.spec.ts")
	data, err := os.ReadFile(specPath)
	require.NoError(t, err)
	spec := string(data)

	assert.Contains(t, spec, `import { test, expect } from '@playwright/test'`)
	assert.Contains(t, spec, `page.goto(`)
	assert.Contains(t, spec, `getByRole("button", { name: "Add to cart" })`)
	assert.Contains(t, spec, "#p2-add")
	assert.Contains(t, spec, `getByText("Total: $30")`)
}
