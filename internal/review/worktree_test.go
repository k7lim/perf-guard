package review

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func gitFixture(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("fixture git: %v %s", e, b)
	}
}
func treeHash(t *testing.T, dir string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	e := filepath.WalkDir(dir, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(dir, p)
		out[rel] = sha256.Sum256(b)
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestWorktreeReadOnlyActualGit(t *testing.T) {
	dir := t.TempDir()
	gitFixture(t, dir, "init")
	gitFixture(t, dir, "config", "user.name", "Fixture")
	gitFixture(t, dir, "config", "user.email", "fixture@example.invalid")
	os.WriteFile(filepath.Join(dir, "tracked"), []byte("base"), 0600)
	gitFixture(t, dir, "add", "tracked")
	gitFixture(t, dir, "commit", "-m", "fixture")
	source := GitWorktreeSource{}
	for _, dirty := range []bool{false, true} {
		if dirty {
			os.WriteFile(filepath.Join(dir, "tracked"), []byte("unsaved"), 0600)
			os.WriteFile(filepath.Join(dir, "untracked"), []byte("keep me"), 0600)
		}
		before := treeHash(t, dir)
		v := source.Inspect(context.Background(), dir)
		after := treeHash(t, dir)
		want := "clean"
		if dirty {
			want = "dirty"
		}
		if v.State != want || !reflect.DeepEqual(before, after) {
			t.Fatalf("state=%s want=%s or files/index changed", v.State, want)
		}
	}
	nonrepo := t.TempDir()
	if v := source.Inspect(context.Background(), nonrepo); v.State != "not_repository" {
		t.Fatal(v)
	}
	if v := source.Inspect(context.Background(), filepath.Join(nonrepo, "missing")); v.State != "unknown" || len(v.Warnings) == 0 {
		t.Fatal(v)
	}
}
func TestWorktreeDirtySubmoduleReadOnly(t *testing.T) {
	parent, child := t.TempDir(), t.TempDir()
	for _, dir := range []string{parent, child} {
		gitFixture(t, dir, "init")
		gitFixture(t, dir, "config", "user.name", "Fixture")
		gitFixture(t, dir, "config", "user.email", "fixture@example.invalid")
		os.WriteFile(filepath.Join(dir, "tracked"), []byte("base"), 0600)
		gitFixture(t, dir, "add", "tracked")
		gitFixture(t, dir, "commit", "-m", "fixture")
	}
	gitFixture(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", child, "nested")
	gitFixture(t, parent, "commit", "-am", "submodule")
	os.WriteFile(filepath.Join(parent, "nested", "tracked"), []byte("dirty nested work"), 0600)
	before := treeHash(t, parent)
	v := (GitWorktreeSource{}).Inspect(context.Background(), parent)
	if v.State != "dirty" || !reflect.DeepEqual(before, treeHash(t, parent)) {
		t.Fatalf("dirty submodule hidden or changed: %s", v.State)
	}
}
func TestWorktreeFailureDeadlineAndInheritedGitScope(t *testing.T) {
	dir := t.TempDir()
	source := GitWorktreeSource{Read: func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("denied")
	}}
	if v := source.Inspect(context.Background(), dir); v.State != "unknown" || v.Warnings[0].Code != "probe_failed" {
		t.Fatal(v)
	}
	source.Read = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	v := source.Inspect(ctx, dir)
	if v.State != "unknown" || v.Warnings[0].Code != "deadline_exceeded" {
		t.Fatal(v)
	}
	// A caller's alternate index/worktree cannot retarget the probe.
	gitFixture(t, dir, "init")
	t.Setenv("GIT_DIR", "/no-such-git")
	t.Setenv("GIT_INDEX_FILE", "/no-such-index")
	v = (GitWorktreeSource{}).Inspect(context.Background(), dir)
	if v.State != "clean" {
		t.Fatal(v)
	}
	// Production allowlist remains fixed; no hooks, writes or optional locks.
	b, e := os.ReadFile("worktree.go")
	if e != nil {
		t.Fatal(e)
	}
	for _, needle := range []string{"--no-optional-locks", "core.fsmonitor=false", "core.hooksPath=/dev/null", "GIT_OPTIONAL_LOCKS=0", "--ignore-submodules=none"} {
		if !strings.Contains(string(b), needle) {
			t.Fatal("missing read-only guard", needle)
		}
	}
}
