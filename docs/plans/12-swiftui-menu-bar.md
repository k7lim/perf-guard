# Ticket 12: SwiftUI Menu Bar App

**Depends on**: Ticket 01 (livestatus package — defines the JSON contract), Ticket 00 (contracts)

**Can be built in parallel with**: Tickets 05-11 (forensic system). Only needs `live-status.json` to exist.

## Goal

Create a native macOS menu bar app that polls `~/.config/perf-guard/live-status.json` and shows memory pressure status as a color-coded icon with a click-to-expand dropdown.

## Context

This replaces the intrusive `osascript` notifications. The daemon writes `live-status.json` every poll cycle (ticket 04). This app reads it. The two processes never communicate directly — the file is the interface.

Kevin has an existing macOS app called `macstate` that follows similar patterns. The patterns below are borrowed from that project. You do NOT need access to the macstate source — all relevant patterns are documented in this ticket.

## Deliverables

### New directory: `menubar/`

```
menubar/
  build.sh                         -- Compile via xcrun swiftc, no Xcode project
  Info.plist                       -- LSUIElement=true (no dock icon)
  Sources/
    PerfGuardMenuApp.swift         -- @main, AppDelegate, StatusBarController setup
    LiveStatusModel.swift          -- Timer-based polling, JSON decoding
    StatusBarController.swift      -- NSStatusItem + NSPopover, icon drawing
    StatusView.swift               -- SwiftUI view for the dropdown content
```

**Target**: macOS 13+ (Ventura). Built with `xcrun swiftc` — no `.xcodeproj` needed.

### `build.sh`

```bash
#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

APP="PerfGuardMenu.app"
BINARY="PerfGuardMenu"

# Compile
xcrun swiftc \
  -target arm64-apple-macos13 \
  -framework Cocoa \
  -framework AppKit \
  -framework SwiftUI \
  -o "$BINARY" \
  Sources/*.swift

# Assemble app bundle (required for Info.plist / LSUIElement to take effect)
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
mv "$BINARY" "$APP/Contents/MacOS/"
cp Info.plist "$APP/Contents/"

echo "Built: $APP"
echo "Run:   open $APP"
```

**Why a bundle**: A bare binary ignores Info.plist. macOS reads `LSUIElement` from the app bundle's `Contents/Info.plist`. Without the bundle, the app gets a dock icon and standard window lifecycle — neither of which we want.

### `Info.plist`

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "...">
<plist version="1.0">
<dict>
    <key>LSUIElement</key>
    <true/>
    <key>CFBundleIdentifier</key>
    <string>com.user.perf-guard-menu</string>
    <key>CFBundleName</key>
    <string>PerfGuardMenu</string>
</dict>
</plist>
```

`LSUIElement=true` means no dock icon, no main window — menu bar only.

### `PerfGuardMenuApp.swift`

- `@main` struct conforming to `App`
- `@NSApplicationDelegateAdaptor(AppDelegate.self)` for lifecycle
- `AppDelegate.applicationDidFinishLaunching` creates `StatusBarController` and retains a reference to it (otherwise ARC will dealloc it and the status item vanishes)
- Use `Settings { EmptyView() }` as the scene body — this satisfies the `App` protocol requirement without creating a visible window. `LSUIElement=true` in the bundle prevents a dock icon.
- **Important**: Without a main window, the app stays alive because `NSStatusItem` keeps a strong reference chain. Do not call `NSApp.terminate` — the user quits via a "Quit" menu item in the popover.

### `LiveStatusModel.swift`

- `ObservableObject` class
- Published properties matching `contracts/live-status.json`:
  - `level: String` (default "unknown")
  - `compressionRate: Double`
  - `helperCount: Int`
  - `topProcesses: [TopProcess]`
  - `lastSpikeTime: String?`
  - `forensicCount: Int`
  - `suspended: [SuspendedBrief]`
  - `vmStopped: Bool`
  - `updatedAt: String`
  - `isStale: Bool` (true if updatedAt is >60s old or file missing)
  - `daemonRunning: Bool` (derived: file exists and not stale)
- **Polling**: `Timer.scheduledTimer` every 3 seconds on `.utility` queue
- **Diff before publish**: Only update `@Published` properties if values actually changed (avoid unnecessary SwiftUI re-renders)
- **Decoding**: `JSONDecoder` with `keyDecodingStrategy: .convertFromSnakeCase`. This auto-converts `schema_version` → `schemaVersion`, `compression_rate` → `compressionRate`, etc. Your Swift struct properties MUST use camelCase:

```swift
struct LiveStatus: Codable {
    let schemaVersion: Int  // Check this for forward compat: if > 1, show "update app"
    let level: String
    let compressionRate: Double
    let helperCount: Int
    let topProcesses: [TopProcess]
    let lastSpikeTime: String?
    let forensicCount: Int
    let suspended: [SuspendedBrief]
    let vmStopped: Bool
    let updatedAt: String
}

struct TopProcess: Codable {
    let pid: Int
    let name: String
    let rssBytes: UInt64
    let cpuPct: Double
}

