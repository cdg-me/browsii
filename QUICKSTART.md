# browsii quickstart

**browsii** is a browser automation tool designed for use by LLMs and Go programs. It wraps a persistent Chromium instance (via go-rod) behind a local HTTP daemon and exposes three usage modes.

---

## Which mode to use

| Situation | Mode |
|---|---|
| LLM issuing discrete browser actions (tool-calling, shell script) | **CLI** |
| Portable, event-driven script (reacts to network/console events, distributable as `.wasm`) | **WASM** |
| Go program that needs browser automation as a library | **Go client** |

**CLI** is the right default. It requires a running daemon and maps every action to one command invocation.

**WASM** is for scripts that need to run as a continuous program — for example, intercepting all network requests during a page load, or reacting to console errors. Scripts compile to `.wasm` via TinyGo and run inside a sandbox managed by the CLI.

**Go client** starts the daemon in-process (no separate binary or shell command). Best when browser automation is one component of a larger Go program.

---

## Mode 1 — CLI

### Daemon lifecycle

```sh
browsii start --port 9222                   # start daemon (headful by default)
# ... issue commands ...
browsii stop --port 9222
```

Modes: `headful` (default, go-rod bundled Chromium, visible), `headless` (bundled, invisible), `user-headless` (system Chrome + persistent profile, headless), `user-headful` (system Chrome + persistent profile, visible).

Set `BROWSII_BIN=/path/to/chrome` to launch any Chromium-based browser instead of the bundled one (signed system builds also avoid macOS ScreenCapture re-consent prompts during screenshots).

The `--port` / `-p` flag defaults to `8000` and is required for every command when using a non-default port.

### Navigation

```sh
browsii navigate "https://example.com" --port 9222
browsii navigate "https://example.com" --wait-until networkidle --port 9222
browsii reload --port 9222
```

### Interaction

```sh
browsii click "#submit-btn" --port 9222
browsii type "#search" "hello world" --port 9222
browsii press "Enter" --port 9222          # also: Control+a, Shift+Tab, Escape, etc.
browsii hover ".dropdown" --port 9222
browsii scroll --down --pixels 500 --port 9222   # --up / --top / --bottom
```

### Forms (fill / select / check)

```sh
# Fill several fields in one call; values work with framework inputs (React, Vue)
browsii fill --field '{"selector":"#email","value":"user@example.com"}' \
             --field '{"ref":2,"value":"Jane"}' --port 9222
browsii fill --field '{"selector":"#email","value":"user@example.com"}' --submit --port 9222

# Select an option by value or label
browsii select "#size" "Large" --port 9222

# Checkboxes and radios (real clicks; idempotent)
browsii check "#tos" --port 9222
browsii check "#tos" --off --port 9222
```

Failures are per field: other fields still apply, and failures list
candidate elements with refs for an immediate retry.

### Element map (refs)

`elements` lists every interactive element with a numbered ref — the preferred
way to act on a page when you don't know its selectors:

```sh
browsii elements --port 9222
# [1] link "Home" -> /home (#logo)
# [2] textbox "Email address" (#email)
# [3] button "Sign in" (#signin)

browsii click 3 --port 9222          # bare numbers are element refs
browsii type 2 "user@example.com" --port 9222
browsii elements --filter "sign in" --port 9222   # substring over text/name/role/selector
browsii elements --all --port 9222   # include hidden elements (marked "hidden")
browsii elements --json --port 9222  # raw JSON with rects, values, checked state
```

Refs stay valid until the page changes. A stale ref fails fast with a hint to
re-run `elements`.

Shadow DOM and iframes are pierced automatically: elements inside open
shadow roots and same-origin iframes appear in `elements` with chained
selectors (`#host >>> #inner`), and every command accepts them — click, fill,
type, expect, element, record/replay.

### Actionable errors

When click/type/hover fail, the error tells you what to do next — similar
elements are listed with ready-to-use refs:

