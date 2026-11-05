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
  -framework ServiceManagement \
  -o "$BINARY" \
  Sources/*.swift

# Assemble app bundle (required for Info.plist / LSUIElement to take effect)
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
mv "$BINARY" "$APP/Contents/MacOS/"
cp Info.plist "$APP/Contents/"

# Build perf-guard CLI binary and place next to .app so the app can find it
echo "Building perf-guard CLI..."
(cd .. && go build -o "menubar/perf-guard" ./cmd/perf-guard)
cp perf-guard "$APP/Contents/MacOS/perf-guard"
/usr/bin/codesign --force --deep --sign - "$APP"

echo "Built: $APP"
echo "Run:   open $APP"
