// Package sync copies files matching gitignore-style patterns between directories.
package sync

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ErrCloneUnsupported is returned by CloneTree / cloneFile when the platform or
// filesystem does not support the fast-clone path; callers fall back to io.Copy.
var ErrCloneUnsupported = errors.New("clone not supported")

// errCloneUnsupported aliases the exported sentinel for package-internal callers.
var errCloneUnsupported = ErrCloneUnsupported

// CloneTree copies src to dst as a CoW clone on platforms that support it
// (Darwin/APFS via clonefile, Linux via copy_file_range).
// Returns ErrCloneUnsupported when the platform or filesystem does not support
// the fast-clone path; callers should fall back to a regular copy.
func CloneTree(src, dst string) error { return cloneTree(src, dst) }

type Config struct {
	SourceDir string
	TargetDir string
}

// SyncResult holds file-transfer metrics from a Sync call.
type SyncResult struct {
	Files int64
	Bytes int64
}

// Syncer copies files matching patterns from SourceDir to TargetDir.
// Patterns follow gitignore syntax: ** matches across directories,
// trailing / matches a directory and all its contents,
// and ! prefix negates a pattern.
// Files absent in SourceDir are silently skipped.
type Syncer interface {
	Sync(patterns []string, cfg Config) (SyncResult, error)
}

type FileSyncer struct{}

func (FileSyncer) Sync(patterns []string, cfg Config) (SyncResult, error) {
	if err := validatePatterns(patterns); err != nil {
		return SyncResult{}, err
	}
	include, exclude := parsePatterns(patterns)

	var result SyncResult

	// Attempt to fast-clone whole-directory include patterns before walking.
	// On Darwin/APFS this turns a 30s node_modules copy into a sub-second clone.
	cloned := make(map[string]bool)
	var walkIncludes []string
	for _, p := range include {
		dir, ok := wholeDirPattern(p)
		if !ok || !canFastClone(dir, exclude) {
			walkIncludes = append(walkIncludes, p)
			continue
		}
		src := filepath.Join(cfg.SourceDir, filepath.FromSlash(dir))
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(cfg.TargetDir, filepath.FromSlash(dir))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			walkIncludes = append(walkIncludes, p)
			continue
		}
		if err := cloneTree(src, dst); err != nil {
			walkIncludes = append(walkIncludes, p)
			continue
		}
		// Stat-walk the source to count files and bytes for the cloned tree.
		_ = filepath.WalkDir(src, func(_ string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			result.Files++
			if d.Type().IsRegular() {
				if info, e := d.Info(); e == nil {
					result.Bytes += info.Size()
				}
			}
			return nil
		})
		cloned[dir] = true
		slog.Debug("cloned tree", "dir", dir)
	}

	err := filepath.WalkDir(cfg.SourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(cfg.SourceDir, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}
		relSlash := filepath.ToSlash(rel)

		if d.Type()&fs.ModeSymlink != 0 {
			if !matchesInclude(relSlash, walkIncludes, exclude) {
				return nil
			}
			dst := filepath.Join(cfg.TargetDir, rel)
			if err := copySymlink(path, dst); err != nil {
				return fmt.Errorf("sync %s: %w", rel, err)
			}
			result.Files++
			slog.Debug("synced symlink", "rel", rel)
			return nil
		}

		if d.IsDir() {
			if rel == "." {
				return nil
			}
			if cloned[relSlash] {
				return fs.SkipDir
			}
			if !dirCouldMatch(relSlash, walkIncludes) {
				return fs.SkipDir
			}
			return nil
		}

		if !matchesInclude(relSlash, walkIncludes, exclude) {
			return nil
		}

		slog.Debug("syncing file", "rel", rel)
		dst := filepath.Join(cfg.TargetDir, rel)
		if err := copyFile(path, dst); err != nil {
			return fmt.Errorf("sync %s: %w", rel, err)
		}
		result.Files++
		if info, e := d.Info(); e == nil {
			result.Bytes += info.Size()
		}
		return nil
	})
	return result, err
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

// wholeDirPattern returns the directory name if pattern represents a complete
// directory tree (e.g. "dir/" or "dir/**") with no wildcards in the path,
// making it eligible for a single cloneTree call.
func wholeDirPattern(pattern string) (string, bool) {
	if dir, ok := strings.CutSuffix(pattern, "/"); ok && isLiteralPath(dir) {
		return dir, true
	}
	if dir, ok := strings.CutSuffix(pattern, "/**"); ok && isLiteralPath(dir) {
		return dir, true
	}
	return "", false
}

func isLiteralPath(p string) bool {
	return p != "" && !strings.ContainsAny(p, "*?[")
}

// canFastClone reports whether a directory may be cloned as a whole: true when
// no exclude pattern targets anything inside it.
func canFastClone(dir string, excludes []string) bool {
	prefix := dir + "/"
	for _, ex := range excludes {
		if ex == dir || strings.HasPrefix(ex, prefix) {
			return false
		}
	}
	return true
}

// dirCouldMatch reports whether any include pattern could match a path inside
// dir. Conservative: returns true when uncertain to avoid incorrect pruning.
func dirCouldMatch(dir string, includes []string) bool {
	prefix := dir + "/"
	for _, p := range includes {
		literalEnd := strings.IndexAny(p, "*?[")
		var literal string
		if literalEnd < 0 {
			literal = strings.TrimSuffix(p, "/")
		} else {
			literal = p[:literalEnd]
		}
		// Pattern targets something inside this dir.
		if strings.HasPrefix(literal, prefix) {
			return true
		}
		// Pattern IS this dir.
		if strings.TrimSuffix(literal, "/") == dir {
			return true
		}
		// dir is a subdirectory of the pattern literal (e.g. ".claude/agents" inside ".claude/").
		if strings.HasPrefix(prefix, literal+"/") {
			return true
		}
		// Wildcard portion could expand into this dir.
		if literalEnd >= 0 && strings.HasPrefix(prefix, literal) {
			return true
		}
	}
	return false
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	if err := cloneFile(src, dst); !errors.Is(err, errCloneUnsupported) {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = in.Close() }()

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
