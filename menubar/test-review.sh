#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
test_dir="${TMPDIR:-/tmp}/perf-guard-review-ui"
mkdir -p "$test_dir"
xcrun swiftc -module-cache-path /tmp/perf-guard-swift-module-cache \
  -target arm64-apple-macos13 -framework AppKit -framework SwiftUI \
  -o "$test_dir/ReviewHarness" \
  menubar/Sources/ReviewModel.swift menubar/Sources/ReviewView.swift menubar/Tests/ReviewHarness.swift
"$test_dir/ReviewHarness" "$@"
