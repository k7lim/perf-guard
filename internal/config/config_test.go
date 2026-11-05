package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultForensicConfig(t *testing.T) {
	cfg := Default()
	if !cfg.Forensic.Enabled {
		t.Error("Forensic.Enabled = false, want true")
	}
	if cfg.Forensic.CDPPort != 9222 {
		t.Errorf("Forensic.CDPPort = %d, want 9222", cfg.Forensic.CDPPort)
	}
	if cfg.Forensic.Cooldown != 30 {
		t.Errorf("Forensic.Cooldown = %d, want 30", cfg.Forensic.Cooldown)
	}
	if cfg.Forensic.WindowSize != 10 {
		t.Errorf("Forensic.WindowSize = %d, want 10", cfg.Forensic.WindowSize)
	}
	if cfg.Forensic.MaxKeep != 100 {
		t.Errorf("Forensic.MaxKeep = %d, want 100", cfg.Forensic.MaxKeep)
	}
	if cfg.Forensic.FSUsage {
		t.Error("Forensic.FSUsage = true, want false")
	}
}

func TestMonitorOnlyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("monitor_only = true\nprotect = [\"custom-protected-app\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.MonitorOnly || len(cfg.Protect) != 1 || cfg.Protect[0] != "custom-protected-app" {
		t.Fatalf("config overlay lost: %#v", cfg)
	}
}

func TestValidateForensicRejects(t *testing.T) {
	tests := []struct {
		name string
		mod  func(*Config)
	}{
		{"cooldown too low", func(c *Config) { c.Forensic.Cooldown = 3 }},
		{"window_size too low", func(c *Config) { c.Forensic.WindowSize = 2 }},
		{"cdp_port negative", func(c *Config) { c.Forensic.CDPPort = -1 }},
		{"cdp_port too high", func(c *Config) { c.Forensic.CDPPort = 70000 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mod(&cfg)
			if err := cfg.validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestValidateForensicAccepts(t *testing.T) {
	tests := []struct {
		name string
		mod  func(*Config)
	}{
		{"minimum cooldown", func(c *Config) { c.Forensic.Cooldown = 5 }},
		{"minimum window_size", func(c *Config) { c.Forensic.WindowSize = 3 }},
		{"minimum max_keep", func(c *Config) { c.Forensic.MaxKeep = 1 }},
		{"valid cdp_port", func(c *Config) { c.Forensic.CDPPort = 9222 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mod(&cfg)
			if err := cfg.validate(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestTOMLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `
[polling]
normal = 30
warning = 5
critical = 1

[thresholds]
warning = 500
high = 1000
critical = 2000
max_helpers = 150

protect = ["Finder"]

[groups.brave]
match = "Brave Browser Helper"
action = "suspend"
priority = 1

[forensic]
enabled = true
cdp_port = 9333
fs_usage = false
max_keep = 50
cooldown = 60
window_size = 15
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if cfg.Forensic.CDPPort != 9333 {
		t.Errorf("CDPPort = %d, want 9333", cfg.Forensic.CDPPort)
	}
	if cfg.Forensic.MaxKeep != 50 {
		t.Errorf("MaxKeep = %d, want 50", cfg.Forensic.MaxKeep)
	}
	if cfg.Forensic.Cooldown != 60 {
		t.Errorf("Cooldown = %d, want 60", cfg.Forensic.Cooldown)
	}
	if cfg.Forensic.WindowSize != 15 {
		t.Errorf("WindowSize = %d, want 15", cfg.Forensic.WindowSize)
	}
}

func TestMissingForensicSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Config without [forensic] section — defaults should apply
	content := `
[polling]
normal = 30
warning = 5
critical = 1

[thresholds]
warning = 500
high = 1000
critical = 2000
max_helpers = 150

protect = ["Finder"]

[groups.brave]
match = "Brave Browser Helper"
action = "suspend"
priority = 1
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	// Default() is used as base, then TOML overlay. Missing [forensic] keeps defaults.
	if !cfg.Forensic.Enabled {
		t.Error("Forensic.Enabled should be true from defaults")
	}
	if cfg.Forensic.CDPPort != 9222 {
		t.Errorf("CDPPort = %d, want 9222 from defaults", cfg.Forensic.CDPPort)
	}
}
