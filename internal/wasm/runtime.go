package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"

	"github.com/cdg-me/browsii/internal/client"
)

const ExpectedSDKVersion = 1

type Runtime struct {
	daemonPort int

	// Event Loop buffering
	eventBuffer             []client.StreamEvent
	eventMu                 sync.Mutex
	networkCallbacksEnabled bool
	consoleCallbacksEnabled bool
}

func NewRuntime(port int) *Runtime {
	return &Runtime{daemonPort: port}
}

// Run executes the provided WASM binary. It complies exactly with the Exit Code Taxonomy
// outlined in the implementation plan.
func (r *Runtime) Run(ctx context.Context, wasmBytes []byte) int {
	wCtx := context.Background()

	wz := wazero.NewRuntime(wCtx)
	defer wz.Close(wCtx) //nolint:errcheck

	// Instantiate WASI for standard I/O and fundamental libc requirements
	wasi_snapshot_preview1.MustInstantiate(wCtx, wz)

	// Instantiate our typed host bindings
	if err := r.instantiateHostExports(wCtx, wz); err != nil {
		r.printHostError("Failed to instantiate host bindings: %v", err)
		return 2
	}

	// Compile the user's WASM module
	compiled, err := wz.CompileModule(wCtx, wasmBytes)
	if err != nil {
		r.printHostError("WASM compilation failed: %v", err)
		return 2
	}

	// Launch the background SSE event listener to buffer events purely for this runtime instance
	ctxSSE, cancelSSE := context.WithCancel(wCtx)
	defer cancelSSE()

	go func() {
		client.SubscribeToEvents(ctxSSE, r.daemonPort, func(e client.StreamEvent) { //nolint:errcheck
			r.eventMu.Lock()
			r.eventBuffer = append(r.eventBuffer, e)
			r.eventMu.Unlock()
		})
	}()

	// Setup WASI system configuration
	config := wazero.NewModuleConfig().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithStdin(os.Stdin).
		WithArgs("browsii-script")

	// Instantiate the module. In WASI, this automatically invokes the `_start` entrypoint.
	_, err = wz.InstantiateModule(wCtx, compiled, config)
	if err != nil {
		// Differentiate between a clean os.Exit() vs an unexpected crash
		if exitErr, ok := err.(*sys.ExitError); ok {
			exitCode := exitErr.ExitCode()
			if exitCode == 0 {
				return 0 // Normal execution
			}
			return 1 // Guest exited with non-zero
		}

		// If it's not a sys.ExitError, the WASM VM panicked or hit an unrecoverable trap
		r.printHostError("WASM execution trapped: %v\n", err)
		return 1
	}

	return 0
}

func (r *Runtime) printHostError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)

	// Structured JSON output for host-level errors (Exit Code 2)
	out := map[string]string{
		"error": msg,
		"type":  "host_runtime_error",
	}
	jsonBytes, _ := json.Marshal(out)
	fmt.Fprintln(os.Stderr, string(jsonBytes))
}

// flushEvents safely interrupts the single-threaded WASM module to dispatch any
// asynchronous events captured by the background SSE connection while the module yielded.
func (r *Runtime) flushEvents(ctx context.Context, m api.Module) {
	r.eventMu.Lock()
	anyEnabled := r.networkCallbacksEnabled || r.consoleCallbacksEnabled
	if !anyEnabled {
		r.eventMu.Unlock()
		return
	}
	networkEnabled := r.networkCallbacksEnabled
	consoleEnabled := r.consoleCallbacksEnabled
	events := r.eventBuffer
	r.eventBuffer = nil
	r.eventMu.Unlock()

	if len(events) == 0 {
		return
	}

	allocFn := m.ExportedFunction("allocate")
	if allocFn == nil {
		return
	}

	onNetReq := m.ExportedFunction("_on_network_request")
	onConsole := m.ExportedFunction("_on_console_event")

	for _, e := range events {
		var dispatchFn api.Function
		switch e.Type {
		case "network_request":
			if !networkEnabled || onNetReq == nil {
				continue
			}
			dispatchFn = onNetReq
		case "console":
			if !consoleEnabled || onConsole == nil {
				continue
			}
			dispatchFn = onConsole
		default:
			continue
		}

		b, err := json.Marshal(e.Payload)
		if err != nil {
			continue
		}

		// Dynamically allocate memory in the Guest heap
		res, err := allocFn.Call(ctx, uint64(len(b)))
		if err != nil || len(res) == 0 {
			continue
		}

		ptr := uint32(res[0])
		m.Memory().Write(ptr, b)

		// Fire the callback
		dispatchFn.Call(ctx, uint64(ptr), uint64(len(b))) //nolint:errcheck
	}
}
