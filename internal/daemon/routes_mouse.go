package daemon

import (
	"encoding/json"
	"net/http"
)

func (s *Server) registerMouseRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/mouse/move", s.handleMouseMove)
	mux.HandleFunc("/mouse/drag", s.handleMouseDrag)
	mux.HandleFunc("/mouse/rightclick", s.handleMouseRightclick)
	mux.HandleFunc("/mouse/doubleclick", s.handleMouseDoubleclick)
}

func (s *Server) handleMouseMove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
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

	page.Mouse.MustMoveTo(req.X, req.Y)
	s.recordAction("mouse_move", map[string]interface{}{"x": req.X, "y": req.Y})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMouseDrag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		X1    float64 `json:"x1"`
		Y1    float64 `json:"y1"`
		X2    float64 `json:"x2"`
		Y2    float64 `json:"y2"`
		Steps int     `json:"steps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Steps <= 0 {
		req.Steps = 10
	}

	page := s.activePage()
	if page == nil {
		http.Error(w, "no active pages", http.StatusBadRequest)
		return
	}

	mouse := page.Mouse
	mouse.MustMoveTo(req.X1, req.Y1)
	mouse.MustDown("left")

	// Interpolate intermediate points for smooth drawing
	for i := 1; i <= req.Steps; i++ {
		t := float64(i) / float64(req.Steps)
		x := req.X1 + t*(req.X2-req.X1)
		y := req.Y1 + t*(req.Y2-req.Y1)
		mouse.MustMoveTo(x, y)
	}

	mouse.MustUp("left")
	s.recordAction("mouse_drag", map[string]interface{}{"x1": req.X1, "y1": req.Y1, "x2": req.X2, "y2": req.Y2, "steps": req.Steps})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMouseRightclick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Selector string `json:"selector"`
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

	el := page.MustElement(req.Selector)
	el.MustScrollIntoView()
	shape := el.MustShape()
	box := shape.Box()
	x := box.X + box.Width/2
	y := box.Y + box.Height/2
	page.Mouse.MustMoveTo(x, y)
	page.Mouse.MustClick("right")
	s.recordAction("mouse_rightclick", map[string]interface{}{"selector": req.Selector})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMouseDoubleclick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Selector string `json:"selector"`
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

	el := page.MustElement(req.Selector)
	el.MustScrollIntoView()
	// Dispatch a real dblclick event via JS — two separate CDP clicks don't fire dblclick
	page.MustEval(`(sel) => {
		const el = document.querySelector(sel);
		el.dispatchEvent(new MouseEvent('dblclick', {bubbles: true, cancelable: true}));
	}`, req.Selector)
	s.recordAction("mouse_doubleclick", map[string]interface{}{"selector": req.Selector})
	w.WriteHeader(http.StatusOK)
}
