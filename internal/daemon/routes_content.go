package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-rod/rod/lib/proto"
)

func (s *Server) registerContentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/scrape", s.handleScrape)
	mux.HandleFunc("/links", s.handleLinks)
	mux.HandleFunc("/screenshot", s.handleScreenshot)
	mux.HandleFunc("/pdf", s.handlePdf)
	mux.HandleFunc("/js", s.handleJS)
	mux.HandleFunc("/cookies", s.handleCookies)
}

func (s *Server) handleScrape(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Format string `json:"format"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if req.Format == "" {
		req.Format = "html"
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages to scrape", http.StatusBadRequest)
		return
	}

	switch req.Format {
	case "text":
		result := page.MustEval(`() => document.body.innerText`)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, result.Str()) //nolint:errcheck
	case "markdown":
		// Convert HTML to markdown via JS in the browser
		result := page.MustEval(`() => {
			function htmlToMd(el) {
				let md = '';
				for (const node of el.childNodes) {
					if (node.nodeType === 3) { // Text node
						const t = node.textContent.trim();
						if (t) md += t + ' ';
					} else if (node.nodeType === 1) { // Element
						const tag = node.tagName.toLowerCase();
						if (/^h[1-6]$/.test(tag)) {
							const level = parseInt(tag[1]);
							md += '\n' + '#'.repeat(level) + ' ' + node.innerText.trim() + '\n\n';
						} else if (tag === 'p') {
							md += node.innerText.trim() + '\n\n';
						} else if (tag === 'a') {
							md += '[' + node.innerText + '](' + node.href + ')';
						} else if (tag === 'ul' || tag === 'ol') {
							const items = node.querySelectorAll(':scope > li');
							items.forEach((li, i) => {
								const prefix = tag === 'ol' ? (i+1) + '. ' : '- ';
								md += prefix + li.innerText.trim() + '\n';
							});
							md += '\n';
						} else if (tag === 'strong' || tag === 'b') {
							md += '**' + node.innerText + '**';
						} else if (tag === 'em' || tag === 'i') {
							md += '*' + node.innerText + '*';
						} else if (tag === 'code') {
							md += '` + "`" + `' + node.innerText + '` + "`" + `';
						} else if (tag === 'pre') {
							md += '\n` + "```" + `\n' + node.innerText + '\n` + "```" + `\n\n';
						} else if (tag === 'br') {
							md += '\n';
						} else if (tag === 'hr') {
							md += '\n---\n\n';
						} else if (tag !== 'script' && tag !== 'style' && tag !== 'head') {
							md += htmlToMd(node);
						}
					}
				}
				return md;
			}
			return htmlToMd(document.body).trim();
		}`)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, result.Str()) //nolint:errcheck
	default: // html
		html := page.MustHTML()
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html) //nolint:errcheck
	}
	s.recordAction("scrape", map[string]interface{}{"format": req.Format})
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Pattern string `json:"pattern"`
	}
	// Pattern is optional, ignore decode errors for GET-like usage
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	// Collect all href values via JS
	result := page.MustEval(`() => {
		return Array.from(document.querySelectorAll('a[href]')).map(a => a.href);
	}`)

	var links []string
	for _, v := range result.Arr() {
		links = append(links, v.Str())
	}

	// Filter by pattern if provided
	if req.Pattern != "" {
		var filtered []string
		for _, link := range links {
			if strings.Contains(link, req.Pattern) {
				filtered = append(filtered, link)
			}
		}
		links = filtered
	}

	s.recordAction("links", map[string]interface{}{"pattern": req.Pattern})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links) //nolint:errcheck
}

func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
		Element  string `json:"element"`
		FullPage bool   `json:"fullPage"`
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

	if req.Element != "" {
		// Element screenshot
		el := page.MustElement(req.Element)
		data, _ := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
		os.WriteFile(req.Filename, data, 0644) //nolint:errcheck
	} else if req.FullPage {
		// Full page screenshot
		page.MustScreenshotFullPage(req.Filename)
	} else {
		// Viewport screenshot
		page.MustScreenshot(req.Filename)
	}
	s.recordAction("screenshot", map[string]interface{}{"filename": req.Filename, "element": req.Element, "fullPage": req.FullPage})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handlePdf(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
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

	// go-rod's PDF API returns a *proto.PagePrintToPDFResult with Data field
	pdfData, err := page.PDF(&proto.PagePrintToPDF{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Read the PDF stream
	data, err := io.ReadAll(pdfData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(req.Filename, data, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleJS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Script string `json:"script"`
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

	res, err := page.Eval(wrapScript(req.Script))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// res.Value holds the evaluated JS object. We marshal its inner generic value.
	jsonBytes, err := json.Marshal(res.Value.Val())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.recordAction("js", map[string]interface{}{"script": req.Script})
	w.Write(jsonBytes) //nolint:errcheck
}

func (s *Server) handleCookies(w http.ResponseWriter, r *http.Request) {
	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	cookies, err := page.Cookies(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.recordAction("cookies", nil)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cookies) //nolint:errcheck
}
