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

// Runner executes a list of hook commands with the provided data.
type Runner interface {
	Run(ctx context.Context, hooks []string, data Data) error
}

// Config holds the hook command lists for each event.
type Config struct {
	PreNew     []string `toml:"pre_new"`
	PostNew    []string `toml:"post_new"`
	PreRemove  []string `toml:"pre_remove"`
	PostRemove []string `toml:"post_remove"`
	PreSync    []string `toml:"pre_sync"`
	PostSync   []string `toml:"post_sync"`
}

// IsZero reports whether no hooks are configured.
func (c Config) IsZero() bool {
	return len(c.PreNew) == 0 && len(c.PostNew) == 0 &&
		len(c.PreRemove) == 0 && len(c.PostRemove) == 0 &&
		len(c.PreSync) == 0 && len(c.PostSync) == 0
}

// For returns the hook commands for the given event.
func (c Config) For(e Event) []string {
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
