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
browsii scrape --format markdown --port 9222     # html | text | markdown
browsii get-links --pattern "github.com" --port 9222  # JSON array of hrefs
browsii js "document.title" --port 9222          # bare expression, returns JSON
browsii js "({url: location.href, h1: document.querySelector('h1')?.textContent})" --port 9222
browsii cookies --port 9222                      # JSON array of cookie objects
browsii screenshot out.png --port 9222
browsii screenshot out.png --element "#chart" --port 9222
browsii screenshot out.png --full-page --port 9222
browsii pdf out.pdf --port 9222
```

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
browsii session new --port 9222    # wipe state and start fresh

# Recordings capture every action for replay
browsii record start myflow --port 9222
# ... perform actions ...
browsii record stop --port 9222
browsii record replay myflow --speed 2.0 --port 9222   # 0=instant, 1=realtime
browsii record list --port 9222

# Isolated browser contexts (incognito)
browsii context create ctx-a --port 9222
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

---

## Key behaviours

**Daemon is stateful.** It holds the browser, tabs, and all capture buffers. Multiple CLI commands share state through the same daemon instance.

**One active tab.** All commands (click, scrape, js, etc.) operate on the active tab. Use `tab switch` or `TabSwitch` to change it.

**`js` auto-wraps bare expressions.** `browsii js "document.title"` is equivalent to `browsii js "() => document.title"`. Named functions and arrow functions pass through unchanged.

**Capture is destructive.** Calling `network capture stop` / `console capture stop` returns and clears the buffer. A second call returns an empty array.

**`session save`** persists cookies and localStorage — not the actual tab URLs. Use it to checkpoint auth state between runs.

**Profile vs session.** `profile setup` is for interactive login to persist credentials permanently. `session save/resume` is for saving and restoring automation state programmatically.
