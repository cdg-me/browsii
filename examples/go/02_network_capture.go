//go:build ignore

// Network capture example — demonstrates --include and --format options.
//
// Run with: go run examples/go/02_network_capture.go
//
// Scenarios covered:
//  1. Base capture (default) — url, method, type, tab only
//  2. --include request-headers — outgoing headers per request
//  3. --include response-headers — status + mimeType + response headers
//  4. --include request-*,response-* — all groups via wildcards (+ fixed receive timing)
//  5. --format har --output — writes a HAR 1.2 file to disk
//  6. --include response-body — full response body text
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cdg-me/browsii/client"
)

const targetURL = "https://example.com"

func main() {
	c, err := client.Start(client.Options{Mode: "headless"})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Stop()

	fmt.Printf("Daemon running on port %d\n\n", c.Port())

	// ── Scenario 1: base capture (backward-compatible default) ───────────────
	scenario("1: base capture (url, method, type, tab only)")

	if err := c.NetworkCaptureStart(client.NetworkCaptureOpts{}); err != nil {
		log.Fatal(err)
	}
	if err := c.Navigate(targetURL); err != nil {
		log.Fatal(err)
	}
	result, err := c.NetworkCaptureStop()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  captured %d requests\n", result.Count)
	for _, r := range result.Requests[:min(3, len(result.Requests))] {
		fmt.Printf("  [tab %d] %s %s\n", r.Tab, r.Method, r.URL)
		// Confirm no optional fields are present
		if r.RequestHeaders != nil || r.Status != 0 {
			log.Fatal("UNEXPECTED: optional fields populated without --include")
		}
	}

	// ── Scenario 2: --include request-headers ────────────────────────────────
	scenario("2: --include request-headers")

	if err := c.NetworkCaptureStart(client.NetworkCaptureOpts{
		Include: []string{"request-headers"},
	}); err != nil {
		log.Fatal(err)
	}
	if err := c.Navigate(targetURL); err != nil {
		log.Fatal(err)
	}
	result, err = c.NetworkCaptureStop()
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range result.Requests[:min(2, len(result.Requests))] {
		fmt.Printf("  %s %s\n", r.Method, r.URL)
		// Print a couple of representative headers
		for _, key := range []string{"User-Agent", "Accept", "user-agent", "accept"} {
			if v, ok := r.RequestHeaders[key]; ok {
				fmt.Printf("    %s: %s\n", key, truncate(v, 70))
				break
			}
		}
	}

	// ── Scenario 3: --include response-headers ────────────────────────────────
	scenario("3: --include response-headers  (status, mimeType, headers)")

	if err := c.NetworkCaptureStart(client.NetworkCaptureOpts{
		Include: []string{"response-headers"},
	}); err != nil {
		log.Fatal(err)
	}
	if err := c.Navigate(targetURL); err != nil {
		log.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // let NetworkResponseReceived events arrive
	result, err = c.NetworkCaptureStop()
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range result.Requests[:min(3, len(result.Requests))] {
		fmt.Printf("  [%d %s] %s\n", r.Status, r.StatusText, r.URL)
		if r.MimeType != "" {
			fmt.Printf("    mimeType: %s\n", r.MimeType)
		}
	}

	// ── Scenario 4: wildcards request-* + response-* ─────────────────────────
	scenario("4: --include 'request-*,response-*'  (all groups, timing, size)")

	if err := c.NetworkCaptureStart(client.NetworkCaptureOpts{
		Include: []string{"request-*", "response-*"},
	}); err != nil {
		log.Fatal(err)
	}
	if err := c.Navigate(targetURL); err != nil {
		log.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	result, err = c.NetworkCaptureStop()
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range result.Requests[:min(3, len(result.Requests))] {
		fmt.Printf("  [%d] %s %s\n", r.Status, r.Method, r.URL)
		if r.Timing != nil {
			fmt.Printf("    dns=%.1fms  connect=%.1fms  wait=%.1fms\n",
				nonNeg(r.Timing.DNS), nonNeg(r.Timing.Connect), nonNeg(r.Timing.Wait))
		}
		if r.TransferSize != nil {
			fmt.Printf("    transferSize=%d bytes\n", *r.TransferSize)
		}
		if r.Timestamp > 0 {
			ts := time.Unix(int64(r.Timestamp), 0)
			fmt.Printf("    started: %s\n", ts.Format(time.RFC3339))
		}
	}

	// ── Scenario 5: HAR format, written to file ───────────────────────────────
	scenario("5: --format har --output /tmp/capture.har")

	harFile := "/tmp/browsii_example.har"
	if err := c.NetworkCaptureStart(client.NetworkCaptureOpts{
		Include: []string{"request-headers", "response-headers", "response-timing", "response-size"},
		Format:  "har",
		Output:  harFile,
	}); err != nil {
		log.Fatal(err)
	}
	if err := c.Navigate(targetURL); err != nil {
		log.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	result, err = c.NetworkCaptureStop()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  wrote %d entries → %s\n", result.Count, result.OutputPath)

	// Read and validate the HAR
	raw, err := os.ReadFile(harFile)
	if err != nil {
		log.Fatal(err)
	}
	var har struct {
		Log struct {
			Version string `json:"version"`
			Entries []struct {
				StartedDateTime string `json:"startedDateTime"`
				Request         struct {
					Method string `json:"method"`
					URL    string `json:"url"`
				} `json:"request"`
				Response struct {
					Status int `json:"status"`
				} `json:"response"`
				Timings struct {
					Wait float64 `json:"wait"`
				} `json:"timings"`
			} `json:"entries"`
		} `json:"log"`
	}
	if err := json.Unmarshal(raw, &har); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  HAR version %s — %d entries\n", har.Log.Version, len(har.Log.Entries))
	for _, e := range har.Log.Entries[:min(3, len(har.Log.Entries))] {
		fmt.Printf("  [%d] %s %s\n", e.Response.Status, e.Request.Method, e.Request.URL)
		if e.Timings.Wait > 0 {
			fmt.Printf("    wait=%.1fms\n", e.Timings.Wait)
		}
	}
	os.Remove(harFile)

	// ── Scenario 6: --include response-body ───────────────────────────────────
	scenario("6: --include response-body  (full response body text)")

	if err := c.NetworkCaptureStart(client.NetworkCaptureOpts{
		Include: []string{"response-headers", "response-body"},
	}); err != nil {
		log.Fatal(err)
	}
	if err := c.Navigate(targetURL); err != nil {
		log.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // allow LoadingFinished + body RPC to complete
	result, err = c.NetworkCaptureStop()
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range result.Requests[:min(3, len(result.Requests))] {
		fmt.Printf("  [%d] %s\n", r.Status, r.URL)
		if r.ResponseBody != "" {
			preview := r.ResponseBody
			if len(preview) > 100 {
				preview = preview[:100] + "…"
			}
			if r.ResponseBodyEncoded {
				fmt.Printf("    body (base64, %d bytes): %s\n", len(r.ResponseBody), preview)
			} else {
				fmt.Printf("    body (%d chars): %s\n", len(r.ResponseBody), preview)
			}
		}
	}

	fmt.Println("\n✓ All scenarios complete")
}

func scenario(label string) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("Scenario %s\n", label)
	fmt.Println(strings.Repeat("─", 60))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func nonNeg(f float64) float64 {
	if f < 0 {
		return 0
	}
	return f
}
