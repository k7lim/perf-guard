# Ticket 07: CDP Facade for Brave Tab Enumeration

**Depends on**: Ticket 03 (forensic config — for `CDPPort` setting)

## Goal

Create `internal/forensic/cdp.go` — a facade over Chrome DevTools Protocol that enumerates open Brave tabs. This translates CDP's upstream JSON into our own `TabInfo` type, isolating the rest of the codebase from CDP's response format.

## Context

Brave (Chromium-based) exposes a debug endpoint when launched with `--remote-debugging-port=9222`. HTTP GET to `http://localhost:9222/json` returns a JSON array of open targets (tabs, extensions, service workers, etc.).

This is a **facade** (not a client library). It makes one HTTP call, translates the response into our types, and returns. If CDP changes its response format, only this file changes. No WebSocket connections, no DevTools commands — just the `/json` list endpoint.

## Deliverables

### New file: `internal/forensic/cdp.go`

```go
package forensic

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type TabInfo struct {
    URL   string `json:"url"`
    Title string `json:"title"`
    Type  string `json:"type"` // "page", "background_page", "service_worker", etc.
}

// CaptureTabs connects to the Chrome DevTools Protocol endpoint and returns
// open browser targets. Returns (nil, err) if the port is unreachable or
// the response is unparseable. A 2-second timeout prevents blocking.
//
// port is the CDP debug port (typically 9222, from config.Forensic.CDPPort).
func CaptureTabs(port int) ([]TabInfo, error)
```

**Implementation**:

1. HTTP GET `http://localhost:{port}/json` with 2-second timeout via `http.Client{Timeout: 2 * time.Second}`
2. CDP returns an array like:
   ```json
   [
     {
       "description": "",
       "devtoolsFrontendUrl": "/devtools/...",
       "id": "ABC123",
       "title": "YouTube",
       "type": "page",
       "url": "https://youtube.com/watch?v=abc",
       "webSocketDebuggerUrl": "ws://..."
     }
   ]
   ```
3. Define an unexported `cdpTarget` struct with just the fields we need (`Title`, `Type`, `URL`). Unmarshal into `[]cdpTarget`.
4. Convert `[]cdpTarget` → `[]TabInfo`. Return ALL targets unfiltered — pages, service workers, background pages, everything. The `Type` field lets callers or report consumers filter downstream. Forensic completeness > clean output.
5. Return semantics: `(nil, err)` on failure (connection refused, timeout, non-200, bad JSON). `([]TabInfo{}, nil)` on success with zero tabs (empty array in response). This distinction matters — `nil` tabs = CDP unavailable, empty slice = CDP worked but no tabs open. Errors should be descriptive (include port number and HTTP status).

**Directory note**: `internal/forensic/` may not exist yet — create if needed.

### New file: `internal/forensic/cdp_test.go`

Test with `httptest.NewServer` returning known CDP JSON. Test cases:
- Valid response → correct TabInfo slice
- Empty array → empty slice, no error
- Connection refused (no server) → nil, error
- Non-200 status → nil, error with status code
- Malformed JSON → nil, error
- Unknown fields in CDP response are ignored (forward compat with Chromium changes)

## Design notes

- **No WebSocket**: We only need the tab list, not DevTools commands. `/json` is a plain HTTP GET.
- **2-second timeout**: CDP is local, should respond in milliseconds. 2s is generous. If it times out, Brave probably isn't running with the debug port.
- **Graceful failure**: This data source is optional. If Brave isn't running or CDP isn't enabled, the forensic snapshot just has `"tabs": null`. The caller (snapshot assembly, ticket 09) handles this.
- **No retry**: Single attempt. The forensic capture happens during a spike — we want speed, not reliability.

## Files touched

| File | Action |
|------|--------|
| `internal/forensic/cdp.go` | Create |
| `internal/forensic/cdp_test.go` | Create |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.7 --append-notes "..." --json`

Downstream: ticket 09 calls `CaptureTabs(cdpPort)` and uses `TabInfo` type.

## Acceptance criteria

- [ ] `go test ./internal/forensic/...` passes
- [ ] Parses real Chromium `/json` response format
- [ ] 2-second HTTP timeout
- [ ] Unreachable port returns `(nil, error)`, not panic
- [ ] Empty CDP array returns `([]TabInfo{}, nil)` — empty slice, NOT nil
- [ ] Unknown CDP fields ignored
- [ ] No WebSocket usage
- [ ] Port validated: `port <= 0` or `port > 65535` returns error without making HTTP call
