package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
)

// newLauncher returns a rod launcher with CI-appropriate flags pre-applied.
func newLauncher() *launcher.Launcher {
	l := launcher.New()
	if os.Getenv("CI") != "" {
		l = l.Set("no-sandbox")
	}
	return l
}

// portCounter provides unique ports across all tests to avoid collisions.
var portCounter int64 = 9900

// nextPort returns a unique port for a test.
func nextPort() int {
	return int(atomic.AddInt64(&portCounter, 1))
}

// binPath returns the absolute path to the compiled browsii binary.
func binPath(t *testing.T) string {
	t.Helper()
	cwd, _ := os.Getwd()
	p := filepath.Join(filepath.Dir(cwd), "browsii")
	_, err := os.Stat(p)
	require.NoError(t, err, "Compiled binary not found. Run 'go build -o browsii cmd/browsii/*.go' first.")
	return p
}

// startDaemon launches the daemon on the given port in headless mode.
// Returns a cleanup function that stops the daemon and kills the process.
func startDaemon(t *testing.T, port int) (bin string, cleanup func()) {
	t.Helper()
	bin = binPath(t)
	startCmd := exec.Command(bin, "start", "--port", fmt.Sprintf("%d", port), "--mode", "headless")
	err := startCmd.Start()
	require.NoError(t, err, "Failed to start daemon")

	cleanup = func() {
		exec.Command(bin, "stop", "--port", fmt.Sprintf("%d", port)).Run() //nolint:errcheck
		if startCmd.Process != nil {
			startCmd.Process.Kill() //nolint:errcheck
		}
	}

	// Poll /ping until the daemon is ready instead of sleeping a fixed duration.
	pingURL := fmt.Sprintf("http://127.0.0.1:%d/ping", port)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(pingURL) //nolint:noctx
		if err == nil {
			resp.Body.Close() //nolint:errcheck
			if resp.StatusCode == http.StatusOK {
				return bin, cleanup
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("daemon did not become ready within 10 seconds")
	return bin, cleanup
}

// runCLI executes a CLI command and returns the combined output as a string.
// It fails the test if the command returns a non-zero exit code.
func runCLI(t *testing.T, bin string, port int, args ...string) string {
	t.Helper()
	fullArgs := append(args, "--port", fmt.Sprintf("%d", port))
	cmd := exec.Command(bin, fullArgs...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "CLI command %v failed: %s", args, string(out))
	return string(out)
}

// runCLIExpectFail executes a CLI command and returns the output.
// It does NOT fail if the command returns a non-zero exit code.
func runCLIExpectFail(t *testing.T, bin string, port int, args ...string) (string, error) {
	t.Helper()
	fullArgs := append(args, "--port", fmt.Sprintf("%d", port))
	cmd := exec.Command(bin, fullArgs...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// setupMockServer creates a local HTTP server serving the standard test bed HTML.
func setupMockServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `
			<!DOCTYPE html>
			<html>
			<head>
				<title>Test Bed</title>
				<style>
					body { font-family: sans-serif; padding: 20px; }
					#target-box { width: 200px; height: 100px; background: blue; color: white; }
				</style>
			</head>
			<body>
				<h1 id="header">Browser CLI Test Bed</h1>
				<p>This is a dummy page to verify go-rod interactions.</p>
				<input type="text" id="inputBox" placeholder="Type here...">
				<div id="target-box">Click Me</div>
				<div id="far-box" style="position: absolute; top: 2000px; left: 2000px; width: 50px; height: 50px; background: red;">Far Box</div>
				<script>
					document.getElementById('target-box').addEventListener('click', function() {
						this.style.backgroundColor = 'green';
						this.innerText = 'Clicked!';
					});
					document.getElementById('far-box').addEventListener('click', function() {
						this.style.backgroundColor = 'green';
						this.innerText = 'Far Clicked!';
					});
				</script>
			</body>
			</html>
		`)
	})
	return httptest.NewServer(mux)
}

// setupNamedServer creates a mock server that identifies itself with the given name.
func setupNamedServer(name string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head><title>%s</title></head><body><h1 id="identity">I am %s</h1></body></html>`, name, name) //nolint:errcheck
	}))
}

// setupTallServer creates a mock server with a very tall page (5000px) for scroll testing.
func setupTallServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Tall Page</title></head><body style="height:5000px"><h1>Top</h1><div style="position:absolute;bottom:0">Bottom</div></body></html>`) //nolint:errcheck
	}))
}

// setupConsoleServer returns a server whose root page fires console calls at all
// standard levels. The /multi path fires console.log with multiple arguments.
func setupConsoleServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/multi":
			fmt.Fprint(w, `<html><body><script>console.log("alpha", "beta", 42);</script></body></html>`) //nolint:errcheck
		default:
			_, _ = fmt.Fprint(w, `<html><body><script>
				console.log("hello log");
				console.warn("hello warn");
				console.error("hello error");
				console.info("hello info");
				console.debug("hello debug");
			</script></body></html>`)
		}
	}))
}

// setupFetchServer returns a server whose root page immediately fires fetch(fetchPath)
// (no setTimeout — synchronous script execution during page load), and fetchPath
// responds with {"ok":true}. Use a unique fetchPath per test to distinguish events.
func setupFetchServer(fetchPath string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == fetchPath {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"ok":true}`) //nolint:errcheck
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><script>fetch('%s')</script></body></html>`, fetchPath) //nolint:errcheck
	}))
}
