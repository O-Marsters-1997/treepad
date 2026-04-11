package editor

import "treepad/internal/worktree"

// Adapter is the port that each editor implements.
// Configure applies all editor-specific setup (workspace file generation,
// config syncing, plugin recommendations) for every worktree.
// It is idempotent — safe to call repeatedly.
type Adapter interface {
	Configure(worktrees []worktree.Worktree, opts Options) error
	Name() string
}

// Options carries everything an adapter might need.
// The command layer populates this; adapters use what they require.
type Options struct {
	SourceDir string
	OutputDir string
	Slug      string
	SyncOnly  bool
}
