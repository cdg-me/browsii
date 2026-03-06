package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/stretchr/testify/assert"
)

// TestProfilePersistence verifies that the UserDataDir is NOT deleted by go-rod's
// Cleanup() when we use the l.Delete(flags.UserDataDir) workaround, and that
// cookies set during one session survive into a second launch.
func TestProfilePersistence(t *testing.T) {
	server := setupMockServer()
	defer server.Close()

	profileDir := filepath.Join(os.TempDir(), "browsii-test-profile")
	os.RemoveAll(profileDir) // Ensure clean start
	defer os.RemoveAll(profileDir)

	// Session 1: Set a cookie, then close gracefully
	l1 := newLauncher().UserDataDir(profileDir).Headless(true)
	u1, err := l1.Launch()
	if err != nil {
		t.Fatalf("Session 1: failed to launch browser: %v", err)
	}
	l1.Delete(flags.UserDataDir) // Prevent Cleanup() from deleting dir

	browser1 := rod.New().ControlURL(u1).MustConnect().DefaultDevice(devices.Clear)
	page1 := browser1.MustPage(server.URL)
	page1.MustWaitLoad()

	// Set a cookie
	page1.MustEval(`() => document.cookie = "testauth=session123; path=/; max-age=86400"`)

	// Verify it's there
	cookieVal1 := page1.MustEval(`() => document.cookie`).String()
	assert.Contains(t, cookieVal1, "testauth=session123",
		"Session 1: cookie should be set immediately")

	// Graceful close flushes cookies to disk
	browser1.MustClose()

	// Session 2: Relaunch with same profile, verify cookie survived
	l2 := newLauncher().UserDataDir(profileDir).Headless(true)
	u2, err := l2.Launch()
	if err != nil {
		t.Fatalf("Session 2: failed to launch browser: %v", err)
	}
	l2.Delete(flags.UserDataDir)

	browser2 := rod.New().ControlURL(u2).MustConnect().DefaultDevice(devices.Clear)
	defer browser2.MustClose()

	page2 := browser2.MustPage(server.URL)
	page2.MustWaitLoad()

	cookieVal2 := page2.MustEval(`() => document.cookie`).String()
	assert.Contains(t, cookieVal2, "testauth=session123",
		"Session 2: cookie should persist across browser restarts when using the same UserDataDir")
}
