package output

// StatusResult is the structured output for the status command.
type StatusResult struct {
	Daemon            *DaemonInfo     `json:"daemon"`
	Level             string          `json:"level"`
	CompressionRate   float64         `json:"compression_rate"`
	DecompressionRate float64         `json:"decompression_rate"`
	SwapInRate        float64         `json:"swap_in_rate"`
	SwapOutRate       float64         `json:"swap_out_rate"`
	PagesInCompressor uint64          `json:"pages_in_compressor"`
	CompressorGB      float64         `json:"compressor_gb"`
	TotalCompressions uint64          `json:"total_compressions"`
	TotalSwapins      uint64          `json:"total_swapins"`
	TotalSwapouts     uint64          `json:"total_swapouts"`
	BrowserHelpers    int             `json:"browser_helpers"`
	Thresholds        ThresholdsInfo  `json:"thresholds"`
	Suspended         []SuspendedInfo `json:"suspended"`
	VMStopped         bool            `json:"vm_stopped"`
}

// A fresh heartbeat is evidence of monitoring; a stale file is not a running daemon.
type DaemonInfo struct {
	MonitorOnly         bool    `json:"monitor_only"`
	UpdatedAt           string  `json:"updated_at"`
	HeartbeatAgeSeconds float64 `json:"heartbeat_age_seconds"`
	Stale               bool    `json:"stale"`
	DiscoveryError      string  `json:"discovery_error,omitempty"`
}

// ThresholdsInfo describes the configured thresholds.
type ThresholdsInfo struct {
	Warning    float64 `json:"warning"`
	High       float64 `json:"high"`
	Critical   float64 `json:"critical"`
	MaxHelpers int     `json:"max_helpers"`
}

// SuspendedInfo describes a single suspended process.
type SuspendedInfo struct {
	PID         int    `json:"pid"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	SuspendedAt string `json:"suspended_at"`
}

// ActionResult describes a single action taken or planned.
type ActionResult struct {
	Action string `json:"action"` // "suspend", "resume", "vm-stop", "vm-start"
	Group  string `json:"group"`
	PID    int    `json:"pid,omitempty"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status"` // "ok", "error", "planned", "skipped"
	Error  string `json:"error,omitempty"`
}

// MutationResult is the structured output for mutating commands.
type MutationResult struct {
	DryRun  bool           `json:"dry_run"`
	Actions []ActionResult `json:"actions"`
	Summary string         `json:"summary"`
}

// InstallResult is the structured output for the install command.
type InstallResult struct {
	CLIPath      string `json:"cli_path"`
	LauncherPath string `json:"launcher_path"`
	BackupPath   string `json:"backup_path,omitempty"`
	MonitorOnly  bool   `json:"monitor_only"`
	PlistPath    string `json:"plist_path"`
	BinaryPath   string `json:"binary_path"`
	Loaded       bool   `json:"loaded"`
	DryRun       bool   `json:"dry_run"`
}

// StatusFields lists all available fields for --fields filtering.
var StatusFields = []string{
	"daemon",
	"level", "compression_rate", "decompression_rate",
	"swap_in_rate", "swap_out_rate",
	"pages_in_compressor", "compressor_gb",
	"total_compressions", "total_swapins", "total_swapouts",
	"browser_helpers", "thresholds", "suspended", "vm_stopped",
}
