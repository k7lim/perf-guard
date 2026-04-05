package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Format controls how output is rendered.
type Format int

const (
	Text Format = iota
	JSON
)

// Response is the JSON envelope for all commands.
type Response struct {
	Command   string `json:"command"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	Data      any    `json:"data"`
	Timestamp string `json:"timestamp"`
}

// Printer handles format-aware output.
type Printer struct {
	Format Format
	Fields []string // if non-empty, filter JSON data keys
	Out    io.Writer
	ErrOut io.Writer
}

// NewPrinter creates a Printer with the resolved format.
func NewPrinter(format Format, fields []string) *Printer {
	return &Printer{
		Format: format,
		Fields: fields,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
}

// Print outputs a successful result. textFn renders human-readable output.
func (p *Printer) Print(command string, data any, textFn func()) {
	if p.Format == Text {
		textFn()
		return
	}
	p.printJSON(command, true, "", data)
}

// PrintError outputs an error result.
func (p *Printer) PrintError(command string, err error) {
	if p.Format == Text {
		fmt.Fprintf(p.ErrOut, "Error: %v\n", err)
		return
	}
	p.printJSON(command, false, err.Error(), nil)
}

// Progress prints a message only in text mode (suppressed in JSON).
func (p *Printer) Progress(msg string) {
	if p.Format == Text {
		fmt.Fprintln(p.Out, msg)
	}
}

func (p *Printer) printJSON(command string, success bool, errMsg string, data any) {
	if len(p.Fields) > 0 && data != nil {
		data = filterFields(data, p.Fields)
	}
	resp := Response{
		Command:   command,
		Success:   success,
		Error:     errMsg,
		Data:      data,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	enc.Encode(resp)
}

// filterFields keeps only the specified keys in a struct by round-tripping through map.
func filterFields(data any, fields []string) any {
	raw, err := json.Marshal(data)
	if err != nil {
		return data
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return data
	}
	allowed := make(map[string]bool, len(fields))
	for _, f := range fields {
		allowed[f] = true
	}
	for k := range m {
		if !allowed[k] {
			delete(m, k)
		}
	}
	return m
}

// ResolveFormat determines the output format from flag, env var, and TTY detection.
func ResolveFormat(flagValue string) Format {
	// 1. Explicit flag
	switch flagValue {
	case "json":
		return JSON
	case "text":
		return Text
	}
	// 2. Env var
	if env := os.Getenv("OUTPUT_FORMAT"); env != "" {
		switch env {
		case "json":
			return JSON
		case "text":
			return Text
		}
	}
	// 3. TTY detection: JSON when piped
	if fi, err := os.Stdout.Stat(); err == nil {
		if fi.Mode()&os.ModeCharDevice == 0 {
			return JSON
		}
	}
	// 4. Default: text
	return Text
}
