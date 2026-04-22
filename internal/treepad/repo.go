package treepad

import (
	"context"
	"fmt"
	"path/filepath"

	"treepad/internal/slug"
	"treepad/internal/worktree"
)

// Probe controls which git queries Snapshots/Inspect run per worktree.
// Zero value means no extra queries beyond the prologue.
type Probe struct {
	Dirty       bool
	AheadBehind bool
	LastCommit  bool
}

// ProbeAll requests every available git query.
var ProbeAll = Probe{Dirty: true, AheadBehind: true, LastCommit: true}

// Snapshot is a worktree with optional git-state fields populated by a Probe.
// Probed is false for prunable worktrees — their state cannot be queried.
type Snapshot struct {
	worktree.Worktree
	Probed      bool
	Dirty       bool
	Ahead       int
	Behind      int
	HasUpstream bool
	LastCommit  worktree.CommitInfo
}

// RepoView is a read-only view of the repo opened once per operation.
// It captures the runner at construction time, eliminating ctx/runner threading at every call site.
type RepoView interface {
	Main() worktree.Worktree
	Worktrees() []worktree.Worktree
	Slug() string
	OutputDir() string
	// Snapshots returns all non-prunable worktrees probed according to p.
	// Prunable worktrees are included with Probed=false.
	Snapshots(ctx context.Context, p Probe) ([]Snapshot, error)
	// Inspect finds the worktree for branch and optionally probes it.
	// Returns an error if the branch has no worktree.
	Inspect(ctx context.Context, branch string, p Probe) (Snapshot, error)
	// MergedInto returns a set of branches merged into base.
	MergedInto(ctx context.Context, base string) (map[string]bool, error)
}

type gitRepoView struct {
	runner    worktree.CommandRunner
	main      worktree.Worktree
	worktrees []worktree.Worktree
	slug      string
	outputDir string
}

// OpenRepo runs the standard prologue (list → main → slug → outputDir) and
// returns a RepoView that captures d.Runner for subsequent queries.
func OpenRepo(ctx context.Context, d Deps, outputDir string) (RepoView, error) {
	wts, err := listWorktrees(ctx, d)
	if err != nil {
		return nil, err
	}
	main, err := worktree.MainWorktree(wts)
	if err != nil {
		return nil, err
	}
	repoSlug := slug.Slug(filepath.Base(main.Path))
	out, err := resolveOutputDir(outputDir, repoSlug)
	if err != nil {
		return nil, err
	}
	return &gitRepoView{
		runner:    d.Runner,
		main:      main,
		worktrees: wts,
		slug:      repoSlug,
		outputDir: out,
	}, nil
}

func (v *gitRepoView) Main() worktree.Worktree       { return v.main }
func (v *gitRepoView) Worktrees() []worktree.Worktree { return v.worktrees }
func (v *gitRepoView) Slug() string                   { return v.slug }
func (v *gitRepoView) OutputDir() string              { return v.outputDir }

func (v *gitRepoView) Snapshots(ctx context.Context, p Probe) ([]Snapshot, error) {
	snaps := make([]Snapshot, 0, len(v.worktrees))
	for _, wt := range v.worktrees {
		s := Snapshot{Worktree: wt}
		if !wt.Prunable {
			s.Probed = true
			if err := v.probe(ctx, wt.Path, p, &s); err != nil {
				return nil, err
			}
		}
		snaps = append(snaps, s)
	}
	return snaps, nil
}

func (v *gitRepoView) Inspect(ctx context.Context, branch string, p Probe) (Snapshot, error) {
	wt, ok := worktree.FindByBranch(v.worktrees, branch)
	if !ok {
		return Snapshot{}, fmt.Errorf("no worktree found for branch %q", branch)
	}
	s := Snapshot{Worktree: wt}
	if !wt.Prunable {
		s.Probed = true
		if err := v.probe(ctx, wt.Path, p, &s); err != nil {
			return Snapshot{}, err
		}
	}
	return s, nil
}

func (v *gitRepoView) MergedInto(ctx context.Context, base string) (map[string]bool, error) {
	branches, err := worktree.MergedBranches(ctx, v.runner, base)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(branches))
	for _, b := range branches {
		set[b] = true
	}
	return set, nil
}

func (v *gitRepoView) probe(ctx context.Context, path string, p Probe, s *Snapshot) error {
	if p.Dirty {
		dirty, err := worktree.Dirty(ctx, v.runner, path)
		if err != nil {
			return err
		}
		s.Dirty = dirty
	}
	if p.AheadBehind {
		ahead, behind, hasUpstream, err := worktree.AheadBehind(ctx, v.runner, path)
		if err != nil {
			return err
		}
		s.Ahead, s.Behind, s.HasUpstream = ahead, behind, hasUpstream
	}
	if p.LastCommit {
		lc, err := worktree.LastCommit(ctx, v.runner, path)
		if err != nil {
			return err
		}
		s.LastCommit = lc
	}
	return nil
}
