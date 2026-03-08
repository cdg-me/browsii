package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInjectJS_FiresBeforeInlinePageScript is the core ordering guarantee.
// The injected script pushes "inject" onto window.__log; the page's own inline
// <script> pushes "page". After navigation the array must be ["inject","page"].
func TestInjectJS_FiresBeforeInlinePageScript(t *testing.T) {
	srv := setupInjectOrderServer()
	defer srv.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "inject", "js", "add",
		`window.__log = window.__log || []; window.__log.push("inject");`)

	runCLI(t, bin, port, "navigate", srv.URL)

	out := runCLI(t, bin, port, "js", `() => window.__log`)
	assert.Contains(t, out, `"inject"`)
	assert.Contains(t, out, `"page"`)

	// Verify ordering: inject must appear before page in the JSON array.
	injectIdx := strings.Index(out, `"inject"`)
	pageIdx := strings.Index(out, `"page"`)
	assert.Less(t, injectIdx, pageIdx, "injected script must run before page's inline script")
}

// TestInjectJS_MultipleAdds_AllFire confirms every registered script executes.
func TestInjectJS_MultipleAdds_AllFire(t *testing.T) {
	srv := setupMockServer()
	defer srv.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "inject", "js", "add", `window.__a = 1;`)
	runCLI(t, bin, port, "inject", "js", "add", `window.__b = 2;`)
	runCLI(t, bin, port, "inject", "js", "add", `window.__c = 3;`)

	runCLI(t, bin, port, "navigate", srv.URL)

	out := runCLI(t, bin, port, "js", `() => [window.__a, window.__b, window.__c]`)
	assert.Contains(t, out, "1")
	assert.Contains(t, out, "2")
	assert.Contains(t, out, "3")
}

// TestInjectJS_MultipleAdds_PreserveOrder confirms scripts run in registration order.
func TestInjectJS_MultipleAdds_PreserveOrder(t *testing.T) {
	srv := setupMockServer()
	defer srv.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "inject", "js", "add", `window.__log = []; window.__log.push(1);`)
	runCLI(t, bin, port, "inject", "js", "add", `window.__log.push(2);`)
	runCLI(t, bin, port, "inject", "js", "add", `window.__log.push(3);`)

	runCLI(t, bin, port, "navigate", srv.URL)

	out := runCLI(t, bin, port, "js", `() => window.__log`)

	// Expect [1,2,3] — the raw JSON output contains the numbers in order.
	idx1 := strings.Index(out, "1")
	idx2 := strings.Index(out, "2")
	idx3 := strings.Index(out, "3")
	require.True(t, idx1 >= 0 && idx2 >= 0 && idx3 >= 0, "all three values must be present")
	assert.Less(t, idx1, idx2, "1 must appear before 2")
	assert.Less(t, idx2, idx3, "2 must appear before 3")
}

// TestInjectJS_Clear_StopsFutureNavigations verifies that after clear the script
// does NOT run on subsequent navigations.
func TestInjectJS_Clear_StopsFutureNavigations(t *testing.T) {
	srv := setupMockServer()
	defer srv.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "inject", "js", "add", `window.__injected = true;`)
	runCLI(t, bin, port, "inject", "js", "clear")

	runCLI(t, bin, port, "navigate", srv.URL)

	out := runCLI(t, bin, port, "js", `() => window.__injected`)
	// JSON null means the global was never set.
	assert.Contains(t, out, "null")
}

// TestInjectJS_Clear_DoesNotCorruptCurrentPage confirms that clear has no effect
// on the already-loaded document.
func TestInjectJS_Clear_DoesNotCorruptCurrentPage(t *testing.T) {
	srv := setupMockServer()
	defer srv.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", srv.URL)

	// Add then immediately clear before any new navigation.
	runCLI(t, bin, port, "inject", "js", "add", `window.__x = 99;`)
	runCLI(t, bin, port, "inject", "js", "clear")

	// The current page must still be functional.
	out := runCLI(t, bin, port, "js", `() => document.title`)
	assert.Contains(t, out, "Test Bed", "current page should be unaffected by clear")
}

