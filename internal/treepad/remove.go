package treepad

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RemoveInput struct {
	Branch    string
	OutputDir string
	// Cwd overrides os.Getwd for testing the cwd-inside guard.
	Cwd string
}

func Remove(ctx context.Context, d Deps, in RemoveInput) error {
	v, err := d.NewRepoView(ctx, in.OutputDir)
	if err != nil {
		return err
	}

	if in.Branch == v.Main().Branch {
		return fmt.Errorf("cannot remove the main worktree")
	}

	s, err := v.Inspect(ctx, in.Branch, Probe{})
	if err != nil {
		return err
	}

	cwd := in.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}
	}
	if rel, relErr := filepath.Rel(s.Path, cwd); relErr == nil && !strings.HasPrefix(rel, "..") {
		return fmt.Errorf("cannot remove the worktree you are currently in; cd elsewhere first")
	}

	return removeWorktreeAndArtifact(ctx, d, s.Worktree, v.Main(), v.OutputDir(), false)
}
