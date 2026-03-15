package daemon

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// snapshotEntry holds the response fields needed to replay a recorded request.
type snapshotEntry struct {
	status      int
	contentType string
	body        []byte
}

// harSnapshot is the subset of the HAR 1.2 format used for snapshot loading.
type harSnapshot struct {
	Log struct {
		Entries []struct {
			Request struct {
				URL string `json:"url"`
			} `json:"request"`
			Response struct {
				Status  int `json:"status"`
				Content struct {
					MimeType string `json:"mimeType"`
					Text     string `json:"text"`
					Encoding string `json:"encoding"` // "base64" for binary bodies
				} `json:"content"`
			} `json:"response"`
		} `json:"entries"`
	} `json:"log"`
}

func (s *Server) registerSnapshotRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/snapshot/load", s.handleSnapshotLoad)
	mux.HandleFunc("/snapshot/clear", s.handleSnapshotClear)
}

// /snapshot/load — reads a HAR file and intercepts all recorded URLs on the
// active page. Subsequent navigations on that page return the recorded
// responses without hitting the network. Calling again replaces the snapshot.
//
// Request body: {"path": "/abs/or/relative/path/to/file.har"}
// Response:     {"loaded": N}  where N is the number of distinct URLs loaded.
func (s *Server) handleSnapshotLoad(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeBodyRequired(w, r, &req) {
		return
	}

	data, err := os.ReadFile(req.Path)
	if err != nil {
		http.Error(w, "cannot read file: "+err.Error(), http.StatusBadRequest)
		return
	}

	var har harSnapshot
	if err := json.Unmarshal(data, &har); err != nil {
		http.Error(w, "invalid HAR: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Build URL → entry map. Later entries for the same URL win (last-write wins),
	// matching how a real browser cache would treat duplicate responses.
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"loaded": len(urlMap)}) //nolint:errcheck
}

// /snapshot/clear — stops the active snapshot router, restoring normal network
// behaviour. Safe to call even when no snapshot is loaded.
func (s *Server) handleSnapshotClear(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	prev := s.snapshotRouter
	s.snapshotRouter = nil
	s.mu.Unlock()

	if prev != nil {
		if err := prev.Stop(); err != nil {
			http.Error(w, "stop router: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// replaceSnapshotRouter stops the previous router (if any), creates a new
// catch-all hijack router at the browser level, and starts it in the background.
// Browser-level interception covers every tab and every navigation — not just
// the page that was active at load time — which is essential for repeatable
// multi-tab LLM test flows.
//
// Requests whose URL is in urlMap are fulfilled from the map.
// All other requests are passed through to the network unchanged.
func (s *Server) replaceSnapshotRouter(urlMap map[string]snapshotEntry) {
	s.mu.Lock()
	prev := s.snapshotRouter
	s.snapshotRouter = nil
	s.mu.Unlock()

	if prev != nil {
		prev.Stop() //nolint:errcheck
	}

	router := s.browser.HijackRequests()
	router.MustAdd("*", func(ctx *rod.Hijack) {
		entry, ok := urlMap[ctx.Request.URL().String()]
		if !ok {
			ctx.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}
		ctx.Response.SetBody(entry.body)
		if entry.contentType != "" {
			ctx.Response.SetHeader("Content-Type", entry.contentType)
		}
		ctx.Response.Payload().ResponseCode = entry.status
	})
	go router.Run()

	s.mu.Lock()
	s.snapshotRouter = router
	s.mu.Unlock()
}