// TestInjectJS_TabSpecific_OnlyFiresOnTargetTab verifies that a script registered
// for --tab active does not run on other tabs.
func TestInjectJS_TabSpecific_OnlyFiresOnTargetTab(t *testing.T) {
	srvA := setupNamedServer("Site A")
	defer srvA.Close()
	srvB := setupNamedServer("Site B")
	defer srvB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate tab 0 to Site A, then inject only for that tab.
	runCLI(t, bin, port, "navigate", srvA.URL)
	runCLI(t, bin, port, "inject", "js", "add", "--tab", "active", `window.__tagged = "tabA";`)

	// Open Site B in a new tab (auto-switches to tab 1).
	runCLI(t, bin, port, "tab", "new", srvB.URL)

	// Site B must not have the tag.
	out := runCLI(t, bin, port, "js", `() => window.__tagged`)
	assert.Contains(t, out, "null", "tab-specific inject must not fire on a different tab")
}

// TestInjectJS_GlobalApplies_ToExistingAndNewTabs verifies that a global inject
// (no --tab) fires on both existing tabs that navigate again and new tabs opened
// after registration.
func TestInjectJS_GlobalApplies_ToExistingAndNewTabs(t *testing.T) {
	srvA := setupNamedServer("Site A")
	defer srvA.Close()
	srvB := setupNamedServer("Site B")
	defer srvB.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Set up tab 0.
	runCLI(t, bin, port, "navigate", srvA.URL)

	// Register global inject (no --tab).
	runCLI(t, bin, port, "inject", "js", "add", `window.__global = true;`)

	// Reload tab 0 — existing tab should pick it up.
	runCLI(t, bin, port, "navigate", srvA.URL)
	outA := runCLI(t, bin, port, "js", `() => window.__global`)
	assert.Contains(t, outA, "true", "global inject must fire on existing tab after re-navigation")

	// Open a new tab — tab opened after registration must also receive the script.
	runCLI(t, bin, port, "tab", "new", srvB.URL)
	outB := runCLI(t, bin, port, "js", `() => window.__global`)
	assert.Contains(t, outB, "true", "global inject must fire on tabs opened after registration")
}

// TestInjectJS_URL_FetchedEagerly confirms that --url content is fetched at
// registration time. The test shuts down the origin server before navigating;
// the script must still run because it was inlined at add time.
func TestInjectJS_URL_FetchedEagerly(t *testing.T) {
	// Serve a tiny JS file.
	scriptSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, `window.__fromURL = 42;`) //nolint:errcheck
	}))

	pageSrv := setupMockServer()
	defer pageSrv.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Register via URL while the script server is still up.
	runCLI(t, bin, port, "inject", "js", "add", "--url", scriptSrv.URL)

	// Shut down the script origin — content must have been inlined already.
	scriptSrv.Close()

	// Navigate; script should still execute from inlined content.
	runCLI(t, bin, port, "navigate", pageSrv.URL)

	out := runCLI(t, bin, port, "js", `() => window.__fromURL`)
	assert.Contains(t, out, "42", "URL-based inject must use eagerly-fetched content")
}

// TestInjectJS_URL_UnreachableReturns502 verifies that an unreachable --url
// causes inject js add to fail (non-zero exit / error response).
func TestInjectJS_URL_UnreachableReturns502(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Port 1 is reserved / always refused.
	out, err := runCLIExpectFail(t, bin, port,
		"inject", "js", "add", "--url", "http://127.0.0.1:1/nonexistent.js")
	assert.Error(t, err, "unreachable URL must fail")
	_ = out // error message is informational; exact text is not asserted
}

// TestInjectJS_Add_MissingBothScriptAndURL_Errors verifies the validation when
// neither a script argument nor --url is supplied.
func TestInjectJS_Add_MissingBothScriptAndURL_Errors(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	_, err := runCLIExpectFail(t, bin, port, "inject", "js", "add")
	assert.Error(t, err, "add with neither script nor URL must fail")
}

// TestInjectJS_Add_BothScriptAndURL_Errors verifies the validation when both
// a script argument and --url are supplied simultaneously.
func TestInjectJS_Add_BothScriptAndURL_Errors(t *testing.T) {
	scriptSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `window.__x = 1;`) //nolint:errcheck
	}))
	defer scriptSrv.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	_, err := runCLIExpectFail(t, bin, port,
		"inject", "js", "add", "--url", scriptSrv.URL, `window.__x = 1;`)
	assert.Error(t, err, "add with both script and URL must fail")
}

