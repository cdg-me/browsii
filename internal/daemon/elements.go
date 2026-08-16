package daemon

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// maxElements caps how many elements a single /elements response returns.
// The enumeration itself is capped higher in JS (jsMaxElements) so that
// --filter/--all can still slice into the full set without re-evaluating.
const maxElements = 300

// jsMaxElements caps the number of descriptors the in-page enumeration builds.
const jsMaxElements = 500

// elementRect is the viewport-space bounding box of an element,
// rounded to integers to keep responses compact.
type elementRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// elementInfo describes one interactive element on a page.
// Refs are stable for as long as the page DOM is unchanged; any navigation
// or re-enumeration (another /elements call) reassigns them.
type elementInfo struct {
	Ref      int          `json:"ref"`
	Tag      string       `json:"tag"`
	Role     string       `json:"role"`
	Text     string       `json:"text,omitempty"`
	Name     string       `json:"name,omitempty"`
	Href     string       `json:"href,omitempty"`
	Type     string       `json:"type,omitempty"`
	Value    string       `json:"value,omitempty"`
	Checked  *bool        `json:"checked,omitempty"`
	Selector string       `json:"selector"`
	Visible  bool         `json:"visible"`
	Disabled bool         `json:"disabled,omitempty"`
	Rect     *elementRect `json:"rect,omitempty"`
}

// elementsJS enumerates interactive elements in the page's main frame and
// returns them as a JSON string (so Go can unmarshal cleanly). Hidden
// elements are included with visible=false; filtering happens daemon-side.
// Built as a var (not const) because it interpolates jsMaxElements.
var elementsJS = `() => {
	const SEL = [
		'a', 'button', 'input', 'select', 'textarea', 'summary', 'label',
		'[contenteditable="true"]', '[contenteditable=""]',
		'[role]', '[tabindex]', '[onclick]'
	].join(',');
	const SKIP = new Set(['script', 'style', 'template', 'noscript', 'svg']);

	function esc(s) {
		return (window.CSS && CSS.escape) ? CSS.escape(s) : s.replace(/([^a-zA-Z0-9_-])/g, '\\$1');
	}
	function unique(sel) {
		try { return document.querySelectorAll(sel).length === 1; } catch (e) { return false; }
	}
	function trunc(s, n) {
		s = s.replace(/\s+/g, ' ').trim();
		return s.length > n ? s.slice(0, n) + '…' : s;
	}
	function selectorFor(el) {
		const tag = el.tagName.toLowerCase();
		if (el.id && unique('#' + esc(el.id))) return '#' + esc(el.id);
		const nm = el.getAttribute('name');
		if (nm) {
			const safe = nm.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
			const s = tag + '[name="' + safe + '"]';
			if (unique(s)) return s;
		}
		const parts = [];
		let node = el, depth = 0;
		while (node && node.nodeType === 1 && depth < 6) {
			const t = node.tagName.toLowerCase();
			let part = t;
			const parent = node.parentElement;
			if (parent) {
				let idx = 1, sib = node;
				while ((sib = sib.previousElementSibling)) {
					if (sib.tagName === node.tagName) idx++;
				}
				if (idx > 1) part = t + ':nth-of-type(' + idx + ')';
			}
			parts.unshift(part);
			const sel = parts.join(' > ');
			if (depth > 0 && unique(sel)) return sel;
			node = parent;
			depth++;
		}
		return parts.join(' > ');
	}
	function roleOf(el, tag) {
		const r = el.getAttribute('role');
		if (r) return r;
		switch (tag) {
			case 'a': return el.getAttribute('href') != null ? 'link' : 'button';
			case 'button': case 'summary': return 'button';
			case 'select': return 'combobox';
			case 'textarea': return 'textbox';
			case 'label': return 'label';
			case 'input': {
				const t = (el.getAttribute('type') || 'text').toLowerCase();
				if (['button', 'submit', 'reset', 'image'].includes(t)) return 'button';
				if (t === 'checkbox') return 'checkbox';
				if (t === 'radio') return 'radio';
				return 'textbox';
			}
		}
		return 'generic';
	}
	function nameOf(el, tag) {
		const aria = el.getAttribute('aria-label');
		if (aria) return trunc(aria, 80);
		const lb = el.getAttribute('aria-labelledby');
		if (lb) {
			try {
				const t = document.getElementById(lb);
				if (t) return trunc(t.textContent, 80);
			} catch (e) {}
		}
		if (el.labels && el.labels.length) return trunc(el.labels[0].textContent, 80);
		if (tag === 'input' || tag === 'textarea') {
			const ph = el.getAttribute('placeholder');
			if (ph) return trunc(ph, 80);
		}
		const title = el.getAttribute('title');
		if (title) return trunc(title, 80);
		return '';
	}

	const out = [];
	const seen = new Set();
	for (const el of document.querySelectorAll(SEL)) {
		if (seen.has(el)) continue;
		seen.add(el);
		const tag = el.tagName.toLowerCase();
		if (SKIP.has(tag)) continue;
		const style = window.getComputedStyle(el);
		const visible = style.display !== 'none' && style.visibility !== 'hidden' &&
			!!(el.offsetWidth || el.offsetHeight || el.getClientRects().length);
		const r = el.getBoundingClientRect();
		const info = {
			tag: tag,
			role: roleOf(el, tag),
			text: trunc(el.innerText || el.textContent || '', 80),
			name: nameOf(el, tag),
			selector: selectorFor(el),
			visible: visible,
			disabled: !!el.disabled,
			rect: visible ? {
				x: Math.round(r.x), y: Math.round(r.y),
				w: Math.round(r.width), h: Math.round(r.height)
			} : null
		};
		if (tag === 'a') {
			const href = el.getAttribute('href');
			if (href) info.href = trunc(href, 120);
		}
		if (tag === 'input') {
			const t = (el.getAttribute('type') || 'text').toLowerCase();
			info.type = t;
			if (t === 'checkbox' || t === 'radio') info.checked = !!el.checked;
			else if (t !== 'password') info.value = trunc(el.value || '', 40);
		}
		if (tag === 'textarea') info.value = trunc(el.value || '', 40);
		out.push(info);
		if (out.length >= ` + strconv.Itoa(jsMaxElements) + `) break;
	}
	out.forEach((info, i) => { info.ref = i + 1; });
	return JSON.stringify(out);
}`

