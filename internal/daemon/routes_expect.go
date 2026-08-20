package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

// expectPollInterval is the re-check cadence while waiting for a condition.
const expectPollInterval = 100 * time.Millisecond

// expectDefaultTimeout bounds a wait when the caller does not set one.
const expectDefaultTimeout = 5 * time.Second

// requestLookbackSecs is how far back a --request condition searches before
// the check started: the natural agent flow is act → expect, so requests
// triggered by the action fire just before expect begins.
const requestLookbackSecs = 10.0

// expectRequest is the /expect payload. Exactly one primary condition is
// allowed; --no-console-errors may be combined with any primary condition
// (or used alone to assert over the recent error history).
type expectRequest struct {
	Text           string `json:"text"`
	TextGone       string `json:"textGone"`
	URLPattern     string `json:"urlPattern"`
	Selector       string `json:"selector"`
	Hidden         bool   `json:"hidden"`
	Enabled        *bool  `json:"enabled"`
	Ref            int    `json:"ref"`
	Value          string `json:"value"`
	Request        string `json:"request"`
	NoConsoleError bool   `json:"noConsoleErrors"`
	TimeoutMs      int    `json:"timeoutMs"`
}

// globToRegexp compiles a simple *-wildcard pattern into a regexp.
// Everything except '*' is matched literally.
func globToRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		if r == '*' {
			b.WriteString(".*")
		} else {
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// parseRequestSpec splits a "METHOD pattern" request matcher into its parts.
// A pattern without a leading uppercase method matches any method.
func parseRequestSpec(spec string) (method, pattern string) {
	spec = strings.TrimSpace(spec)
	if i := strings.IndexByte(spec, ' '); i > 0 {
		head, rest := spec[:i], strings.TrimSpace(spec[i+1:])
		if head == strings.ToUpper(head) && regexp.MustCompile(`^[A-Z]+$`).MatchString(head) {
			return head, rest
		}
	}
	return "", spec
}

// matchRequestSpec reports whether a ring entry satisfies the "METHOD glob"
// spec (empty method matches all verbs).
func matchRequestSpec(spec string, e netRingEntry) bool {
	method, pattern := parseRequestSpec(spec)
	if method != "" && method != e.Method {
		return false
	}
	return globToRegexp(pattern).MatchString(e.URL)
}

// handleExpect waits (up to timeoutMs) for a page-level assertion to become
// true. Failures are actionable: they include the current URL, the closest
// matching text on the page, or the requests/console errors that did occur.
func (s *Server) handleExpect(w http.ResponseWriter, r *http.Request) {
	var req expectRequest
	if !decodeBodyRequired(w, r, &req) {
		return
	}

	page := s.activePage()

	timeout := expectDefaultTimeout
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	hasTarget := req.Selector != "" || req.Ref > 0
	primary := 0
	for _, b := range []bool{req.Text != "", req.TextGone != "", req.URLPattern != "", req.Value != "", req.Request != "", hasTarget && req.Value == ""} {
		if b {
			primary++
		}
	}
	if req.Value != "" && !hasTarget {
		writeAPIError(w, &apiError{
			Status:  http.StatusBadRequest,
			Message: "value condition requires selector or ref",
			Hint:    "pass {\"value\": \"...\", \"selector\": \"...\"} or a ref from 'elements'",
		})
		return
	}
	if primary == 0 && !req.NoConsoleError {
		writeAPIError(w, &apiError{
			Status:  http.StatusBadRequest,
			Message: "no condition given",
			Hint:    "use one of: text, text-gone, url-pattern, selector (+hidden), value, request, no-console-errors",
		})
		return
	}
	if primary > 1 {
		writeAPIError(w, &apiError{
			Status:  http.StatusBadRequest,
			Message: "multiple primary conditions given — expect takes exactly one",
			Hint:    "combine checks with separate expect calls; no-console-errors may accompany any condition",
		})
		return
	}
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	sinceSeq := s.currentEventSeq()
	cond, cerr := s.expectCondition(page, &req, sinceSeq)
	if cerr != nil {
		writeAPIError(w, cerr)
		return
	}

	s.recordExpect(&req, timeout)

	start := time.Now()
	describe := expectDescribe(&req)

	for {
		ok, detail := cond()
		if ok {
			if req.NoConsoleError {
				if errs := s.consoleErrorsSince(sinceSeq); len(errs) > 0 {
					s.writeExpectFail(w, page, describe, timeout,
						"console errors detected while waiting",
						[]string{firstConsoleErrors(errs, 5)}, "")
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":     true,
				"detail": fmt.Sprintf("%s (after %dms)", expectDescribe(&req), time.Since(start).Milliseconds()),
			})
			return
		}
		if detail != "" {
			describe = detail // refine description with live state
		}
		if time.Since(start) >= timeout {
			s.writeExpectTimeout(w, page, &req, describe, timeout, sinceSeq)
			return
		}
		time.Sleep(expectPollInterval)
	}
}

// recordExpect appends the expect call to the active recording so replays
// enforce it as a checkpoint. Refs are not stable across sessions, so only
// selector-based conditions are recorded verbatim; a ref condition is
// recorded with its resolved selector.
func (s *Server) recordExpect(req *expectRequest, timeout time.Duration) {
	if !s.recording {
		return
	}
	params := map[string]interface{}{}
	if req.Text != "" {
		params["text"] = req.Text
	}
	if req.TextGone != "" {
		params["textGone"] = req.TextGone
	}
	if req.URLPattern != "" {
		params["urlPattern"] = req.URLPattern
	}
	if req.Selector != "" {
		params["selector"] = req.Selector
	}
	if req.Value != "" {
		params["value"] = req.Value
	}
	if req.Request != "" {
		params["request"] = req.Request
	}
	if req.Hidden {
		params["hidden"] = true
	}
	if req.Enabled != nil {
		params["enabled"] = *req.Enabled
	}
	if req.NoConsoleError {
		params["noConsoleErrors"] = true
	}
	s.recMu.Lock()
	s.recordEvents = append(s.recordEvents, RecordedEvent{
		T:         time.Since(s.recordStart).Milliseconds(),
		Action:    "expect",
		Params:    params,
		TimeoutMs: int(timeout.Milliseconds()),
	})
	s.recMu.Unlock()
}

// expectCondition returns the polling closure for the request's primary
// condition (validation already happened in handleExpect). detail returned
// by the closure updates the failure description. sinceSeq anchors
// ring-based conditions to the moment the check started.
func (s *Server) expectCondition(page *rod.Page, req *expectRequest, sinceSeq int64) (func() (bool, string), *apiError) {
	switch {
	case req.Text != "":
		return func() (bool, string) {
			res, err := page.Eval(pageTextIncludesJS, req.Text)
			return err == nil && res.Value.Bool(), ""
		}, nil

	case req.TextGone != "":
		return func() (bool, string) {
			res, err := page.Eval(pageTextNotIncludesJS, req.TextGone)
			return err == nil && res.Value.Bool(), ""
		}, nil

	case req.URLPattern != "":
		re := globToRegexp(req.URLPattern)
		return func() (bool, string) {
			res, err := page.Eval(`() => location.href`)
			if err != nil {
				return false, ""
			}
			href := res.Value.Str()
			if re.MatchString(href) {
				return true, ""
			}
			return false, "url is " + href
		}, nil

	case req.Value != "":
		selector, aerr := s.resolveElementTarget(page, req.Ref, req.Selector)
		if aerr != nil {
			return nil, aerr
		}
		return func() (bool, string) {
			res, err := page.Eval(`(sel) => {`+elementsHelpersJS+`
				const el = resolveOne(sel);
				return el ? (el.value !== undefined ? el.value : el.textContent) : null;
			}`, selector)
			if err != nil {
				return false, ""
			}
			v := res.Value.Str()
			if v == req.Value {
				return true, ""
			}
			return false, fmt.Sprintf("value of %s is %q", selector, v)
		}, nil

	case req.Selector != "" || req.Ref > 0:
		selector, aerr := s.resolveElementTarget(page, req.Ref, req.Selector)
		if aerr != nil {
			return nil, aerr
		}
		switch {
		case req.Enabled != nil:
			wantEnabled := *req.Enabled
			return func() (bool, string) {
				res, err := page.Eval(`(sel) => {`+elementsHelpersJS+`
					const el = resolveOne(sel);
					if (!el) return "missing";
					if (el.disabled) return "disabled";
					if (el.tagName === 'A' || el.tagName === 'AREA') return el.getAttribute('aria-disabled') === 'true' ? "disabled" : "enabled";
					return "enabled";
				}`, selector)
				if err != nil {
					return false, ""
				}
				state := res.Value.Str()
				if wantEnabled {
					return state == "enabled", "element is " + state
				}
				return state != "enabled", "element is " + state
			}, nil
		default:
			wantVisible := !req.Hidden
			return func() (bool, string) {
				res, err := page.Eval(`(sel) => {`+elementsHelpersJS+`
					const el = resolveOne(sel);
					if (!el) return "missing";
					const vis = !!(el.offsetWidth || el.offsetHeight || el.getClientRects().length);
					return vis ? "visible" : "hidden";
				}`, selector)
				if err != nil {
					return false, ""
				}
				state := res.Value.Str()
				if wantVisible {
					return state == "visible", "element is " + state
				}
				return state == "hidden" || state == "missing", "element is " + state
			}, nil
		}

	case req.Request != "":
		spec := req.Request
		return func() (bool, string) {
			cutoff := float64(time.Now().UnixNano())/1e9 - requestLookbackSecs
			for _, e := range s.netRequestsSince(0) {
				if e.Seq > sinceSeq || e.TS >= cutoff {
					if matchRequestSpec(spec, e) {
						return true, ""
					}
				}
			}
			return false, ""
		}, nil
	}

	// Standalone no-console-errors: assert over the retained history.
	return func() (bool, string) {
		errs := s.consoleErrorsSince(0)
		if len(errs) == 0 {
			return true, ""
		}
		return false, "recent console errors:\n" + firstConsoleErrors(errs, 5)
	}, nil
}

// expectDescribe renders the human description of the assertion.
func expectDescribe(req *expectRequest) string {
	switch {
	case req.Text != "":
		return fmt.Sprintf("text %q visible", req.Text)
	case req.TextGone != "":
		return fmt.Sprintf("text %q gone", req.TextGone)
	case req.URLPattern != "":
		return fmt.Sprintf("url matches %s", req.URLPattern)
	case req.Value != "":
		return fmt.Sprintf("value = %q", req.Value)
	case req.Enabled != nil && !*req.Enabled:
		return "element disabled"
	case req.Enabled != nil:
		return "element enabled"
	case req.Hidden:
		return "element hidden"
	case req.Selector != "" || req.Ref > 0:
		target := req.Selector
		if req.Ref > 0 {
			target = fmt.Sprintf("ref %d", req.Ref)
		}
		return fmt.Sprintf("element visible: %s", target)
	case req.Request != "":
		return fmt.Sprintf("request matched %s", req.Request)
	default:
		return "no console errors"
	}
}

// firstConsoleErrors renders up to n console errors, one per line.
func firstConsoleErrors(errs []consoleRingEntry, n int) string {
	var b strings.Builder
	for i, e := range errs {
		if i >= n {
			fmt.Fprintf(&b, "… and %d more", len(errs)-n)
			break
		}
		fmt.Fprintf(&b, "  [%s] %s\n", e.Level, e.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

// writeExpectFail writes a failed assertion (non-timeout path).
func (s *Server) writeExpectFail(w http.ResponseWriter, page *rod.Page, describe string, timeout time.Duration, reason string, details []string, hint string) {
	if hint == "" {
		hint = "the assertion failed as soon as it was checked — the page state contradicts it"
	}
	s.writeExpectError(w, page, describe, timeout, reason, details, hint)
}

// writeExpectTimeout writes a timed-out assertion with actionable diffs.
func (s *Server) writeExpectTimeout(w http.ResponseWriter, page *rod.Page, req *expectRequest, describe string, timeout time.Duration, sinceSeq int64) {
	var details []string
	hint := "the condition never became true within the timeout"

	var reqs []netRingEntry
	if req.Request != "" {
		cutoff := float64(time.Now().UnixNano())/1e9 - requestLookbackSecs
		for _, e := range s.netRequestsSince(0) {
			if e.Seq > sinceSeq || e.TS >= cutoff {
				reqs = append(reqs, e)
			}
		}
		if len(reqs) == 0 {
			details = append(details, "no requests fired at all since the check started")
			hint = "no network activity was observed — the action that should have triggered the request may not have fired"
		} else {
			var b strings.Builder
			b.WriteString("requests that fired:\n")
			last := len(reqs) - 5
			if last < 0 {
				last = 0
			}
			for _, e := range reqs[last:] {
				b.WriteString("  " + formatNetEntry(e) + "\n")
			}
			details = append(details, strings.TrimRight(b.String(), "\n"))
			hint = "the expected request did not fire — check the action that should have triggered it"
		}
	} else if req.Text != "" {
		if closest := s.closestTexts(page, req.Text); len(closest) > 0 {
			details = append(details, "closest text on page:\n  "+strings.Join(closest, "\n  "))
			hint = "no exact match — the closest strings are listed; adjust the expected text or wait for the UI update"
		}
	} else if req.NoConsoleError {
		if errs := s.consoleErrorsSince(sinceSeq); len(errs) > 0 {
			details = append(details, "console errors:\n"+firstConsoleErrors(errs, 5))
			hint = "errors were logged while waiting — fix the page error or scope the check"
		}
	}

	s.writeExpectError(w, page, describe, timeout, "timed out", details, hint)
}

// writeExpectError serializes the failed-expect response.
func (s *Server) writeExpectError(w http.ResponseWriter, page *rod.Page, describe string, timeout time.Duration, reason string, details []string, hint string) {
	e := &apiError{
		Status:  http.StatusExpectationFailed,
		Message: fmt.Sprintf("expect failed: %s — %s after %dms", describe, reason, timeout.Milliseconds()),
		Hint:    hint,
	}
	for _, d := range details {
		e.Message += "\n" + d
	}
	writeAPIError(w, e)
}

// closestTexts returns up to 3 lines of visible page text most similar to
// want (token-overlap scoring) — the "did you mean" for text assertions.
func (s *Server) closestTexts(page *rod.Page, want string) []string {
	res, err := page.Eval(`() => document.body ? document.body.innerText : ""`)
	if err != nil {
		return nil
	}
	wantTokens := tokenSet(want)
	type scored struct {
		line  string
		score int
	}
	var matches []scored
	for _, line := range strings.Split(res.Value.Str(), "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}
		score := 0
		for tok := range tokenSet(line) {
			if wantTokens[tok] {
				score++
				continue
			}
			// Prefix overlap catches morphological variants: save/saved,
			// order/orders — the "did you mean" cases.
			for w := range wantTokens {
				if strings.HasPrefix(tok, w) || strings.HasPrefix(w, tok) {
					score++
					break
				}
			}
		}
		if score > 0 {
			matches = append(matches, scored{line: line, score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	out := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, m := range matches {
		if seen[m.line] {
			continue
		}
		seen[m.line] = true
		if len(m.line) > 80 {
			m.line = m.line[:80] + "…"
		}
		out = append(out, fmt.Sprintf("%q", m.line))
		if len(out) == 3 {
			break
		}
	}
	return out
}

// tokenSet splits s into a lowercase alphanumeric token set.
func tokenSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, tok := range selectorTokens(s) {
		set[tok] = true
	}
	return set
}

func (s *Server) registerExpectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/expect", s.handleExpect)
}

// pageTextIncludesJS checks whether text appears anywhere in the rendered
// page, including open shadow roots and same-origin iframes. innerText
// alone excludes shadow content, so expectations on shadow-rendered text
// would never match.
var pageTextIncludesJS = `(s) => {
	` + elementsHelpersJS + `
	function allText(root, parts) {
		for (const el of root.querySelectorAll('*')) {
			if (el.shadowRoot) allText(el.shadowRoot, parts);
			else if (el.tagName === 'IFRAME') {
				try { if (el.contentDocument) allText(el.contentDocument, parts); } catch (e) {}
			}
		}
		if (root === document) {
			if (document.body) parts.push(document.body.innerText);
		} else if (root.body) {
			// iframe document
			parts.push(root.body.innerText);
		} else {
			// shadow root: no body; read the root's own rendered text
			parts.push(root.textContent);
		}
	}
	const parts = [];
	allText(document, parts);
	return parts.some(p => p.includes(s));
}`

var pageTextNotIncludesJS = `(s) => {
	` + elementsHelpersJS + `
	function allText(root, parts) {
		for (const el of root.querySelectorAll('*')) {
			if (el.shadowRoot) allText(el.shadowRoot, parts);
			else if (el.tagName === 'IFRAME') {
				try { if (el.contentDocument) allText(el.contentDocument, parts); } catch (e) {}
			}
		}
		if (root === document) {
			if (document.body) parts.push(document.body.innerText);
		} else if (root.body) {
			// iframe document
			parts.push(root.body.innerText);
		} else {
			// shadow root: no body; read the root's own rendered text
			parts.push(root.textContent);
		}
	}
	const parts = [];
	allText(document, parts);
	return !parts.some(p => p.includes(s));
}`
