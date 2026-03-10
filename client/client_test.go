package client_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/cdg-me/browsii/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okServer returns an httptest.Server that responds 200 to every request.
func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// serverPort extracts the port number from an httptest.Server URL.
func serverPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	p, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return p
}

// freePort returns a port that is not currently in use.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestAttach_Succeeds(t *testing.T) {
	srv := okServer(t)
	c, err := client.Attach(serverPort(t, srv))
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestAttach_ReturnsCorrectPort(t *testing.T) {
	srv := okServer(t)
	port := serverPort(t, srv)
	c, err := client.Attach(port)
	require.NoError(t, err)
	assert.Equal(t, port, c.Port())
}

func TestAttach_FailsWhenNothingListening(t *testing.T) {
	_, err := client.Attach(freePort(t))
	require.Error(t, err)
}

func TestAttach_FailsOnNon200Response(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	_, err := client.Attach(serverPort(t, srv))
	require.Error(t, err)
}

func TestStop_IsNoOpOnAttachedClient(t *testing.T) {
	srv := okServer(t)
	port := serverPort(t, srv)

	c, err := client.Attach(port)
	require.NoError(t, err)

	c.Stop() // must not panic, must not shut down the external server

	// Server should still respond after Stop.
	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