// enumerateElements runs the in-page enumeration and refreshes the ref store
// for the page. Refs are reassigned on every call; they remain valid until
// the page changes or the next enumeration.
func (s *Server) enumerateElements(page *rod.Page) ([]elementInfo, error) {
	res, err := page.Eval(elementsJS)
	if err != nil {
		return nil, err
	}
	var elems []elementInfo
	if err := json.Unmarshal([]byte(res.Value.Str()), &elems); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.elementRefs[page.TargetID] = elems
	s.mu.Unlock()
	return elems, nil
}

// invalidateElementRefs drops the ref store for a page (e.g. on navigation).
func (s *Server) invalidateElementRefs(targetID proto.TargetTargetID) {
	s.mu.Lock()
	delete(s.elementRefs, targetID)
	s.mu.Unlock()
}

// lookupElementRef resolves ref against the page's ref store, refreshing the
// store once when the ref is unknown (the store may predate a DOM change).
// Returns nil when the ref does not exist even after a refresh.
func (s *Server) lookupElementRef(page *rod.Page, ref int) *elementInfo {
	if ref < 1 {
		return nil
	}
	if e := s.lookupRefInStore(page.TargetID, ref); e != nil {
		return e
	}
	if _, err := s.enumerateElements(page); err != nil {
		return nil
	}
	return s.lookupRefInStore(page.TargetID, ref)
}

