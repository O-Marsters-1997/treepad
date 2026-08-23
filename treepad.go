// Package treepad cuts git worktrees from Go code. Unlike the tp CLI, a caller
// names the repository it means rather than standing in it, so one process can
// serve several repositories concurrently.
//
// A worktree cut here is indistinguishable from one cut by tp new: config sync
// runs and lifecycle hooks fire.
package treepad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/lifecycle"
	"github.com/O-Marsters-1997/treepad/internal/treepad/repo"
	"github.com/O-Marsters-1997/treepad/internal/worktree"
)

// NewOptions parameterises New. Branch and RepoDir are required.
type NewOptions struct {
	Branch string
	Base   string
	// RepoDir is an absolute path inside the target repository.
	RepoDir string
	// OutputDir is where the editor artifact is written. Empty means
	// $HOME/<repo-slug>-workspaces, matching the CLI.
	OutputDir string
	// Stderr receives the same narrative the CLI prints. Nil discards it.
	Stderr io.Writer
}

// Worktree is a created worktree.
type Worktree struct {
	Path    string
	Branch  string
	BaseSHA string
}

// ErrPostHook reports that a post hook failed. The cut itself succeeded: the
// returned Worktree is complete and on disk.
var ErrPostHook = errors.New("post hook failed")

// repoLocks serialises cuts per repository. Two concurrent git worktree adds in
// one repository contend on the index and ref locks; a caller that manages a
// repo from several goroutines should not have to know that.
var repoLocks sync.Map // main worktree path -> *sync.Mutex

// New creates a worktree for Branch off Base in the repository at RepoDir,
// syncing configs and firing hooks exactly as tp new does.
func New(ctx context.Context, o NewOptions) (Worktree, error) {
	if o.Branch == "" {
		return Worktree{}, errors.New("treepad: NewOptions.Branch is required")
	}
	if o.RepoDir == "" {
		return Worktree{}, errors.New("treepad: NewOptions.RepoDir is required")
	}
	if !filepath.IsAbs(o.RepoDir) {
		return Worktree{}, fmt.Errorf("treepad: NewOptions.RepoDir must be absolute, got %q", o.RepoDir)
	}

	stderr := o.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	d := deps.DefaultDepsIn(o.RepoDir, io.Discard, stderr, nil)

	worktrees, err := repo.ListWorktrees(ctx, d.Runner)
	if err != nil {
		return Worktree{}, err
	}
	main, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return Worktree{}, err
	}
	defer lockRepo(main.Path)()

	baseSHA, err := d.Runner.Run(ctx, "git", "rev-parse", o.Base+"^{commit}")
	if err != nil {
		return Worktree{}, fmt.Errorf("resolve Base %q: %w", o.Base, err)
	}

	res, err := lifecycle.CreateWorktreeWithSync(ctx, d, o.Branch, o.Base, o.OutputDir)
	if err != nil {
		return Worktree{}, err
	}

	wt := Worktree{
		Path:    res.WorktreePath,
		Branch:  o.Branch,
		BaseSHA: strings.TrimSpace(string(baseSHA)),
	}
	if res.PostErr != nil {
		return wt, fmt.Errorf("%w: %w", ErrPostHook, res.PostErr)
	}
	return wt, nil
}

func lockRepo(mainPath string) (unlock func()) {
	v, _ := repoLocks.LoadOrStore(mainPath, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// RemoveOptions parameterises Remove. Branch and RepoDir are required.
type RemoveOptions struct {
	Branch string
	// RepoDir is an absolute path inside the target repository.
	RepoDir string
	// OutputDir is where the editor artifact was written. It must match the
	// OutputDir the worktree was cut with, or the artifact is left behind.
	OutputDir string
	// Force deletes a branch git considers unmerged — what a squash merge
	// leaves. It never overrides the dirty-worktree refusal.
	Force bool
	// Stderr receives the same narrative the CLI prints. Nil discards it.
	Stderr io.Writer
}

// ErrNotFound reports that no worktree is checked out on the named branch.
// Remove is idempotent against it: a second call on the same branch returns it.
var ErrNotFound = errors.New("worktree not found")

// ErrDirty reports uncommitted changes in the target worktree. Nothing has been
// touched. Force does not override it — destroying uncommitted work is a
// decision for a human who can see it.
var ErrDirty = errors.New("worktree has uncommitted changes")

// Remove deletes the worktree on Branch, its branch and its artifact, firing
// remove hooks exactly as tp remove does. Unlike the CLI it does not care where
// the calling process stands, so a caller inside the target worktree succeeds.
func Remove(ctx context.Context, o RemoveOptions) error {
	if o.Branch == "" {
		return errors.New("treepad: RemoveOptions.Branch is required")
	}
	if o.RepoDir == "" {
		return errors.New("treepad: RemoveOptions.RepoDir is required")
	}
	if !filepath.IsAbs(o.RepoDir) {
		return fmt.Errorf("treepad: RemoveOptions.RepoDir must be absolute, got %q", o.RepoDir)
	}

	stderr := o.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	d := deps.DefaultDepsIn(o.RepoDir, io.Discard, stderr, nil)

	rc, err := repo.Load(ctx, d.Runner, o.OutputDir)
	if err != nil {
		return err
	}
	if o.Branch == rc.Main.Branch {
		return fmt.Errorf("treepad: cannot remove the main worktree (branch %q)", o.Branch)
	}
	defer lockRepo(rc.Main.Path)()

	target, ok := worktree.FindByBranch(rc.Worktrees, o.Branch)
	if !ok {
		return fmt.Errorf("%w: branch %q", ErrNotFound, o.Branch)
	}

	dirty, err := worktree.Dirty(ctx, d.Runner, target.Path)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("%w: %s", ErrDirty, target.Path)
	}

	// git deletes the branch last, so without this a non-forced call on an
	// unmerged branch would remove the worktree and then refuse the branch,
	// leaving the caller half torn down.
	if !o.Force {
		if _, err := d.Runner.Run(ctx, "git", "merge-base", "--is-ancestor", o.Branch, rc.Main.Branch); err != nil {
			return fmt.Errorf("treepad: branch %q is not merged into %s; set Force to delete it anyway",
				o.Branch, rc.Main.Branch)
		}
	}

	postErr, err := lifecycle.RemoveWorktreeAndArtifact(ctx, d, target, rc.Main, rc.OutputDir, o.Force)
	if err != nil {
		return err
	}
	if postErr != nil {
		return fmt.Errorf("%w: %w", ErrPostHook, postErr)
	}
	return nil
}
