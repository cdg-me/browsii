package daemon

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// ── wrapScript ────────────────────────────────────────────────────────────────

func TestWrapScript(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		// Bare expressions must be wrapped
		{"bare identifier", "document.title", "() => (document.title)"},
		{"arithmetic", "2+2", "() => (2+2)"},
		{"method call", "window.scrollY", "() => (window.scrollY)"},

		// Leading/trailing whitespace is trimmed before wrapping
		{"whitespace trimmed", "  document.title  ", "() => (document.title)"},

		// Named functions pass through unchanged
		{"named function", "function() { return 1; }", "function() { return 1; }"},
		{"async function", "async function() { return 1; }", "async function() { return 1; }"},

		// Arrow functions (starts with '(' AND contains '=>') pass through
		{"explicit arrow no args", "() => document.title", "() => document.title"},
		{"explicit arrow with body", "() => { return 1; }", "() => { return 1; }"},
		{"arrow with param", "(x) => x + 1", "(x) => x + 1"},

		// Parenthesized expression with NO '=>' is wrapped (not an arrow function)
		{"parenthesized no arrow", "(1 + 2)", "() => ((1 + 2))"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapScript(tc.input)
			if got != tc.want {
				t.Errorf("wrapScript(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── parseKeyCombo ─────────────────────────────────────────────────────────────

func TestParseKeyCombo(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []input.Key
	}{
		// Named special keys
		{"Enter", "Enter", []input.Key{input.Enter}},
		{"enter lc", "enter", []input.Key{input.Enter}},
		{"Escape", "Escape", []input.Key{input.Escape}},
		{"Tab", "Tab", []input.Key{input.Tab}},
		{"Backspace", "Backspace", []input.Key{input.Backspace}},
		{"Delete", "Delete", []input.Key{input.Delete}},
		{"Space", "Space", []input.Key{input.Space}},
		{"ArrowUp", "ArrowUp", []input.Key{input.ArrowUp}},
		{"ArrowDown", "ArrowDown", []input.Key{input.ArrowDown}},
		{"ArrowLeft", "ArrowLeft", []input.Key{input.ArrowLeft}},
		{"ArrowRight", "ArrowRight", []input.Key{input.ArrowRight}},
		{"Home", "Home", []input.Key{input.Home}},
		{"End", "End", []input.Key{input.End}},
		{"PageUp", "PageUp", []input.Key{input.PageUp}},
		{"PageDown", "PageDown", []input.Key{input.PageDown}},

		// Single character keys (case-insensitive lookup)
		{"lowercase a", "a", []input.Key{input.KeyA}},
		{"uppercase A", "A", []input.Key{input.KeyA}},
		{"digit 0", "0", []input.Key{input.Digit0}},
		{"digit 9", "9", []input.Key{input.Digit9}},

		// Key combos
		{"Control+a", "Control+a", []input.Key{input.ControlLeft, input.KeyA}},
		{"Ctrl+a", "Ctrl+a", []input.Key{input.ControlLeft, input.KeyA}},
		{"Shift+a", "Shift+a", []input.Key{input.ShiftLeft, input.KeyA}},
		{"Meta+a", "Meta+a", []input.Key{input.MetaLeft, input.KeyA}},
		{"Command+a", "Command+a", []input.Key{input.MetaLeft, input.KeyA}},
		{"Alt+Tab", "Alt+Tab", []input.Key{input.AltLeft, input.Tab}},
		{"Control+Shift+a", "Control+Shift+a", []input.Key{input.ControlLeft, input.ShiftLeft, input.KeyA}},

		// Unknown / unsupported produce nil
		{"unknown F12", "F12", nil},
		{"empty string", "", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseKeyCombo(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseKeyCombo(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ── matchesLevelFilter ────────────────────────────────────────────────────────

func TestMatchesLevelFilter(t *testing.T) {
	cases := []struct {
		name   string
		level  string
		filter string
		want   bool
	}{
		// Empty filter always passes
		{"empty filter log", "log", "", true},
		{"empty filter error", "error", "", true},

		// Exact single match
		{"exact match", "error", "error", true},
		{"no match", "log", "error", false},

		// Comma-separated
		{"multi first", "error", "error,warn", true},
		{"multi second", "warn", "error,warn", true},
		{"multi miss", "log", "error,warn", false},
		{"info in list", "info", "info,debug", true},
		{"debug in list", "debug", "info,debug", true},

		// Whitespace trimming around filter entries
		{"space before", "warn", " warn", true},
		{"space after", "warn", "warn ", true},
		{"space around", "warn", "error, warn", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesLevelFilter(tc.level, tc.filter)
			if got != tc.want {
				t.Errorf("matchesLevelFilter(%q, %q) = %v, want %v",
					tc.level, tc.filter, got, tc.want)
			}
		})
	}
}

// ── normalizeConsoleLevel ─────────────────────────────────────────────────────

func TestNormalizeConsoleLevel(t *testing.T) {
	cases := []struct {
		input proto.RuntimeConsoleAPICalledType
		want  string
	}{
		{proto.RuntimeConsoleAPICalledTypeWarning, "warn"},
		{proto.RuntimeConsoleAPICalledTypeLog, "log"},
		{proto.RuntimeConsoleAPICalledTypeError, "error"},
		{proto.RuntimeConsoleAPICalledTypeInfo, "info"},
		{proto.RuntimeConsoleAPICalledTypeDebug, "debug"},
	}

	for _, tc := range cases {
		t.Run(string(tc.input), func(t *testing.T) {
			got := normalizeConsoleLevel(tc.input)
			if got != tc.want {
				t.Errorf("normalizeConsoleLevel(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── resolveTabAlias ───────────────────────────────────────────────────────────

func TestResolveTabAlias(t *testing.T) {
	idA := proto.TargetTargetID("aaa")
	idB := proto.TargetTargetID("bbb")
	idC := proto.TargetTargetID("ccc")
	pageOrder := []proto.TargetTargetID{idA, idB, idC}

	cases := []struct {
		name     string
		alias    string
		activeID proto.TargetTargetID
		want     int
	}{
		// Empty and "all" → -1 (all tabs)
		{"empty alias", "", "", -1},
		{"explicit all", "all", "", -1},

		// "active" with a valid active page ID → its index
		{"active = first", "active", idA, 0},
		{"active = second", "active", idB, 1},
		{"active = third", "active", idC, 2},

		// "active" with no active page → -1
		{"active no page", "active", "", -1},

		// "next" → len(pageOrder) (index the next new tab will occupy)
		{"next", "next", "", 3},

		// "last" → len-1
		{"last", "last", "", 2},

		// Numeric string → parsed integer
		{"numeric 0", "0", "", 0},
		{"numeric 2", "2", "", 2},
		{"numeric 5", "5", "", 5}, // out of range is returned as-is; caller bounds-checks

		// Unknown string → -1
		{"unknown", "xyz", "", -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveTabAlias(tc.alias, pageOrder, tc.activeID)
			if got != tc.want {
				t.Errorf("resolveTabAlias(%q, pageOrder, %q) = %d, want %d",
					tc.alias, tc.activeID, got, tc.want)
			}
		})
	}
}

// resolveTabAlias with empty pageOrder
func TestResolveTabAliasEmptyOrder(t *testing.T) {
	empty := []proto.TargetTargetID{}

	if got := resolveTabAlias("next", empty, ""); got != 0 {
		t.Errorf("next on empty order = %d, want 0", got)
	}
	if got := resolveTabAlias("last", empty, ""); got != -1 {
		t.Errorf("last on empty order = %d, want -1", got)
	}
}

// ── decodeBody ────────────────────────────────────────────────────────────────

func TestDecodeBody(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	t.Run("valid JSON populates struct", func(t *testing.T) {
		body := strings.NewReader(`{"name":"alice"}`)
		r := httptest.NewRequest(http.MethodPost, "/", body) //nolint:noctx
		w := httptest.NewRecorder()
		var p payload
		ok := decodeBody(w, r, &p)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if p.Name != "alice" {
			t.Errorf("Name = %q, want %q", p.Name, "alice")
		}
	})

	t.Run("empty body returns true with zero value", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(nil)) //nolint:noctx
		w := httptest.NewRecorder()
		var p payload
		ok := decodeBody(w, r, &p)
		if !ok {
			t.Fatal("expected ok=true for empty body")
		}
		if p.Name != "" {
			t.Errorf("Name should be empty, got %q", p.Name)
		}
	})

	t.Run("malformed JSON returns false and 400", func(t *testing.T) {
		body := strings.NewReader(`not json`)
		r := httptest.NewRequest(http.MethodPost, "/", body) //nolint:noctx
		w := httptest.NewRecorder()
		var p payload
		ok := decodeBody(w, r, &p)
		if ok {
			t.Fatal("expected ok=false for malformed JSON")
		}
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})
}

func TestDecodeBodyRequired(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	t.Run("valid JSON populates struct", func(t *testing.T) {
		body := strings.NewReader(`{"name":"bob"}`)
		r := httptest.NewRequest(http.MethodPost, "/", body) //nolint:noctx
		w := httptest.NewRecorder()
		var p payload
		ok := decodeBodyRequired(w, r, &p)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if p.Name != "bob" {
			t.Errorf("Name = %q, want %q", p.Name, "bob")
		}
	})

	t.Run("empty body returns false and 400", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(nil)) //nolint:noctx
		w := httptest.NewRecorder()
		var p payload
		ok := decodeBodyRequired(w, r, &p)
		if ok {
			t.Fatal("expected ok=false for empty body")
		}
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("malformed JSON returns false and 400", func(t *testing.T) {
		body := strings.NewReader(`{bad`)
		r := httptest.NewRequest(http.MethodPost, "/", body) //nolint:noctx
		w := httptest.NewRecorder()
		var p payload
		ok := decodeBodyRequired(w, r, &p)
		if ok {
			t.Fatal("expected ok=false for malformed JSON")
		}
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})
}
