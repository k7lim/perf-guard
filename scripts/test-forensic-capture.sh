#!/bin/bash
# Test forensic CLI by seeding a snapshot file
set -euo pipefail
cd "$(dirname "$0")/.."

FORENSICS_DIR="$HOME/.config/perf-guard/forensics"
BACKUP_DIR=""
TIMESTAMP="2026-04-04T120000"
SNAPSHOT_FILE="$FORENSICS_DIR/$TIMESTAMP.json"

# Build
go build -o perf-guard ./cmd/perf-guard

# Backup existing forensics dir if present
if [ -d "$FORENSICS_DIR" ]; then
    BACKUP_DIR=$(mktemp -d)
    cp -r "$FORENSICS_DIR" "$BACKUP_DIR/"
fi

# Seed a test snapshot
mkdir -p "$FORENSICS_DIR"
cat > "$SNAPSHOT_FILE" << 'EOF'
{
  "schema_version": 1,
  "timestamp": "2026-04-04T12:00:00Z",
  "level": "critical",
  "compression_rate": 3200.5,
  "decompression_rate": 1800.2,
  "swap_in_rate": 45.0,
  "swap_out_rate": 120.3,
  "processes": [{"pid": 1234, "name": "test-process", "rss_bytes": 1000000, "vsz_bytes": 2000000, "cpu_pct": 50.0, "mem_pct": 2.0}],
  "tabs": [{"url": "https://example.com", "title": "Test", "type": "page"}],
  "io": {"read_bytes": 100, "write_bytes": 200, "source": "ioreg"},
  "vmstat": {}
}
EOF

echo "=== forensics list ==="
./perf-guard forensics list --output json | jq -e '.data.items | length > 0'

echo "=== forensics show ==="
./perf-guard forensics show "$TIMESTAMP" --output json | jq -e '.data.level == "critical"'

echo "=== forensics report ==="
./perf-guard forensics report --days 30 --output json | jq -e '.data.total_spikes > 0'

# Cleanup
rm -f "$SNAPSHOT_FILE"
if [ -n "$BACKUP_DIR" ]; then
    rm -rf "$FORENSICS_DIR"
    mv "$BACKUP_DIR/forensics" "$FORENSICS_DIR"
    rmdir "$BACKUP_DIR"
fi
rm -f perf-guard

echo "Forensic capture tests passed"
