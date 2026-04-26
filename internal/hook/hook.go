// Package hook runs lifecycle hooks defined in .treepad.toml.
package hook

import (
	"context"
	"fmt"
)

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

func (c Config) IsZero() bool {
	return len(c.PreNew) == 0 && len(c.PostNew) == 0 &&
		len(c.PreRemove) == 0 && len(c.PostRemove) == 0 &&
		len(c.PreSync) == 0 && len(c.PostSync) == 0
}

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

// PostErr holds a post-hook failure. The caller should log it as a warning —
// post failures are non-blocking; the main operation is already complete.
type PostErr struct {
	Event Event
	Err   error
}

func (p *PostErr) Error() string {
	return fmt.Sprintf("%s hook failed: %v", p.Event, p.Err)
}

// Run executes the hooks for event e, returning an error on any hook failure.
func Run(ctx context.Context, r Runner, cfg Config, e Event, data Data) error {
	entries := cfg.For(e)
	if len(entries) == 0 {
		return nil
	}
	data.HookType = string(e)
	return r.Run(ctx, entries, data)
}

// RunSandwich runs pre → do → post. Pre failure aborts and returns an error.
// Post failure returns a non-nil *PostErr with a nil main error — the caller
// should log it as a warning.
func RunSandwich(
	ctx context.Context, r Runner, cfg Config,
	pre, post Event, data Data, do func() error,
) (*PostErr, error) {
	if err := Run(ctx, r, cfg, pre, data); err != nil {
		return nil, fmt.Errorf("%s hook: %w", pre, err)
	}
	if err := do(); err != nil {
		return nil, err
	}
	if err := Run(ctx, r, cfg, post, data); err != nil {
		return &PostErr{Event: post, Err: err}, nil
	}
	return nil, nil
}
