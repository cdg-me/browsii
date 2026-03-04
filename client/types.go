package client

import "encoding/json"

// ScrapeFormat controls the output format of Scrape.
type ScrapeFormat string

const (
	HTML     ScrapeFormat = "html"
	Text     ScrapeFormat = "text"
	Markdown ScrapeFormat = "markdown"
)

// Tab represents an open browser tab.
type Tab struct {
	Index int    `json:"index"`
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

// NetworkRequest is a captured HTTP request from the browser.
// The base fields (URL, Method, Type, Tab) are always present.
// Optional fields are populated when the corresponding --include group
// was active at capture start time.
type NetworkRequest struct {
	// Always present
	URL    string `json:"url"`
	Method string `json:"method"`
	Type   string `json:"type"`
	Tab    int    `json:"tab"`

	// request-timestamp: wall-clock time the request was initiated (seconds since epoch)
	Timestamp float64 `json:"timestamp,omitempty"`

	// request-initiator: what triggered the request
	Initiator *NetworkInitiator `json:"initiator,omitempty"`

	// request-headers: outgoing request headers
	RequestHeaders map[string]string `json:"requestHeaders,omitempty"`

	// request-body: POST/PUT body
	PostData string `json:"postData,omitempty"`

	// response-headers: status + response headers + MIME type
	Status          int               `json:"status,omitempty"`
	StatusText      string            `json:"statusText,omitempty"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
	MimeType        string            `json:"mimeType,omitempty"`

	// response-timing: breakdown of connection + transfer phases (ms, -1 = N/A)
	Timing *NetworkTiming `json:"timing,omitempty"`

	// response-size: bytes transferred over the wire
	TransferSize *int64 `json:"transferSize,omitempty"`
}

// NetworkInitiator describes what triggered a network request.
type NetworkInitiator struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
}

// NetworkTiming contains timing breakdowns for a request/response cycle
// in milliseconds. A value of -1 means the phase was not applicable.
type NetworkTiming struct {
	DNS     float64 `json:"dns"`
	Connect float64 `json:"connect"`
	SSL     float64 `json:"ssl"`
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"`
	Receive float64 `json:"receive"`
}

// NetworkCaptureOpts configures a network capture session.
type NetworkCaptureOpts struct {
	// Tab filters which tab to capture.
	// Values: "" or "all" (all tabs), "active", "next", "last", or a numeric index.
	Tab string

	// Include is the list of optional field groups to capture.
	// Each string may be comma-separated; wildcards are supported.
	//
	// Available groups:
	//   request-headers, request-body, request-initiator, request-timestamp
	//   response-headers, response-timing, response-size
	//
	// Wildcards:
	//   request-*  → all request-* groups
	//   response-* → all response-* groups (never includes response-body)
	Include []string

	// Format controls how captured data is serialized on stop.
	// Values: "" or "json" (default array), "ndjson", "har".
	Format string

	// Output is a file path to write results to when capture stops.
	// When set, NetworkCaptureStop returns a confirmation (Count, OutputPath)
	// rather than inline data (Requests / Raw).
	Output string
}

// NetworkCaptureStopResult is returned by NetworkCaptureStop.
type NetworkCaptureStopResult struct {
	// Requests contains the parsed entries when Format is "" or "json" and
	// no Output file was configured. Nil otherwise.
	Requests []NetworkRequest

	// Raw contains the serialized output when Format is "ndjson" or "har"
	// and no Output file was configured.
	Raw []byte

	// OutputPath is set when data was written to a file.
	OutputPath string

	// Count is the total number of captured entries.
	Count int
}

// ConsoleArg is a single argument from a console.log call.
type ConsoleArg struct {
	Type        string          `json:"type"`
	Value       json.RawMessage `json:"value,omitempty"`
	Description string          `json:"description,omitempty"`
	Class       string          `json:"class,omitempty"`
}

// ConsoleEntry is a captured browser console message.
type ConsoleEntry struct {
	Level string       `json:"level"`
	Text  string       `json:"text"`
	Tab   int          `json:"tab"`
	Args  []ConsoleArg `json:"args,omitempty"`
}

// ListEntry is a named item with a modification timestamp, used for
// session and recording lists.
type ListEntry struct {
	Name     string `json:"name"`
	Modified string `json:"modified"`
}

// RecordStopResult is returned by RecordStop.
type RecordStopResult struct {
	Name   string `json:"name"`
	Events int    `json:"events"`
}