```
Click failed: element not found: .submit
hint: similar elements listed below — retry with a ref or a corrected selector
  [3] button "Submit order" selector: body > button:nth-of-type(2)
  [4] button "Submit payment" (disabled) selector: body > button:nth-of-type(3)
```

### Dialogs (alert / confirm / prompt / beforeunload)

Dialogs never stall the session: the daemon auto-handles them per policy and
reports what happened. `click`, `press`, `navigate`, and `js` print any dialog
they trigger inline.

```sh
browsii click "#delete" --port 9222
# Successfully clicked '#delete'
# Dialog dismissed (confirm): "Delete this item?"

browsii dialogs --port 9222                    # policy + recently handled dialogs
browsii dialogs --policy accept --port 9222    # confirm() returns true from now on
browsii dialogs --policy accept --prompt-text "Alice" --port 9222   # prompt() input
browsii dialogs --clear --port 9222            # forget history
```

Default policy is `dismiss`: `confirm()` returns false, and a dismissed
`beforeunload` cancels the navigation that triggered it.

### Verifying actions (expect + receipts)

Every click/press/navigate returns a receipt of what the action actually
caused — navigation, requests fired, dialogs, console errors:

```sh
browsii click "#submit" --port 9222
# Successfully clicked '#submit'
# → navigated to https://app.example.com/orders/123
# ⟳ 2 request(s): POST /api/orders (201), GET /api/orders/123 (200)
```

Use `--no-evidence` on any of them to skip the settle window when speed
matters more than verification.

`expect` verifies outcomes and doubles as the wait primitive for SPA updates —
it polls until the condition holds (default 5s) and fails with actionable
diffs, not just "timeout":

```sh
browsii expect --text "Saved" --port 9222                       # text visible
browsii expect --text-gone "Loading…" --port 9222               # text disappeared
browsii expect --url-pattern "*/orders/*" --port 9222           # URL glob
  browsii expect --selector ".results" --port 9222                # element visible
  browsii expect --selector ".spinner" --hidden --port 9222       # element hidden/gone
  browsii expect --selector "#buy" --enabled --port 9222          # element enabled (also --disabled)
browsii expect --ref 3 --value "user@x.com" --port 9222         # input value equals
browsii expect --request "POST */api/order*" --port 9222        # request fired
browsii expect --no-console-errors --port 9222                  # no error-level console entries
browsii expect --text "Saved" --timeout 10000 --port 9222       # custom budget
```

`--no-console-errors` may accompany any condition. `--request` observes
traffic without a capture session and looks back 10s, so the natural flow is
`click` then `expect --request`. Failures explain themselves:

```
browsii expect --request "POST */api/orders" 
expect failed: request matched POST */api/orders — timed out after 5000ms
requests that fired:
  GET /api/session (200)
  GET /api/cart (200)
hint: the expected request did not fire — check the action that should have triggered it
```

The act → receipt → expect loop is the intended verification workflow:
actions report evidence, expect independently asserts outcomes.

### Mouse

```sh
browsii mouse move 640 400 --port 9222
browsii mouse drag 100 100 400 300 --steps 20 --port 9222
browsii mouse right-click ".item" --port 9222
browsii mouse double-click ".item" --port 9222
```

### Tabs

```sh
browsii tab new "https://example.com" --port 9222
browsii tab list --port 9222          # → JSON [{index, id, url, title}]
browsii tab switch 1 --port 9222      # zero-based index
browsii tab close --port 9222
```

### Content extraction

