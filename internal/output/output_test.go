package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestResolveFormat_Flag(t *testing.T) {
	if f := ResolveFormat("json"); f != JSON {
		t.Errorf("ResolveFormat(json) = %v, want JSON", f)
	}
	if f := ResolveFormat("text"); f != Text {
		t.Errorf("ResolveFormat(text) = %v, want Text", f)
	}
}

func TestResolveFormat_EnvVar(t *testing.T) {
	os.Setenv("OUTPUT_FORMAT", "json")
	defer os.Unsetenv("OUTPUT_FORMAT")
	if f := ResolveFormat(""); f != JSON {
		t.Errorf("ResolveFormat('') with OUTPUT_FORMAT=json = %v, want JSON", f)
	}
}

func TestPrinterJSON(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: JSON, Out: &buf, ErrOut: &buf}

	data := StatusResult{
		Level:           "warning",
		CompressionRate: 723.5,
		BrowserHelpers:  47,
		Suspended:       []SuspendedInfo{},
	}

	p.Print("status", data, func() {
		t.Error("text function should not be called in JSON mode")
	})

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, buf.String())
	}
	if resp.Command != "status" {
		t.Errorf("command = %q, want status", resp.Command)
	}
	if !resp.Success {
		t.Error("success should be true")
	}
	if resp.Timestamp == "" {
		t.Error("timestamp should be set")
	}
}

func TestPrinterText(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: Text, Out: &buf, ErrOut: &buf}

	called := false
	p.Print("status", nil, func() { called = true })
	if !called {
		t.Error("text function should be called in Text mode")
	}
}

func TestFieldsFiltering(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: JSON, Fields: []string{"level", "browser_helpers"}, Out: &buf, ErrOut: &buf}

	data := StatusResult{
		Level:             "critical",
		CompressionRate:   8000,
		BrowserHelpers:    372,
		TotalCompressions: 999999,
		Suspended:         []SuspendedInfo{},
	}

	p.Print("status", data, func() {})

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	m, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not a map: %T", resp.Data)
	}

	if _, exists := m["level"]; !exists {
		t.Error("filtered data should contain 'level'")
	}
	if _, exists := m["browser_helpers"]; !exists {
		t.Error("filtered data should contain 'browser_helpers'")
	}
	if _, exists := m["compression_rate"]; exists {
		t.Error("filtered data should NOT contain 'compression_rate'")
	}
	if _, exists := m["total_compressions"]; exists {
		t.Error("filtered data should NOT contain 'total_compressions'")
	}
}

func TestPrinterProgress(t *testing.T) {
	var buf bytes.Buffer
	textP := &Printer{Format: Text, Out: &buf, ErrOut: &buf}
	textP.Progress("hello")
	if buf.String() != "hello\n" {
		t.Errorf("text progress = %q, want 'hello\\n'", buf.String())
	}

	buf.Reset()
	jsonP := &Printer{Format: JSON, Out: &buf, ErrOut: &buf}
	jsonP.Progress("hello")
	if buf.Len() != 0 {
		t.Errorf("json progress should be suppressed, got %q", buf.String())
	}
}

func TestPrintError(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: JSON, Out: &buf, ErrOut: &buf}

	p.PrintError("focus", fmt.Errorf("something broke"))

	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Success {
		t.Error("success should be false")
	}
	if resp.Error != "something broke" {
		t.Errorf("error = %q, want 'something broke'", resp.Error)
	}
}
