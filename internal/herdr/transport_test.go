package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/k7lim/perf-guard/internal/review"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTransportRejectsControlBeforeDial(t *testing.T) {
	for _, m := range []string{"pane.send_input", "pane.close", "server.stop", "agent.start", "server.live_handoff", "integration.install", "signal", "update", "install"} {
		if _, e := (SocketTransport{}).Read(context.Background(), "unix:///no-such-socket", m, ""); e == nil || e.Error() != "forbidden Herdr method" {
			t.Fatal(m, e)
		}
	}
	if _, e := (SocketTransport{}).Read(context.Background(), "unix:///no-such-socket", "pane.process_info", ""); e == nil {
		t.Fatal("implicit focused pane allowed")
	}
}

func testSocket(t *testing.T, serve func(net.Conn)) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "pgh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "h.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		c, e := listener.Accept()
		if e == nil {
			defer c.Close()
			serve(c)
		}
	}()
	return "unix://" + path
}
func request(t *testing.T, c net.Conn) map[string]any {
	t.Helper()
	b, e := bufio.NewReader(c).ReadBytes('\n')
	if e != nil {
		t.Error(e)
		return nil
	}
	var v map[string]any
	if e = json.Unmarshal(b, &v); e != nil {
		t.Error(e)
	}
	return v
}
func TestSocketReadFramingAndExactTarget(t *testing.T) {
	received := make(chan map[string]any, 1)
	endpoint := testSocket(t, func(c net.Conn) {
		r := request(t, c)
		received <- r
		_ = json.NewEncoder(c).Encode(map[string]any{"id": r["id"], "result": map[string]any{"type": "pong", "version": "0.9.0", "protocol": 22}})
	})
	raw, e := (SocketTransport{}).Read(context.Background(), endpoint, "ping", "")
	if e != nil {
		t.Fatal(e)
	}
	var pong map[string]any
	if json.Unmarshal(raw, &pong) != nil || pong["type"] != "pong" {
		t.Fatal(string(raw))
	}
	r := <-received
	if r["method"] != "ping" || len(r["params"].(map[string]any)) != 0 {
		t.Fatal(r)
	}
}
func TestSocketRejectsMismatchedResponseAndRedactsErrors(t *testing.T) {
	for _, kind := range []string{"wrong-id", "server-error", "missing-result", "invalid-json"} {
		t.Run(kind, func(t *testing.T) {
			endpoint := testSocket(t, func(c net.Conn) {
				r := request(t, c)
				switch kind {
				case "wrong-id":
					json.NewEncoder(c).Encode(map[string]any{"id": "other", "result": map[string]any{}})
				case "server-error":
					json.NewEncoder(c).Encode(map[string]any{"id": r["id"], "error": map[string]any{"code": "unsupported", "message": "SECRET-COMMAND-ARGUMENTS"}})
				case "missing-result":
					json.NewEncoder(c).Encode(map[string]any{"id": r["id"]})
				case "invalid-json":
					c.Write([]byte("bad json\n"))
				}
			})
			_, e := (SocketTransport{}).Read(context.Background(), endpoint, "ping", "")
			if e == nil || strings.Contains(e.Error(), "SECRET") {
				t.Fatal(e)
			}
		})
	}
}
func TestSocketHonorsCancelledAndOversizedReads(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		endpoint := testSocket(t, func(c net.Conn) { _ = request(t, c); var b [1]byte; _, _ = c.Read(b[:]) })
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		start := time.Now()
		if _, e := (SocketTransport{}).Read(ctx, endpoint, "ping", ""); e == nil || time.Since(start) > time.Second {
			t.Fatal(e)
		}
	})
	t.Run("oversize", func(t *testing.T) {
		endpoint := testSocket(t, func(c net.Conn) {
			_ = request(t, c)
			_, _ = c.Write([]byte(strings.Repeat("x", maxResponseBytes+1) + "\n"))
		})
		if _, e := (SocketTransport{}).Read(context.Background(), endpoint, "ping", ""); e == nil {
			t.Fatal("oversize allowed")
		}
	})
}

// Opt-in only: explicit private target file, never default/focused session.
// Inventory contents stay in memory; output contains generalized facts only.
func TestReadOnlyHostSmoke(t *testing.T) {
	path := os.Getenv("PERF_GUARD_HERDR_SMOKE_TARGET_FILE")
	if path == "" {
		t.Skip("explicit authorized Mac target required")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var target struct {
		Endpoint  string `json:"endpoint"`
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(b, &target) != nil {
		t.Fatal("invalid private target file")
	}
	s := NewSource(nil)
	v, e := s.Collect(context.Background(), review.HerdrTarget{Endpoint: target.Endpoint, SessionID: target.SessionID})
	if e != nil || v.Source.Status == "unavailable" || v.ServerVersion == nil || v.ClientVersion == nil {
		t.Fatal("host Herdr read unavailable")
	}
	positive, unknown := 0, 0
	for _, o := range v.Occupants {
		if o.Correlation == "positive" {
			positive++
		} else {
			unknown++
		}
	}
	t.Logf("client=%s server=%s capabilities=%v occupants=%d positively_correlated=%d unknown=%d status=%s (identity resolver intentionally absent; #8 wires platform resolver)", *v.ClientVersion, *v.ServerVersion, v.Capabilities, len(v.Occupants), positive, unknown, v.Source.Status)
}
