package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/cdg-me/browsii/internal/client"
)

// instantiateHostExports registers the strongly-typed ABI methods into the `browsii` WASM module namespace
func (r *Runtime) instantiateHostExports(ctx context.Context, wz wazero.Runtime) error {
	_, err := wz.NewHostModuleBuilder("browsii").
		NewFunctionBuilder().
		WithFunc(r.exportNavigate).
		Export("_navigate").
		NewFunctionBuilder().
		WithFunc(r.exportClick).
		Export("_click").
		NewFunctionBuilder().
		WithFunc(r.exportWaitVisible).
		Export("_wait_visible").
		NewFunctionBuilder().
		WithFunc(r.exportWaitIdle).
		Export("_wait_idle").
		NewFunctionBuilder().
		WithFunc(r.exportSetResult).
		Export("_set_result").
		NewFunctionBuilder().
		WithFunc(r.exportRegisterNetworkListener).
		Export("_register_network_listener").
		NewFunctionBuilder().
		WithFunc(r.exportRegisterConsoleListener).
		Export("_register_console_listener").
		Instantiate(ctx)
	return err
}

func (r *Runtime) exportRegisterNetworkListener(ctx context.Context, m api.Module, callbackID uint32) {
	r.eventMu.Lock()
	r.networkCallbacksEnabled = true
	r.eventMu.Unlock()
}

func (r *Runtime) exportRegisterConsoleListener(ctx context.Context, m api.Module, callbackID uint32) {
	r.eventMu.Lock()
	r.consoleCallbacksEnabled = true
	r.eventMu.Unlock()
}

// readString extracts a string from WASM memory given a pointer and length
func readString(m api.Module, ptr uint32, length uint32) (string, bool) {
	bytes, ok := m.Memory().Read(ptr, length)
	if !ok {
		return "", false
	}
	return string(bytes), true
}

// writeError fills the pre-allocated error buffer in WASM memory, up to maxLen bytes.
// Returns the actual length of the error written.
func writeError(m api.Module, errPtr uint32, maxLen uint32, errorMsg string) uint32 {
	if errorMsg == "" {
		return 0
	}
	// Truncate if necessary
	errBytes := []byte(errorMsg)
	writeLen := uint32(len(errBytes))
	if writeLen > maxLen {
		writeLen = maxLen
	}
	m.Memory().Write(errPtr, errBytes[:writeLen])
	return writeLen
}

// _navigate(url_ptr i32, url_len i32, err_ptr i32, err_maxlen i32) -> i32
func (r *Runtime) exportNavigate(ctx context.Context, m api.Module, urlPtr, urlLen, errPtr, maxLen uint32) uint32 {
	url, ok := readString(m, urlPtr, urlLen)
	if !ok {
		return writeError(m, errPtr, maxLen, "host panic: out of bounds memory read for URL")
	}

	payload := map[string]string{"url": url}
	_, err := client.SendCommand(r.daemonPort, "navigate", payload)
	if err != nil {
		return writeError(m, errPtr, maxLen, err.Error())
	}

	r.flushEvents(ctx, m)
	return 0 // Success
}

// _click(selector_ptr i32, selector_len i32, err_ptr i32, err_maxlen i32) -> i32
func (r *Runtime) exportClick(ctx context.Context, m api.Module, selPtr, selLen, errPtr, maxLen uint32) uint32 {
	selector, ok := readString(m, selPtr, selLen)
	if !ok {
		return writeError(m, errPtr, maxLen, "host panic: out of bounds memory read for selector")
	}

	payload := map[string]string{"selector": selector}
	_, err := client.SendCommand(r.daemonPort, "click", payload)
	if err != nil {
		return writeError(m, errPtr, maxLen, err.Error())
	}

	r.flushEvents(ctx, m)
	return 0
}

// _wait_visible(selector_ptr i32, selector_len i32, err_ptr i32, err_maxlen i32) -> i32
func (r *Runtime) exportWaitVisible(ctx context.Context, m api.Module, selPtr, selLen, errPtr, maxLen uint32) uint32 {
	selector, ok := readString(m, selPtr, selLen)
	if !ok {
		return writeError(m, errPtr, maxLen, "host panic: out of bounds memory read")
	}

	// For wait_visible, we can reuse 'js' evaluation as a quick shim until the daemon gets a dedicated wait endpoint
	script := fmt.Sprintf(`() => {
		return new Promise((resolve) => {
			const check = () => {
				const el = document.querySelector('%s');
				if (el && el.offsetHeight > 0) resolve();
				else requestAnimationFrame(check);
			};
			check();
		});
	}`, selector)

	payload := map[string]string{"script": script}
	_, err := client.SendCommand(r.daemonPort, "js", payload)
	if err != nil {
		return writeError(m, errPtr, maxLen, err.Error())
	}

	r.flushEvents(ctx, m)
	return 0
}

// _wait_idle(ms i32, err_ptr i32, err_maxlen i32) -> i32
func (r *Runtime) exportWaitIdle(ctx context.Context, m api.Module, ms uint32, errPtr, maxLen uint32) uint32 {
	// Yield and flush immediately
	r.flushEvents(ctx, m)

	// In a real implementation we'd probably use a context-aware sleep or timer,
	// but for the sake of flushing the event loop correctly during the mock phase:
	// we just sleep and then flush again before returning control to the WASM module.
	time.Sleep(time.Duration(ms) * time.Millisecond)

	r.flushEvents(ctx, m)
	return 0
}

// _set_result(ptr i32, len i32)
// Captures structured JSON output from the standard API and prints it to stdout cleanly
func (r *Runtime) exportSetResult(ctx context.Context, m api.Module, ptr, length uint32) {
	jsonStr, ok := readString(m, ptr, length)
	if !ok {
		r.printHostError("host panic: _set_result failed to read memory bounds")
		return
	}

	// Validate it's valid JSON
	var js json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &js); err != nil {
		r.printHostError("Guest called _set_result with invalid JSON: %v", err)
		return
	}

	// Compact it to ensure it's a single parseable line
	buffer := new(bytes.Buffer)
	json.Compact(buffer, js) //nolint:errcheck
	fmt.Println(buffer.String())
}
