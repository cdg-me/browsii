package daemon

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// keyMap maps lowercase key names to go-rod input constants.
// Defined at package level so it is not reallocated on every parseKeyCombo call.
var keyMap = map[string]input.Key{
	"enter":      input.Enter,
	"tab":        input.Tab,
	"escape":     input.Escape,
	"backspace":  input.Backspace,
	"delete":     input.Delete,
	"space":      input.Space,
	"arrowup":    input.ArrowUp,
	"arrowdown":  input.ArrowDown,
	"arrowleft":  input.ArrowLeft,
	"arrowright": input.ArrowRight,
	"control":    input.ControlLeft,
	"ctrl":       input.ControlLeft,
	"shift":      input.ShiftLeft,
	"alt":        input.AltLeft,
	"meta":       input.MetaLeft,
	"command":    input.MetaLeft,
	"home":       input.Home,
	"end":        input.End,
	"pageup":     input.PageUp,
	"pagedown":   input.PageDown,
}

// charMap maps lowercase ASCII characters to go-rod input constants.
var charMap = map[byte]input.Key{
	'a': input.KeyA, 'b': input.KeyB, 'c': input.KeyC, 'd': input.KeyD,
	'e': input.KeyE, 'f': input.KeyF, 'g': input.KeyG, 'h': input.KeyH,
	'i': input.KeyI, 'j': input.KeyJ, 'k': input.KeyK, 'l': input.KeyL,
	'm': input.KeyM, 'n': input.KeyN, 'o': input.KeyO, 'p': input.KeyP,
	'q': input.KeyQ, 'r': input.KeyR, 's': input.KeyS, 't': input.KeyT,
	'u': input.KeyU, 'v': input.KeyV, 'w': input.KeyW, 'x': input.KeyX,
	'y': input.KeyY, 'z': input.KeyZ,
	'0': input.Digit0, '1': input.Digit1, '2': input.Digit2, '3': input.Digit3,
	'4': input.Digit4, '5': input.Digit5, '6': input.Digit6, '7': input.Digit7,
	'8': input.Digit8, '9': input.Digit9,
}

// parseKeyCombo splits a key combo string like "Control+a" into a slice of input.Key.
func parseKeyCombo(combo string) []input.Key {
	parts := strings.Split(combo, "+")
	var keys []input.Key
	for _, part := range parts {
		part = strings.TrimSpace(part)
		lower := strings.ToLower(part)
		if k, ok := keyMap[lower]; ok {
			keys = append(keys, k)
		} else if len(part) == 1 {
			ch := strings.ToLower(part)[0]
			if k, ok := charMap[ch]; ok {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

// wrapScript ensures s is a valid go-rod Eval expression (a callable function).
// go-rod calls .apply() on whatever it receives, so bare expressions like
// "document.title" or "2+2" must be wrapped in an arrow function.
// Strings that already start with "function", "async", or look like an arrow
// function ("(...) =>") are passed through unchanged.
func wrapScript(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "function") || strings.HasPrefix(s, "async") {
		return s
	}
	// Arrow function: starts with '(' and contains '=>'
	if strings.HasPrefix(s, "(") && strings.Contains(s, "=>") {
		return s
	}
	return "() => (" + s + ")"
}

// normalizeConsoleLevel maps CDP's "warning" to the JS API name "warn".
// All other types are returned as-is (log, error, info, debug, …).
func normalizeConsoleLevel(t proto.RuntimeConsoleAPICalledType) string {
	if t == proto.RuntimeConsoleAPICalledTypeWarning {
		return "warn"
	}
	return string(t)
}

// consoleArgText extracts a human-readable string from a single console argument.
// Priority: UnserializableValue (Infinity/NaN/-0) → Description (objects/errors)
// → primitive Value → empty string.
func consoleArgText(arg *proto.RuntimeRemoteObject) string {
	if string(arg.UnserializableValue) != "" {
		return string(arg.UnserializableValue)
	}
	if arg.Description != "" {
		return arg.Description
	}
	if v := arg.Value.Val(); v != nil {
		switch val := v.(type) {
		case string:
			return val
		default:
			b, err := json.Marshal(val)
			if err == nil {
				return string(b)
			}
		}
	}
	return ""
}

// formatConsoleArgs joins all args into a single space-separated string,
// matching the DevTools console display format.
func formatConsoleArgs(args []*proto.RuntimeRemoteObject) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, consoleArgText(arg))
	}
	return strings.Join(parts, " ")
}

// buildConsoleArgs converts raw CDP remote objects into a structured slice
// that preserves type, value, description, and class for SDK consumers.
func buildConsoleArgs(rawArgs []*proto.RuntimeRemoteObject) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(rawArgs))
	for _, arg := range rawArgs {
		entry := map[string]interface{}{
			"type": string(arg.Type),
		}
		if arg.Description != "" {
			entry["description"] = arg.Description
		}
		if arg.ClassName != "" {
			entry["class"] = arg.ClassName
		}
		if string(arg.UnserializableValue) != "" {
			entry["value"] = string(arg.UnserializableValue)
		} else if v := arg.Value.Val(); v != nil {
			entry["value"] = v
		}
		result = append(result, entry)
	}
	return result
}

// matchesLevelFilter returns true when filter is empty (all levels pass)
// or when level appears in the comma-separated filter string.
func matchesLevelFilter(level, filter string) bool {
	if filter == "" {
		return true
	}
	for _, f := range strings.Split(filter, ",") {
		if strings.TrimSpace(f) == level {
			return true
		}
	}
	return false
}

// resolveTabAlias converts a tab alias string ("", "all", "active", "next",
// "last", or a numeric index string) into an integer tab index.
// Returns -1 to represent "all tabs". pageOrder and activeID are used to
// resolve "active" to its current index.
func resolveTabAlias(alias string, pageOrder []proto.TargetTargetID, activeID proto.TargetTargetID) int {
	switch alias {
	case "", "all":
		return -1
	case "active":
		if activeID != "" {
			for i, id := range pageOrder {
				if id == activeID {
					return i
				}
			}
		}
		return -1
	case "next":
		return len(pageOrder) // index the next new tab will occupy
	case "last":
		if len(pageOrder) > 0 {
			return len(pageOrder) - 1
		}
		return -1
	default:
		if n, err := strconv.Atoi(alias); err == nil {
			return n
		}
		return -1
	}
}

// decodeBody decodes a JSON body into dst. An empty body is silently accepted
// (leaving dst at its zero value), which is appropriate for endpoints where the
// body is optional. Returns false and writes HTTP 400 if the body is present
// but malformed.
func decodeBody(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if err == io.EOF {
			return true // empty body — optional fields stay at zero value
		}
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// decodeBodyRequired decodes a required JSON body into dst. Returns false and
// writes HTTP 400 on any error (including an empty body).
func decodeBodyRequired(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}
