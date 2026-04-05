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
