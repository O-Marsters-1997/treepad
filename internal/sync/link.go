package sync

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Linker symlinks entries in TargetDir to the same path under SourceDir.
type Linker struct {
	// Tracked reports whether rel is tracked in git in SourceDir.
	Tracked func(rel string) bool
	// Ignored reports whether rel is ignored by git in TargetDir.
	Ignored func(rel string) bool
	// Force backs up a differing copy as <path>.treepad-bak and links over it.
	Force bool
}

// LinkSkip is an entry Reconcile left alone, with the reason.
type LinkSkip struct {
	Path   string
	Reason string
}

// LinkResult lists what Reconcile did per entry.
type LinkResult struct {
	Created   []string
	Replaced  []string
	Removed   []string
	Unchanged []string
	Skipped   []LinkSkip
	// Unignored lists created links that git does not ignore in TargetDir.
	Unignored []string
}

// LinkIssue is an entry whose link in TargetDir is not healthy.
type LinkIssue struct {
	Path   string
	Kind   string // "broken", "replaced" or "unignored"
	Detail string
}

// expand turns entries into concrete relative paths. A trailing "/" names one
// directory link, a glob expands to its untracked matches in SourceDir, and
// paths under a linked directory are dropped as covered by it.
func (l Linker) expand(entries []string, cfg Config, res *LinkResult) []string {
	var dirs, paths []string
	for _, e := range entries {
		switch {
		case strings.HasSuffix(e, "/"):
			d := strings.TrimSuffix(e, "/")
			dirs = append(dirs, d)
			paths = append(paths, d)
		case doublestar.ValidatePattern(e) && strings.ContainsAny(e, "*?[{"):
			matches, _ := doublestar.Glob(os.DirFS(cfg.SourceDir), e)
			for _, m := range matches {
				if l.Tracked == nil || !l.Tracked(m) {
					paths = append(paths, m)
				}
			}
		default:
			paths = append(paths, e)
		}
	}
	var out []string
	for _, p := range paths {
		if parent, ok := coveredBy(p, dirs); ok {
			res.skip(p, "covered by parent "+parent)
			continue
		}
		out = append(out, p)
	}
	return out
}

func coveredBy(p string, dirs []string) (string, bool) {
	for _, d := range dirs {
		if strings.HasPrefix(p, d+"/") {
			return d, true
		}
	}
	return "", false
}

// Reconcile converges TargetDir on entries: a file, a trailing-"/" directory
// or a glob. It links where the path is absent, converts a copy identical to
// the source (any copy under Force, after backing it up), and removes owned
// symlinks no longer wanted. A symlink is owned only if it targets
// SourceDir/<its own relative path>.
func (l Linker) Reconcile(entries []string, cfg Config) (LinkResult, error) {
	var res LinkResult
	paths := l.expand(entries, cfg, &res)
	if err := pruneOwned(paths, cfg, &res); err != nil {
		return res, err
	}
	for _, e := range paths {
		src := filepath.Join(cfg.SourceDir, e)
		dst := filepath.Join(cfg.TargetDir, e)

		if _, err := os.Lstat(src); err != nil {
			res.skip(e, "absent in source")
			continue
		}
		if l.Tracked != nil && l.Tracked(e) {
			res.skip(e, "tracked in git")
			continue
		}

		target, err := os.Readlink(dst)
		switch {
		case err == nil && target == src:
			res.Unchanged = append(res.Unchanged, e)
			continue
		case err == nil:
			res.skip(e, "path is a symlink not owned by treepad")
			continue
		case errors.Is(err, os.ErrNotExist):
		default:
			if _, statErr := os.Lstat(dst); statErr == nil {
				replaced, err := l.replaceCopy(e, src, dst, &res)
				if err != nil {
					return res, err
				}
				if !replaced {
					continue
				}
				if err := os.Symlink(src, dst); err != nil {
					return res, fmt.Errorf("link %s: %w", e, err)
				}
				res.Replaced = append(res.Replaced, e)
				continue
			}
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return res, fmt.Errorf("create parent of %s: %w", e, err)
		}
		if err := os.Symlink(src, dst); err != nil {
			return res, fmt.Errorf("link %s: %w", e, err)
		}
		res.Created = append(res.Created, e)
		if l.Ignored != nil && !l.Ignored(e) {
			res.Unignored = append(res.Unignored, e)
		}
	}
	return res, nil
}

