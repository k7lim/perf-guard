package vm

import (
	"fmt"
	"os/exec"
	"strings"
)

// DefaultLimactl is the default path to limactl.
const DefaultLimactl = "/opt/homebrew/bin/limactl"

// Controller manages Lima VM lifecycle.
type Controller struct {
	Limactl string // path to limactl binary
}

// NewController creates a Controller, resolving limactl from PATH or using the default.
func NewController() *Controller {
	path, err := exec.LookPath("limactl")
	if err != nil {
		path = DefaultLimactl
	}
	return &Controller{Limactl: path}
}

// Stop gracefully stops the default Lima VM instance.
func (c *Controller) Stop() error {
	instances, err := c.runningInstances()
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		return nil // nothing to stop
	}
	for _, name := range instances {
		if err := exec.Command(c.Limactl, "stop", name).Run(); err != nil {
			return fmt.Errorf("limactl stop %s: %w", name, err)
		}
	}
	return nil
}

// Start starts the default Lima VM instance.
func (c *Controller) Start() error {
	if err := exec.Command(c.Limactl, "start", "default").Run(); err != nil {
		return fmt.Errorf("limactl start: %w", err)
	}
	return nil
}

// HasRunning returns true if any Lima VMs are running.
func (c *Controller) HasRunning() bool {
	instances, _ := c.runningInstances()
	return len(instances) > 0
}

func (c *Controller) runningInstances() ([]string, error) {
	out, err := exec.Command(c.Limactl, "list", "--format", "{{.Name}} {{.Status}}").Output()
	if err != nil {
		return nil, err
	}
	var running []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "Running" {
			running = append(running, fields[0])
		}
	}
	return running, nil
}
