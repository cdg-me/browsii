package daemon

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// dialogLogCap bounds the retained history of auto-handled dialogs.
const dialogLogCap = 50

// dialogEntry records a JavaScript dialog that the daemon auto-handled.
type dialogEntry struct {
	Type     string  `json:"type"` // alert, confirm, prompt, beforeunload
	Message  string  `json:"message"`
	Accepted bool    `json:"accepted"`
	Tab      int     `json:"tab"`
	TS       float64 `json:"ts"`

	targetID proto.TargetTargetID `json:"-"`
}

// attachDialogListener auto-handles JavaScript dialogs (alert/confirm/prompt/
// beforeunload) on a page according to the current policy.
//
// This is required for correctness, not convenience: with the Page CDP domain
// engaged (which go-rod always does), an unhandled dialog stalls all page
// execution — every later eval, click, or navigation would hang until the
// 30s client timeout. Idempotent: one listener per page, ever.
func (s *Server) attachDialogListener(page *rod.Page) {
	if _, already := s.dialogListenedPages[page.TargetID]; already {
		return
	}
	s.dialogListenedPages[page.TargetID] = struct{}{}

	wait := page.EachEvent(func(e *proto.PageJavascriptDialogOpening) {
		s.mu.Lock()
		accept := s.dialogPolicy == "accept"
		prompt := s.dialogPromptText

		tabIdx := -1
		for i, id := range s.pageOrder {
			if id == page.TargetID {
				tabIdx = i
				break
			}
		}

		entry := dialogEntry{
			Type:     string(e.Type),
			Message:  e.Message,
			Accepted: accept,
			Tab:      tabIdx,
			TS:       float64(time.Now().UnixNano()) / 1e9,
			targetID: page.TargetID,
		}
		s.dialogLog = append(s.dialogLog, entry)
		if len(s.dialogLog) > dialogLogCap {
			s.dialogLog = s.dialogLog[len(s.dialogLog)-dialogLogCap:]
		}
		s.mu.Unlock()

		if err := (proto.PageHandleJavaScriptDialog{
			Accept:     accept,
			PromptText: prompt,
		}).Call(page); err != nil {
			log.Printf("Dialog handling failed on %s: %v", page.TargetID, err)
		}

		s.broadcastEvent(StreamEvent{
			Type: EventDialog,
			Payload: map[string]interface{}{
				"type":     entry.Type,
				"message":  entry.Message,
				"accepted": entry.Accepted,
				"tab":      entry.Tab,
			},
		})
	})

	go wait()
}

// peekDialogs returns a copy of the retained dialog history.
func (s *Server) peekDialogs() []dialogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]dialogEntry, len(s.dialogLog))
	copy(out, s.dialogLog)
	return out
}

// clearDialogs empties the retained dialog history.
func (s *Server) clearDialogs() {
	s.mu.Lock()
	s.dialogLog = nil
	s.mu.Unlock()
}

// drainDialogsFor removes and returns the entries recorded for one page,
// leaving other pages' entries in the log.
func (s *Server) drainDialogsFor(targetID proto.TargetTargetID) []dialogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var drained, kept []dialogEntry
	for _, e := range s.dialogLog {
		if e.targetID == targetID {
			drained = append(drained, e)
		} else {
			kept = append(kept, e)
		}
	}
	s.dialogLog = kept
	return drained
}

// dialogReportWait gives dialogs triggered by an action (e.g. a click whose
// handler calls confirm()) time to arrive over CDP before the response is
// finalised. Kept small: it is only paid by dialog-producing actions.
const dialogReportWait = 150 * time.Millisecond

// maybeReportDialogs waits briefly for dialogs opened by the action that just
// ran and, if any were auto-handled for this page, writes them into the
// response body as {"dialogs":[...]}. Returns false (writing nothing) when no
// dialogs occurred, leaving the caller to write its own empty 200.
func (s *Server) maybeReportDialogs(w http.ResponseWriter, page *rod.Page) bool {
	time.Sleep(dialogReportWait)
	drained := s.drainDialogsFor(page.TargetID)
	if len(drained) == 0 {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"dialogs": drained})
	return true
}

// evidenceSettle is the post-action window during which triggered work
// (navigations, fetches, console errors) is observed before the receipt is
// written. Same order of magnitude as playwright-mcp's settle default.
const evidenceSettle = 400 * time.Millisecond

// actionEvidence is the receipt attached to click/press/navigate responses:
// what the action actually caused, so agents can verify instead of trusting.
type actionEvidence struct {
	Navigated      bool          `json:"navigated,omitempty"`
	URL            string        `json:"url,omitempty"`
	Dialogs        []dialogEntry `json:"dialogs,omitempty"`
	Requests       int           `json:"requests"`
	RequestSamples []string      `json:"requestSamples,omitempty"`
	ConsoleErrors  int           `json:"consoleErrors"`
}

// writeEvidence collects and writes the post-action receipt. urlBefore is the
// page URL before the action ("" when unknown — navigation detection is
// skipped). sinceSeq anchors ring queries to the moment before the action.
// When skip is true the settle window is omitted and a bare 200 is written.
func (s *Server) writeEvidence(w http.ResponseWriter, page *rod.Page, urlBefore string, sinceSeq int64, skip bool) {
	if skip {
		w.WriteHeader(http.StatusOK)
		return
	}
	time.Sleep(evidenceSettle)

	ev := &actionEvidence{}
	if href, err := s.pageURL(page); err == nil {
		ev.URL = href
		ev.Navigated = urlBefore != "" && href != urlBefore
	}
	ev.Dialogs = s.drainDialogsFor(page.TargetID)

	reqs := s.netRequestsSince(sinceSeq)
	ev.Requests = len(reqs)
	last := len(reqs) - 5
	if last < 0 {
		last = 0
	}
	for _, e := range reqs[last:] {
		ev.RequestSamples = append(ev.RequestSamples, formatNetEntry(e))
	}
	ev.ConsoleErrors = len(s.consoleErrorsSince(sinceSeq))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ev)
}

// pageURL returns the page's current URL via a single eval.
func (s *Server) pageURL(page *rod.Page) (string, error) {
	res, err := page.Eval(`() => location.href`)
	if err != nil {
		return "", err
	}
	return res.Value.Str(), nil
}
