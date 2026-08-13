// Package skills embeds the agent skills treepad ships, so `tp skill install`
// can copy them onto disk without a separate download step.
package skills

import (
	"embed"
	"io/fs"
	"sort"
)

//go:embed all:treepad
var FS embed.FS

// Names returns the embedded skill names, sorted.
func Names() ([]string, error) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Open returns the filesystem rooted at the named skill's directory.
func Open(name string) (fs.FS, error) {
	return fs.Sub(FS, name)
}
