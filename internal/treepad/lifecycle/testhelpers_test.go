package lifecycle

import (
	"os"
	"path/filepath"
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

func writeTOML(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".treepad.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write .treepad.toml: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
}