struct SuspendedBrief: Codable {
    let pid: Int
    let name: String
}
```
- **Schema compat**: If `schema_version > 1`, set a `schemaTooNew: Bool` flag. Show "Update menu bar app" in UI.
- **Unknown fields**: Decoder ignores them (forward compatible).
- **File path**: `~/.config/perf-guard/live-status.json` (expand `~` via `FileManager.default.homeDirectoryForCurrentUser`)
- **File missing**: Set `daemonRunning = false`, show gray icon

### `StatusBarController.swift`

- Creates `NSStatusItem` with `NSStatusBar.system.statusItem(withLength: .variable)`
- **Icon**: `NSImage(size: CGSize(width: 18, height: 18), flipped: false)` custom drawing:
  - Green circle for `normal`
  - Yellow circle for `warning`
  - Orange circle for `high`
  - Red circle for `critical`
  - Gray circle when daemon not running / stale
- Optional: show compression rate as text next to icon (e.g., "842/s") using `NSFont.monospacedDigitSystemFont` for stable width
- **Click**: Toggle `NSPopover` with `.transient` behavior (auto-dismiss on click outside)
- Popover hosts `StatusView` via `NSHostingView`

### `StatusView.swift`

SwiftUI view shown in the popover dropdown:

```
┌─────────────────────────────────┐
│  Level: ●  WARNING              │
│  Compression: 842 pages/s       │
│  Browser Helpers: 127           │
│                                 │
│  Top Processes:                 │
│  1. Brave Helper   1.2 GB  34% │
│  2. node           512 MB  12% │
│  3. Code Helper    400 MB   8% │
│                                 │
│  Last Spike: 2 min ago          │
│  Snapshots: 7                   │
│                                 │
│  VM Stopped: No                 │
│  Suspended: 0 processes         │
└─────────────────────────────────┘
```

- Color-coded level indicator (circle + text)
- Compression rate with unit
- Top 5 processes with RSS (human-readable: MB/GB) and CPU%
- Last spike time (relative: "2 min ago", "never")
- Forensic snapshot count
- VM and suspension status
- If `schemaTooNew`: show "Daemon is newer than this app. Please update." banner
- If `!daemonRunning`: show "Daemon not running" with gray styling
- Use `NSFont.monospacedDigitSystemFont` equivalent in SwiftUI (`.monospacedDigit()`) for numeric values

## Concurrency model (P14)

**Single writer (Go daemon), single reader (this app)**. The daemon writes `live-status.json` atomically via temp file + `os.Rename`. This app polls every 3 seconds via `Timer`. No locking needed — the reader sees either the old file or the new file, never a partial write (APFS atomic rename guarantee).

**Accepted race**: The app may display data up to 3 seconds stale (one poll interval). This is fine for a status display.

**Stale detection**: If `updatedAt` is >60 seconds old, the daemon has likely stopped. Show gray icon + "Daemon not running."

## Patterns from macstate to use

These are known-good patterns from Kevin's existing macOS menu bar app:

1. `@NSApplicationDelegateAdaptor` → create `StatusBarController` in `applicationDidFinishLaunching`
2. `NSStatusItem` + `NSPopover(.transient)` for click-to-expand
3. `Timer.scheduledTimer` on `.utility` queue, diff before publishing
4. `NSImage(size:flipped:drawingHandler:)` for custom icon drawing
5. `NSFont.monospacedDigitSystemFont` for stable-width numbers

## Patterns from macstate to SKIP

1. All `*Service` classes — we read JSON, not system APIs
2. `IOKit` / `SMCService` — not needed
3. Process panels — forensic CLI handles this
4. Multi-segment status bar rendering — single icon + optional text

## Files touched

| File | Action |
|------|--------|
| `menubar/build.sh` | Create |
| `menubar/Info.plist` | Create |
| `menubar/Sources/PerfGuardMenuApp.swift` | Create |
| `menubar/Sources/LiveStatusModel.swift` | Create |
| `menubar/Sources/StatusBarController.swift` | Create |
| `menubar/Sources/StatusView.swift` | Create |

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.14 --append-notes "..." --json`

Leaf task — no downstream code dependencies. Consumes `live-status.json` written by ticket 04a/10.

## Acceptance criteria

- [ ] `./menubar/build.sh` compiles without errors and produces `PerfGuardMenu.app` bundle (on macOS with Xcode CLI tools)
- [ ] App appears in menu bar with colored circle icon, no dock icon
- [ ] Click shows popover with live data from `live-status.json`
- [ ] Icon color matches `level` field (green/yellow/orange/red/gray)
- [ ] Stale or missing file → gray icon + "Daemon not running"
- [ ] `schema_version > 1` → "Update menu bar app" message
- [ ] Numbers use monospaced digit font (no width jitter)
- [ ] Popover dismisses on click outside
- [ ] No crashes if JSON has unknown fields
- [ ] App bundle has correct structure: `PerfGuardMenu.app/Contents/{Info.plist,MacOS/PerfGuardMenu}`

**Note**: Criteria marked with UI behavior (icon appears, click shows popover, etc.) require manual verification on macOS. Build compilation is the automated gate.
