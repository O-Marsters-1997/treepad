package worktree

import (
	"context"
	"errors"
	"testing"
)

type stubRunner struct {
	output []byte
	err    error
}

func (s stubRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return s.output, s.err
}

var samplePorcelain = []byte(`worktree /home/user/myrepo
HEAD abc123
branch refs/heads/main

worktree /home/user/myrepo-feature
HEAD def456
branch refs/heads/feature/my-work

`)

func TestList(t *testing.T) {
	ctx := context.Background()
	worktrees, err := List(ctx, stubRunner{output: samplePorcelain})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(worktrees) != 2 {
		t.Fatalf("got %d worktrees, want 2", len(worktrees))
	}

	cases := []struct {
		idx    int
		path   string
		branch string
	}{
		{0, "/home/user/myrepo", "main"},
		{1, "/home/user/myrepo-feature", "feature/my-work"},
	}
	for _, tc := range cases {
		wt := worktrees[tc.idx]
		if wt.Path != tc.path {
			t.Errorf("worktrees[%d].Path = %q, want %q", tc.idx, wt.Path, tc.path)
		}
		if wt.Branch != tc.branch {
			t.Errorf("worktrees[%d].Branch = %q, want %q", tc.idx, wt.Branch, tc.branch)
		}
	}
}

func TestList_runnerError(t *testing.T) {
	_, err := List(context.Background(), stubRunner{err: errors.New("exit status 128")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestList_detachedHead(t *testing.T) {
	input := []byte(`worktree /home/user/myrepo
HEAD abc123
detached

`)
	worktrees, err := List(context.Background(), stubRunner{output: input})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if worktrees[0].Branch != "(detached)" {
		t.Errorf("Branch = %q, want \"(detached)\"", worktrees[0].Branch)
	}
}

func TestList_noTrailingNewline(t *testing.T) {
	// porcelain output without a final blank line (some git versions)
	input := []byte(`worktree /home/user/myrepo
HEAD abc123
branch refs/heads/main`)

	worktrees, err := List(context.Background(), stubRunner{output: input})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(worktrees) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(worktrees))
	}
}

func TestMainWorktree(t *testing.T) {
	worktrees := []Worktree{
		{Path: "/a", Branch: "feature", IsMain: false},
		{Path: "/b", Branch: "main", IsMain: true},
	}
	got, err := MainWorktree(worktrees)
	if err != nil {
		t.Fatalf("MainWorktree() error: %v", err)
	}
	if got.Path != "/b" {
		t.Errorf("Path = %q, want \"/b\"", got.Path)
	}
}

func TestMainWorktree_noneFound(t *testing.T) {
	_, err := MainWorktree([]Worktree{{Path: "/a", IsMain: false}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