```sh
browsii scrape --format markdown --port 9222     # html | text | markdown | readable
browsii scrape --format readable --port 9222     # article text only (nav/footer stripped)
browsii find "regulatory" --port 9222            # grep page text: count + matching lines
browsii find --regex '/Total: \$\d+/' --port 9222
browsii element "#checkout" --port 9222          # one element's attrs/state/rect
browsii get-links --pattern "github.com" --port 9222  # JSON array of hrefs
browsii js "document.title" --port 9222          # bare expression, returns JSON
browsii js "({url: location.href, h1: document.querySelector('h1')?.textContent})" --port 9222
browsii cookies --port 9222                      # JSON array of cookie objects
browsii screenshot out.png --port 9222
browsii screenshot out.png --element "#chart" --port 9222
browsii screenshot out.png --full-page --port 9222
browsii pdf out.pdf --port 9222
```

`readable` uses Mozilla's Readability (Firefox Reader View). It fails with
a clear error on pages without article-like content — fall back to `text`.
`text` includes shadow-DOM and iframe content; `html` and `markdown` cover
the light DOM only. `find` searches page text (shadow and iframe content
included) and exits 1 on zero matches (grep convention); for interactive
controls use `elements --filter`.

### Network & console capture

```sh
# Capture all requests, then stop and collect
browsii network capture start --port 9222
browsii navigate "https://example.com" --port 9222
browsii network capture stop --port 9222   # → JSON [{url, method, type, tab}]

# Only capture requests from the tab opened next
browsii network capture start --tab next --port 9222
# Tab aliases: active | next | last | <index 0-N> | (omit = all)

# Throttle (bytes/sec, -1 = unlimited)
browsii network throttle --latency 100 --download 50000 --port 9222

# Mock a URL pattern
browsii network mock --pattern "*/api/users*" --body '{"users":[]}' \
  --content-type application/json --status 200 --port 9222

# Console capture
browsii console capture start --level "error,warn" --port 9222
browsii navigate "https://example.com" --port 9222
browsii console capture stop --port 9222   # → JSON [{level, text, args, tab}]
```

### Sessions & recording

```sh
# Sessions persist cookies + localStorage + tabs to ~/.browsii/sessions/
browsii session save mysession --port 9222
browsii session resume mysession --port 9222
browsii session list --port 9222
browsii session delete mysession --port 9222
browsii session new fresh --port 9222    # wipe state and start fresh

# Recordings capture every action for replay
browsii record start myflow --port 9222
# ... perform actions (expect calls are recorded as checkpoints) ...
browsii record stop --port 9222
browsii record replay myflow --port 9222   # instant by default
browsii record export myflow --port 9222   # Playwright spec
browsii record list --port 9222

# Isolated browser contexts (incognito)
browsii context create --name ctx-a --port 9222
browsii context switch ctx-a --port 9222
browsii context switch default --port 9222
```

### Persistent auth profile

```sh
# Opens a real Chrome window so you can log in manually.
# Credentials are saved to ~/.browsii/profile and reused by the daemon.
browsii profile setup "https://github.com"
```

---

## Parallel browser sessions

### Pattern 1 — Multiple daemons (true parallelism)

Run one daemon per session on different ports. Each has its own browser window and is fully independent. Drive them concurrently from separate terminals, backgrounded shell jobs, or parallel LLM tool calls.

```sh
browsii start --port 9001   # session A
browsii start --port 9002   # session B

# Issue commands to each independently
browsii navigate "https://site-a.com" -p 9001
browsii navigate "https://site-b.com" -p 9002

browsii scrape -p 9001
browsii scrape -p 9002

browsii stop -p 9001
browsii stop -p 9002
```

Best for: genuinely concurrent work, different accounts, independent visible windows side-by-side.

### Pattern 2 — Browser contexts (isolated identities, one daemon)

`context create` opens an incognito context inside the running daemon — separate cookies, storage, and login state. Switch between them with `context switch`. Note: contexts share one daemon so switching is sequential, not concurrent.

