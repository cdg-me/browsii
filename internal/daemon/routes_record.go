package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (s *Server) registerRecordRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/record/start", s.handleRecordStart)
	mux.HandleFunc("/record/stop", s.handleRecordStop)
	mux.HandleFunc("/record/replay", s.handleRecordReplay)
	mux.HandleFunc("/record/list", s.handleRecordList)
	mux.HandleFunc("/record/delete", s.handleRecordDelete)
	mux.HandleFunc("/record/export", s.handleRecordExport)
}

func recordingPath(name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".browsii", "recordings", name+".json")
}

// /record/start — begins recording actions.
// captureHar also records network traffic to a HAR file alongside the
// recording, written when recording stops.
func (s *Server) handleRecordStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		CaptureHar bool   `json:"captureHar"`
	}
	if !decodeBodyRequired(w, r, &req) {
		return
	}

	s.recMu.Lock()
	s.recording = true
	s.recordName = req.Name
	s.recordStart = time.Now()
	s.recordEvents = nil
	s.recordHar = req.CaptureHar
	s.recMu.Unlock()

	if req.CaptureHar {
		harPath := strings.TrimSuffix(recordingPath(req.Name), ".json") + ".har"
		if err := s.startHarCapture(harPath); err != nil {
			s.recMu.Lock()
			s.recording = false
			s.recMu.Unlock()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	recordURL := ""
	if p := s.activePage(); p != nil {
		if href, err := s.pageURL(p); err == nil {
			recordURL = href
		}
	}
	s.recMu.Lock()
	s.recordURL = recordURL
	s.recMu.Unlock()

	if req.CaptureHar {
		harPath := strings.TrimSuffix(recordingPath(req.Name), ".json") + ".har"
		if err := s.startHarCapture(harPath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

// /record/stop — stops recording and saves to disk.
func (s *Server) handleRecordStop(w http.ResponseWriter, r *http.Request) {
	s.recMu.Lock()
	s.recording = false
	name := s.recordName
	events := s.recordEvents
	captureHar := s.recordHar
	recordURL := s.recordURL
	s.recordHar = false
	s.recMu.Unlock()

	var harPath string
	if captureHar {
		harPath = strings.TrimSuffix(recordingPath(name), ".json") + ".har"
		if err := s.stopHarCapture(harPath); err != nil {
			http.Error(w, "har capture: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	recording := map[string]interface{}{
		"name":   name,
		"url":    recordURL,
		"events": events,
	}
	if harPath != "" {
		recording["har"] = harPath
	}

	recFile := recordingPath(name)
	if err := os.MkdirAll(filepath.Dir(recFile), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(recFile, data, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"name":   name,
		"events": len(events),
		"har":    harPath,
	})
}

// startHarCapture begins a network capture that includes response bodies,
// written as HAR to path on stop.
func (s *Server) startHarCapture(path string) error {
	_, err := sendLocal(func() (*http.Response, error) {
		return postLocal(fmt.Sprintf("http://127.0.0.1:%d/network/capture/start", s.port),
			`{"include":["request-headers","request-body","response-headers","response-body","response-timing","response-size","request-timestamp"],"format":"har","output":"`+path+`"}`)
	})
	return err
}

// stopHarCapture ends the capture and verifies the HAR file exists. Capture
// commands are not recorded as events.
func (s *Server) stopHarCapture(path string) error {
	s.recMu.Lock()
	s.recording = false
	s.recMu.Unlock()

	if _, err := sendLocal(func() (*http.Response, error) {
		return postLocal(fmt.Sprintf("http://127.0.0.1:%d/network/capture/stop", s.port), "")
	}); err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("no har written to %s", path)
	}
	return nil
}

// postLocal issues a POST with a short timeout against this daemon.
func postLocal(url, body string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

// sendLocal executes an HTTP call against this daemon's own API. Used by
// record flows that compose existing endpoints.
func sendLocal(do func() (*http.Response, error)) ([]byte, error) {
	resp, err := do()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(body)))
	}
	return body, nil
}

// replayReport summarizes a replay run.
type replayReport struct {
	Name        string            `json:"name"`
	Steps       int               `json:"steps"`
	Checkpoints replayCheckpoints `json:"checkpoints"`
	Healed      []replayHealNote  `json:"healed,omitempty"`
	DurationMs  int64             `json:"durationMs"`
	FailedStep  int               `json:"failedStep,omitempty"`
	Error       string            `json:"error,omitempty"`
}

type replayCheckpoints struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
}

type replayHealNote struct {
	Step int    `json:"step"`
	From string `json:"from"`
	To   string `json:"to"`
}

// /record/replay — replays a recorded session.
//
// name    recording name or absolute path
// speed   0 = instant (default), 1 = recorded timing, 2 = half timing
// live    skip the recorded HAR snapshot; requests hit the network
// session resume this saved session before replaying
func (s *Server) handleRecordReplay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string  `json:"name"`
		Speed   float64 `json:"speed"`
		Live    bool    `json:"live"`
		Session string  `json:"session"`
	}
	if !decodeBodyRequired(w, r, &req) {
		return
	}

	data, err := os.ReadFile(recordingPath(req.Name))
	if err != nil {
		http.Error(w, fmt.Sprintf("recording %q not found", req.Name), http.StatusNotFound)
		return
	}

	var recording struct {
		URL    string          `json:"url"`
		HAR    string          `json:"har"`
		Events []RecordedEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &recording); err != nil {
		http.Error(w, "invalid recording: "+err.Error(), http.StatusBadRequest)
		return
	}

	report := replayReport{Name: req.Name, Steps: len(recording.Events)}
	start := time.Now()
	defer func() {
		report.DurationMs = time.Since(start).Milliseconds()
		s.writeReplayReport(w, &report)
	}()

	if req.Session != "" {
		if err := s.resumeSession(req.Session); err != nil {
			report.Error = "session " + req.Session + ": " + err.Error()
			return
		}
	}

	snapshotLoaded := false
	if !req.Live {
		if recording.HAR != "" {
			if _, err := os.Stat(recording.HAR); err == nil {
				if err := s.loadSnapshot(recording.HAR); err != nil {
					report.Error = "har " + recording.HAR + ": " + err.Error()
					return
				}
				snapshotLoaded = true
			}
		}
	} else {
		// A snapshot left over from a previous replay would silently defeat
		// --live; clear it.
		s.clearSnapshot()
	}

	wasRecording := s.recording
	s.recording = false
	defer func() { s.recording = wasRecording }()

	for i := range recording.Events {
		ev := recording.Events[i]
		if req.Speed > 0 && i > 0 {
			delay := ev.T - recording.Events[i-1].T
			if delay > 0 {
				time.Sleep(time.Duration(float64(delay)/req.Speed) * time.Millisecond)
			}
		}

		page := s.activePage()
		if page == nil {
			report.FailedStep = i + 1
			report.Error = "no active page"
			return
		}

		if ev.Action == "expect" {
			report.Checkpoints.Total++
			ok, msg := s.replayExpect(page, ev)
			if !ok {
				report.FailedStep = i + 1
				report.Error = msg
				return
			}
			report.Checkpoints.Passed++
			continue
		}

		err := s.replayActionSafely(page, ev, &report, i+1)
		if err != nil {
			report.FailedStep = i + 1
			report.Error = err.Error()
			return
		}
	}

	if snapshotLoaded {
		s.clearSnapshot()
	}
}

