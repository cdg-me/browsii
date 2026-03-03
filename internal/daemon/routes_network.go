package daemon

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (s *Server) registerNetworkRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/network/capture/start", s.handleNetworkCaptureStart)
	mux.HandleFunc("/network/capture/stop", s.handleNetworkCaptureStop)
	mux.HandleFunc("/network/throttle", s.handleNetworkThrottle)
	mux.HandleFunc("/network/mock", s.handleNetworkMock)
}

// /network/capture/start endpoint — begins capturing network requests.
// Optional JSON body: {"tab": "<alias>"} where alias is one of:
//
//	"all" or "" — capture all tabs (default)
//	"active"    — only the tab currently active at start time
//	"next"      — only the next tab opened after this call
//	"last"      — only the most recently opened tab at start time
//	"<N>"       — only the tab at integer index N
func (s *Server) handleNetworkCaptureStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tab    string `json:"tab"`
		Output string `json:"output"`
	}
	// Ignore decode errors — body is optional; default is all tabs, no output file.
	json.NewDecoder(r.Body).Decode(&req)

	// Resolve the tab alias to an integer index before acquiring the lock.
	var activeID proto.TargetTargetID
	if ap := s.activePage(); ap != nil {
		activeID = ap.TargetID
	}
	tabFilter := resolveTabAlias(req.Tab, s.pageOrder, activeID)

	s.mu.Lock()
	s.capturing = true
	s.capturedReqs = nil
	s.captureTabFilter = tabFilter
	s.captureOutputPath = req.Output
	s.mu.Unlock()

	// Events are captured by the hub registered in attachNetworkListener/trackPage —
	// no additional EachEvent goroutine needed here.
	s.recordAction("network_capture_start", map[string]interface{}{"tab": req.Tab, "output": req.Output})
	w.WriteHeader(http.StatusOK)
}

// /network/capture/stop endpoint — stops capture, writes to file if configured, returns results.
func (s *Server) handleNetworkCaptureStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.capturing = false
	reqs := s.capturedReqs
	s.capturedReqs = nil
	outputPath := s.captureOutputPath
	s.captureOutputPath = ""
	s.mu.Unlock()

	if reqs == nil {
		reqs = []map[string]interface{}{}
	}

	s.recordAction("network_capture_stop", nil)
	w.Header().Set("Content-Type", "application/json")

	if outputPath != "" {
		data, err := json.Marshal(reqs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(outputPath, data, 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"path":  outputPath,
			"count": len(reqs),
		})
		return
	}

	json.NewEncoder(w).Encode(reqs)
}

// /network/throttle endpoint — sets network emulation conditions
func (s *Server) handleNetworkThrottle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Latency  int `json:"latency"`  // ms
		Download int `json:"download"` // bytes/sec
		Upload   int `json:"upload"`   // bytes/sec
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	err := proto.NetworkEmulateNetworkConditions{
		Offline:            false,
		Latency:            float64(req.Latency),
		DownloadThroughput: float64(req.Download),
		UploadThroughput:   float64(req.Upload),
	}.Call(page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// /network/mock endpoint — intercepts matching requests and returns a custom response
func (s *Server) handleNetworkMock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Pattern     string `json:"pattern"`
		Body        string `json:"body"`
		ContentType string `json:"contentType"`
		StatusCode  int    `json:"statusCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.StatusCode == 0 {
		req.StatusCode = 200
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	router := page.HijackRequests()
	router.MustAdd(req.Pattern, func(ctx *rod.Hijack) {
		ctx.Response.SetBody(req.Body)
		if req.ContentType != "" {
			ctx.Response.SetHeader("Content-Type", req.ContentType)
		}
		ctx.Response.Payload().ResponseCode = req.StatusCode
	})
	go router.Run()

	s.recordAction("network_mock", map[string]interface{}{"pattern": req.Pattern, "body": req.Body, "contentType": req.ContentType, "statusCode": req.StatusCode})
	w.WriteHeader(http.StatusOK)
}