```sh
browsii start --port 9222

# Create two isolated contexts
browsii context create --name alice -p 9222
browsii navigate "https://app.example.com" -p 9222   # logged in as alice

browsii context create --name bob -p 9222
browsii navigate "https://app.example.com" -p 9222   # fresh session for bob

# Switch back and forth
browsii context switch alice -p 9222
browsii scrape -p 9222   # alice's view

browsii context switch bob -p 9222
browsii scrape -p 9222   # bob's view

browsii context switch default -p 9222   # back to main context
browsii stop -p 9222
```

Best for: multi-user flows, comparing auth states, A/B testing without multiple browser processes.

---

## Mode 2 — WASM (TinyGo)

Scripts are regular Go files compiled to `wasip1` by TinyGo. The CLI manages the daemon and sandbox.

### Setup

```sh
browsii install-runtimes   # installs TinyGo SDK to ~/.browsii/sdk (once)
```

### Run a script

```sh
browsii run examples/wasm/01_basics.go   # compiles with TinyGo and runs
browsii run script.wasm                   # run pre-compiled binary directly
```

### SDK

```go
//go:build ignore   // omit for files you want to compile with TinyGo

package main

import sdk "browsii/sdk"   // module path written by install-runtimes

func main() {
    sdk.Navigate("https://example.com")
    sdk.WaitVisible("h1")
    sdk.WaitIdle(500)
    sdk.Click("#accept")

    // Return structured data to the CLI host
    sdk.SetResult(map[string]any{
        "title": "scraped",
    })
}
```

**SDK surface:**

| Function | Description |
|---|---|
| `Navigate(url string) error` | Navigate active tab |
| `Click(selector string) error` | Click element |
| `WaitVisible(selector string) error` | Block until element is in DOM |
| `WaitIdle(ms int) error` | Pause for ms milliseconds |
| `SetResult(v any)` | JSON-encode v and return to host |
| `OnNetworkRequest(cb func(NetworkEvent))` | Callback per browser request |
| `OnConsoleEvent(cb func(ConsoleEvent))` | Callback per console.log call |

**Event types:**
```go
type NetworkEvent struct { URL, Method, Type string; Tab int }
type ConsoleEvent  struct { Level, Text string; Tab int; Args []ConsoleArg }
```

WASM is the right choice when you need `OnNetworkRequest` / `OnConsoleEvent` callbacks (the CLI capture commands only collect requests, whereas WASM can react to them in real-time).

---

## Mode 3 — Go client package

The daemon runs in-process. No CLI binary or separate process needed.

