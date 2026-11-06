//go:build darwin

package process

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReviewPSParsing(t *testing.T) {
	rows, w := parseReviewPS("10 1 ttys1 2 /some path/codex\nmalformed\n11 bad ?? 2 node\n12 10 ?? -2 node\n")
	if len(rows) != 1 || rows[0].Name != "/some path/codex" || *rows[0].RSSBytes != 2048 || len(w) != 3 {
		t.Fatalf("%+v %+v", rows, w)
	}
}
func TestReviewHostSmoke(t *testing.T) {
	if os.Getenv("PERF_GUARD_PROCESS_SMOKE") != "1" {
		t.Skip("opt-in read-only macOS host smoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, e := ReadIdentity(ctx, os.Getpid())
	if e != nil {
		t.Fatal(e)
	}
	b, e := ReadIdentity(ctx, os.Getpid())
	if e != nil || a != b || !strings.HasPrefix(a.StartID, "darwin-start:") {
		t.Fatal("unstable kernel start identity", e)
	}
	start := time.Now()
	inv, e := NewReviewSource().Collect(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if inv.Source.Status == "unavailable" || inv.Source.CollectedAt == nil {
		t.Fatal("host source unavailable")
	}
	for _, p := range inv.Processes {
		if p.Identity != nil && p.Identity.PID != p.PID {
			t.Fatal("identity mismatch")
		}
	}
	t.Logf("read-only host smoke: status=%s processes=%d groups=%d elapsed=%s; no private identifiers emitted", inv.Source.Status, len(inv.Processes), len(inv.Groups), time.Since(start).Round(time.Millisecond))
}
