package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage the persistent automation profile",
}

var setupCmd = &cobra.Command{
	Use:   "setup [url]",
	Short: "Launch a native, foreground Chrome instance for manual login",
	Long: `Opens a standard Chrome window pointing directly at the CLI's dedicated ~/.browsii/profile data directory.
	
This runs in the foreground. Log into any web services you wish the agent to have access to. 

You can optionally provide a startup URL to automatically navigate the window upon opening.

When you are finished, simply close the browser window normally. The authenticated session state (cookies, localStorage, etc) will be saved to the profile.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to resolve user home directory: %v", err)
		}

		userDataDir := filepath.Join(homeDir, ".browsii", "profile")

		path, ok := launcher.LookPath()
		if !ok {
			log.Fatal("Could not find a native Chrome installation on this system.")
		}

		fmt.Println("==================================================")
		fmt.Printf(" Launching Native Chrome Profile: %s\n", userDataDir)
		fmt.Println("==================================================")
		fmt.Println("\nINSTRUCTIONS:")
		fmt.Println(" 1. The browser window will appear shortly.")
		fmt.Println(" 2. Navigate and log into any services you need (GitHub, forums, etc).")
		fmt.Println(" 3. When finished, completely close the browser window (Cmd+Q or red 'x').")
		fmt.Println("\nWaiting for browser to be closed natively...")

		// Launch explicitly in foreground
		l := launcher.New().
			Bin(path).
			UserDataDir(userDataDir).
			Headless(false)

		u, err := l.Launch()
		if err != nil {
			log.Fatalf("Failed to launch profile setup window: %v", err)
		}

		// Prevent go-rod's Cleanup() from deleting the profile directory on exit.
		// Cleanup() calls os.RemoveAll(l.Get(flags.UserDataDir)), so clearing it makes it a no-op.
		l.Delete(flags.UserDataDir)

		// Connect briefly to force a visible page, circumventing viewport limits
		browser := rod.New().ControlURL(u).MustConnect().DefaultDevice(devices.Clear)

		var page *rod.Page
		if len(args) > 0 {
			page = browser.MustPage(args[0])
		} else {
			page = browser.MustPage("about:blank")
		}

		// Track the page's target ID so we know when our specific page is destroyed
		pageTargetID := page.TargetID

		// Wait for the user to close our specific page/tab
		wait := browser.EachEvent(func(e *proto.TargetTargetDestroyed) bool {
			return e.TargetID == pageTargetID
		})
		wait()

		fmt.Println("\nPage closed. Flushing session state to disk...")

		// Gracefully close the browser via CDP. This sends Browser.close which gives
		// Chrome time to flush in-memory cookies/state to the SQLite DB on disk.
		browser.MustClose()

		// Small grace period for the OS to finish writing
		time.Sleep(500 * time.Millisecond)

		fmt.Println("Profile saved successfully. You can now use `--mode user-headful`.")
	},
}

func init() {
	profileCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(profileCmd)
}
