package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (s *Server) registerFormRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/fill", s.handleFill)
	mux.HandleFunc("/select", s.handleSelect)
	mux.HandleFunc("/check", s.handleCheck)
}

// fillField is one field of a fill batch. Exactly one of Ref or Selector
// identifies the element; Value is the new content (empty clears).
// Submit marks the field whose form's submit button is clicked after the
// batch applies.
type fillField struct {
	Ref      int    `json:"ref"`
	Selector string `json:"selector"`
	Value    string `json:"value"`
	Submit   bool   `json:"submit"`
}

// fillFailure reports one field that could not be set.
type fillFailure struct {
	Field      string             `json:"field"`
	Error      string             `json:"error"`
	Hint       string             `json:"hint,omitempty"`
	Candidates []elementCandidate `json:"candidates,omitempty"`
}

// setNativeValueJS focuses the element, sets the value through the
// property's native setter, and fires input and change events — the
// sequence framework controlled inputs (React et al.) treat as user input.
const setNativeValueJS = `(sel, value) => {
	const el = document.querySelector(sel);
	if (!el) return false;
	if (el.tagName === 'INPUT') {
		const t = (el.getAttribute('type') || 'text').toLowerCase();
		if (t === 'checkbox' || t === 'radio' || t === 'file' || t === 'button' || t === 'submit' || t === 'reset' || t === 'image') {
			return false;
		}
	}
	const proto = el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype
		: el.tagName === 'INPUT' ? HTMLInputElement.prototype
		: null;
	if (proto) {
		const desc = Object.getOwnPropertyDescriptor(proto, 'value');
		if (desc && desc.set) desc.set.call(el, value); else el.value = value;
	} else if (el.isContentEditable) {
		el.textContent = value;
	} else {
		return false;
	}
	el.dispatchEvent(new Event('input', { bubbles: true }));
	el.dispatchEvent(new Event('change', { bubbles: true }));
	el.focus();
	return true;
}`

// formSubmitJS clicks the submit control of the form containing sel.
const formSubmitJS = `(sel) => {
	const el = document.querySelector(sel);
	if (!el || !el.form) return false;
	const f = el.form;
	const btn = f.querySelector('input[type=submit], button[type=submit], button:not([type])');
	if (btn) { btn.click(); return true; }
	if (typeof f.requestSubmit === 'function') { f.requestSubmit(); return true; }
	return false;
}`

// handleFill sets a batch of form fields in one call. Fields apply in array
// order; failures are collected per field without aborting the batch. A
// field marked submit (or the batch-level submit flag) clicks its form's
// submit button after all fields are set, skipped when any field failed.
func (s *Server) handleFill(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Fields     []fillField `json:"fields"`
		Submit     bool        `json:"submit"`
		NoEvidence bool        `json:"noEvidence"`
	}
	if !decodeBodyRequired(w, r, &req) {
		return
	}
	if len(req.Fields) == 0 {
		writeAPIError(w, &apiError{
			Status:  http.StatusBadRequest,
			Message: "fields array required",
			Hint:    `pass {"fields":[{"ref":2,"value":"..."}]} or [{"selector":"#x","value":"..."}]`,
		})
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	var failures []fillFailure
	filled := 0
	fieldSelectors := make([]string, len(req.Fields))
	for i, f := range req.Fields {
		if (f.Ref == 0) == (f.Selector == "") {
			failures = append(failures, fillFailure{
				Field: fmt.Sprintf("field %d", i+1),
				Error: "exactly one of ref or selector is required",
			})
			continue
		}
		selector, aerr := s.resolveElementTarget(page, f.Ref, f.Selector)
		if aerr != nil {
			failures = append(failures, fillFailure{
				Field:      f.Selector,
				Error:      aerr.Message,
				Hint:       aerr.Hint,
				Candidates: aerr.Candidates,
			})
			continue
		}
		fieldSelectors[i] = selector
		if _, aerr := s.findElement(page, selector); aerr != nil {
			failures = append(failures, fillFailure{
				Field:      selector,
				Error:      aerr.Message,
				Hint:       aerr.Hint,
				Candidates: aerr.Candidates,
			})
			continue
		}
		res, err := page.Eval(setNativeValueJS, selector, f.Value)
		if err != nil || res == nil || !res.Value.Bool() {
			failures = append(failures, fillFailure{
				Field: selector,
				Error: "value could not be set (checkboxes/radios need check, buttons need click)",
			})
			continue
		}
		filled++
	}

	urlBefore, sinceSeq := s.actionAnchors(page)

	if len(failures) == 0 && (req.Submit || anySubmit(req.Fields)) {
		for i, f := range req.Fields {
			if f.Submit || (req.Submit && fieldSelectors[i] != "") {
				_, _ = page.Eval(formSubmitJS, fieldSelectors[i])
				break
			}
		}
	}

	s.recordFill(req.Fields)

	s.writeFormReceipt(w, page, urlBefore, sinceSeq, map[string]interface{}{
		"filled":   filled,
		"failures": failures,
	}, req.NoEvidence)
}

