// Package sync copies files matching gitignore-style patterns between directories.
package sync

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type Config struct {
	SourceDir string
	TargetDir string
}

// Syncer copies files matching patterns from SourceDir to TargetDir.
// Patterns follow gitignore syntax: ** matches across directories,
// trailing / matches a directory and all its contents,
// and ! prefix negates a pattern.
// Files absent in SourceDir are silently skipped.
type Syncer interface {
	Sync(patterns []string, cfg Config) error
}

type FileSyncer struct{}

func (FileSyncer) Sync(patterns []string, cfg Config) error {
	if err := validatePatterns(patterns); err != nil {
		return err
	}
	include, exclude := parsePatterns(patterns)

	return filepath.WalkDir(cfg.SourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(cfg.SourceDir, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}

		if d.Type()&fs.ModeSymlink != 0 {
			if !matchesInclude(filepath.ToSlash(rel), include, exclude) {
				return nil
			}
			dst := filepath.Join(cfg.TargetDir, rel)
			if err := copySymlink(path, dst); err != nil {
				return fmt.Errorf("sync %s: %w", rel, err)
			}
			slog.Debug("synced symlink", "rel", rel)
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if !matchesInclude(filepath.ToSlash(rel), include, exclude) {
			return nil
		}

		slog.Debug("syncing file", "rel", rel)
		dst := filepath.Join(cfg.TargetDir, rel)
		if err := copyFile(path, dst); err != nil {
			return fmt.Errorf("sync %s: %w", rel, err)
		}
		return nil
	})
}

func parsePatterns(patterns []string) (include, exclude []string) {
	for _, p := range patterns {
		if rest, ok := strings.CutPrefix(p, "!"); ok {
			exclude = append(exclude, rest)
		} else {
			include = append(include, p)
		}
	}
	return
}

// validatePatterns checks all patterns for invalid syntax before the walk begins.
func validatePatterns(patterns []string) error {
	for _, p := range patterns {
		pat, _ := strings.CutPrefix(p, "!")
		pat, _ = strings.CutSuffix(pat, "/")
		if _, err := doublestar.Match(pat, ""); err != nil {
			return fmt.Errorf("invalid pattern %q: %w", p, err)
		}
	}
	return nil
}

// matchesInclude reports whether a forward-slash relative path should be synced.
// A pattern with a trailing / matches the named directory and all descendants.
// A ! prefix excludes an otherwise-matched path.
func matchesInclude(rel string, include, exclude []string) bool {
	for _, p := range include {
		if matchPattern(p, rel) {
			for _, ep := range exclude {
				if matchPattern(ep, rel) {
					return false
				}
			}
			return true
		}
	}
	return false
}

func matchPattern(pattern, rel string) bool {
	if dir, ok := strings.CutSuffix(pattern, "/"); ok {
		if ok, _ := doublestar.Match(dir, rel); ok {
			return true
		}
		ok, _ := doublestar.Match(dir+"/**", rel)
		return ok
	}
	ok, _ := doublestar.Match(pattern, rel)
	return ok
}

func copySymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("read symlink: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing destination: %w", err)
	}
	if err := os.Symlink(target, dst); err != nil {
		return fmt.Errorf("create symlink: %w", err)
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
