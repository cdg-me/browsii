package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
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
}

// /record/start endpoint — begins recording actions
func (s *Server) handleRecordStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.recMu.Lock()
	s.recording = true
	s.recordName = req.Name
	s.recordStart = time.Now()
	s.recordEvents = nil
	s.recMu.Unlock()

	w.WriteHeader(http.StatusOK)
}

// /record/stop endpoint — stops recording and saves to disk
func (s *Server) handleRecordStop(w http.ResponseWriter, r *http.Request) {
	s.recMu.Lock()
	s.recording = false
	name := s.recordName
	events := s.recordEvents
	s.recMu.Unlock()

	recording := map[string]interface{}{
		"name":   name,
		"events": events,
	}

	var recFile string
	if filepath.IsAbs(name) {
		recFile = name
		os.MkdirAll(filepath.Dir(recFile), 0755)
	} else {
		homeDir, _ := os.UserHomeDir()
		recDir := filepath.Join(homeDir, ".browsii", "recordings")
		os.MkdirAll(recDir, 0755)
		recFile = filepath.Join(recDir, name+".json")
	}

	data, _ := json.MarshalIndent(recording, "", "  ")
	if err := os.WriteFile(recFile, data, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":   name,
		"events": len(events),
	})
}

// /record/replay endpoint — replays a recorded session
func (s *Server) handleRecordReplay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string  `json:"name"`
		Speed float64 `json:"speed"` // 0 = instant, 1 = real-time, 2 = 2x
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var recFile string
	if filepath.IsAbs(req.Name) {
		recFile = req.Name
	} else {
		homeDir, _ := os.UserHomeDir()
		recFile = filepath.Join(homeDir, ".browsii", "recordings", req.Name+".json")
	}

	data, err := os.ReadFile(recFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("recording %q not found", req.Name), http.StatusNotFound)
		return
	}

	var recording struct {
		Events []RecordedEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &recording); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Disable recording during replay to avoid re-recording
	wasRecording := s.recording
	s.recording = false

	for i, event := range recording.Events {
		// Catch panics per-action to avoid crashing the whole replay
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Replay of action %q panicked: %v", event.Action, r)
				}
			}()

			// Apply timing delay
			if req.Speed > 0 && i > 0 {
				delay := event.T - recording.Events[i-1].T
				time.Sleep(time.Duration(float64(delay)/req.Speed) * time.Millisecond)
			}

			// Dispatch the action directly
			page := s.activePage()
			if page == nil {
				return
			}

			switch event.Action {
			case "tab_new":
				if url, ok := event.Params["url"].(string); ok {
					s.activePg = s.browser.MustPage(url).MustWaitLoad()
					s.trackPage(s.activePg)
				}
			case "tab_close":
				s.untrackPage(page)
				page.MustClose()
				s.activePg = nil
			case "tab_switch":
				if idxF, ok := event.Params["index"].(float64); ok {
					pages := s.orderedPages()
					idx := int(idxF)
					if idx >= 0 && idx < len(pages) {
						s.activePg = pages[idx]
						s.activePg.MustActivate()
					}
				}
			case "mouse_move":
				if x, ok := event.Params["x"].(float64); ok {
					if y, ok := event.Params["y"].(float64); ok {
						page.Mouse.MustMoveTo(x, y)
					}
				}
			case "mouse_drag":
				x1, _ := event.Params["x1"].(float64)
				y1, _ := event.Params["y1"].(float64)
				x2, _ := event.Params["x2"].(float64)
				y2, _ := event.Params["y2"].(float64)
				stepsF, ok := event.Params["steps"].(float64)
				steps := 10
				if ok && stepsF > 0 {
					steps = int(stepsF)
				}
				page.Mouse.MustMoveTo(x1, y1)
				page.Mouse.MustDown("left")
				for i := 1; i <= steps; i++ {
					t := float64(i) / float64(steps)
					page.Mouse.MustMoveTo(x1+t*(x2-x1), y1+t*(y2-y1))
				}
				page.Mouse.MustUp("left")
			case "mouse_rightclick":
				if sel, ok := event.Params["selector"].(string); ok {
					el := page.MustElement(sel)
					el.MustScrollIntoView()
					box := el.MustShape().Box()
					page.Mouse.MustMoveTo(box.X+box.Width/2, box.Y+box.Height/2)
					page.Mouse.MustClick("right")
				}
			case "mouse_doubleclick":
				if sel, ok := event.Params["selector"].(string); ok {
					js := fmt.Sprintf(`(sel) => { document.querySelector(sel).dispatchEvent(new MouseEvent('dblclick', {bubbles: true, cancelable: true})); }`)
					page.MustEval(js, sel)
				}
			case "upload":
				sel, ok1 := event.Params["selector"].(string)
				filesI, ok2 := event.Params["files"].([]interface{})
				if ok1 && ok2 {
					var files []string
					for _, f := range filesI {
						if fs, ok := f.(string); ok {
							files = append(files, fs)
						}
					}
					page.MustElement(sel).MustSetFiles(files...)
				}
			case "screenshot":
				filename, _ := event.Params["filename"].(string)
				el, _ := event.Params["element"].(string)
				fullPage, _ := event.Params["fullPage"].(bool)
				if el != "" {
					data, _ := page.MustElement(el).Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
					os.WriteFile(filename, data, 0644)
				} else if fullPage {
					page.MustScreenshotFullPage(filename)
				} else {
					page.MustScreenshot(filename)
				}
			case "pdf":
				if filename, ok := event.Params["filename"].(string); ok {
					pdfData, _ := page.PDF(&proto.PagePrintToPDF{})
					data, _ := io.ReadAll(pdfData)
					os.WriteFile(filename, data, 0644)
				}
			case "js":
				if script, ok := event.Params["script"].(string); ok {
					page.MustEval(script)
				}
			case "network_throttle":
				lat, _ := event.Params["latency"].(float64)
				dl, _ := event.Params["download"].(float64)
				up, _ := event.Params["upload"].(float64)
				proto.NetworkEmulateNetworkConditions{Offline: false, Latency: lat, DownloadThroughput: dl, UploadThroughput: up}.Call(page)
			case "network_mock":
				pat, _ := event.Params["pattern"].(string)
				body, _ := event.Params["body"].(string)
				ct, _ := event.Params["contentType"].(string)
				sc, _ := event.Params["statusCode"].(float64)
				if sc == 0 {
					sc = 200
				}
				router := page.HijackRequests()
				router.MustAdd(pat, func(ctx *rod.Hijack) {
					ctx.Response.SetBody(body)
					if ct != "" {
						ctx.Response.SetHeader("Content-Type", ct)
					}
					ctx.Response.Payload().ResponseCode = int(sc)
				})
				go router.Run()
			case "navigate":
				if url, ok := event.Params["url"].(string); ok {
					page.MustNavigate(url).MustWaitLoad()
				}
			case "click":
				if sel, ok := event.Params["selector"].(string); ok {
					page.MustElement(sel).MustClick()
				}
			case "type":
				sel, _ := event.Params["selector"].(string)
				text, _ := event.Params["text"].(string)
				if sel != "" {
					page.MustElement(sel)
					js := fmt.Sprintf(`() => {
					const el = document.querySelector('%s');
					if (el) { el.value = ''; el.focus(); }
				}`, sel)
					page.MustEval(js)
					page.MustInsertText(text)
				}
			case "scroll":
				dir, _ := event.Params["direction"].(string)
				pixels := 300
				if p, ok := event.Params["pixels"].(float64); ok && p > 0 {
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
			case "hover":
				if sel, ok := event.Params["selector"].(string); ok {
					page.MustElement(sel).MustHover()
				}
			case "press":
				if key, ok := event.Params["key"].(string); ok {
					keys := parseKeyCombo(key)
					ka := page.KeyActions()
					for _, k := range keys {
						ka = ka.Press(k)
					}
					ka.MustDo()
				}
			case "reload":
				page.MustReload().MustWaitLoad()
			case "back":
				page.MustNavigateBack().MustWaitLoad()
			case "forward":
				page.MustNavigateForward().MustWaitLoad()
			}
		}()
	}

	s.recording = wasRecording
	w.WriteHeader(http.StatusOK)
}

// /record/list endpoint — returns available recordings
func (s *Server) handleRecordList(w http.ResponseWriter, r *http.Request) {
	homeDir, _ := os.UserHomeDir()
	recDir := filepath.Join(homeDir, ".browsii", "recordings")

	entries, err := os.ReadDir(recDir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
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
	json.NewEncoder(w).Encode(recordings)
}

// /record/delete endpoint — removes a recording
func (s *Server) handleRecordDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var recFile string
	if filepath.IsAbs(req.Name) {
		recFile = req.Name
	} else {
		homeDir, _ := os.UserHomeDir()
		recFile = filepath.Join(homeDir, ".browsii", "recordings", req.Name+".json")
	}

	if err := os.Remove(recFile); err != nil {
		http.Error(w, fmt.Sprintf("recording %q not found", req.Name), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}
