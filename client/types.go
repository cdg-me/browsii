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
type NetworkRequest struct {
	URL    string `json:"url"`
	Method string `json:"method"`
	Type   string `json:"type"`
	Tab    int    `json:"tab"`
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