// replayActionSafely runs replayAction, converting panics into errors so a
// failing step is reported instead of aborting the response.
func (s *Server) replayActionSafely(page *rod.Page, ev RecordedEvent, report *replayReport, step int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return s.replayAction(page, ev, report, step)
}

// replayFill re-applies a recorded fill event, healing each field's
// selector by its recorded fingerprint. One failing field fails the step.
func (s *Server) replayFill(page *rod.Page, ev RecordedEvent) error {
	raw, ok := ev.Params["fields"]
	if !ok {
		return fmt.Errorf("fill event has no fields")
	}
	enc, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var fields []struct {
		Ref      int              `json:"ref"`
		Selector string           `json:"selector"`
		Value    string           `json:"value"`
		FP       *elementIdentity `json:"fp"`
		FPIndex  int              `json:"fpIndex"`
	}
	if err := json.Unmarshal(enc, &fields); err != nil {
		return err
	}
	for i, f := range fields {
		selector := f.Selector
		if f.FP != nil {
			synthetic := RecordedEvent{FP: f.FP, FPIndex: f.FPIndex}
			var resolved string
			var herr error
			deadline := time.Now().Add(replayActionWait)
			for {
				resolved, herr = s.resolveRecordedTarget(page, synthetic, selector, nil, 0)
				if herr == nil {
					break
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("field %d: %s", i+1, herr.Error())
				}
				time.Sleep(100 * time.Millisecond)
			}
			selector = resolved
		}
		if selector == "" {
			return fmt.Errorf("field %d: no selector or fingerprint", i+1)
		}
		res, err := page.Eval(setNativeValueJS, selector, f.Value)
		if err != nil || res == nil || !res.Value.Bool() {
			return fmt.Errorf("field %d: value could not be set", i+1)
		}
	}
	return nil
}

