// Package hook runs lifecycle hooks defined in .treepad.toml.
package hook

import "context"

// Event identifies a lifecycle point in a treepad operation.
type Event string

const (
	PreNew     Event = "pre_new"
	PostNew    Event = "post_new"
	PreRemove  Event = "pre_remove"
	PostRemove Event = "post_remove"
	PreSync    Event = "pre_sync"
	PostSync   Event = "post_sync"
)

// Data is the context available when rendering hook command templates.
type Data struct {
	Branch       string // raw branch name
	WorktreePath string // absolute path of the worktree on disk
	Slug         string // repo slug
	HookType     string // event being fired, e.g. "post_new"
	OutputDir    string // artifact output directory
}

// HookEntry is a single hook command with optional branch filters.
// Only and Except use glob patterns (** crosses path separators).
// If Only is non-empty the branch must match at least one pattern.
// If Except is non-empty the branch must not match any pattern.
// Both conditions apply when both are set.
type HookEntry struct {
	Command string   `toml:"command"`
	Only    []string `toml:"only"`
	Except  []string `toml:"except"`
}

// Runner executes a list of hook entries with the provided data.
type Runner interface {
	Run(ctx context.Context, hooks []HookEntry, data Data) error
}

// Config holds the hook entries for each event.
type Config struct {
	PreNew     []HookEntry `toml:"pre_new"`
	PostNew    []HookEntry `toml:"post_new"`
	PreRemove  []HookEntry `toml:"pre_remove"`
	PostRemove []HookEntry `toml:"post_remove"`
	PreSync    []HookEntry `toml:"pre_sync"`
	PostSync   []HookEntry `toml:"post_sync"`
}

// IsZero reports whether no hooks are configured.
func (c Config) IsZero() bool {
	return len(c.PreNew) == 0 && len(c.PostNew) == 0 &&
		len(c.PreRemove) == 0 && len(c.PostRemove) == 0 &&
		len(c.PreSync) == 0 && len(c.PostSync) == 0
}

// For returns the hook entries for the given event.
func (c Config) For(e Event) []HookEntry {
	switch e {
	case PreNew:
		return c.PreNew
	case PostNew:
		return c.PostNew
	case PreRemove:
		return c.PreRemove
	case PostRemove:
		return c.PostRemove
	case PreSync:
		return c.PreSync
	case PostSync:
		return c.PostSync
	default:
		return nil
	}
}
