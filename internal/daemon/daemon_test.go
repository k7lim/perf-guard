package daemon

import (
	"github.com/k7lim/perf-guard/internal/config"
	"github.com/k7lim/perf-guard/internal/pressure"
	"github.com/k7lim/perf-guard/internal/state"
	"reflect"
	"testing"
)

func TestCrossedPressureBoundaries(t *testing.T) {
	cfg := config.Default()
	cfg.Groups["lima"] = config.GroupConfig{Match: "lima", Action: "vm-stop"}
	d := New(cfg)
	if got := d.plannedActions(pressure.Critical, pressure.Normal); !reflect.DeepEqual(got, []string{"vm-stop", "suspend"}) {
		t.Fatalf("Normal -> Critical actions = %v", got)
	}
	d.state.AddSuspended(999999, "fixture", "test")
	if got := d.plannedActions(pressure.High, pressure.Critical); len(got) != 0 {
		t.Fatalf("premature recovery: %v", got)
	}
	if got := d.plannedActions(pressure.Normal, pressure.High); !reflect.DeepEqual(got, []string{"resume"}) {
		t.Fatalf("staged recovery actions = %v", got)
	}
}

func TestMonitorOnlyNeverChangesManagedState(t *testing.T) {
	cfg := config.Default()
	cfg.MonitorOnly = true
	d := New(cfg)
	d.state = state.State{VMStopped: true}
	d.state.AddSuspended(999999, "fixture", "test")
	before := d.state
	for _, from := range []pressure.Level{pressure.Normal, pressure.Warning, pressure.High, pressure.Critical} {
		for _, to := range []pressure.Level{pressure.Normal, pressure.Warning, pressure.High, pressure.Critical} {
			if got := d.plannedActions(to, from); len(got) != 0 {
				t.Fatalf("monitor-only actions: %v", got)
			}
			d.act(to, from)
		}
	}
	if !reflect.DeepEqual(d.state, before) {
		t.Fatal("monitor-only mutated tracked state")
	}
}
