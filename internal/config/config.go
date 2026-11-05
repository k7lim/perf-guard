package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/k7lim/perf-guard/internal/pressure"
)

// Config is the top-level configuration.
type Config struct {
	MonitorOnly bool                   `toml:"monitor_only"`
	Polling     PollingConfig          `toml:"polling"`
	Thresholds  ThresholdConfig        `toml:"thresholds"`
	Protect     []string               `toml:"protect"`
	Groups      map[string]GroupConfig `toml:"groups"`
	LogPath     string                 `toml:"log_path"`
	Forensic    ForensicConfig         `toml:"forensic"`
}

// ForensicConfig controls forensic spike capture behavior.
type ForensicConfig struct {
	Enabled    bool `toml:"enabled"`
	CDPPort    int  `toml:"cdp_port"`
	FSUsage    bool `toml:"fs_usage"`
	MaxKeep    int  `toml:"max_keep"`
	Cooldown   int  `toml:"cooldown"`    // seconds between captures
	WindowSize int  `toml:"window_size"` // rolling window for spike detection
}

// PollingConfig controls adaptive poll intervals in seconds.
type PollingConfig struct {
	Normal   int `toml:"normal"`
	Warning  int `toml:"warning"`
	Critical int `toml:"critical"`
}

// ThresholdConfig maps to pressure.Thresholds.
type ThresholdConfig struct {
	Warning    float64 `toml:"warning"`
	High       float64 `toml:"high"`
	Critical   float64 `toml:"critical"`
	MaxHelpers int     `toml:"max_helpers"`
}

// GroupConfig defines a process group to manage.
type GroupConfig struct {
	Match    string `toml:"match"`
	Action   string `toml:"action"`   // "suspend" or "vm-stop"
	Priority int    `toml:"priority"` // lower = acted on first
}

// Default returns a Config with sensible defaults that works without a config file.
func Default() Config {
	return Config{
		Polling: PollingConfig{
			Normal:   30,
			Warning:  5,
			Critical: 1,
		},
		Thresholds: ThresholdConfig{
			Warning:    500,
			High:       1000,
			Critical:   2000,
			MaxHelpers: 150,
		},
		Protect: []string{
			"Finder", "loginwindow", "WindowServer",
			"sshd", "Spotlight", "iTerm2", "Terminal",
		},
		Groups: map[string]GroupConfig{
			"brave":   {Match: "Brave Browser Helper", Action: "suspend", Priority: 1},
			"firefox": {Match: "firefox", Action: "suspend", Priority: 2},
			"vscode":  {Match: "Code Helper", Action: "suspend", Priority: 3},
		},
		LogPath: "/tmp/perf-guard.log",
		Forensic: ForensicConfig{
			Enabled:    true,
			CDPPort:    9222,
			FSUsage:    false,
			MaxKeep:    100,
			Cooldown:   30,
			WindowSize: 10,
		},
	}
}

// ConfigPath returns the default config file path.
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "perf-guard", "config.toml")
}

// Load reads config from the default path, falling back to defaults.
func Load() (Config, error) {
	return LoadFrom(ConfigPath())
}

// LoadFrom reads config from a specific path, falling back to defaults.
func LoadFrom(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // no config file, use defaults
		}
		return cfg, err
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	if err := cfg.validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// validate checks config values after parsing.
func (c Config) validate() error {
	validActions := map[string]bool{"suspend": true, "vm-stop": true}
	for name, gc := range c.Groups {
		if gc.Match == "" {
			return fmt.Errorf("group %q: match must not be empty", name)
		}
		if len(gc.Match) > 256 {
			return fmt.Errorf("group %q: match too long (%d chars, max 256)", name, len(gc.Match))
		}
		for i, ch := range gc.Match {
			if ch < 0x20 && ch != '\t' {
				return fmt.Errorf("group %q: match contains control character at position %d", name, i)
			}
		}
		if !validActions[gc.Action] {
			return fmt.Errorf("group %q: action must be \"suspend\" or \"vm-stop\", got %q", name, gc.Action)
		}
	}
	// Forensic validation
	if c.Forensic.Cooldown < 5 && c.Forensic.Cooldown != 0 {
		return fmt.Errorf("forensic.cooldown must be >= 5, got %d", c.Forensic.Cooldown)
	}
	if c.Forensic.WindowSize < 3 && c.Forensic.WindowSize != 0 {
		return fmt.Errorf("forensic.window_size must be >= 3, got %d", c.Forensic.WindowSize)
	}
	if c.Forensic.MaxKeep < 1 && c.Forensic.MaxKeep != 0 {
		return fmt.Errorf("forensic.max_keep must be >= 1, got %d", c.Forensic.MaxKeep)
	}
	if c.Forensic.CDPPort < 0 || c.Forensic.CDPPort > 65535 {
		return fmt.Errorf("forensic.cdp_port must be 0-65535, got %d", c.Forensic.CDPPort)
	}

	return nil
}

// PressureThresholds converts the config thresholds to the pressure package type.
func (c Config) PressureThresholds() pressure.Thresholds {
	return pressure.Thresholds{
		WarningRate:  c.Thresholds.Warning,
		HighRate:     c.Thresholds.High,
		CriticalRate: c.Thresholds.Critical,
		MaxHelpers:   c.Thresholds.MaxHelpers,
	}
}