// replayExpect runs a recorded expect event through the same wait loop the
// /expect endpoint uses.
func (s *Server) replayExpect(page *rod.Page, ev RecordedEvent) (bool, string) {
	req := expectRequest{
		Text:           paramString(ev.Params, "text"),
		TextGone:       paramString(ev.Params, "textGone"),
		URLPattern:     paramString(ev.Params, "urlPattern"),
		Selector:       paramString(ev.Params, "selector"),
		Hidden:         paramBool(ev.Params, "hidden"),
		Enabled:        paramBoolPtr(ev.Params, "enabled"),
		Ref:            0,
		Value:          paramString(ev.Params, "value"),
		Request:        paramString(ev.Params, "request"),
		NoConsoleError: paramBool(ev.Params, "noConsoleErrors"),
		TimeoutMs:      ev.TimeoutMs,
	}
	cond, aerr := s.expectCondition(page, &req, s.currentEventSeq())
	if aerr != nil {
		return false, aerr.Message
	}
	timeout := expectDefaultTimeout
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	start := time.Now()
	for {
		ok, _ := cond()
		if ok {
			return true, ""
		}
		if time.Since(start) >= timeout {
			return false, fmt.Sprintf("checkpoint failed: %s — timed out after %dms",
				expectDescribe(&req), timeout.Milliseconds())
		}
		time.Sleep(expectPollInterval)
	}
}

func paramString(p map[string]interface{}, key string) string {
	v, _ := p[key].(string)
	return v
}

func paramBool(p map[string]interface{}, key string) bool {
	v, _ := p[key].(bool)
	return v
}

// paramBoolPtr reads an optional tri-state boolean field.
func paramBoolPtr(p map[string]interface{}, key string) *bool {
	v, ok := p[key].(bool)
	if !ok {
		return nil
	}
	return &v
}

// replayActionWait bounds how long a replayed element action waits for its
// target to appear. Replays run at instant speed, so elements that the
// recorded session waited for via natural timing (SPA setTimeouts, lazy
// renders) may not exist yet when the action fires; without this budget,
// recordings self-destruct on their own timing.
const replayActionWait = 5 * time.Second

