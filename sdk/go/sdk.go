//go:build ignore

package sdk

import (
	"encoding/json"
	"errors"
	"reflect"
	"unsafe"
)

// expectedSDKVersion must exactly match the host daemon's expectation.
const expectedSDKVersion int32 = 1

// The maximum size of an error string returned by host functions.
const maxErrLen uint32 = 4096

// _browsii_sdk_version is executed by wazero before _start to protect against version drift.
//
//go:wasmexport _browsii_sdk_version
func _sdkVersion() int32 {
	return expectedSDKVersion
}

// Low-level WASI bindings to the `browsii` host module.
//
//go:wasmimport browsii _navigate
func _navigate(urlPtr, urlLen, errPtr, errMaxLen uint32) uint32

//go:wasmimport browsii _click
func _click(selPtr, selLen, errPtr, errMaxLen uint32) uint32

//go:wasmimport browsii _wait_visible
func _wait_visible(selPtr, selLen, errPtr, errMaxLen uint32) uint32

//go:wasmimport browsii _wait_idle
func _wait_idle(ms uint32, errPtr, errMaxLen uint32) uint32

//go:wasmimport browsii _set_result
func _set_result(ptr, len uint32)

// --- Ergonomic Go Wrappers ---

// Navigate instructs the active browser tab to navigate to the provided URL.
func Navigate(url string) error {
	p, l := stringToPtr(url)

	errBuf := make([]byte, maxErrLen)
	errPtr := ptrToBytes(errBuf)

	status := _navigate(p, l, errPtr, maxErrLen)
	if status > 0 {
		return errors.New("host error: " + string(errBuf[:status]))
	}
	return nil
}

// Click instructs the browser to click the element matching the CSS selector.
func Click(selector string) error {
	p, l := stringToPtr(selector)

	errBuf := make([]byte, maxErrLen)
	errPtr := ptrToBytes(errBuf)

	status := _click(p, l, errPtr, maxErrLen)
	if status > 0 {
		return errors.New("host error: " + string(errBuf[:status]))
	}
	return nil
}

// WaitVisible pauses execution until the element matching the selector is visible in the DOM.
func WaitVisible(selector string) error {
	p, l := stringToPtr(selector)

	errBuf := make([]byte, maxErrLen)
	errPtr := ptrToBytes(errBuf)

	status := _wait_visible(p, l, errPtr, maxErrLen)
	if status > 0 {
		return errors.New("host error: " + string(errBuf[:status]))
	}
	return nil
}

// WaitIdle pauses script execution for the specified milliseconds.
// Use this instead of `time.Sleep` to allow background NetworkEvents to flush.
func WaitIdle(ms int) error {
	errBuf := make([]byte, maxErrLen)
	errPtr := ptrToBytes(errBuf)

	status := _wait_idle(uint32(ms), errPtr, maxErrLen)
	if status > 0 {
		return errors.New("host error: " + string(errBuf[:status]))
	}
	return nil
}

// SetResult serializes the provided value to JSON and passes it back to the CLI host.
// This is the canonical way for automation scripts to return structured data.
func SetResult(v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		// If serialization fails, we attempt to pass a basic error JSON back.
		fallback := `{"error":"failed to serialize result schema"}`
		fp, fl := stringToPtr(fallback)
		_set_result(fp, fl)
		return
	}

	p, l := bytesToPtr(b)
	_set_result(p, l)
}

// --- Memory Utilities ---

func stringToPtr(s string) (uint32, uint32) {
	if s == "" {
		return 0, 0
	}
	buf := []byte(s)
	return ptrToBytes(buf), uint32(len(buf))
}

func bytesToPtr(b []byte) (uint32, uint32) {
	if len(b) == 0 {
		return 0, 0
	}
	return ptrToBytes(b), uint32(len(b))
}

func ptrToBytes(b []byte) uint32 {
	header := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	return uint32(header.Data)
}

// allocate allows the Host to allocate memory sequentially inside the Go Guest heap.
// Used primarily for passing dynamic SSE event payloads asynchronously.
//
//go:wasmexport allocate
func allocate(size uint32) uint32 {
	buf := make([]byte, size)
	return ptrToBytes(buf)
}

// --- Event Callbacks ---

// NetworkEvent represents an intercepted browser request.
type NetworkEvent struct {
	URL    string `json:"url"`
	Method string `json:"method"`
	Type   string `json:"type"`
	Tab    int    `json:"tab"`
}

var networkListener func(NetworkEvent)

//go:wasmimport browsii _register_network_listener
func _register_network_listener(callbackID int32)

// OnNetworkRequest registers a callback that invoked asynchronously by the host
// over the SSE stream whenever the browser fires a network request.
func OnNetworkRequest(cb func(NetworkEvent)) {
	networkListener = cb
	_register_network_listener(1) // Registering ID 1 signals the host we want network events
}

// _on_network_request is called by the Host when the SSE stream broadcasts an event
//
//go:wasmexport _on_network_request
func _onNetworkRequest(ptr, length uint32) {
	if networkListener == nil {
		return
	}

	// Reconstruct the bytes from the Host's caller memory
	// Warning: The Host must have used the Guest's `allocate` to place these bytes safely.
	b := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)

	var event NetworkEvent
	if err := json.Unmarshal(b, &event); err == nil {
		networkListener(event)
	}
}

// --- Console Event Callbacks ---

// ConsoleArg represents a single argument passed to a console call.
// Type is the CDP type string (e.g. "string", "number", "object", "boolean").
// Value holds the primitive value for simple types; Description holds a
// human-readable representation for objects, arrays, and errors.
type ConsoleArg struct {
	Type        string      `json:"type"`
	Value       interface{} `json:"value,omitempty"`
	Description string      `json:"description,omitempty"`
	Class       string      `json:"class,omitempty"`
}

// ConsoleEvent represents a single browser console call (log, warn, error, etc.).
type ConsoleEvent struct {
	Level string       `json:"level"`
	Text  string       `json:"text"`
	Args  []ConsoleArg `json:"args"`
	Tab   int          `json:"tab"`
}

var consoleListener func(ConsoleEvent)

//go:wasmimport browsii _register_console_listener
func _register_console_listener(callbackID int32)

// OnConsoleEvent registers a callback that is invoked asynchronously by the host
// over the SSE stream whenever the browser fires a console call.
func OnConsoleEvent(cb func(ConsoleEvent)) {
	consoleListener = cb
	_register_console_listener(1) // Registering ID 1 signals the host we want console events
}

// _on_console_event is called by the Host when the SSE stream broadcasts a console event.
//
//go:wasmexport _on_console_event
func _onConsoleEvent(ptr, length uint32) {
	if consoleListener == nil {
		return
	}

	b := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)

	var event ConsoleEvent
	if err := json.Unmarshal(b, &event); err == nil {
		consoleListener(event)
	}
}
