// Package sync copies files matching gitignore-style patterns between directories.
package sync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/sync/errgroup"
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

// Config controls where FileSyncer reads from and writes to.
type Config struct {
	SourceDir string
	TargetDir string
	// Stage is an optional profiler hook; nil means no-op.
	// Call: done := cfg.Stage("name"); defer done()
	Stage   func(string) func()
	Workers int // concurrent copy goroutines; 0 = runtime.NumCPU()
}

func stageOf(cfg Config) func(string) func() {
	if cfg.Stage != nil {
		return cfg.Stage
	}
	return func(_ string) func() { return func() {} }
}

// SyncResult holds file-transfer metrics from a Sync call.
type SyncResult struct {
	Files int64
}

// Syncer copies files matching patterns from SourceDir to TargetDir.
// Patterns follow gitignore syntax: ** matches across directories,
// trailing / matches a directory and all its contents,
// and ! prefix negates a pattern.
// Files absent in SourceDir are silently skipped.
type Syncer interface {
	Sync(patterns []string, cfg Config) (SyncResult, error)
}

// FileSyncer is the production Syncer implementation.
type FileSyncer struct{}

// copyJob is one unit of copy work delivered from the walk producer to the worker pool.
type copyJob struct {
	src     string
	dst     string
	rel     string // forward-slash relative path, for error messages
	symlink bool
}

func (FileSyncer) Sync(patterns []string, cfg Config) (SyncResult, error) {
	if err := validatePatterns(patterns); err != nil {
		return SyncResult{}, err
	}
	include, exclude := parsePatterns(patterns)
	stage := stageOf(cfg)

	// Whole-directory fast-clone phase: run each eligible clone concurrently.
	// Each goroutine writes to results[i] — a unique index — so no mutex is needed.
	// On Darwin/APFS clonefile is metadata-only; on Linux copy_file_range copies bytes.
	// Errors are soft: failed clones fall back to the walk phase.
	type cloneResult struct{ dir, pattern string }
	results := make([]cloneResult, len(include))
	for i, p := range include {
		dir, ok := wholeDirPattern(p)
		if !ok || !canFastClone(dir, exclude) {
			results[i].pattern = p
		}
	}
	cloneDone := stage("sync.clone_pass")
	var cg errgroup.Group
	for i, p := range include {
		dir, ok := wholeDirPattern(p)
		if !ok || !canFastClone(dir, exclude) {
			continue
		}
		i, p, dir := i, p, dir
		srcDir := filepath.Join(cfg.SourceDir, filepath.FromSlash(dir))
		dstDir := filepath.Join(cfg.TargetDir, filepath.FromSlash(dir))
		cg.Go(func() error {
			if _, err := os.Stat(srcDir); err != nil {
				return nil // source absent; skip silently
			}
			if err := os.MkdirAll(filepath.Dir(dstDir), 0o755); err != nil {
				results[i].pattern = p
				return nil
			}
			if err := cloneTree(srcDir, dstDir); err != nil {
				results[i].pattern = p
				return nil
			}
			slog.Debug("cloned tree", "dir", dir)
			results[i].dir = dir
			return nil
		})
	}
	_ = cg.Wait()
	cloneDone()

	var result SyncResult
	cloned := make(map[string]bool)
	var walkIncludes []string
	for _, r := range results {
		if r.dir != "" {
			cloned[r.dir] = true
			result.Files++ // one entry per cloned tree
		}
		if r.pattern != "" {
			walkIncludes = append(walkIncludes, r.pattern)
		}
	}

	if len(walkIncludes) == 0 {
		return result, nil
	}

	// Walk phase: producer goroutine walks the source tree and enqueues copy jobs;
	// a bounded worker pool copies files concurrently. Each worker keeps its own
	// madeDir set to avoid redundant MkdirAll calls within that goroutine's work.
	workers := cfg.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	jobs := make(chan copyJob, workers*2)

	var fileCount atomic.Int64
	g, gctx := errgroup.WithContext(context.Background())

	walkDone := stage("sync.walk")
	for range workers {
		g.Go(func() error {
			madeDir := make(map[string]struct{})
			for j := range jobs {
				if gctx.Err() != nil {
					continue // drain cancelled jobs without doing work
				}
				var err error
				if j.symlink {
					err = copySymlinkCached(j.src, j.dst, madeDir)
					if err == nil {
						fileCount.Add(1)
						slog.Debug("synced symlink", "rel", j.rel)
					}
				} else {
					err = copyFileCached(j.src, j.dst, madeDir)
					if err == nil {
						fileCount.Add(1)
						slog.Debug("synced file", "rel", j.rel)
					}
				}
				if err != nil {
					return fmt.Errorf("sync %s: %w", j.rel, err)
				}
			}
			return nil
		})
	}

	g.Go(func() error {
		defer close(jobs)
		return filepath.WalkDir(cfg.SourceDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if gctx.Err() != nil {
				return filepath.SkipAll
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
				select {
				case jobs <- copyJob{src: path, dst: dst, rel: relSlash, symlink: true}:
				case <-gctx.Done():
					return filepath.SkipAll
				}
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

			dst := filepath.Join(cfg.TargetDir, rel)
			select {
			case jobs <- copyJob{src: path, dst: dst, rel: relSlash}:
			case <-gctx.Done():
				return filepath.SkipAll
			}
			return nil
		})
	})

	walkErr := g.Wait()
	walkDone()
	result.Files += fileCount.Load()
	return result, walkErr
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

func copySymlinkCached(src, dst string, made map[string]struct{}) error {
	target, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("read symlink: %w", err)
	}
	if err := ensureParentDir(dst, made); err != nil {
		return err
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
	return copyFileCached(src, dst, nil)
}

func copyFileCached(src, dst string, made map[string]struct{}) error {
	if err := ensureParentDir(dst, made); err != nil {
		return err
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

// ensureParentDir calls MkdirAll on dir(dst) at most once per run when made is non-nil.
func ensureParentDir(dst string, made map[string]struct{}) error {
	dir := filepath.Dir(dst)
	if made != nil {
		if _, ok := made[dir]; ok {
			return nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	if made != nil {
		made[dir] = struct{}{}
	}
	return nil
}
