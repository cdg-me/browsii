package daemon

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
)

// EventType categorizes the stream events
type EventType string

const (
	EventNetworkRequest EventType = "network_request"
	EventConsole        EventType = "console"
	EventOverflow       EventType = "overflow_warning"
)

// StreamEvent is the JSON payload sent over SSE
type StreamEvent struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// contextState holds the page state for a named browser context.
type contextState struct {
	browser *rod.Browser // incognito browser context
	page    *rod.Page
}

// RecordedEvent represents a single captured action.
type RecordedEvent struct {
	T      int64                  `json:"t"`
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// Server holds the state for the running browser daemon.
type Server struct {
	port         int
	mode         string
	browser      *rod.Browser
	server       *http.Server
	activePg     *rod.Page
	capturedReqs []map[string]interface{}
	capturing    bool
	mu           sync.Mutex
	contexts     map[string]*contextState // named browser contexts
	activeCtx    string                   // current active context name ("" = default)
	// Recording state
	recording    bool
	recordName   string
	recordStart  time.Time
	recordEvents []RecordedEvent
	recMu        sync.Mutex

	// Stable insertion-order tab list. browser.Pages() returns tabs in
	// Chrome's internal order (by TargetID), not creation order, so we
	// maintain our own slice to give tabs stable, predictable indices.
	pageOrder []proto.TargetTargetID

	// listenedPages tracks which pages already have a network listener so that
	// attachNetworkListener is idempotent (one listener per page, ever).
	listenedPages map[proto.TargetTargetID]struct{}

	// captureTabFilter is the tab index to capture (-1 = all tabs).
	captureTabFilter int

	// consoleListenedPages tracks which pages already have a console listener
	// so that attachConsoleListener is idempotent (one listener per page, ever).
	consoleListenedPages map[proto.TargetTargetID]struct{}

	// consoleCapturing controls whether console entries are buffered.
	consoleCapturing bool

	// consoleCapturedEntries holds entries collected during an active capture session.
	consoleCapturedEntries []map[string]interface{}

	// consoleTabFilter is the tab index to capture (-1 = all tabs).
	consoleTabFilter int

	// consoleLevelFilter is a comma-separated allowlist of levels ("" = all).
	consoleLevelFilter string

	// SSE Broadcasting
	sseClients map[chan StreamEvent]struct{}
	sseMu      sync.RWMutex
}

// recordAction captures an action event if recording is active.
func (s *Server) recordAction(action string, params map[string]interface{}) {
	if !s.recording {
		return
	}
	s.recMu.Lock()
	defer s.recMu.Unlock()
	s.recordEvents = append(s.recordEvents, RecordedEvent{
		T:      time.Since(s.recordStart).Milliseconds(),
		Action: action,
		Params: params,
	})
	return
}

// NewServer creates a new Daemon configuration.
func NewServer(port int, mode string) *Server {
	return &Server{
		port:                 port,
		mode:                 mode,
		sseClients:           make(map[chan StreamEvent]struct{}),
		listenedPages:        make(map[proto.TargetTargetID]struct{}),
		captureTabFilter:     -1,
		consoleListenedPages: make(map[proto.TargetTargetID]struct{}),
		consoleTabFilter:     -1,
	}
}

// Start visualizes the browser based on mode and boots the local API server.
func (s *Server) Start() error {
	log.Printf("Starting browsii daemon on port %d in mode: %s\n", s.port, s.mode)

	// 1. Configure the Launcher based on mode
	l := launcher.New()

	// user-* modes attach to system Chrome and use a persistent profile.
	if s.mode == "user-headful" || s.mode == "user-headless" {
		path, ok := launcher.LookPath()
		if ok {
			l = l.Bin(path)
		}
		homeDir, err := os.UserHomeDir()
		if err == nil {
			userDataDir := filepath.Join(homeDir, ".browsii", "profile")
			l = l.UserDataDir(userDataDir)
			log.Printf("Using persistent automation profile: %s", userDataDir)
		}
	}

	// headful modes show a visible window; all others run headless.
	if s.mode == "headful" || s.mode == "user-headful" {
		l = l.Headless(false)
	} else {
		l = l.Headless(true)
	}

	// 2. Launch the browser
	u, err := l.Launch()
	if err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	// Prevent go-rod's Cleanup() from deleting the persistent profile directory on exit.
	if s.mode == "user-headful" || s.mode == "user-headless" {
		l.Delete(flags.UserDataDir)
	}

	// Disable the default 1200x900 viewport binding to let the page fill the physical window
	s.browser = rod.New().ControlURL(u).MustConnect().DefaultDevice(devices.Clear)
	log.Println("Browser launched and connected successfully.")

	// Register signal handler so Ctrl+C / SIGTERM always kills Chrome cleanly
	s.HandleSignals()

	// Watch for browser disconnect (e.g. user quits Chrome from Dock)
	go func() {
		<-s.browser.GetContext().Done()
		log.Println("Browser disconnected (quit from Dock or crashed). Shutting down daemon...")
		s.Stop()
		os.Exit(0)
	}()

	// 3. Start the HTTP API Server (Localhost only)
	mux := http.NewServeMux()
	s.registerCoreRoutes(mux)
	s.registerTabRoutes(mux)
	s.registerNavigationRoutes(mux)
	s.registerInteractionRoutes(mux)
	s.registerMouseRoutes(mux)
	s.registerContentRoutes(mux)
	s.registerNetworkRoutes(mux)
	s.registerConsoleRoutes(mux)
	s.registerSessionRoutes(mux)
	s.registerRecordRoutes(mux)
	s.registerContextRoutes(mux)

	s.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: recoverMiddleware(mux),
	}

	log.Printf("Daemon API listening on http://127.0.0.1:%d\n", s.port)
	return s.server.ListenAndServe()
}
