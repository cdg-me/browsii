package daemon

import "time"

// capturedRequest holds all collected data for a single network request.
// The base fields (URL, Method, Type, Tab) are always present. Optional
// fields are populated progressively as CDP events arrive, gated by the
// active captureInclude set (from --include on start).
type capturedRequest struct {
	// Always present
	URL    string `json:"url"`
	Method string `json:"method"`
	Type   string `json:"type"`
	Tab    int    `json:"tab"`

	// request-timestamp: wall-clock time the request was initiated (seconds since epoch)
	Timestamp float64 `json:"timestamp,omitempty"`

	// request-initiator: what triggered this request
	Initiator *capturedInitiator `json:"initiator,omitempty"`

	// request-headers: outgoing request headers
	RequestHeaders map[string]string `json:"requestHeaders,omitempty"`

	// request-body: POST/PUT body
	PostData string `json:"postData,omitempty"`

	// response-headers: status line + response headers + MIME type
	Status          int               `json:"status,omitempty"`
	StatusText      string            `json:"statusText,omitempty"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
	MimeType        string            `json:"mimeType,omitempty"`

	// response-timing: breakdown of connection + transfer phases (ms, -1 = N/A)
	Timing *capturedTiming `json:"timing,omitempty"`

	// response-size: bytes transferred over the wire (from NetworkLoadingFinished)
	TransferSize *int64 `json:"transferSize,omitempty"`

	// startedAt is the wall-clock time for HAR serialization; not marshaled to JSON.
	startedAt time.Time
}

// capturedInitiator describes what triggered a network request.
type capturedInitiator struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
}

// capturedTiming contains timing breakdowns for a request/response cycle
// in milliseconds. A value of -1 means the phase was not applicable.
type capturedTiming struct {
	DNS     float64 `json:"dns"`
	Connect float64 `json:"connect"`
	SSL     float64 `json:"ssl"`
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"`
	Receive float64 `json:"receive"`
}
