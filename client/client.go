package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	iclient "github.com/cdg-me/browsii/internal/client"
	"github.com/cdg-me/browsii/internal/daemon"
)

// Options configures a new Client.
type Options struct {
	// Port is the TCP port the daemon listens on.
	// If 0, a free port is chosen automatically.
	Port int

	// Mode is the browser launch mode passed to the daemon.
	// Supported values: "headful" (default), "headless", "user-headless", "user-headful".
	Mode string
}

// Client controls a browser daemon running in the same process.
type Client struct {
	port   int
	server *daemon.Server
}

// Start launches an in-process browser daemon and returns a connected Client.
// Call Stop (or defer c.Stop()) when done.
//
// Dev mode: if the BROWSII_PORT environment variable is set, Start attaches to
// an already-running daemon at that port instead of launching a new one. This
// lets you iterate on Go client code without restarting Chrome on every run.
// Stop is a no-op in this case — the daemon lifecycle is not owned.
func Start(opts Options) (*Client, error) {
	if envPort := os.Getenv("BROWSII_PORT"); envPort != "" {
		p, err := strconv.Atoi(envPort)
		if err != nil {
			return nil, fmt.Errorf("client: invalid BROWSII_PORT %q: %w", envPort, err)
		}
		return Attach(p)
	}
	if opts.Port == 0 {
		p, err := freePort()
		if err != nil {
			return nil, fmt.Errorf("client: find free port: %w", err)
		}
		opts.Port = p
	}
	if opts.Mode == "" {
		opts.Mode = "headful"
	}

	s := daemon.NewServer(opts.Port, opts.Mode)

	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	c := &Client{port: opts.Port, server: s}
	if err := c.waitReady(errCh); err != nil {
		s.Stop()
		return nil, fmt.Errorf("client: daemon did not become ready: %w", err)
	}
	return c, nil
}

// Attach connects to an already-running daemon at the given port and returns a
// Client that does not own the daemon lifecycle. Calling Stop on an attached
// Client is a no-op; the daemon keeps running.
func Attach(port int) (*Client, error) {
	c := &Client{port: port}
	if err := c.ping(); err != nil {
		return nil, fmt.Errorf("client: no daemon responding at port %d: %w", port, err)
	}
	return c, nil
}

// Stop gracefully shuts down the in-process daemon.
// It is a no-op when called on a Client created via Attach.
func (c *Client) Stop() {
	if c.server == nil {
		return
	}
	c.server.Stop()
}

// Port returns the TCP port the daemon is listening on.
// Useful when the port was auto-assigned (Options.Port == 0).
func (c *Client) Port() int {
	return c.port
}

// send posts payload (as JSON) to the daemon endpoint and returns the raw body.
func (c *Client) send(endpoint string, payload any) ([]byte, error) {
	return iclient.SendCommand(c.port, endpoint, payload)
}

// ping issues a single GET /ping and returns nil on a 200 response.
func (c *Client) ping() error {
	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("http://127.0.0.1:%d/ping", c.port), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// waitReady polls /ping until the daemon responds or times out.
func (c *Client) waitReady(errCh <-chan error) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			return err
		default:
		}
		if err := c.ping(); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("timed out after 15s")
}

// freePort asks the OS for an available TCP port.
func freePort() (int, error) {
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close() //nolint:errcheck
	return l.Addr().(*net.TCPAddr).Port, nil
}