// replayAction executes one non-expect event. Element actions resolve their
// selector through the recorded fingerprint: when the selector no longer
// matches, the element is relocated by fingerprint and occurrence index, and
// the substitution is noted in the report.
func (s *Server) replayAction(page *rod.Page, ev RecordedEvent, report *replayReport, step int) error {
	selector := paramString(ev.Params, "selector")

	switch ev.Action {
	case "click", "hover", "type", "select", "check":
		if selector != "" {
			var resolved string
			var err error
			deadline := time.Now().Add(replayActionWait)
			for {
				resolved, err = s.resolveRecordedTarget(page, ev, selector, report, step)
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					return err
				}
				time.Sleep(100 * time.Millisecond)
			}
			selector = resolved
		}
	}

	switch ev.Action {
	case "navigate":
		page.MustNavigate(paramString(ev.Params, "url")).MustWaitLoad()
	case "click":
		el, aerr := s.findElement(page, selector)
		if aerr != nil {
			return fmt.Errorf("%s", aerr.Message)
		}
		if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
			return err
		}
	case "type":
		if _, aerr := s.findElement(page, selector); aerr != nil {
			return fmt.Errorf("%s", aerr.Message)
		}
		_, _ = page.Eval(`(sel) => {`+elementsHelpersJS+`
			const el = resolveOne(sel);
			if (el) { el.value = ''; el.focus(); }
		}`, selector)
		page.MustInsertText(paramString(ev.Params, "text"))
	case "hover":
		el, aerr := s.findElement(page, selector)
		if aerr != nil {
			return fmt.Errorf("%s", aerr.Message)
		}
		if err := el.Hover(); err != nil {
			return err
		}
	case "fill":
		return s.replayFill(page, ev)
	case "select":
		res, err := page.Eval(selectOptionJS, selector,
			ev.Params["value"], ev.Params["label"], ev.Params["index"], paramBool(ev.Params, "multiple"))
		if err != nil || res == nil || res.Value.Val() == nil {
			return fmt.Errorf("select failed: %s", errString(err))
		}
		var out struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if jsonErr := json.Unmarshal([]byte(res.Value.Str()), &out); jsonErr != nil {
			return fmt.Errorf("select failed: unexpected page result")
		}
		if !out.OK {
			return fmt.Errorf("select failed: %s", out.Error)
		}
	case "check":
		el, aerr := s.findElement(page, selector)
		if aerr != nil {
			return fmt.Errorf("%s", aerr.Message)
		}
		want := paramBool(ev.Params, "checked")
		_, state, cerr := elementCheckState(el)
		if cerr != nil {
			return fmt.Errorf("not a checkbox or radio: %s", selector)
		}
		if state != want {
			if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
				return err
			}
			_, now, _ := elementCheckState(el)
			if now != want {
				return fmt.Errorf("click did not change state: %s", selector)
			}
		}
	case "press":
		ka := page.KeyActions()
		for _, k := range parseKeyCombo(paramString(ev.Params, "key")) {
			ka = ka.Press(k)
		}
		ka.MustDo()
	case "reload":
		page.MustReload().MustWaitLoad()
	case "back":
		page.MustNavigateBack().MustWaitLoad()
	case "forward":
		page.MustNavigateForward().MustWaitLoad()
	case "scroll":
		dir := paramString(ev.Params, "direction")
		pixels := 300
		if p, ok := ev.Params["pixels"].(float64); ok && p > 0 {
			pixels = int(p)
		}
		switch dir {
		case "down":
			page.MustEval(fmt.Sprintf("() => window.scrollBy(0, %d)", pixels))
		case "up":
			page.MustEval(fmt.Sprintf("() => window.scrollBy(0, -%d)", pixels))
		case "top":
			page.MustEval("() => window.scrollTo(0, 0)")
		case "bottom":
			page.MustEval("() => window.scrollTo(0, document.body.scrollHeight)")
		}
	case "js":
		page.MustEval(wrapScript(paramString(ev.Params, "script")))
	case "tab_new":
		s.activePg = s.browser.MustPage(paramString(ev.Params, "url")).MustWaitLoad()
		s.trackPage(s.activePg)
	case "tab_close":
		s.untrackPage(page)
		page.MustClose()
		s.activePg = nil
	case "tab_switch":
		if idxF, ok := ev.Params["index"].(float64); ok {
			pages := s.orderedPages()
			idx := int(idxF)
			if idx >= 0 && idx < len(pages) {
				s.activePg = pages[idx]
				s.activePg.MustActivate()
			}
		}
	case "mouse_move":
		if x, ok := ev.Params["x"].(float64); ok {
			if y, ok := ev.Params["y"].(float64); ok {
				page.Mouse.MustMoveTo(x, y)
			}
		}
	case "mouse_rightclick":
		el, elErr := elementForSelector(page, selector, replayActionWait)
		if elErr != nil || el == nil {
			return fmt.Errorf("element not found: %s", selector)
		}
		el.MustScrollIntoView()
		box := el.MustShape().Box()
		page.Mouse.MustMoveTo(box.X+box.Width/2, box.Y+box.Height/2)
		page.Mouse.MustClick("right")
	case "mouse_doubleclick":
		page.MustEval(`(sel) => {`+elementsHelpersJS+`
			resolveOne(sel).dispatchEvent(new MouseEvent('dblclick', {bubbles: true, cancelable: true}));
		}`, selector)
	case "upload":
		var files []string
		if list, ok := ev.Params["files"].([]interface{}); ok {
			for _, f := range list {
				if fs, ok := f.(string); ok {
					files = append(files, fs)
				}
			}
		}
		upEl, upErr := elementForSelector(page, selector, replayActionWait)
		if upErr != nil || upEl == nil {
			return fmt.Errorf("element not found: %s", selector)
		}
		upEl.MustSetFiles(files...)
	case "screenshot":
		filename := paramString(ev.Params, "filename")
		el := paramString(ev.Params, "element")
		if el != "" {
			shotEl, shotErr := elementForSelector(page, el, replayActionWait)
			if shotErr != nil || shotEl == nil {
				return fmt.Errorf("element not found: %s", el)
			}
			data, err := shotEl.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
			if err == nil {
				_ = os.WriteFile(filename, data, 0644)
			}
		} else if paramBool(ev.Params, "fullPage") {
			page.MustScreenshotFullPage(filename)
		} else {
			page.MustScreenshot(filename)
		}
	case "pdf":
		if filename := paramString(ev.Params, "filename"); filename != "" {
			pdfData, _ := page.PDF(&proto.PagePrintToPDF{})
			data, _ := io.ReadAll(pdfData)
			_ = os.WriteFile(filename, data, 0644)
		}
	case "network_throttle":
		lat, _ := ev.Params["latency"].(float64)
		dl, _ := ev.Params["download"].(float64)
		up, _ := ev.Params["upload"].(float64)
		_ = proto.NetworkEmulateNetworkConditions{Offline: false, Latency: lat, DownloadThroughput: dl, UploadThroughput: up}.Call(page)
	case "network_mock":
		pat := paramString(ev.Params, "pattern")
		router := page.HijackRequests()
		router.MustAdd(pat, func(ctx *rod.Hijack) {
			ctx.Response.SetBody(paramString(ev.Params, "body"))
			if ct := paramString(ev.Params, "contentType"); ct != "" {
				ctx.Response.SetHeader("Content-Type", ct)
			}
			sc, _ := ev.Params["statusCode"].(float64)
			if sc == 0 {
				sc = 200
			}
			ctx.Response.Payload().ResponseCode = int(sc)
		})
		go router.Run()
	default:
		return fmt.Errorf("unsupported action %q", ev.Action)
	}
	return nil
}

