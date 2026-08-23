// Package batch resolves Manifests into scheduling Members: the Chains and
// Ticket-to-branch derivation that no fleet tool has an equivalent for. It is
// the deep module for treepad's batch orchestration feature — every
// scheduling predicate that can be dangerously wrong lands here as a pure,
// table-testable function.
package batch

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

const defaultBranchPrefix = "feat/"

// ponytail: hardcoded rather than resolved from origin/HEAD — correct for the
// common case, wrong for a repo whose default branch is "master". Upgrade to
// `git symbolic-ref refs/remotes/origin/HEAD` if that repo shape shows up.
const defaultBase = "main"

// Manifest declares one Batch: a named collection of Chains. Written by an
// agent that read the Tracker — never by treepad, never by hand.
type Manifest struct {
	Name         string  `toml:"name"`
	BranchPrefix string  `toml:"branch_prefix"`
	Base         string  `toml:"base"`
	Chains       []Chain `toml:"chain"`
}

// Chain is an ordered run of Tickets, each worktree branched from the one
// before it. Base overrides where position 0 branches from, empty inherits
// Manifest.Base; pointing it at another Chain's branch declares a fan-out.
type Chain struct {
	Base    string   `toml:"base"`
	Tickets []string `toml:"tickets"`
}

// Runner is the shape of worktree.CommandRunner, redeclared here so no
// exported signature in this package names an internal type.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// CommonDir resolves the git common dir for the repository containing the
// process's current directory. In a linked worktree .git is a file, not a
// directory, so this must be used instead of filepath.Join(path, ".git").
func CommonDir(ctx context.Context, r Runner) (string, error) {
	out, err := r.Run(ctx, "git", "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-common-dir: %w", err)
	}
	dir := strings.TrimSpace(string(out))
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve common dir %q: %w", dir, err)
	}
	return abs, nil
}

// Load reads every *.toml file under <commonDir>/treepad/batches and unions
// them. A repo with no Manifests is normal, so a missing directory returns
// nil rather than an error.
func Load(commonDir string) ([]Manifest, error) {
	batchesDir := filepath.Join(commonDir, "treepad", "batches")
	paths, err := filepath.Glob(filepath.Join(batchesDir, "*.toml"))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", batchesDir, err)
	}
	if len(paths) == 0 {
		return nil, nil
	}
	sort.Strings(paths)

	manifests := make([]Manifest, 0, len(paths))
	for _, path := range paths {
		var m Manifest
		if _, err := toml.DecodeFile(path, &m); err != nil {
			return nil, fmt.Errorf("parse manifest %s: %w", path, err)
		}
		if m.Name == "" {
			stem := filepath.Base(path)
			m.Name = strings.TrimSuffix(stem, filepath.Ext(stem))
		}
		if m.BranchPrefix == "" {
			m.BranchPrefix = defaultBranchPrefix
		}
		if m.Base == "" {
			m.Base = defaultBase
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}
