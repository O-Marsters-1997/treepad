package worktree

import (
	"context"
	"errors"
	"testing"
)

type fakeRunner struct {
	output []byte
	err    error
}

func (f fakeRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return f.output, f.err
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
	worktrees, err := List(ctx, fakeRunner{output: samplePorcelain})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(worktrees) != 2 {
		t.Fatalf("got %d worktrees, want 2", len(worktrees))
	}

	cases := []struct {
		name   string
		idx    int
		path   string
		branch string
	}{
		{"main worktree", 0, "/home/user/myrepo", "main"},
		{"feature worktree", 1, "/home/user/myrepo-feature", "feature/my-work"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wt := worktrees[tc.idx]
			if wt.Path != tc.path {
				t.Errorf("Path = %q, want %q", wt.Path, tc.path)
			}
			if wt.Branch != tc.branch {
				t.Errorf("Branch = %q, want %q", wt.Branch, tc.branch)
			}
		})
	}
}

func TestList_runnerError(t *testing.T) {
	_, err := List(context.Background(), fakeRunner{err: errors.New("exit status 128")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestList_detachedHead(t *testing.T) {
	input := []byte(`worktree /home/user/myrepo
HEAD abc123
detached

`)
	worktrees, err := List(context.Background(), fakeRunner{output: input})
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

	worktrees, err := List(context.Background(), fakeRunner{output: input})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(worktrees) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(worktrees))
	}
}

func TestParsePorcelain_doesNotSetIsMain(t *testing.T) {
	wts, err := parsePorcelain(samplePorcelain)
	if err != nil {
		t.Fatal(err)
	}
	for i, wt := range wts {
		if wt.IsMain {
			t.Errorf("worktrees[%d].IsMain = true, want false (parsePorcelain must not touch filesystem)", i)
		}
	}
}

func TestMergedBranches(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		output  []byte
		err     error
		base    string
		want    []string
		wantErr bool
	}{
		{
			name:   "returns merged branches excluding base",
			output: []byte("main\nfeat\nfix/bug\n"),
			base:   "main",
			want:   []string{"feat", "fix/bug"},
		},
		{
			name:   "blank lines ignored",
			output: []byte("\nfeat\n\n"),
			base:   "main",
			want:   []string{"feat"},
		},
		{
			name:   "nothing merged besides base",
			output: []byte("main\n"),
			base:   "main",
			want:   nil,
		},
		{
			name:    "runner error",
			err:     errors.New("git not found"),
			base:    "main",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergedBranches(ctx, fakeRunner{output: tt.output, err: tt.err}, tt.base)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMainWorktree(t *testing.T) {
	tests := []struct {
		name      string
		worktrees []Worktree
		wantPath  string
		wantErr   bool
	}{
		{
			name: "returns main worktree",
			worktrees: []Worktree{
				{Path: "/a", Branch: "feature", IsMain: false},
				{Path: "/b", Branch: "main", IsMain: true},
			},
			wantPath: "/b",
		},
		{
			name:      "error when none found",
			worktrees: []Worktree{{Path: "/a", IsMain: false}},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MainWorktree(tt.worktrees)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", got.Path, tt.wantPath)
			}
		})
	}
}
