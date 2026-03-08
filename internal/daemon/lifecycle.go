package daemon

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// Stop gracefully closes the browser and the HTTP server.
func (s *Server) Stop() {
	log.Println("Stopping daemon...")
	if s.browser != nil {
		s.browser.MustClose()
		log.Println("Browser closed.")
	}
	if s.server != nil {
		s.server.Shutdown(context.Background()) //nolint:errcheck
		log.Println("HTTP server shutdown.")
	}
}

// HandleSignals listens for SIGINT/SIGTERM and cleanly stops the daemon.
// This ensures Chrome is killed even if the parent script is Ctrl+C'd.
func (s *Server) HandleSignals() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Received signal %s, shutting down...\n", sig)
		s.Stop()
		os.Exit(0)
	}()
}
