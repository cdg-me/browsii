//go:build ignore

// Inject JS example — register scripts that run before any page code.
//
// Run with: go run examples/go/03_inject_js.go
//
// Scenarios covered:
//  1. Global inject — script fires before inline page scripts on every navigation
//  2. Multiple adds — all fire, in registration order
//  3. --url eager fetch — content inlined at add time; origin can go offline
//  4. Tab-scoped inject — only the targeted tab receives the script
//  5. Global applies to new tabs — tabs opened after add() pick it up
//  6. Clear — scripts stop firing on future navigations; current page unaffected
//  7. List — inspect registered entries before clearing
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/cdg-me/browsii/client"
)

const target = "https://example.com"

func main() {
	c, err := client.Start(client.Options{Mode: "headless"})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Stop()

	fmt.Printf("Daemon running on port %d\n\n", c.Port())

	// ── Scenario 1: global inject fires before inline page scripts ────────────
	scenario("1: global inject — fires before any page-owned script")

	id, err := c.InjectJSAdd(
		`window.__sdk = true; window.__sdkEnv = "staging";`,
		"", // tab="" = all tabs
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  registered %s\n", id)

	if err := c.Navigate(target); err != nil {
		log.Fatal(err)
	}

	result, err := c.JS(`() => ({ sdk: window.__sdk, env: window.__sdkEnv })`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  globals after navigate: %s\n", result)

	// ── Scenario 2: multiple adds fire in registration order ──────────────────
	scenario("2: multiple adds — execution order matches registration order")

	if err := c.InjectJSClear(""); err != nil {
		log.Fatal(err)
	}

	for i, script := range []string{
		`window.__order = []; window.__order.push("first");`,
		`window.__order.push("second");`,
		`window.__order.push("third");`,
	} {
		id, err := c.InjectJSAdd(script, "")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  registered inject-%d as %s\n", i+1, id)
	}

	if err := c.Navigate(target); err != nil {
		log.Fatal(err)
	}

	order, err := c.JS(`() => window.__order`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  execution order: %s\n", order)

	// ── Scenario 3: --url fetches eagerly at registration time ────────────────
	scenario("3: InjectJSAddURL — content inlined at add time")

	if err := c.InjectJSClear(""); err != nil {
		log.Fatal(err)
	}

	scriptSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprint(w, `window.__fromURL = "fetched-eagerly";`) //nolint:errcheck
	}))

	id, err = c.InjectJSAddURL(scriptSrv.URL, "")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  registered %s (URL content inlined)\n", id)

	scriptSrv.Close()

	if err := c.Navigate(target); err != nil {
		log.Fatal(err)
	}

	urlResult, err := c.JS(`() => window.__fromURL`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  window.__fromURL (origin offline): %s\n", urlResult)

	// ── Scenario 4: tab-scoped inject — only one tab receives it ─────────────
	scenario("4: tab-scoped inject (tab=\"active\") — other tabs unaffected")

	if err := c.InjectJSClear(""); err != nil {
		log.Fatal(err)
	}

	if err := c.Navigate(target); err != nil {
		log.Fatal(err)
	}

	id, err = c.InjectJSAdd(`window.__tabScoped = "tab0-only";`, "active")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  registered %s for active tab only\n", id)

	if err := c.TabNew(target); err != nil {
		log.Fatal(err)
	}

	tab1Result, err := c.JS(`() => window.__tabScoped`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  tab 1 __tabScoped (should be null): %s\n", tab1Result)

	if err := c.TabSwitch(0); err != nil {
		log.Fatal(err)
	}
	if err := c.Navigate(target); err != nil {
		log.Fatal(err)
	}

	tab0Result, err := c.JS(`() => window.__tabScoped`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  tab 0 __tabScoped (should be \"tab0-only\"): %s\n", tab0Result)

	// ── Scenario 5: global inject applies to tabs opened after add ────────────
	scenario("5: global inject — new tabs opened after registration pick it up")

	if err := c.InjectJSClear(""); err != nil {
		log.Fatal(err)
	}

	id, err = c.InjectJSAdd(`window.__global = "everywhere";`, "")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  registered global %s\n", id)

	if err := c.TabNew(target); err != nil {
		log.Fatal(err)
	}

	newTabResult, err := c.JS(`() => window.__global`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  new tab __global (should be \"everywhere\"): %s\n", newTabResult)

	// ── Scenario 6: list — inspect registered entries ─────────────────────────
	scenario("6: InjectJSList — inspect what's registered")

	if err := c.InjectJSClear(""); err != nil {
		log.Fatal(err)
	}

	c.InjectJSAdd(`window.__x = 1;`, "")       //nolint:errcheck
	c.InjectJSAdd(`window.__y = 2;`, "active") //nolint:errcheck

	entries, err := c.InjectJSList("")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  %d registered entries:\n", len(entries))
	for _, e := range entries {
		scope := e.Tab
		if scope == "" {
			scope = "all"
		}
		preview := e.Script
		if len(preview) > 50 {
			preview = preview[:50] + "…"
		}
		fmt.Printf("    %s [tab=%s] %s\n", e.ID, scope, preview)
	}

	// ── Scenario 7: clear — scripts stop on future navigations ───────────────
	scenario("7: InjectJSClear — scripts stop firing; current page unaffected")

	if err := c.InjectJSClear(""); err != nil {
		log.Fatal(err)
	}

	title, err := c.JS(`() => document.title`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  current page title (unaffected by clear): %s\n", title)

	if err := c.Navigate(target); err != nil {
		log.Fatal(err)
	}

	afterClear, err := c.JS(`() => ({ x: window.__x, y: window.__y })`)
	if err != nil {
		log.Fatal(err)
	}

	var vals map[string]interface{}
	json.Unmarshal(afterClear, &vals) //nolint:errcheck
	fmt.Printf("  __x after clear (should be null): %v\n", vals["x"])
	fmt.Printf("  __y after clear (should be null): %v\n", vals["y"])

	fmt.Println("\n✓ All scenarios complete")
}

func scenario(label string) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("Scenario %s\n", label)
	fmt.Println(strings.Repeat("─", 60))
}
