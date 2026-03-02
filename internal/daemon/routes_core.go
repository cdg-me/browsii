package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

func (s *Server) registerCoreRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ping", s.handlePing)
	mux.HandleFunc("/events/stream", s.handleEventsStream)
	mux.HandleFunc("/shutdown", s.handleShutdown)
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "pong")
}

func (s *Server) handleEventsStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Create a buffered channel (Ring buffer size: 5000)
	// This prevents bursts of MSE events from blocking the daemon.
	clientChan := make(chan StreamEvent, 5000)

	s.sseMu.Lock()
	s.sseClients[clientChan] = struct{}{}
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.sseClients, clientChan)
		close(clientChan)
		s.sseMu.Unlock()
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Flush headers immediately so the HTTP client gets the 200 OK and returns from Do/Get
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return // Client disconnected
		case event := <-clientChan:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}
	}
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "shutting down")
	// We trigger the shutdown in a goroutine so the HTTP response completes
	go func() {
		s.Stop()
		os.Exit(0)
	}()
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Recovered from panic in HTTP handler: %v", err)
				http.Error(w, fmt.Sprintf("internal error: %v", err), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
