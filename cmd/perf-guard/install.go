package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/k7lim/perf-guard/internal/output"
	"github.com/spf13/cobra"
)

const serviceLabel = "com.user.perf-guard"

func installCmd() *cobra.Command {
	var monitorOnly bool
	cmd := &cobra.Command{
		Use: "install", Short: "Install or restore the user LaunchAgent",
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := os.Executable()
			if err != nil {
				return err
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			support := filepath.Join(home, "Library", "Application Support", "perf-guard")
			binary := filepath.Join(support, "bin", "perf-guard")
			launcher := filepath.Join(support, "bin", "Performance Guard - Monitor")
			cliPath := filepath.Join(home, ".local", "bin", "perf-guard")
			plistPath := filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")
			old, err := os.ReadFile(plistPath)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			settings := map[string]any{}
			if len(old) > 0 {
				out, err := exec.Command("/usr/bin/plutil", "-convert", "json", "-o", "-", plistPath).Output()
				if err != nil {
					return fmt.Errorf("reading existing plist: %w", err)
				}
				if err := json.Unmarshal(out, &settings); err != nil {
					return err
				}
				if settings["Label"] != serviceLabel {
					return fmt.Errorf("unexpected service label")
				}
			}
			// Preserve the existing mode unless this install explicitly selects one.
			if !cmd.Flags().Changed("monitor-only") {
				if argv, ok := settings["ProgramArguments"].([]any); ok {
					for _, arg := range argv {
						if arg == "--monitor-only" {
							monitorOnly = true
						}
					}
				}
			}
			settings = installSettings(settings, binary, launcher, monitorOnly)
			raw, err := json.Marshal(settings)
			if err != nil {
				return err
			}
			convert := exec.Command("/usr/bin/plutil", "-convert", "xml1", "-o", "-", "-")
			convert.Stdin = bytes.NewReader(raw)
			plist, err := convert.Output()
			if err != nil {
				return fmt.Errorf("generating plist: %w", err)
			}
			result := output.InstallResult{PlistPath: plistPath, BinaryPath: binary, LauncherPath: launcher, CLIPath: cliPath, MonitorOnly: monitorOnly, DryRun: dryRunFlag}
			if dryRunFlag {
				printer.Print("install", result, func() {
					fmt.Printf("[dry-run] Install %s, preserve existing plist settings, enable %s (monitor_only=%t)\n", binary, serviceLabel, monitorOnly)
				})
				return nil
			}
			backup := filepath.Join(support, "backups", time.Now().Format("20060102T150405.000000000"))
			if err := os.MkdirAll(backup, 0700); err != nil {
				return err
			}
			result.BackupPath = backup
			for _, path := range []string{plistPath, binary, launcher, cliPath} {
				if info, err := os.Lstat(path); err == nil {
					if !info.Mode().IsRegular() {
						return fmt.Errorf("refusing to replace nonregular file %s", path)
					}
					data, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					backupName := filepath.Base(path)
					if path == cliPath {
						backupName = "cli-perf-guard"
					}
					if err := os.WriteFile(filepath.Join(backup, backupName), data, info.Mode().Perm()); err != nil {
						return err
					}
				} else if !os.IsNotExist(err) {
					return err
				}
			}
			payload, err := os.ReadFile(source)
			if err != nil {
				return err
			}
			wrapper := "#!/bin/sh\n# perf-guard managed launcher\nexec " + shellQuote(binary) + " \"$@\"\n"
			if err := atomicInstall(binary, payload, 0755); err != nil {
				return err
			}
			if err := atomicInstall(launcher, []byte(wrapper), 0755); err != nil {
				return err
			}
			if err := atomicInstall(cliPath, []byte(wrapper), 0755); err != nil {
				return err
			}
			// Detect an intervening plist edit before replacing it.
			current, err := os.ReadFile(plistPath)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if !bytes.Equal(old, current) {
				return fmt.Errorf("plist changed during installation; backup: %s", backup)
			}
			if err := atomicInstall(plistPath, plist, 0644); err != nil {
				return err
			}
			domain := "gui/" + strconv.Itoa(os.Getuid())
			target := domain + "/" + serviceLabel
			if exec.Command("/bin/launchctl", "print", target).Run() == nil {
				if out, err := exec.Command("/bin/launchctl", "bootout", target).CombinedOutput(); err != nil {
					return fmt.Errorf("bootout: %w: %s", err, out)
				}
			}
			if out, err := exec.Command("/bin/launchctl", "enable", target).CombinedOutput(); err != nil {
				return fmt.Errorf("enable: %w: %s", err, out)
			}
			if out, err := exec.Command("/bin/launchctl", "bootstrap", domain, plistPath).CombinedOutput(); err != nil {
				return fmt.Errorf("bootstrap: %w: %s (backup: %s)", err, out, backup)
			}
			result.Loaded = true
			printer.Print("install", result, func() {
				fmt.Printf("Performance Guard - Monitor enabled (monitor_only=%t). Backup: %s\n", monitorOnly, backup)
			})
			return nil
		},
	}
	cmd.Flags().BoolVar(&monitorOnly, "monitor-only", false, "Persist monitoring and alerts without automatic process or VM actions")
	return cmd
}

func installSettings(existing map[string]any, binary, launcher string, monitorOnly bool) map[string]any {
	if len(existing) == 0 {
		existing["RunAtLoad"] = true
		existing["KeepAlive"] = true
		existing["StandardOutPath"] = "/tmp/perf-guard.log"
		existing["StandardErrorPath"] = "/tmp/perf-guard.log"
		existing["ProcessType"] = "Background"
	}
	existing["Label"] = serviceLabel
	existing["Program"] = launcher
	args := []string{binary, "daemon"}
	if monitorOnly {
		args = append(args, "--monitor-only")
	}
	existing["ProgramArguments"] = args
	return existing
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func atomicInstall(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".perf-guard-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
