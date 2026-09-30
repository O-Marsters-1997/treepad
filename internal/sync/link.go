package sync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Linker symlinks entries in TargetDir to the same path under SourceDir.
type Linker struct {
	// Tracked reports whether rel is tracked in git in SourceDir.
	Tracked func(rel string) bool
}

// LinkSkip is an entry Reconcile left alone, with the reason.
type LinkSkip struct {
	Path   string
	Reason string
}

// LinkResult lists what Reconcile did per entry.
type LinkResult struct {
	Created   []string
	Unchanged []string
	Skipped   []LinkSkip
}

// Reconcile links each literal file entry. It only creates a symlink where the
// path is absent or already a link to SourceDir/entry; anything else is skipped.
func (l Linker) Reconcile(entries []string, cfg Config) (LinkResult, error) {
	var res LinkResult
	for _, e := range entries {
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
				res.skip(e, "path exists and is not a symlink")
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
	}
	return res, nil
}

func (r *LinkResult) skip(path, reason string) {
	r.Skipped = append(r.Skipped, LinkSkip{Path: path, Reason: reason})
}
