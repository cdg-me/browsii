package tests

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpload_SetsFileInput(t *testing.T) {
	// Create a temp file to upload
	tmpFile, err := os.CreateTemp("", "upload_test_*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name()) //nolint:errcheck
	tmpFile.WriteString("hello from upload test") //nolint:errcheck
	tmpFile.Close()                               //nolint:errcheck

	// Server with a file input form that reads the filename via JS
	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			// Handle multipart upload
			r.ParseMultipartForm(10 << 20) //nolint:errcheck
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			defer file.Close() //nolint:errcheck
			body, _ := io.ReadAll(file)
			fmt.Fprintf(w, "Received: %s (%d bytes)", header.Filename, len(body)) //nolint:errcheck
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>
			<form id="uploadForm" enctype="multipart/form-data">
				<input type="file" id="fileInput" name="file">
				<div id="result">no file</div>
			</form>
			<script>
				document.getElementById('fileInput').addEventListener('change', function(e) {
					document.getElementById('result').innerText = 'selected: ' + e.target.files[0].name;
				});
			</script>
		</body></html>`)
	}))
	defer uploadServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	runCLI(t, bin, port, "navigate", uploadServer.URL)
	runCLI(t, bin, port, "upload", "#fileInput", tmpFile.Name())

	// Check that the file was selected
	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('result').innerText")
	assert.Contains(t, jsOut, "selected:", "File input should have a file selected")
	assert.Contains(t, jsOut, filepath.Base(tmpFile.Name()), "Should show the uploaded filename")
}

func TestNavigate_WaitUntil(t *testing.T) {
	// Server that loads slowly (delayed resource)
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><head><title>Wait Test</title></head><body>
			<div id="content">Loaded</div>
			<script>
				var loaded = false;
				setTimeout(function() {
					loaded = true;
					document.getElementById('content').innerText = 'Fully Loaded';
				}, 100);
			</script>
		</body></html>`)
	}))
	defer slowServer.Close()

	port := nextPort()
	bin, cleanup := startDaemon(t, port)
	defer cleanup()

	// Navigate with --wait-until networkidle
	runCLI(t, bin, port, "navigate", slowServer.URL, "--wait-until", "load")

	jsOut := runCLI(t, bin, port, "js", "() => document.getElementById('content').innerText")
	assert.Contains(t, jsOut, "Loaded", "Page should be loaded after wait-until")
}
