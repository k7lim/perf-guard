#!/bin/bash
# Smoke test: build and run basic perf-guard commands
set -euo pipefail
cd "$(dirname "$0")/.."

echo "=== Building perf-guard ==="
go build -o perf-guard ./cmd/perf-guard

echo "=== status ==="
./perf-guard status --output json | jq -e '.success == true'

echo "=== describe ==="
./perf-guard describe --output json | jq -e '.commands.forensics'

echo "=== forensics list (empty is OK) ==="
./perf-guard forensics list --output json | jq -e '.success == true'

echo "=== forensics report ==="
./perf-guard forensics report --output json | jq -e '.success == true'

rm -f perf-guard
echo "All smoke tests passed"
