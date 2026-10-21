#!/bin/bash
# Test that the SwiftUI menu bar app compiles and has correct bundle structure
set -euo pipefail
cd "$(dirname "$0")/../menubar"

echo "=== Building menu bar app ==="
./build.sh

APP="PerfGuardMenu.app"

echo "=== Checking bundle structure ==="
test -d "$APP/Contents/MacOS" || { echo "FAIL: Missing Contents/MacOS"; exit 1; }
test -x "$APP/Contents/MacOS/PerfGuardMenu" || { echo "FAIL: Binary not executable"; exit 1; }
test -f "$APP/Contents/Info.plist" || { echo "FAIL: Missing Info.plist"; exit 1; }

echo "=== Checking LSUIElement ==="
/usr/libexec/PlistBuddy -c "Print :LSUIElement" "$APP/Contents/Info.plist" | grep -q "true" || { echo "FAIL: LSUIElement not true"; exit 1; }

rm -rf "$APP"
echo "Menu bar build tests passed"