// Unlink removes the symlinks in TargetDir that Reconcile would have created,
// leaving any other file or symlink alone. It returns the removed paths.
func (l Linker) Unlink(entries []string, cfg Config) ([]string, error) {
	var removed []string
	for _, e := range l.expand(entries, cfg, &LinkResult{}) {
		dst := filepath.Join(cfg.TargetDir, e)
		if target, err := os.Readlink(dst); err != nil || target != filepath.Join(cfg.SourceDir, e) {
			continue
		}
		if err := os.Remove(dst); err != nil {
			return removed, fmt.Errorf("unlink %s: %w", e, err)
		}
		removed = append(removed, e)
	}
	return removed, nil
}

// Inspect reports entries whose link in TargetDir is dangling, has been
// replaced by a regular file or foreign symlink, or is not ignored by git.
// Tracked entries and entries not present in TargetDir are not issues.
func (l Linker) Inspect(entries []string, cfg Config) []LinkIssue {
	var issues []LinkIssue
	for _, e := range l.expand(entries, cfg, &LinkResult{}) {
		src := filepath.Join(cfg.SourceDir, e)
		dst := filepath.Join(cfg.TargetDir, e)
		if l.Tracked != nil && l.Tracked(e) {
			continue
		}
		if _, err := os.Lstat(dst); err != nil {
			continue
		}
		target, err := os.Readlink(dst)
		if err != nil || target != src {
			if _, srcErr := os.Lstat(src); srcErr == nil {
				issues = append(issues, LinkIssue{e, "replaced", "no longer a link to " + src})
			}
			continue
		}
		if _, err := os.Stat(dst); err != nil {
			issues = append(issues, LinkIssue{e, "broken", "link target " + src + " is missing"})
			continue
		}
		if l.Ignored != nil && !l.Ignored(e) {
			issues = append(issues, LinkIssue{e, "unignored", UnignoredHint})
		}
	}
	return issues
}

// UnignoredHint explains a link that git does not ignore.
const UnignoredHint = "link is not ignored by git; a trailing-slash gitignore pattern " +
	"does not match a symlink, drop the slash or use .git/info/exclude"

func (r *LinkResult) skip(path, reason string) {
	r.Skipped = append(r.Skipped, LinkSkip{Path: path, Reason: reason})
}

// replaceCopy clears the copy at dst so a link can take its place, reporting
// false (with a skip) when the copy must be kept.
func (l Linker) replaceCopy(e, src, dst string, res *LinkResult) (bool, error) {
	if !l.Force && !sameFile(src, dst) {
		res.skip(e, "differs from source; use --force to back up and link")
		return false, nil
	}
	if l.Force && !sameFile(src, dst) {
		bak := dst + ".treepad-bak"
		if _, err := os.Lstat(bak); err == nil {
			res.skip(e, "backup "+e+".treepad-bak already exists")
			return false, nil
		}
		if err := os.Rename(dst, bak); err != nil {
			return false, fmt.Errorf("back up %s: %w", e, err)
		}
		return true, nil
	}
	if err := os.Remove(dst); err != nil {
		return false, fmt.Errorf("replace %s: %w", e, err)
	}
	return true, nil
}

func sameFile(a, b string) bool {
	ai, aerr := os.Lstat(a)
	bi, berr := os.Lstat(b)
	if aerr != nil || berr != nil || !ai.Mode().IsRegular() || !bi.Mode().IsRegular() {
		return false
	}
	ab, aerr := os.ReadFile(a)
	bb, berr := os.ReadFile(b)
	return aerr == nil && berr == nil && bytes.Equal(ab, bb)
}

// pruneOwned removes owned symlinks under TargetDir that are not in keep.
func pruneOwned(keep []string, cfg Config, res *LinkResult) error {
	want := make(map[string]bool, len(keep))
	for _, p := range keep {
		want[p] = true
	}
	return filepath.WalkDir(cfg.TargetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(cfg.TargetDir, path)
		if relErr != nil || rel == "." {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink == 0 || want[rel] {
			return nil
		}
		if target, err := os.Readlink(path); err != nil || target != filepath.Join(cfg.SourceDir, rel) {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove link %s: %w", rel, err)
		}
		res.Removed = append(res.Removed, rel)
		return nil
	})
}
