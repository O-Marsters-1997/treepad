package worktree

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type Worktree struct {
	Path           string
	Branch         string // stripped of refs/heads/ prefix; "(detached)" when detached
	IsMain         bool   // true when .git entry is a directory, not a file
	Prunable       bool   // true when git considers the worktree stale (directory deleted)
	PrunableReason string // git's human-readable reason, e.g. "gitdir file points to non-existent location"
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, bytes.TrimSpace(exitErr.Stderr))
		}
		return nil, err
	}
	return out, nil
}

func List(ctx context.Context, runner CommandRunner) ([]Worktree, error) {
	slog.Debug("listing git worktrees")
	out, err := runner.Run(ctx, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	wts, err := parsePorcelain(out)
	if err != nil {
		return nil, err
	}
	for i := range wts {
		wts[i].IsMain = isMainWorktree(wts[i].Path)
	}
	return wts, nil
}

// MainWorktree returns the worktree whose .git entry is a directory (the main repo).
func MainWorktree(wts []Worktree) (Worktree, error) {
	for _, wt := range wts {
		if wt.IsMain {
			return wt, nil
		}
	}
	return Worktree{}, fmt.Errorf("could not find main worktree (no .git directory found)")
}

func parsePorcelain(data []byte) ([]Worktree, error) {
	var worktrees []Worktree
	var current Worktree
	inEntry := false

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if inEntry {
				worktrees = append(worktrees, current)
				current = Worktree{}
				inEntry = false
			}
			continue
		}

		key, value, _ := strings.Cut(line, " ")
		inEntry = true

		switch key {
		case "worktree":
			current.Path = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			current.Branch = "(detached)"
		case "prunable":
			current.Prunable = true
			current.PrunableReason = value
		}
	}

	// flush final entry if output doesn't end with a blank line
	if inEntry {
		worktrees = append(worktrees, current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parsing worktree list: %w", err)
	}

	return worktrees, nil
}

// MergedBranches returns local branches already merged into base. It excludes
// base itself and any branch whose tip points at exactly the same commit as
// base — such branches have no unique history yet (freshly created, or fast-
// forwarded with no subsequent activity) and are not safe to auto-prune, since
// a fresh worktree's work-in-progress has no commits to mark it distinct.
func MergedBranches(ctx context.Context, runner CommandRunner, base string) ([]string, error) {
	baseOut, err := runner.Run(ctx, "git", "rev-parse", base+"^{commit}")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse %s: %w", base, err)
	}
	baseSHA := strings.TrimSpace(string(baseOut))

	out, err := runner.Run(ctx, "git", "for-each-ref",
		"--merged="+base,
		"--format=%(refname:short) %(objectname)",
		"refs/heads/")
	if err != nil {
		return nil, fmt.Errorf("git for-each-ref --merged: %w", err)
	}
	var branches []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, sha, ok := strings.Cut(line, " ")
		if !ok || name == base || sha == baseSHA {
			continue
		}
		branches = append(branches, name)
	}
	return branches, nil
}

type CommitInfo struct {
	ShortSHA  string    `json:"sha"`
	Subject   string    `json:"subject"`
	Committed time.Time `json:"committed"`
}

func Dirty(ctx context.Context, runner CommandRunner, path string) (bool, error) {
	out, err := runner.Run(ctx, "git", "-C", path, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return len(bytes.TrimSpace(out)) > 0, nil
}

// AheadBehind returns the number of commits the branch at path is ahead of and
// behind its upstream. hasUpstream is false when no upstream is configured; this
// is not an error.
func AheadBehind(
	ctx context.Context, runner CommandRunner, path string,
) (ahead, behind int, hasUpstream bool, err error) {
	if _, err := runner.Run(ctx, "git", "-C", path, "rev-parse", "--abbrev-ref", "@{upstream}"); err != nil {
		return 0, 0, false, nil
	}
	out, err := runner.Run(ctx, "git", "-C", path, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return 0, 0, true, fmt.Errorf("git rev-list: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0, true, fmt.Errorf("unexpected rev-list output: %q", string(out))
	}
	a, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, true, fmt.Errorf("parse ahead count: %w", err)
	}
	b, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, true, fmt.Errorf("parse behind count: %w", err)
	}
	return a, b, true, nil
}

func LastCommit(ctx context.Context, runner CommandRunner, path string) (CommitInfo, error) {
	out, err := runner.Run(ctx, "git", "-C", path, "log", "-1", "--format=%h%x00%s%x00%cI")
	if err != nil {
		return CommitInfo{}, fmt.Errorf("git log: %w", err)
	}
	trimmed := strings.TrimRight(string(out), "\n")
	if trimmed == "" {
		return CommitInfo{}, nil
	}
	parts := strings.SplitN(trimmed, "\x00", 3)
	if len(parts) != 3 {
		return CommitInfo{}, fmt.Errorf("unexpected log output: %q", string(out))
	}
	committed, err := time.Parse(time.RFC3339, parts[2])
	if err != nil {
		return CommitInfo{}, fmt.Errorf("parse commit time %q: %w", parts[2], err)
	}
	return CommitInfo{
		ShortSHA:  parts[0],
		Subject:   parts[1],
		Committed: committed,
	}, nil
}

func FindByBranch(wts []Worktree, branch string) (Worktree, bool) {
	for _, wt := range wts {
		if wt.Branch == branch {
			return wt, true
		}
	}
	return Worktree{}, false
}

// ErrNotFound is returned when no worktree matches the requested branch.
type ErrNotFound struct {
	Branch string
}

func (e ErrNotFound) Error() string {
	return fmt.Sprintf("no worktree found for branch %q; run `tp sync` to list worktrees", e.Branch)
}

// FindOrErr wraps FindByBranch, returning ErrNotFound when no match exists.
func FindOrErr(wts []Worktree, branch string) (Worktree, error) {
	wt, ok := FindByBranch(wts, branch)
	if !ok {
		return Worktree{}, ErrNotFound{Branch: branch}
	}
	return wt, nil
}

func isMainWorktree(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}
