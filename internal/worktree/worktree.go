// Package worktree defines the shared Worktree value type used across packages.
// It has no project-internal imports and must never acquire any.
package worktree

// Worktree represents a single git worktree checkout on disk.
type Worktree struct {
	Path   string
	Branch string // stripped of refs/heads/ prefix; "(detached)" when detached
	IsMain bool   // true when .git entry is a directory, not a file
}
