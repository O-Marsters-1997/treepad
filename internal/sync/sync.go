package sync

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"treepad/internal/git"
)

type Config struct {
	SourceDir string
	TargetDir string
}

type Syncer interface {
	SyncVSCode(cfg Config) error
	SyncClaudeSettings(cfg Config) error
	SyncEnvFiles(cfg Config) error
}

type FileSyncer struct{}

// SyncVSCode copies settings.json as-is from source; callers are responsible for filtering it first.
func (FileSyncer) SyncVSCode(cfg Config) error {
	srcDir := filepath.Join(cfg.SourceDir, ".vscode")
	dstDir := filepath.Join(cfg.TargetDir, ".vscode")

	static := []string{"settings.json", "tasks.json", "launch.json", "extensions.json"}
	for _, name := range static {
		if err := copyFile(filepath.Join(srcDir, name), filepath.Join(dstDir, name)); err != nil {
			return fmt.Errorf("sync .vscode/%s: %w", name, err)
		}
	}

	snippets, err := filepath.Glob(filepath.Join(srcDir, "*.code-snippets"))
	if err != nil {
		return fmt.Errorf("glob code-snippets: %w", err)
	}
	for _, src := range snippets {
		dst := filepath.Join(dstDir, filepath.Base(src))
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("sync %s: %w", filepath.Base(src), err)
		}
	}
	return nil
}

func (FileSyncer) SyncClaudeSettings(cfg Config) error {
	src := filepath.Join(cfg.SourceDir, ".claude", "settings.local.json")
	dst := filepath.Join(cfg.TargetDir, ".claude", "settings.local.json")
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("sync .claude/settings.local.json: %w", err)
	}
	return nil
}

func (FileSyncer) SyncEnvFiles(cfg Config) error {
	for _, name := range []string{".env", ".env.docker-compose"} {
		src := filepath.Join(cfg.SourceDir, name)
		dst := filepath.Join(cfg.TargetDir, name)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("sync %s: %w", name, err)
		}
	}
	return nil
}

func SyncAll(syncer Syncer, source string, worktrees []git.Worktree) error {
	srcAbs, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolve source path: %w", err)
	}

	for _, wt := range worktrees {
		wtAbs, err := filepath.Abs(wt.Path)
		if err != nil {
			return fmt.Errorf("resolve worktree path %s: %w", wt.Path, err)
		}
		if wtAbs == srcAbs {
			continue // source and target are the same directory
		}

		cfg := Config{SourceDir: srcAbs, TargetDir: wtAbs}
		fmt.Printf("  syncing configs → %s (%s)\n", wt.Branch, wt.Path)

		if err := syncer.SyncVSCode(cfg); err != nil {
			return err
		}
		if err := syncer.SyncClaudeSettings(cfg); err != nil {
			return err
		}
		if err := syncer.SyncEnvFiles(cfg); err != nil {
			return err
		}
	}
	return nil
}

// copyFile returns nil when src does not exist rather than an error.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
