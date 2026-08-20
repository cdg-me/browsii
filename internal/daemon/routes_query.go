package daemon

import (
	"encoding/json"
	"net/http"
)

func (s *Server) registerQueryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/find", s.handleFind)
	mux.HandleFunc("/element", s.handleElement)
}

// findResponse is the /find result: the true match count plus up to three
// matching lines with their line numbers.
type findResponse struct {
	Count   int             `json:"count"`
	Matches []findMatchLine `json:"matches"`
}

type findMatchLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// handleFind searches the rendered page text (body.innerText split into
// lines). text is a case-insensitive substring; regex is a JS regular
// expression, optionally in /pattern/flags form. Pure predicate: no
// selection or scroll side effects.
func (s *Server) handleFind(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text  string `json:"text"`
		Regex string `json:"regex"`
	}
	if !decodeBodyRequired(w, r, &req) {
		return
	}
	if req.Text != "" && req.Regex != "" {
		writeAPIError(w, &apiError{
			Status:  http.StatusBadRequest,
			Message: "text and regex are mutually exclusive",
		})
		return
	}
	if req.Text == "" && req.Regex == "" {
		writeAPIError(w, &apiError{
			Status:  http.StatusBadRequest,
			Message: "text or regex is required",
		})
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	mode := "text"
	query := req.Text
	if req.Regex != "" {
		mode = "regex"
		query = req.Regex
	}

	res, err := page.Eval(`(mode, query) => {
		`+elementsHelpersJS+`
		// Corpus: innerText of the light DOM plus textContent of open shadow
	// roots and innerText of same-origin iframe bodies — innerText alone
	// excludes shadow content.
	const lines = (() => {
		const parts = [];
		(function allText(root) {
			for (const el of root.querySelectorAll('*')) {
				if (el.shadowRoot) allText(el.shadowRoot);
				else if (el.tagName === 'IFRAME') {
					try { if (el.contentDocument) allText(el.contentDocument); } catch (e) {}
				}
			}
			if (root === document) {
				if (document.body) parts.push(document.body.innerText);
			} else if (root.body) {
				parts.push(root.body.innerText);
			} else {
				parts.push(root.textContent);
			}
		})(document);
		return parts.join('\n');
	})().split('\n');
		const out = [];
		let count = 0;
		let re = null;
		if (mode === 'regex') {
			let src = query, flags = '';
			const m = query.match(/^\/(.*)\/([a-z]*)$/s);
			if (m) { src = m[1]; flags = m[2]; }
			try { re = new RegExp(src, flags); } catch (e) { return JSON.stringify({error: e.message}); }
		}
		for (let i = 0; i < lines.length; i++) {
			const line = lines[i].trim();
			if (!line) continue;
			const hit = re ? re.test(line) : line.toLowerCase().includes(query.toLowerCase());
			if (hit) {
				count++;
				if (out.length < 3) out.push({line: i + 1, text: line.length > 200 ? line.slice(0, 200) + '…' : line});
			}
		}
		return JSON.stringify({count, matches: out});
	}`, mode, query)
	if err != nil {
		http.Error(w, "find failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var probe struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(res.Value.Str()), &probe) == nil && probe.Error != "" {
		writeAPIError(w, &apiError{
			Status:  http.StatusBadRequest,
			Message: "invalid regex: " + probe.Error,
		})
		return
	}

	var out findResponse
	if err := json.Unmarshal([]byte(res.Value.Str()), &out); err != nil {
		http.Error(w, "find failed: unexpected page result", http.StatusInternalServerError)
		return
	}
	if out.Matches == nil {
		out.Matches = []findMatchLine{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// elementDetail is the /element response: the enumeration descriptor plus
// attributes and owning form.
type elementDetail struct {
	elementInfo
	Attrs map[string]string `json:"attrs,omitempty"`
	Form  *elementFormRef   `json:"form,omitempty"`
}

type elementFormRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// handleElement returns one element's full detail by ref or selector. A
// stale ref is healed by fingerprint like the interaction paths, and the
// resolved selector is returned.
func (s *Server) handleElement(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref      int    `json:"ref"`
		Selector string `json:"selector"`
	}
	if !decodeBodyRequired(w, r, &req) {
		return
	}
	if req.Ref == 0 && req.Selector == "" {
		writeAPIError(w, &apiError{
			Status:  http.StatusBadRequest,
			Message: "ref or selector is required",
		})
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	selector, aerr := s.resolveElementTarget(page, req.Ref, req.Selector)
	if aerr != nil {
		writeAPIError(w, aerr)
		return
	}

	res, err := page.Eval(elementDetailJS, selector)
	if err != nil || res == nil || res.Value.Val() == nil {
		writeAPIError(w, s.elementNotFound(page, selector))
		return
	}
	var detail elementDetail
	if err := json.Unmarshal([]byte(res.Value.Str()), &detail); err != nil {
		http.Error(w, "element failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	detail.Selector = selector
	if req.Ref > 0 {
		detail.Ref = req.Ref
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

// elementDetailJS gathers one element's descriptor. Shares the identity and
// selector helpers with the enumeration so fields match /elements output.
var elementDetailJS = `(sel) => {` + elementsHelpersJS + `
	const el = resolveOne(sel);
	if (!el) return null;
	const tag = el.tagName.toLowerCase();
	const style = window.getComputedStyle(el);
	const visible = style.display !== 'none' && style.visibility !== 'hidden' &&
		!!(el.offsetWidth || el.offsetHeight || el.getClientRects().length);
	const r = el.getBoundingClientRect();
	const id = identityOf(el, tag);
	id.selector = sel;
	id.visible = visible;
	id.disabled = !!el.disabled;
	id.rect = visible ? {
		x: Math.round(r.x), y: Math.round(r.y),
		w: Math.round(r.width), h: Math.round(r.height)
	} : null;
	if (tag === 'input') {
		if (id.type === 'checkbox' || id.type === 'radio') id.checked = !!el.checked;
		else if (id.type !== 'password') id.value = trunc(el.value || '', 40);
	}
	if (tag === 'textarea') id.value = trunc(el.value || '', 40);
	const attrs = {};
	let n = 0;
	for (const a of el.attributes) {
		if (a.name === 'id' || n >= 30) continue;
		attrs[a.name] = a.value;
		n++;
	}
	id.attrs = attrs;
	if (el.form) {
		id.form = {id: el.form.id || '', name: el.form.getAttribute('name') || ''};
	}
	return JSON.stringify(id);
}`
