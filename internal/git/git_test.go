package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setup creates a throwaway repo (dir) with one commit, plus a local bare
// remote wired up as "origin" with the main branch pushed.
func setup(t *testing.T) (dir, remote string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir = t.TempDir()
	remote = t.TempDir()
	mk := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mk("-C", dir, "init", "-b", "main")
	mk("-C", dir, "config", "user.name", "fixture")
	mk("-C", dir, "config", "user.email", "fixture@localhost")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mk("-C", dir, "add", "-A")
	mk("-C", dir, "commit", "-m", "init")
	mk("-C", remote, "init", "--bare")
	mk("-C", dir, "remote", "add", "origin", remote)
	mk("-C", dir, "push", "-u", "origin", "main")
	t.Cleanup(func() { _ = remote })
	return dir, remote
}

func TestStatus(t *testing.T) {
	dir, _ := setup(t)
	s, err := Status(dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if s.Branch != "main" || s.DirtyFiles != 0 || s.Ahead != 0 || s.Behind != 0 || !s.HasUpstream {
		t.Fatalf("bad initial status: %+v", s)
	}
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("two\n"), 0644)
	s, _ = Status(dir, "", "")
	if s.DirtyFiles != 1 {
		t.Fatalf("want 1 dirty file, got %+v", s)
	}
}

func TestCommitAndIdentity(t *testing.T) {
	dir, _ := setup(t)
	// Identity comes from the explicit user/email (env export), not the
	// repo's own config.
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one two\n"), 0644)
	res := Commit(dir, "change a", "hugoStack", "hugostack@localhost")
	if !res.OK {
		t.Fatalf("commit not ok: %s", res.Output)
	}
	if !strings.Contains(res.Output, "1 file changed") {
		t.Fatalf("unexpected commit output: %s", res.Output)
	}
	out, _ := exec.Command("git", "-C", dir, "log", "-1", "--format=%s|%an|%ae").CombinedOutput()
	if got := strings.TrimSpace(string(out)); got != "change a|hugoStack|hugostack@localhost" {
		t.Fatalf("log: %q", got)
	}
	// Soft ok: clean tree.
	res = Commit(dir, "again", "hugoStack", "hugostack@localhost")
	if !res.OK || !strings.Contains(res.Output, "nothing to commit") {
		t.Fatalf("clean commit should be soft ok: %+v", res)
	}
	// Error path: empty message.
	res = Commit(dir, "  ", "u", "e")
	if res.OK || !strings.Contains(res.Output, "empty commit message") {
		t.Fatalf("empty message should fail softly: %+v", res)
	}
}

func TestPush(t *testing.T) {
	dir, remote := setup(t)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one two\n"), 0644)
	res := Commit(dir, "second commit", "hugoStack", "hugostack@localhost")
	if !res.OK {
		t.Fatal(res.Output)
	}
	// Good ref: new branch appears in the bare remote.
	res = Push(dir, "release", "u", "e")
	if !res.OK {
		t.Fatalf("push not ok: %s", res.Output)
	}
	out, _ := exec.Command("git", "-C", remote, "branch", "--format=%(refname:short)").CombinedOutput()
	if !strings.Contains(string(out), "release") {
		t.Fatalf("bare remote branches: %s", out)
	}
	// Bad ref: rejected, stderr visible.
	res = Push(dir, "bad..ref", "u", "e")
	if res.OK {
		t.Fatalf("bad ref should fail: %s", res.Output)
	}
	if !strings.Contains(res.Output, "invalid ref") {
		t.Fatalf("want verbatim rejection, got: %s", res.Output)
	}
	// Real git error: release is ahead of local HEAD -> non-fast-forward.
	if out, err := exec.Command("git", "-C", dir, "reset", "--hard", "HEAD~1").CombinedOutput(); err != nil {
		t.Fatalf("reset: %v\n%s", err, out)
	}
	res = Push(dir, "release", "u", "e")
	if res.OK {
		t.Fatalf("rewind push should fail: %s", res.Output)
	}
	if !strings.Contains(strings.ToLower(res.Output), "non-fast-forward") &&
		!strings.Contains(res.Output, "rejected") {
		t.Fatalf("want git's rejection verbatim, got: %s", res.Output)
	}
}

func TestValidRef(t *testing.T) {
	good := []string{"main", "hugo", "v1.2.3", "release/x", "feature/test-1", "main-hotfix"}
	for _, s := range good {
		if !ValidRef(s) {
			t.Errorf("ValidRef(%q) = false, want true", s)
		}
	}
	bad := []string{"", "..", "a..b", "-x", "/x", "x/", "x.y.", "a b", "a|b", "a@{u}", "a?b", "refs/heads/x:", "a.b.lock", "a//b", "a/b/"}
	for _, s := range bad {
		if ValidRef(s) {
			t.Errorf("ValidRef(%q) = true, want false", s)
		}
	}
}
