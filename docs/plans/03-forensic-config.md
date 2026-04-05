# Ticket 03: Forensic Config Additions

**Depends on**: Nothing (independent)

## Goal

Add a `[forensic]` section to the config system so downstream tickets (spike detector, CDP facade, snapshot storage) can read their settings from `config.toml`.

## Context

`internal/config/config.go` defines a `Config` struct (line 13) parsed from TOML. The existing sections are `[polling]`, `[thresholds]`, and `[groups]`. We need to add `[forensic]` with settings for the forensic capture system.

## Deliverables

### Modify `internal/config/config.go`

1. **Add `ForensicConfig` struct**:

```go
type ForensicConfig struct {
    Enabled    bool `toml:"enabled"`
    CDPPort    int  `toml:"cdp_port"`
    FSUsage    bool `toml:"fs_usage"`
    MaxKeep    int  `toml:"max_keep"`
    Cooldown   int  `toml:"cooldown"`    // seconds between captures
    WindowSize int  `toml:"window_size"` // rolling window for spike detection
}
```

2. **Add `Forensic ForensicConfig` field to `Config` struct** (line 13-19):

```go
type Config struct {
    Polling    PollingConfig          `toml:"polling"`
    Thresholds ThresholdConfig        `toml:"thresholds"`
    Protect    []string               `toml:"protect"`
    Groups     map[string]GroupConfig  `toml:"groups"`
    LogPath    string                 `toml:"log_path"`
    Forensic   ForensicConfig         `toml:"forensic"`
}
```

3. **Set defaults in `Default()` function** (line 44):

```go
Forensic: ForensicConfig{
    Enabled:    true,
    CDPPort:    9222,
    FSUsage:    false,
    MaxKeep:    100,
    Cooldown:   30,
    WindowSize: 10,
},
```

4. **Add validation in `validate()`** (line 106):
   - `Cooldown` must be >= 5 (prevent capture storms)
   - `WindowSize` must be >= 3 (need baseline data)
   - `MaxKeep` must be >= 1
   - `CDPPort` must be > 0 and <= 65535

### Modify `config/default.toml`

Add the new section:

```toml
[forensic]
enabled = true
cdp_port = 9222
fs_usage = false
max_keep = 100
cooldown = 30
window_size = 10
```

## Existing code to reference

- `internal/config/config.go` — full file. Module: `github.com/k7lim/perf-guard/internal/config`
- `Default()` at line 44 sets defaults for all config sections
- `validate()` at line 106 validates post-parse
- Uses `github.com/BurntSushi/toml` for parsing

## Files touched

| File | Action |
|------|--------|
| `internal/config/config.go` | Edit — add `ForensicConfig` struct, field, defaults, validation |
| `config/default.toml` | Edit — add `[forensic]` section |

## Testing

**No existing `config_test.go`** — create `internal/config/config_test.go`. Test cases:

- `Default()` returns correct forensic defaults (Enabled=true, CDPPort=9222, Cooldown=30, WindowSize=10, MaxKeep=100, FSUsage=false)
- `validate()` rejects: Cooldown=3, WindowSize=2, MaxKeep=0, CDPPort=0, CDPPort=70000
- `validate()` accepts: Cooldown=5, WindowSize=3, MaxKeep=1, CDPPort=9222
- TOML round-trip: load a config with `[forensic]` section → correct struct values
- Missing `[forensic]` section in TOML → zero-value ForensicConfig (Enabled=false, etc.) — this is fine because `Default()` fills in before `LoadFrom` overlays

**Onboarding**: The existing validation pattern in `validate()` (line 106) uses `return fmt.Errorf(...)` for the first failing check. Follow the same pattern — return early on first error.

## Scratchpad

Use the bead's notes field for progress: `bd update perf-ect-j0x.3 --append-notes "..." --json`

Downstream: tickets 05 (WindowSize, Cooldown), 07 (CDPPort), 08 (FSUsage), 10 (all fields) read `cfg.Forensic.*`.

## Acceptance criteria

- [ ] `go build ./...` compiles
- [ ] `go test ./internal/config/...` passes
- [ ] `config.Default().Forensic.Enabled == true`
- [ ] `config.Default().Forensic.CDPPort == 9222`
- [ ] Validation rejects `Cooldown < 5`, `WindowSize < 3`, `MaxKeep < 1`
- [ ] Existing config loading still works (new section is optional in TOML)