// resolveRecordedTarget verifies the recorded selector against the recorded
// fingerprint. On mismatch the element is relocated by fingerprint plus
// occurrence index; the substitution is recorded in the report. Relocation
// failure returns an error naming the original element.
func (s *Server) resolveRecordedTarget(page *rod.Page, ev RecordedEvent, selector string, report *replayReport, step int) (string, error) {
	if ev.FP == nil {
		return selector, nil
	}
	want := fingerprintParts(ev.FP.Tag, ev.FP.Role, ev.FP.Text, ev.FP.Name, ev.FP.Href, ev.FP.Type)
	if live, err := liveFingerprint(page, selector); err == nil && live == want {
		return selector, nil
	}
	if to, ok := s.findByFingerprint(page, want, ev.FPIndex); ok {
		report.Healed = append(report.Healed, replayHealNote{Step: step, From: selector, To: to})
		return to, nil
	}
	label := ev.FP.Text
	if label == "" {
		label = ev.FP.Name
	}
	return "", fmt.Errorf("element no longer matches: was %s %q → %s", ev.FP.Role, label, selector)
}

// writeReplayReport writes the replay outcome. Failures use 417 to match
// expect semantics; success is 200 with the full report.
func (s *Server) writeReplayReport(w http.ResponseWriter, report *replayReport) {
	status := http.StatusOK
	if report.Error != "" {
		status = http.StatusExpectationFailed
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(report)
}

// /record/list — returns available recordings.
func (s *Server) handleRecordList(w http.ResponseWriter, r *http.Request) {
	homeDir, _ := os.UserHomeDir()
	recDir := filepath.Join(homeDir, ".browsii", "recordings")

	entries, err := os.ReadDir(recDir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]") //nolint:errcheck
		return
	}

	var recordings []map[string]string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		info, _ := e.Info()
		recordings = append(recordings, map[string]string{
			"name":     name,
			"modified": info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	if recordings == nil {
		recordings = []map[string]string{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recordings)
}

// /record/delete — removes a recording and its HAR if present.
func (s *Server) handleRecordDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBodyRequired(w, r, &req) {
		return
	}

	recFile := recordingPath(req.Name)
	if err := os.Remove(recFile); err != nil {
		http.Error(w, fmt.Sprintf("recording %q not found", req.Name), http.StatusNotFound)
		return
	}
	harPath := strings.TrimSuffix(recFile, ".json") + ".har"
	if _, err := os.Stat(harPath); err == nil {
		_ = os.Remove(harPath)
	}

	w.WriteHeader(http.StatusOK)
}

// /record/export — writes a Playwright TypeScript spec for the recording.
func (s *Server) handleRecordExport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Out  string `json:"out"`
	}
	if !decodeBodyRequired(w, r, &req) {
		return
	}

	data, err := os.ReadFile(recordingPath(req.Name))
	if err != nil {
		http.Error(w, fmt.Sprintf("recording %q not found", req.Name), http.StatusNotFound)
		return
	}

	var recording struct {
		URL    string          `json:"url"`
		HAR    string          `json:"har"`
		Events []RecordedEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &recording); err != nil {
		http.Error(w, "invalid recording: "+err.Error(), http.StatusBadRequest)
		return
	}

	spec := buildPlaywrightSpec(recording.URL, recording.HAR, recording.Events)

	out := req.Out
	if out == "" {
		out = strings.TrimSuffix(recordingPath(req.Name), ".json") + ".spec.ts"
	}
	if err := os.WriteFile(out, []byte(spec), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"path": out})
}

