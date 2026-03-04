package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (s *Server) registerNetworkRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/network/capture/start", s.handleNetworkCaptureStart)
	mux.HandleFunc("/network/capture/stop", s.handleNetworkCaptureStop)
	mux.HandleFunc("/network/throttle", s.handleNetworkThrottle)
	mux.HandleFunc("/network/mock", s.handleNetworkMock)
}

// /network/capture/start — begins buffering network requests.
//
// Optional JSON body fields:
//
//	tab     — tab alias ("active", "next", "last", "<N>", "" = all)
//	output  — file path to write results on stop ("" = return in response)
//	include — field groups to capture, e.g. ["request-headers","response-*"]
//	          supports comma-separated values and wildcards (request-*, response-*)
//	format  — output format: "json" (default), "ndjson", or "har"
func (s *Server) handleNetworkCaptureStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tab     string   `json:"tab"`
		Output  string   `json:"output"`
		Include []string `json:"include"`
		Format  string   `json:"format"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck — body is optional

	var activeID proto.TargetTargetID
	if ap := s.activePage(); ap != nil {
		activeID = ap.TargetID
	}
	tabFilter := resolveTabAlias(req.Tab, s.pageOrder, activeID)
	include := expandInclude(req.Include)

	s.mu.Lock()
	s.capturing = true
	s.capturedReqs = nil
	s.captureTabFilter = tabFilter
	s.captureOutputPath = req.Output
	s.captureInclude = include
	s.captureFormat = req.Format
	s.inFlightReqs = make(map[proto.NetworkRequestID]*capturedRequest)
	s.mu.Unlock()

	s.recordAction("network_capture_start", map[string]interface{}{
		"tab": req.Tab, "output": req.Output,
		"include": req.Include, "format": req.Format,
	})
	w.WriteHeader(http.StatusOK)
}

// /network/capture/stop — stops capture, writes to file if configured, returns results.
func (s *Server) handleNetworkCaptureStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.capturing = false
	reqs := s.capturedReqs
	s.capturedReqs = nil
	outputPath := s.captureOutputPath
	s.captureOutputPath = ""
	format := s.captureFormat
	s.captureFormat = ""
	s.captureInclude = nil
	s.inFlightReqs = make(map[proto.NetworkRequestID]*capturedRequest)
	s.mu.Unlock()

	if reqs == nil {
		reqs = []*capturedRequest{}
	}

	s.recordAction("network_capture_stop", nil)

	out, err := formatNetworkEntries(reqs, format)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if outputPath != "" {
		if err := os.WriteFile(outputPath, out, 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"path":  outputPath,
			"count": len(reqs),
		})
		return
	}

	if format == "" || format == "json" {
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "text/plain")
	}
	w.Write(out) //nolint:errcheck
}

// expandInclude expands wildcard groups and returns a deduplicated set.
// Each element in groups may itself be comma-separated.
//
//	request-* → request-headers, request-body, request-initiator, request-timestamp
//	response-* → response-headers, response-timing, response-size
//
// response-body is never included in any wildcard (future explicit opt-in).
func expandInclude(groups []string) map[string]bool {
	if len(groups) == 0 {
		return nil
	}
	result := make(map[string]bool)
	for _, g := range groups {
		for _, part := range strings.Split(g, ",") {
			part = strings.TrimSpace(part)
			switch part {
			case "request-*":
				result["request-headers"] = true
				result["request-body"] = true
				result["request-initiator"] = true
				result["request-timestamp"] = true
			case "response-*":
				result["response-headers"] = true
				result["response-timing"] = true
				result["response-size"] = true
			default:
				if part != "" {
					result[part] = true
				}
			}
		}
	}
	return result
}

// formatNetworkEntries serializes captured requests in the requested format.
func formatNetworkEntries(reqs []*capturedRequest, format string) ([]byte, error) {
	switch format {
	case "", "json":
		return json.Marshal(reqs)
	case "ndjson":
		var buf bytes.Buffer
		for _, r := range reqs {
			line, err := json.Marshal(r)
			if err != nil {
				return nil, err
			}
			buf.Write(line)
			buf.WriteByte('\n')
		}
		return buf.Bytes(), nil
	case "har":
		return marshalHAR(reqs)
	default:
		return nil, fmt.Errorf("unknown format %q: use json, ndjson, or har", format)
	}
}

// marshalHAR converts captured requests to a HAR 1.2 document.
func marshalHAR(reqs []*capturedRequest) ([]byte, error) {
	type harNameValue struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	type harPostData struct {
		MimeType string `json:"mimeType"`
		Text     string `json:"text"`
	}
	type harRequest struct {
		Method      string         `json:"method"`
		URL         string         `json:"url"`
		HTTPVersion string         `json:"httpVersion"`
		Headers     []harNameValue `json:"headers"`
		QueryString []harNameValue `json:"queryString"`
		PostData    *harPostData   `json:"postData,omitempty"`
		HeadersSize int            `json:"headersSize"`
		BodySize    int            `json:"bodySize"`
	}
	type harContent struct {
		Size     int64  `json:"size"`
		MimeType string `json:"mimeType"`
	}
	type harResponse struct {
		Status      int            `json:"status"`
		StatusText  string         `json:"statusText"`
		HTTPVersion string         `json:"httpVersion"`
		Headers     []harNameValue `json:"headers"`
		Content     harContent     `json:"content"`
		RedirectURL string         `json:"redirectURL"`
		HeadersSize int            `json:"headersSize"`
		BodySize    int            `json:"bodySize"`
	}
	type harTimings struct {
		DNS     float64 `json:"dns"`
		Connect float64 `json:"connect"`
		SSL     float64 `json:"ssl"`
		Send    float64 `json:"send"`
		Wait    float64 `json:"wait"`
		Receive float64 `json:"receive"`
	}
	type harEntry struct {
		StartedDateTime string      `json:"startedDateTime"`
		Time            float64     `json:"time"`
		Request         harRequest  `json:"request"`
		Response        harResponse `json:"response"`
		Timings         harTimings  `json:"timings"`
		Cache           struct{}    `json:"cache"`
	}
	type harCreator struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	type harLog struct {
		Version string     `json:"version"`
		Creator harCreator `json:"creator"`
		Pages   []struct{} `json:"pages"`
		Entries []harEntry `json:"entries"`
	}
	type harDoc struct {
		Log harLog `json:"log"`
	}

	entries := make([]harEntry, 0, len(reqs))
	for _, req := range reqs {
		// Request headers
		reqHeaders := make([]harNameValue, 0, len(req.RequestHeaders))
		for k, v := range req.RequestHeaders {
			reqHeaders = append(reqHeaders, harNameValue{Name: k, Value: v})
		}

		// Query string parsed from URL
		queryString := []harNameValue{}
		if u, err := url.Parse(req.URL); err == nil {
			for k, vals := range u.Query() {
				for _, v := range vals {
					queryString = append(queryString, harNameValue{Name: k, Value: v})
				}
			}
		}

		harReq := harRequest{
			Method:      req.Method,
			URL:         req.URL,
			HTTPVersion: "HTTP/1.1",
			Headers:     reqHeaders,
			QueryString: queryString,
			HeadersSize: -1,
			BodySize:    -1,
		}
		if req.PostData != "" {
			harReq.PostData = &harPostData{MimeType: "", Text: req.PostData}
			harReq.BodySize = len(req.PostData)
		}

		// Response headers
		respHeaders := make([]harNameValue, 0, len(req.ResponseHeaders))
		for k, v := range req.ResponseHeaders {
			respHeaders = append(respHeaders, harNameValue{Name: k, Value: v})
		}
		harResp := harResponse{
			Status:      req.Status,
			StatusText:  req.StatusText,
			HTTPVersion: "HTTP/1.1",
			Headers:     respHeaders,
			Content:     harContent{MimeType: req.MimeType},
			RedirectURL: "",
			HeadersSize: -1,
			BodySize:    -1,
		}
		if req.TransferSize != nil {
			harResp.Content.Size = *req.TransferSize
			harResp.BodySize = int(*req.TransferSize)
		}

		// Timings (-1 = N/A per HAR spec)
		timings := harTimings{DNS: -1, Connect: -1, SSL: -1, Send: -1, Wait: -1, Receive: -1}
		if req.Timing != nil {
			timings = harTimings{
				DNS:     req.Timing.DNS,
				Connect: req.Timing.Connect,
				SSL:     req.Timing.SSL,
				Send:    req.Timing.Send,
				Wait:    req.Timing.Wait,
				Receive: req.Timing.Receive,
			}
		}

		// Total time: sum of non-negative phases
		totalTime := 0.0
		for _, v := range []float64{timings.DNS, timings.Connect, timings.SSL, timings.Send, timings.Wait, timings.Receive} {
			if v > 0 {
				totalTime += v
			}
		}

		// startedDateTime
		started := req.startedAt
		if started.IsZero() {
			started = time.Now()
		}

		entries = append(entries, harEntry{
			StartedDateTime: started.UTC().Format(time.RFC3339Nano),
			Time:            totalTime,
			Request:         harReq,
			Response:        harResp,
			Timings:         timings,
		})
	}

	doc := harDoc{
		Log: harLog{
			Version: "1.2",
			Creator: harCreator{Name: "browsii", Version: "0.1.0"},
			Pages:   []struct{}{},
			Entries: entries,
		},
	}
	return json.MarshalIndent(doc, "", "  ")
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

	s.recordAction("network_mock", map[string]interface{}{
		"pattern": req.Pattern, "body": req.Body,
		"contentType": req.ContentType, "statusCode": req.StatusCode,
	})
	w.WriteHeader(http.StatusOK)
}