func anySubmit(fields []fillField) bool {
	for _, f := range fields {
		if f.Submit {
			return true
		}
	}
	return false
}

// recordFill appends a fill event with per-field fingerprints, so replay
// can heal each field independently.
func (s *Server) recordFill(fields []fillField) {
	if !s.recording {
		return
	}
	type recField struct {
		Ref      int              `json:"ref,omitempty"`
		Selector string           `json:"selector,omitempty"`
		Value    string           `json:"value"`
		FP       *elementIdentity `json:"fp,omitempty"`
		FPIndex  int              `json:"fpIndex,omitempty"`
	}
	out := make([]recField, 0, len(fields))
	for _, f := range fields {
		rf := recField{Ref: f.Ref, Selector: f.Selector, Value: f.Value}
		if f.Selector != "" {
			if page := s.activePage(); page != nil {
				if id, idx, err := liveFingerprintEx(page, f.Selector); err == nil && id != nil {
					fp := *id
					rf.FP = &fp
					rf.FPIndex = idx
				}
			}
		}
		out = append(out, rf)
	}
	s.recordAction("fill", map[string]interface{}{"fields": out})
}

// writeFormReceipt writes a form-action response with the evidence receipt
// merged in.
func (s *Server) writeFormReceipt(w http.ResponseWriter, page *rod.Page, urlBefore string, sinceSeq int64, body map[string]interface{}, noEvidence bool) {
	if noEvidence {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(body)
		return
	}
	ev := s.collectEvidence(page, urlBefore, sinceSeq)
	for k, v := range body {
		ev[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ev)
}

// selectRequest is the /select payload. One spec among Value, Label, Index.
type selectRequest struct {
	Ref        int         `json:"ref"`
	Selector   string      `json:"selector"`
	Value      interface{} `json:"value"`
	Label      interface{} `json:"label"`
	Index      interface{} `json:"index"`
	Multiple   bool        `json:"multiple"`
	NoEvidence bool        `json:"noEvidence"`
}

// handleSelect picks option(s) in a select element. Matching precedence per
// option spec: value, then label, then index. Fires input and change.
func (s *Server) handleSelect(w http.ResponseWriter, r *http.Request) {
	var req selectRequest
	if !decodeBodyRequired(w, r, &req) {
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

	res, err := page.Eval(selectOptionJS, selector, req.Value, req.Label, req.Index, req.Multiple)
	if err != nil || res == nil || res.Value.Val() == nil {
		http.Error(w, "select failed: "+errString(err), http.StatusInternalServerError)
		return
	}
	var out struct {
		OK       bool     `json:"ok"`
		Selected []string `json:"selected"`
		Error    string   `json:"error"`
		Hint     string   `json:"hint"`
	}
	if jsonErr := json.Unmarshal([]byte(res.Value.Str()), &out); jsonErr != nil {
		http.Error(w, "select failed: unexpected page result", http.StatusInternalServerError)
		return
	}
	if !out.OK {
		writeAPIError(w, &apiError{Status: http.StatusUnprocessableEntity, Message: out.Error, Hint: out.Hint})
		return
	}

	urlBefore, sinceSeq := s.actionAnchors(page)
	s.recordInteraction("select", map[string]interface{}{"selector": selector, "value": firstSpec(req)}, page, selector)
	s.writeFormReceipt(w, page, urlBefore, sinceSeq, map[string]interface{}{"selected": out.Selected}, req.NoEvidence)
}

func firstSpec(req selectRequest) interface{} {
	if req.Value != nil {
		return req.Value
	}
	if req.Label != nil {
		return req.Label
	}
	return req.Index
}

// selectOptionJS resolves the option spec against the select's options and
// applies the selection. Value matches exact option value, label matches
// exact option text, index is zero-based. Multiple allows several.
const selectOptionJS = `(sel, value, label, index, multiple) => {
	const el = document.querySelector(sel);
	if (!el) return JSON.stringify({ok: false, error: 'element not found: ' + sel});
	if (el.tagName !== 'SELECT') {
		return JSON.stringify({ok: false, error: 'not a select element: ' + sel,
			hint: 'select targets <select> elements; for buttons use click'});
	}
	if (!multiple && !Array.isArray(value) && !Array.isArray(label) && !Array.isArray(index)) {
		el.selectedIndex = -1;
	} else {
		// A list spec replaces the selection (Playwright selectOption
		// semantics), so clear before applying.
		el.selectedIndex = -1;
	}
	const opts = [...el.options];
	const asList = (v) => Array.isArray(v) ? v.map(String) : [String(v)];
	const want = (o) => {
		if (value !== null && value !== undefined) {
			return asList(value).includes(o.value) || asList(value).includes(o.label);
		}
		if (label !== null && label !== undefined) {
			return asList(label).includes(o.label) || asList(label).includes(o.value);
		}
		if (index !== null && index !== undefined) {
			const idxs = Array.isArray(index) ? index : [index];
			return idxs.includes(o.index);
		}
		return false;
	};
	const matched = opts.filter(want);
	const anySpec = (value !== null && value !== undefined) || (label !== null && label !== undefined) || (index !== null && index !== undefined);
	if (anySpec && matched.length === 0) {
		const list = opts.slice(0, 20).map(o => '"' + o.label + '" (value ' + o.value + ')').join(', ');
		return JSON.stringify({ok: false,
			error: 'no matching option in ' + sel,
			hint: 'options: ' + list});
	}
	for (const o of matched) o.selected = true;
	el.dispatchEvent(new Event('input', { bubbles: true }));
	el.dispatchEvent(new Event('change', { bubbles: true }));
	return JSON.stringify({ok: true, selected: [...el.selectedOptions].map(o => o.value)});
}`

// checkRequest is the /check payload.
type checkRequest struct {
	Ref        int    `json:"ref"`
	Selector   string `json:"selector"`
	Checked    bool   `json:"checked"`
	NoEvidence bool   `json:"noEvidence"`
}

// handleCheck checks or unchecks a checkbox or radio with a real click,
// skipping the click when already in the desired state, and verifying the
// state actually flipped.
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	var req checkRequest
	if !decodeBodyRequired(w, r, &req) {
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
	el, aerr := s.findElement(page, selector)
	if aerr != nil {
		writeAPIError(w, aerr)
		return
	}

	kind, state, cerr := elementCheckState(el)
	if cerr != nil {
		writeAPIError(w, &apiError{
			Status:  http.StatusUnprocessableEntity,
			Message: "not a checkbox or radio: " + selector,
			Hint:    "check targets checkboxes and radios; for text inputs use fill",
		})
		return
	}

	was := state
	if state != req.Checked {
		urlBefore, sinceSeq := s.actionAnchors(page)
		if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
			http.Error(w, "check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_, now, _ := elementCheckState(el)
		if now != req.Checked {
			writeAPIError(w, &apiError{
				Status:  http.StatusInternalServerError,
				Message: "click did not change state: " + selector,
				Hint:    "page handler may preventDefault the click",
			})
			return
		}
		s.recordInteraction("check", map[string]interface{}{"selector": selector, "checked": req.Checked, "kind": kind}, page, selector)
		s.writeFormReceipt(w, page, urlBefore, sinceSeq, map[string]interface{}{"was": was, "now": now}, req.NoEvidence)
		return
	}

	s.recordInteraction("check", map[string]interface{}{"selector": selector, "checked": req.Checked, "kind": kind}, page, selector)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"was": was, "now": state})
}

// elementCheckState returns the input kind ("checkbox"/"radio") and current
// checked state of el.
func elementCheckState(el *rod.Element) (string, bool, error) {
	res, err := el.Eval(`() => {
		if (this.tagName !== 'INPUT') return null;
		const t = (this.getAttribute('type') || '').toLowerCase();
		if (t !== 'checkbox' && t !== 'radio') return null;
		return JSON.stringify({kind: t, checked: this.checked});
	}`)
	if err != nil || res == nil || res.Value.Val() == nil {
		return "", false, fmt.Errorf("not a checkable element")
	}
	var out struct {
		Kind    string `json:"kind"`
		Checked bool   `json:"checked"`
	}
	if err := json.Unmarshal([]byte(res.Value.Str()), &out); err != nil {
		return "", false, err
	}
	return out.Kind, out.Checked, nil
}

func errString(err error) string {
	if err == nil {
		return "unexpected page result"
	}
	return err.Error()
}
