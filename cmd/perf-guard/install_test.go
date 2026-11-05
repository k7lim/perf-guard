package main

import (
	"reflect"
	"testing"
)

func TestUpgradePreservesExistingLaunchSettings(t *testing.T) {
	settings := map[string]any{"Label": serviceLabel, "KeepAlive": false, "WorkingDirectory": "/some/path", "EnvironmentVariables": map[string]any{"PATH": "/custom/bin"}, "StandardErrorPath": "/custom/log"}
	got := installSettings(settings, "/new/bin", "/named/launcher", true)
	if got["KeepAlive"] != false || got["WorkingDirectory"] != "/some/path" || got["StandardErrorPath"] != "/custom/log" {
		t.Fatalf("upgrade lost settings: %v", got)
	}
	if !reflect.DeepEqual(got["ProgramArguments"], []string{"/new/bin", "daemon", "--monitor-only"}) {
		t.Fatal(got["ProgramArguments"])
	}
	if got["Program"] != "/named/launcher" {
		t.Fatal(got["Program"])
	}
}
