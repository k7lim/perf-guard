#!/bin/bash
# Fixture-only CLI -> production Swift model acceptance. No live target needed.
set -euo pipefail
cd "$(dirname "$0")/.."
export GOPATH=/tmp/perf-guard-go GOMODCACHE=/tmp/perf-guard-go-mod GOCACHE=/tmp/perf-guard-go-cache
acceptance_dir="$(mktemp -d "${TMPDIR:-/tmp}/perf-guard-m1-parity.XXXXXX")"
trap 'rm -rf "$acceptance_dir"' EXIT
go test -c -o "$acceptance_dir/cli-tests" ./cmd/perf-guard
python3 - "$acceptance_dir" <<'PY'
import json
import os
from pathlib import Path
import subprocess
import sys

directory = Path(sys.argv[1])
environment = dict(os.environ, PERF_GUARD_CLI_TEST_CHILD="1", OUTPUT_FORMAT="json")
base = [str(directory / "cli-tests"), "-test.run=^TestReviewCLIChild$", "--"]

def run(name, arguments):
    result = subprocess.run(base + arguments, env=environment, cwd="cmd/perf-guard", check=True, capture_output=True)
    envelope = json.loads(result.stdout)
    assert envelope["success"] is True
    (directory / name).write_bytes(result.stdout)
    return envelope["data"]

review = run("review.json", ["review", "--output", "json"])
arguments = ["review", "plan", "--output", "json"]
for record in review["records"]:
    if record["reviewable"]:
        arguments += ["--session", record["id"]]
plan = run("plan.json", arguments)
assert plan["execution_supported"] is False
assert len(plan["selections"]) == 2
PY
bash menubar/test-review.sh --parity "$acceptance_dir"