// resumeSession restores a saved session by name, without the HTTP layer.
func (s *Server) resumeSession(name string) error {
	homeDir, _ := os.UserHomeDir()
	sessFile := filepath.Join(homeDir, ".browsii", "sessions", name+".json")
	data, err := os.ReadFile(sessFile)
	if err != nil {
		return fmt.Errorf("not found")
	}

	var session struct {
		ActiveTab int `json:"activeTab"`
		Tabs      []struct {
			URL     string `json:"url"`
			ScrollX int    `json:"scrollX"`
			ScrollY int    `json:"scrollY"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return err
	}

	oldPages, _ := s.browser.Pages()
	s.pageOrder = nil
	s.listenedPages = make(map[proto.TargetTargetID]struct{})
	s.consoleListenedPages = make(map[proto.TargetTargetID]struct{})
	for i, tab := range session.Tabs {
		page := s.browser.MustPage(tab.URL).MustWaitLoad()
		s.trackPage(page)
		if tab.ScrollX != 0 || tab.ScrollY != 0 {
			page.MustEval(fmt.Sprintf("() => window.scrollTo(%d, %d)", tab.ScrollX, tab.ScrollY))
		}
		if i == session.ActiveTab {
			s.activePg = page
			page.MustActivate()
		}
	}
	for _, p := range oldPages {
		p.MustClose()
	}
	return nil
}

// clearSnapshot stops any active snapshot router without touching the HTTP
// layer.
func (s *Server) clearSnapshot() {
	s.mu.Lock()
	prev := s.snapshotRouter
	s.snapshotRouter = nil
	s.mu.Unlock()
	if prev != nil {
		prev.Stop() //nolint:errcheck
	}
}

// loadSnapshot installs the HAR snapshot router for offline replay.
func (s *Server) loadSnapshot(harPath string) error {
	data, err := os.ReadFile(harPath)
	if err != nil {
		return err
	}
	var har harSnapshot
	if err := json.Unmarshal(data, &har); err != nil {
		return err
	}
	urlMap := make(map[string]snapshotEntry, len(har.Log.Entries))
	for _, e := range har.Log.Entries {
		if e.Request.URL == "" {
			continue
		}
		var body []byte
		if e.Response.Content.Encoding == "base64" {
			body, _ = base64.StdEncoding.DecodeString(e.Response.Content.Text)
		} else {
			body = []byte(e.Response.Content.Text)
		}
		status := e.Response.Status
		if status == 0 {
			status = 200
		}
		urlMap[e.Request.URL] = snapshotEntry{
			status:      status,
			contentType: e.Response.Content.MimeType,
			body:        body,
		}
	}
	s.replaceSnapshotRouter(urlMap)
	return nil
}
