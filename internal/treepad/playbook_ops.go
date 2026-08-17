package treepad

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/repo"
)

type PlaybookNewInput struct {
	// Name is the playbook's basename, without the .md extension.
	Name  string
	Force bool
}

// PlaybookNew writes stdin verbatim to .claude/playbooks/<name>.md in the main
// worktree. Treepad composes nothing and interpolates nothing: see ADR 0002.
func PlaybookNew(ctx context.Context, d deps.Deps, in PlaybookNewInput) error {
	if in.Name == "" || strings.ContainsAny(in.Name, `/\`) {
		return fmt.Errorf("invalid playbook name %q: must be a bare name with no path separators", in.Name)
	}

	rc, err := repo.Load(ctx, d.Runner, "")
	if err != nil {
		return err
	}

	body, err := io.ReadAll(d.In)
	if err != nil {
		return fmt.Errorf("read playbook body: %w", err)
	}
	if len(body) == 0 {
		return fmt.Errorf("playbook body is empty; pipe the body on stdin")
	}

	dir := filepath.Join(rc.Main.Path, ".claude", "playbooks")
	path := filepath.Join(dir, in.Name+".md")
	if err := ensureClear(path, in.Force); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write playbook: %w", err)
	}
	d.Log.OK("wrote playbook to %s", path)
	return nil
}
