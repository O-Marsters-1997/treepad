package batch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TestCommonDir_LinkedWorktree is the acceptance criterion a naive
// filepath.Join(path, ".git") implementation fails: in a linked worktree
// .git is a file, not a directory.
func TestCommonDir_LinkedWorktree(t *testing.T) {
	mainDir := t.TempDir()
	runGit(t, mainDir, "init", "-b", "main")
	runGit(t, mainDir, "config", "user.email", "test@example.com")
	runGit(t, mainDir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(mainDir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, mainDir, "add", ".")
	runGit(t, mainDir, "commit", "-m", "init")

	linkedDir := filepath.Join(t.TempDir(), "linked")
	runGit(t, mainDir, "worktree", "add", "-b", "feat/x", linkedDir)

	wantCommonDir, err := filepath.EvalSymlinks(filepath.Join(mainDir, ".git"))
	if err != nil {
		t.Fatal(err)
	}

	t.Chdir(linkedDir)

	got, err := CommonDir(context.Background(), execRunner{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != wantCommonDir {
		t.Errorf("CommonDir() = %q, want %q", resolved, wantCommonDir)
	}
}
