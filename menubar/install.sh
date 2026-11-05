#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
target="$HOME/Applications/Performance Guard.app"
if [[ "${1:---dry-run}" == "--dry-run" ]]; then
  echo "Would build and install: $target"
  echo "Would register Performance Guard to open at login and launch its menu bar."
  echo "The existing background monitor and its mode are unchanged."
  exit 0
fi
if [[ "${1:-}" != "--apply" ]]; then
  echo "Usage: $0 [--dry-run|--apply]" >&2
  exit 2
fi
./build.sh
mkdir -p "$(dirname "$target")"
if [[ -e "$target" ]]; then
  backup="$HOME/Library/Application Support/perf-guard/backups/menu-$(date +%Y%m%dT%H%M%S)"
  mkdir -p "$backup"
  /usr/bin/ditto "$target" "$backup/Performance Guard.app"
  echo "Backup: $backup"
fi
/usr/bin/ditto PerfGuardMenu.app "$target"
/usr/bin/codesign --verify --deep --strict "$target"
/usr/bin/open "$target" --args --enable-login
echo "Launched $target; inspect Login Items if macOS requests approval."
