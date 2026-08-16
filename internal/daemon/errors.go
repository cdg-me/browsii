package daemon

import (
	"encoding/json"
	"net/http"
)

// apiError is an error with recovery context for LLM agents: a human-readable
// message, an optional hint describing the next action to try, and optional
// candidate elements that fuzzy-match what the caller asked for.
type apiError struct {
	Status     int                `json:"-"`
	Message    string             `json:"error"`
	Hint       string             `json:"hint,omitempty"`
	Candidates []elementCandidate `json:"candidates,omitempty"`
}

// writeAPIError serialises e as a JSON error response.
func writeAPIError(w http.ResponseWriter, e *apiError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(e)
}

// notFoundError builds a 404 with candidate elements attached.
func notFoundError(message, hint string, candidates []elementCandidate) *apiError {
	if len(candidates) == 0 && hint == "" {
		hint = "run 'elements' to list interactive elements with refs"
	} else if len(candidates) > 0 && hint == "" {
		hint = "similar elements listed below — retry with a ref or a corrected selector"
	}
	return &apiError{
		Status:     http.StatusNotFound,
		Message:    message,
		Hint:       hint,
		Candidates: candidates,
	}
}
