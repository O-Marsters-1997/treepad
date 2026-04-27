package lifecycle

import (
	"io/fs"
	"path/filepath"
)

// statTree walks root and returns the count of non-directory entries and their
// total byte size. Walk errors are silently skipped; the function always returns
// whatever was counted before the error. Only called when profiling is enabled.
func statTree(root string) (files, bytes int64) {
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		files++
		if d.Type().IsRegular() {
			if info, e := d.Info(); e == nil {
				bytes += info.Size()
			}
		}
		return nil
	})
	return
}
