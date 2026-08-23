// Package hook runs lifecycle hooks defined in .treepad.toml.
package hook

import (
	"context"
	"fmt"

	"github.com/O-Marsters-1997/treepad/internal/profile"
)

// Event identifies a lifecycle point in a treepad operation.
type Event string

const (
	PreNew         Event = "pre_new"
	PostNew        Event = "post_new"
	PreRemove      Event = "pre_remove"
	PostRemove     Event = "post_remove"
	PreSync        Event = "pre_sync"
	PostSync       Event = "post_sync"
	PostConfigInit Event = "post_config_init"
)

// CutEvents are the events a worktree cut fires, in the order it fires them.
// PreSync and PostSync are on the list because config sync happens inside the
// cut, so a caller that only knew about pre_new and post_new would miss half of
// what a cut can run. Declared beside the constants so a new event is named and
// listed in the same file.
var CutEvents = []Event{PreNew, PreSync, PostSync, PostNew}

var TeardownEvents = []Event{PreRemove, PostRemove}

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
//
// Interactive gives the command the caller's terminal so it can prompt.
// Without it the command runs with no TTY and its output is discarded, which
// makes pickers and confirmation prompts silently fall back to non-interactive
// mode.
type HookEntry struct {
	Command     string   `toml:"command"`
	Only        []string `toml:"only"`
	Except      []string `toml:"except"`
	Interactive bool     `toml:"interactive,omitempty"`
}

// Runner executes a list of hook entries with the provided data.
type Runner interface {
	Run(ctx context.Context, hooks []HookEntry, data Data) error
}

// Config holds the hook entries for each event.
type Config struct {
	PreNew         []HookEntry `toml:"pre_new,omitempty"`
	PostNew        []HookEntry `toml:"post_new,omitempty"`
	PreRemove      []HookEntry `toml:"pre_remove,omitempty"`
	PostRemove     []HookEntry `toml:"post_remove,omitempty"`
	PreSync        []HookEntry `toml:"pre_sync,omitempty"`
	PostSync       []HookEntry `toml:"post_sync,omitempty"`
	PostConfigInit []HookEntry `toml:"post_config_init,omitempty"`
}

func (c Config) IsZero() bool {
	return len(c.PreNew) == 0 && len(c.PostNew) == 0 &&
		len(c.PreRemove) == 0 && len(c.PostRemove) == 0 &&
		len(c.PreSync) == 0 && len(c.PostSync) == 0 &&
		len(c.PostConfigInit) == 0
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
	case PostConfigInit:
		return c.PostConfigInit
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
// p times the pre and post hook phases as "<event>_hooks" stages.
func RunSandwich(
	ctx context.Context, p profile.Profiler, r Runner, cfg Config,
	pre, post Event, data Data, do func() error,
) (*PostErr, error) {
	preDone := p.Stage(string(pre) + "_hooks")
	preErr := Run(ctx, r, cfg, pre, data)
	preDone()
	if preErr != nil {
		return nil, fmt.Errorf("%s hook: %w", pre, preErr)
	}
	if err := do(); err != nil {
		return nil, err
	}
	postDone := p.Stage(string(post) + "_hooks")
	postRunErr := Run(ctx, r, cfg, post, data)
	postDone()
	if postRunErr != nil {
		return &PostErr{Event: post, Err: postRunErr}, nil
	}
	return nil, nil
}