```go
import "github.com/cdg-me/browsii/client"

c, err := client.Start(client.Options{
    // Mode: "headful" is the default; also: "headless", "user-headless", "user-headful"
    // Port: 0 picks a free port automatically
})
if err != nil { log.Fatal(err) }
defer c.Stop()

// Navigation
c.Navigate("https://example.com")
c.Reload()
c.Back()
c.Forward()
c.Scroll("down", 300)   // "up" | "down" | "top" | "bottom"
c.Upload("#file", []string{"/tmp/file.pdf"})

// Interaction
c.Click("#btn")
c.Type("#input", "hello")
c.Press("Control+a")
c.Hover(".menu")
c.MouseMove(640, 400)
c.MouseDrag(100, 100, 400, 300, 20)
c.MouseRightClick(".item")
c.MouseDoubleClick(".item")

// Forms
c.Fill([]client.FillField{
    {Selector: "#email", Value: "user@example.com"},
    {Ref: 2, Value: "Jane"},
}, true)                                        // fields, submit
selected, _ := c.Select("#size", "Large")       // value or label; → ["l"]
state, _ := c.Check("#tos", true)               // → CheckResult{Was, Now}

// Reading
article, _ := c.Readable()                      // article text, clutter stripped
hits, _ := c.Find("regulatory", "")             // or regex: c.Find("", '/Total: \$\d+/')
detail, _ := c.ElementDetailOf("#checkout")     // attrs, form, rect, state

// Element map (refs)
list, _ := c.Elements(client.ElementOpts{Filter: "sign in"})
// list.Elements[i] = client.Element{Ref, Tag, Role, Text, Name, Selector, Visible, ...}
c.ClickRef(list.Elements[0].Ref)
c.TypeRef(list.Elements[0].Ref, "hello")

// Dialogs
state, _ := c.Dialogs()                     // policy + recent history
c.SetDialogPolicy("accept", "Alice", false) // policy, prompt text, clear history

// Verification
c.Expect(client.ExpectOpts{Text: "Saved"})                        // waits until visible
c.Expect(client.ExpectOpts{Request: "POST */api/order*"})         // request fired
receipt, _ := c.ClickWithEvidence("#submit")                      // action receipt
// receipt.Navigated, receipt.URL, receipt.RequestSamples, receipt.ConsoleErrors

// Tabs
c.TabNew("https://example.com")
tabs, _ := c.TabList()           // []client.Tab{Index, ID, URL, Title}
c.TabSwitch(1)
c.TabClose()

// Content
text, _  := c.Scrape(client.Markdown)   // client.HTML | client.Text | client.Markdown
links, _ := c.Links("github.com")        // []string
result, _ := c.JS("document.title")      // json.RawMessage
cookies, _ := c.Cookies()               // []map[string]any
c.Screenshot("out.png", "#chart", false) // filename, element (or ""), fullPage
c.PDF("out.pdf")

// Network capture
c.NetworkCaptureStart("all")    // "all" | "active" | "next" | "last" | "0"
c.Navigate("https://example.com")
reqs, _ := c.NetworkCaptureStop()   // []client.NetworkRequest{URL,Method,Type,Tab}

c.NetworkThrottle(100, 50000, -1)   // latency ms, download B/s, upload B/s
c.NetworkMock("*/api/*", `{"ok":true}`, "application/json", 200)

// Console capture
c.ConsoleCaptureStart("all", "error,warn")
entries, _ := c.ConsoleCaptureStop()    // []client.ConsoleEntry{Level,Text,Tab,Args}

// Async event subscriptions (block until ctx cancelled)
ctx, cancel := context.WithCancel(context.Background())
go c.OnNetworkRequest(ctx, func(r client.NetworkRequest) { fmt.Println(r.URL) })
go c.OnConsoleEvent(ctx, func(e client.ConsoleEntry) { fmt.Println(e.Text) })
cancel()

// Sessions
c.SessionSave("mysession")
c.SessionResume("mysession")
sessions, _ := c.SessionList()   // []client.ListEntry{Name, Modified}
c.SessionDelete("mysession")
c.SessionNew("")

// Recording
c.RecordStart("myflow")
c.RecordStop()   // client.RecordStopResult{Name, Events}
c.RecordReplay("myflow", 1.0)
recordings, _ := c.RecordList()
c.RecordDelete("myflow")

// Isolated contexts
c.ContextCreate("ctx-a")
c.ContextSwitch("ctx-a")
c.ContextSwitch("default")

fmt.Printf("Daemon on port %d\n", c.Port())
```

### Dev mode — iterating on Go client code

Every `go run .` calls `client.Start()`, which cold-boots Chrome and loses all session state (cookies, login, tabs). To iterate without that:

```sh
# Start Chrome once, log in manually if needed
browsii start --port 8000 --mode headful

# Every iteration — attach to the running Chrome, run, exit
browsii dev --port 8000 -- go run .
```

`browsii dev` sets `BROWSII_PORT=8000` in the subprocess environment. `client.Start()` sees it and calls `client.Attach()` instead of launching Chrome. No code changes needed. Chrome and its session persist across runs.

`client.Stop()` is a no-op when attached — it does not kill the daemon.

`--watch` re-runs the command on any `.go` file change. Useful if a human is watching a terminal; LLMs should just call `browsii dev -- go run .` directly each iteration.

---

## Snapshots — testing without a live site

Record the real network responses once, replay them offline in every test run. No auth, no flakiness, no dependency on external sites.

