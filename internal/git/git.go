// Package git wraps the system git binary (os/exec, no go-git dep) for the
// hugo repo: status, commit, push. Errors carry git's combined output
// verbatim so auth/SSH failures surface to the user.
package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Default identity used when the config leaves git_user/git_email blank.
const (
	DefaultUser  = "hugoStack"
	DefaultEmail = "hugostack@localhost"
)

// Result is a mutating git op's outcome. OK=false means git refused;
// Output always carries git's combined stdout+stderr verbatim.
type Result struct {
	OK     bool
	Output string
}

// RepoStatus is a porcelain-ish snapshot of the repo.
type RepoStatus struct {
	Branch      string
	DirtyFiles  int
	Ahead       int
	Behind      int
	HasUpstream bool
}

// identity returns the per-exec env overrides. ponytail: appended to
// os.Environ() so later duplicates win (os/exec semantics).
// GIT_TERMINAL_PROMPT=0 makes missing credentials fail fast instead of
// hanging the API on an interactive prompt.
func identity(user, email string) []string {
	if user == "" {
		user = DefaultUser
	}
	if email == "" {
		email = DefaultEmail
	}
	return []string{
		"GIT_AUTHOR_NAME=" + user,
		"GIT_AUTHOR_EMAIL=" + email,
		"GIT_COMMITTER_NAME=" + user,
		"GIT_COMMITTER_EMAIL=" + email,
		"GIT_TERMINAL_PROMPT=0",
	}
}

func run(dir, user, email string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), identity(user, email)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Status summarizes branch, dirty file count and ahead/behind vs upstream.
func Status(dir, user, email string) (RepoStatus, error) {
	var s RepoStatus
	out, err := run(dir, user, email, 10*time.Second, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return s, fmt.Errorf("git status: %s", strings.TrimSpace(out))
	}
	s.Branch = strings.TrimSpace(out)

	// ponytail: "dirty" = file count, not the diff; the deploy panel only
	// needs a number. Upgrade path: per-file list if the UI shows it.
	out, _ = run(dir, user, email, 10*time.Second, "status", "--porcelain")
	for _, line := range strings.Split(out, "\n") {
		if line != "" {
			s.DirtyFiles++
		}
	}

	count := func(ref string) (string, error) {
		return run(dir, user, email, 15*time.Second, "rev-list", "--left-right", "--count", "HEAD..."+ref)
	}
	out, err = count("@{upstream}")
	if err != nil && s.Branch != "HEAD" {
		// No upstream configured; compare against origin/<branch> if it exists.
		if out2, err2 := count("origin/" + s.Branch); err2 == nil {
			out, err = out2, nil
		}
	}
	if err == nil {
		s.HasUpstream = true
		fields := strings.Fields(out)
		if len(fields) == 2 {
			s.Ahead, _ = strconv.Atoi(fields[0])
			s.Behind, _ = strconv.Atoi(fields[1])
		}
	}
	return s, nil
}

// Commit stages everything (git add -A) and commits with msg.
// "nothing to commit" is a soft ok, not an error.
func Commit(dir, msg, user, email string) Result {
	if strings.TrimSpace(msg) == "" {
		return Result{Output: "empty commit message"}
	}
	out, err := run(dir, user, email, 60*time.Second, "add", "-A")
	if err != nil {
		return Result{Output: out}
	}
	out, err = run(dir, user, email, 60*time.Second, "commit", "-m", msg)
	if err == nil {
		return Result{OK: true, Output: out}
	}
	if strings.Contains(out, "nothing to commit") {
		return Result{OK: true, Output: out}
	}
	return Result{Output: out}
}

// Push runs `git push origin HEAD:<destRef>`.
func Push(dir, destRef, user, email string) Result {
	if !ValidRef(destRef) {
		return Result{Output: "invalid ref: " + destRef}
	}
	out, err := run(dir, user, email, 120*time.Second, "push", "origin", "HEAD:"+destRef)
	if err != nil {
		return Result{Output: out}
	}
	return Result{OK: true, Output: out}
}

// ValidRef reports whether s is usable as the <destRef> in
// `git push origin HEAD:<ref>`: a plain refname, no magic.
func ValidRef(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	if strings.ContainsAny(s, " ~^:?*[\\|@") {
		return false
	}
	if strings.Contains(s, "..") {
		return false
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "/") ||
		strings.HasSuffix(s, "/") || strings.HasSuffix(s, ".lock") ||
		strings.HasSuffix(s, ".") {
		return false
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}
