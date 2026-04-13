// Package sync copies files matching glob patterns between directories.
package sync

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Config holds the source and target directories for a single sync operation.
type Config struct {
	SourceDir string
	TargetDir string
}

// Syncer copies files matching patterns from SourceDir to TargetDir.
// Patterns are relative paths or globs anchored at SourceDir.
// Files absent in SourceDir are silently skipped.
type Syncer interface {
	Sync(patterns []string, cfg Config) error
}

type FileSyncer struct{}

func (FileSyncer) Sync(patterns []string, cfg Config) error {
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(cfg.SourceDir, pattern))
		if err != nil {
			return fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}
		slog.Debug("pattern matched", "pattern", pattern, "matches", len(matches))
		for _, src := range matches {
			rel, err := filepath.Rel(cfg.SourceDir, src)
			if err != nil {
				return fmt.Errorf("relative path for %s: %w", src, err)
			}
			dst := filepath.Join(cfg.TargetDir, rel)
			if err := copyFile(src, dst); err != nil {
				return fmt.Errorf("sync %s: %w", rel, err)
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}
	return out.Close()
}