### Record

```sh
browsii network capture start --include response-headers,response-body --format har --output testdata/snap.har
browsii navigate "https://example.com/page"
browsii network capture stop
```

Or from the Go client:

```go
c.NetworkCaptureStart(client.NetworkCaptureOpts{
    Include: []string{"response-headers", "response-body"},
    Format:  "har",
    Output:  "testdata/snap.har",
})
c.Navigate("https://example.com/page")
c.NetworkCaptureStop()
```

### Replay

```sh
browsii snapshot load testdata/snap.har   # intercepts matching URLs on the active page
browsii navigate "https://example.com/page"
browsii scrape                             # returns recorded content, no network needed
browsii snapshot clear                     # restore normal network behaviour
```

```go
c.SnapshotLoad("testdata/snap.har")
c.Navigate("https://example.com/page")
text, _ := c.Scrape(client.Markdown)
c.SnapshotClear()
```

URLs not present in the HAR pass through to the network unchanged — sub-resources from a CDN that weren't recorded will still be fetched live.

The snapshot is scoped to the active page at load time. It persists until `snapshot clear` or the daemon restarts.

### Typical test pattern

```go
func TestScrapeNotifications(t *testing.T) {
    c, _ := client.Start(client.Options{Mode: "headless"})
    defer c.Stop()

    c.SnapshotLoad("testdata/github_notifications.har")
    c.Navigate("https://github.com/notifications")

    titles, _ := scrapeUnreadPRTitles(c)   // the function under test
    assert.Equal(t, []string{"fix: race condition", "feat: dark mode"}, titles)
}
```

To update the fixture, delete the HAR and re-record against the live site with a logged-in session.

---

## Key behaviours

**Daemon is stateful.** It holds the browser, tabs, and all capture buffers. Multiple CLI commands share state through the same daemon instance.

**One active tab.** All commands (click, scrape, js, etc.) operate on the active tab. Use `tab switch` or `TabSwitch` to change it.

**`js` auto-wraps bare expressions.** `browsii js "document.title"` is equivalent to `browsii js "() => document.title"`. Named functions and arrow functions pass through unchanged.

**Element refs are per-page-state.** Refs from `elements` are valid until the page navigates or the DOM changes materially; a stale ref fails fast with a hint to re-run `elements`. When in doubt, re-enumerate — it is cheap.

**Interaction errors are actionable.** Failed click/type/hover return similar elements with refs. Treat the candidate list as the retry menu rather than re-guessing selectors.

**Dialogs are auto-handled, never blocking.** alert/confirm/prompt/beforeunload are resolved per the current policy (default: dismiss) and reported inline by the action that triggered them. A dismissed beforeunload cancels its navigation.

**Actions carry receipts.** click/press/navigate append what the action caused: navigation, requests (up to 5 samples), dialogs, console-error count. `expect` then independently asserts outcomes — the two compose into a verifiable act→check loop.

**Replay is fingerprint-healed, not selector-bound.** Recordings store each target element's fingerprint (tag, role, text, name, href) plus its position among identical siblings. On replay, a selector that no longer matches its element is healed by relocating the fingerprint; the substitution is reported (`healed: step 2 #p2-add → …`). A semantic change — the element is gone or relabelled — fails at that step with the original identity. Record with `--capture-har` and replay runs fully offline against the recorded traffic (`--live` opts out); `--session <name>` restores a saved login first. `record export` writes a Playwright spec with role-based locators that keeps the same healing properties.

**Capture is destructive.** Calling `network capture stop` / `console capture stop` returns and clears the buffer. A second call returns an empty array.

**`session save`** persists cookies and localStorage — not the actual tab URLs. Use it to checkpoint auth state between runs.

**Profile vs session.** `profile setup` is for interactive login to persist credentials permanently. `session save/resume` is for saving and restoring automation state programmatically.
