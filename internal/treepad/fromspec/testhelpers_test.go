package fromspec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeMainWorktree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// worktreePathFromCD extracts the worktree path from a __TREEPAD_CD__ sentinel line.
func worktreePathFromCD(t *testing.T, out string) string {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		if after, ok := strings.CutPrefix(line, "__TREEPAD_CD__\t"); ok {
			return after
		}
	}
	t.Fatalf("no __TREEPAD_CD__ sentinel in output: %q", out)
	return ""
}

func writeTOML(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".treepad.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write .treepad.toml: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
}