func (s *Server) lookupRefInStore(targetID proto.TargetTargetID, ref int) *elementInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	elems := s.elementRefs[targetID]
	if ref > len(elems) {
		return nil
	}
	e := elems[ref-1]
	return &e
}

// elementCandidate is an elementInfo annotated with why it might be what the
// caller was looking for.
type elementCandidate struct {
	elementInfo
	Note string `json:"note,omitempty"`
}

// findCandidates scores enumerated elements against a failed selector string
// and returns the best fuzzy matches (highest score first). Scoring is purely
// lexical: tokens from the selector are matched against the element's tag,
// role, text, name, href, type, and selector.
func findCandidates(elems []elementInfo, failed string, limit int) []elementCandidate {
	tokens := selectorTokens(failed)
	if len(tokens) == 0 {
		return nil
	}
	whole := normalizeMatchString(failed)

	type scored struct {
		elem  elementInfo
		score int
	}
	var matches []scored
	for _, e := range elems {
		hay := normalizeMatchString(strings.Join([]string{
			e.Tag, e.Role, e.Text, e.Name, e.Href, e.Type, e.Selector,
		}, " "))
		score := 0
		if whole != "" && strings.Contains(hay, whole) {
			score += 3
		}
		for _, tok := range tokens {
			if strings.Contains(hay, tok) {
				score++
			}
		}
		if score > 0 {
			matches = append(matches, scored{elem: e, score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].elem.Ref < matches[j].elem.Ref
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	if len(matches) == 0 {
		return nil
	}

	out := make([]elementCandidate, 0, len(matches))
	for _, m := range matches {
		c := elementCandidate{elementInfo: m.elem}
		switch {
		case !m.elem.Visible && m.elem.Disabled:
			c.Note = "hidden, disabled"
		case !m.elem.Visible:
			c.Note = "hidden"
		case m.elem.Disabled:
			c.Note = "disabled"
		}
		out = append(out, c)
	}
	return out
}

// selectorTokens splits a selector into lowercase alphanumeric tokens,
// dropping punctuation and single characters (which match too broadly).
func selectorTokens(sel string) []string {
	fields := strings.FieldsFunc(strings.ToLower(sel), func(r rune) bool {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		return !isLower && !isDigit
	})
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) >= 2 {
			tokens = append(tokens, f)
		}
	}
	return tokens
}

// normalizeMatchString lowercases and collapses a string for substring matching.
func normalizeMatchString(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// candidatesFor runs a fresh enumeration (so candidate refs are immediately
// usable) and returns fuzzy matches for the failed selector.
func (s *Server) candidatesFor(page *rod.Page, failedSelector string) []elementCandidate {
	elems, err := s.enumerateElements(page)
	if err != nil {
		return nil
	}
	return findCandidates(elems, failedSelector, 5)
}

// handleElements lists the interactive elements of the active page.
// POST /elements {"all": bool, "filter": "substring"}
func (s *Server) handleElements(w http.ResponseWriter, r *http.Request) {
	var req struct {
		All    bool   `json:"all"`
		Filter string `json:"filter"`
	}
	if !decodeBody(w, r, &req) {
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	elems, err := s.enumerateElements(page)
	if err != nil {
		http.Error(w, "element enumeration failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filter := normalizeMatchString(req.Filter)
	filtered := make([]elementInfo, 0, len(elems))
	for _, e := range elems {
		if !req.All && !e.Visible {
			continue
		}
		if filter != "" {
			hay := normalizeMatchString(strings.Join([]string{
				e.Tag, e.Role, e.Text, e.Name, e.Href, e.Type, e.Selector,
			}, " "))
			if !strings.Contains(hay, filter) {
				continue
			}
		}
		filtered = append(filtered, e)
	}

	truncated := false
	if len(filtered) > maxElements {
		filtered = filtered[:maxElements]
		truncated = true
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"count":     len(filtered),
		"truncated": truncated,
		"elements":  filtered,
	})
}

func (s *Server) registerElementRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/elements", s.handleElements)
}
