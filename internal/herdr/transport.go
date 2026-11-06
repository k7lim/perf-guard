// Package herdr contains the M1 read-only, explicitly scoped Herdr adapter.
package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

// Transport exposes no input, pane closure, service control or install methods.
// Read must accept only ping, session.snapshot and pane.process_info.
type Transport interface {
	Read(context.Context, string, string, string) (json.RawMessage, error)
}

// SocketTransport uses one bounded request/response connection per read. It does
// not invoke the Herdr CLI (which can create a server in other command modes).
type SocketTransport struct{}

var requestSequence atomic.Uint64

const maxResponseBytes = 16 << 20

func allowedRead(method string) bool {
	return method == "ping" || method == "session.snapshot" || method == "pane.process_info"
}

// NormalizeEndpoint accepts only an explicit local Unix socket. No default
// session lookup, UI focus, discovery, SSH, or HTTP fallback is performed.
func NormalizeEndpoint(endpoint string) (string, error) {
	if endpoint == "" || strings.IndexFunc(endpoint, unicode.IsControl) >= 0 {
		return "", errors.New("invalid explicit Herdr endpoint")
	}
	path := endpoint
	if strings.HasPrefix(endpoint, "unix://") {
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme != "unix" || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
			return "", errors.New("Herdr endpoint must be an absolute Unix socket")
		}
		path = u.Path
	}
	if !filepath.IsAbs(path) || strings.Contains(path, "%") || strings.Contains(path, "://") {
		return "", errors.New("Herdr endpoint must be an absolute Unix socket")
	}
	return "unix://" + filepath.Clean(path), nil
}

func (SocketTransport) Read(ctx context.Context, endpoint, method, paneID string) (json.RawMessage, error) {
	if !allowedRead(method) {
		return nil, errors.New("forbidden Herdr method")
	}
	if method == "pane.process_info" && (paneID == "" || strings.IndexFunc(paneID, unicode.IsControl) >= 0) {
		return nil, errors.New("pane.process_info requires an exact pane ID")
	}
	canonical, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", strings.TrimPrefix(canonical, "unix://"))
	if err != nil {
		return nil, errors.New("Herdr endpoint unavailable")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	id := fmt.Sprintf("perf-guard-review-%d", requestSequence.Add(1))
	params := map[string]string{}
	if method == "pane.process_info" {
		params["pane_id"] = paneID
	}
	request := struct {
		ID     string            `json:"id"`
		Method string            `json:"method"`
		Params map[string]string `json:"params"`
	}{id, method, params}
	if err = json.NewEncoder(conn).Encode(request); err != nil {
		return nil, errors.New("Herdr read request failed")
	}
	limited := io.LimitReader(conn, maxResponseBytes+1)
	line, err := bufio.NewReader(limited).ReadBytes('\n')
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("Herdr read response incomplete")
	}
	if len(line) > maxResponseBytes {
		return nil, errors.New("Herdr read response exceeds limit")
	}
	var response struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(line, &response) != nil || response.ID != id {
		return nil, errors.New("invalid Herdr read response")
	}
	if response.Error != nil {
		return nil, errors.New("Herdr read method unavailable")
	}
	if len(response.Result) == 0 || string(response.Result) == "null" {
		return nil, errors.New("missing Herdr read result")
	}
	return response.Result, nil
}
