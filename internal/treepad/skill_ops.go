package treepad

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/repo"

	"treepad/skills"
)

type SkillInstallInput struct {
	// Names selects which embedded skills to install. Empty means all.
	Names []string
	// Local writes to .agents/skills in the main worktree instead of the
	// user's home directory.
	Local bool
	// Force overwrites a skill directory (or compat symlink) that already exists.
	Force bool
}

// compatHarness is an agent harness that does not yet read the shared
// .agents/skills location (https://agentskills.io) and needs a symlink
// pointing back at the canonical copy. Codex, Cursor, Gemini CLI, Copilot CLI
// and opencode all read .agents/skills natively and are deliberately absent
// from this list.
type compatHarness struct {
	name string
	dir  string // harness config dir, relative to the install base, e.g. ".claude"
}

var compatHarnesses = []compatHarness{
	{name: "claude-code", dir: ".claude"},
}

func SkillInstall(ctx context.Context, d deps.Deps, in SkillInstallInput) error {
	base, err := installBase(ctx, d, in.Local)
	if err != nil {
		return err
	}

	available, err := skills.Names()
	if err != nil {
		return err
	}

	names := in.Names
	if len(names) == 0 {
		names = available
	}

	canonicalRoot := filepath.Join(base, ".agents", "skills")
	linked := 0
	for _, name := range names {
		if !slices.Contains(available, name) {
			return fmt.Errorf("unknown skill %q; available: %v", name, available)
		}
		sub, err := skills.Open(name)
		if err != nil {
			return fmt.Errorf("open skill %s: %w", name, err)
		}

		dest := filepath.Join(canonicalRoot, name)
		if err := ensureClear(dest, in.Force); err != nil {
			return err
		}
		if err := os.MkdirAll(canonicalRoot, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", canonicalRoot, err)
		}
		if err := os.CopyFS(dest, sub); err != nil {
			return fmt.Errorf("install skill %s: %w", name, err)
		}
		d.Log.OK("installed skill %s to %s", name, dest)

		for _, h := range compatHarnesses {
			if _, err := os.Stat(filepath.Join(base, h.dir)); err != nil {
				continue // harness not present on this machine; skip silently
			}
			if err := linkCompat(d, dest, base, h, name, in.Force); err != nil {
				return err
			}
			linked++
		}
	}
	if linked == 0 {
		d.Log.Info("no known agent harness detected alongside %s; install one to pick up the skill automatically", base)
	}
	return nil
}

// linkCompat symlinks a compat harness's skills dir entry back at the
// canonical copy, using a relative target so the link keeps resolving after
// being synced into a new worktree (see internal/sync.copySymlinkCached,
// which recreates symlinks verbatim rather than dereferencing them).
func linkCompat(d deps.Deps, dest, base string, h compatHarness, name string, force bool) error {
	linkPath := filepath.Join(base, h.dir, "skills", name)
	if err := ensureClear(linkPath, force); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(linkPath), err)
	}
	target, err := filepath.Rel(filepath.Dir(linkPath), dest)
	if err != nil {
		return fmt.Errorf("resolve link target for %s: %w", h.name, err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		return fmt.Errorf("link skill %s for %s: %w", name, h.name, err)
	}
	d.Log.OK("linked skill %s for %s", name, h.name)
	return nil
}

// ensureClear removes a stale path when force is set, or errors if the path
// already exists. Lstat (not Stat) is used deliberately so a compat symlink
// counts as "already exists" even when its target is unreadable.
func ensureClear(path string, force bool) error {
	if !force {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("%s already exists; pass --force to overwrite", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("check %s: %w", path, err)
		}
		return nil
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

type SkillListInput struct{}

func SkillList(_ context.Context, d deps.Deps, _ SkillListInput) error {
	names, err := skills.Names()
	if err != nil {
		return err
	}
	for _, name := range names {
		if _, err := fmt.Fprintln(d.Out, name); err != nil {
			return err
		}
	}
	return nil
}

func installBase(ctx context.Context, d deps.Deps, local bool) (string, error) {
	if !local {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home directory: %w", err)
		}
		return home, nil
	}

	rc, err := repo.Load(ctx, d.Runner, "")
	if err != nil {
		return "", err
	}
	return rc.Main.Path, nil
}