// TestInjectJS_List_ReturnsRegisteredEntries verifies the list output contains
// both an inline entry and a URL-based entry.
func TestInjectJS_List_ReturnsRegisteredEntries(t *testing.T) {
	scriptSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, `window.__b = 2;`) //nolint:errcheck
	}))
	defer scriptSrv.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "inject", "js", "add", `window.__a = 1;`)
	runCLI(t, bin, port, "inject", "js", "add", "--url", scriptSrv.URL)

	out := runCLI(t, bin, port, "inject", "js", "list")

	var entries []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &entries), "list must return valid JSON array")
	require.Len(t, entries, 2, "expected 2 registered entries")

	// First entry: inline script
	assert.Equal(t, "inject-1", entries[0]["id"])
	assert.Contains(t, entries[0]["script"], "window.__a")
	assert.Empty(t, entries[0]["isURL"]) // false / omitted

	// Second entry: URL-based (content inlined, isURL=true)
	assert.Equal(t, "inject-2", entries[1]["id"])
	assert.Contains(t, entries[1]["script"], "window.__b")
	assert.Equal(t, true, entries[1]["isURL"])
	assert.Equal(t, scriptSrv.URL, entries[1]["url"])
}

// TestInjectJS_List_EmptyWhenNoneRegistered verifies list returns an empty array.
func TestInjectJS_List_EmptyWhenNoneRegistered(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	out := runCLI(t, bin, port, "inject", "js", "list")

	var entries []interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &entries))
	assert.Empty(t, entries, "list with no registrations must return empty array")
}

// TestInjectJS_ClearRespectsTabScope verifies that a tab-scoped clear removes
// only that tab's per-tab entries and leaves global entries intact.
func TestInjectJS_ClearRespectsTabScope(t *testing.T) {
	srvA := setupNamedServer("Site A")
	defer srvA.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", srvA.URL)

	// Register one tab-specific and one global entry.
	runCLI(t, bin, port, "inject", "js", "add", "--tab", "active", `window.__tabOnly = true;`)
	runCLI(t, bin, port, "inject", "js", "add", `window.__global = true;`)

	// Clear only the active tab's per-tab entries.
	runCLI(t, bin, port, "inject", "js", "clear", "--tab", "active")

	// Navigate to apply whatever remains.
	runCLI(t, bin, port, "navigate", srvA.URL)

	outTab := runCLI(t, bin, port, "js", `() => window.__tabOnly`)
	assert.Contains(t, outTab, "null", "tab-specific entry must be cleared")

	outGlobal := runCLI(t, bin, port, "js", `() => window.__global`)
	assert.Contains(t, outGlobal, "true", "global entry must survive a tab-scoped clear")
}

// TestInjectJS_RecordedInSession verifies that inject js add and clear actions
// are captured in a recording session.
func TestInjectJS_RecordedInSession(t *testing.T) {
	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	const recName = "injecttest"
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	recFile := filepath.Join(homeDir, ".browsii", "recordings", recName+".json")
	defer os.Remove(recFile) //nolint:errcheck

	runCLI(t, bin, port, "record", "start", recName)
	runCLI(t, bin, port, "inject", "js", "add", `window.__rec = true;`)
	runCLI(t, bin, port, "inject", "js", "clear")
	runCLI(t, bin, port, "record", "stop")

	// Read the recording file and verify both actions were captured.
	data, err := os.ReadFile(recFile)
	require.NoError(t, err, "recording file must exist after stop")

	var recording struct {
		Events []struct {
			Action string `json:"action"`
		} `json:"events"`
	}
	require.NoError(t, json.Unmarshal(data, &recording))

	actions := make([]string, 0, len(recording.Events))
	for _, e := range recording.Events {
		actions = append(actions, e.Action)
	}
	assert.Contains(t, actions, "inject_js_add", "recording must include inject_js_add")
	assert.Contains(t, actions, "inject_js_clear", "recording must include inject_js_clear")

	runCLI(t, bin, port, "record", "delete", recName)
}
