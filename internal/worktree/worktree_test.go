package worktree

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type seqRunner struct {
	responses []struct {
		output []byte
		err    error
	}
	idx int
}

func (s *seqRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	if s.idx >= len(s.responses) {
		return nil, fmt.Errorf("unexpected runner call %d", s.idx)
	}
	r := s.responses[s.idx]
	s.idx++
	return r.output, r.err
}

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

func TestList_prunable(t *testing.T) {
	input := []byte(`worktree /home/user/myrepo
HEAD abc123
branch refs/heads/main

worktree /home/user/myrepo-stale
HEAD def456
branch refs/heads/stale-branch
prunable gitdir file points to non-existent location

`)
	worktrees, err := List(context.Background(), fakeRunner{output: input})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(worktrees) != 2 {
		t.Fatalf("got %d worktrees, want 2", len(worktrees))
	}
	if worktrees[0].Prunable {
		t.Error("main worktree should not be prunable")
	}
	stale := worktrees[1]
	if !stale.Prunable {
		t.Error("stale worktree Prunable = false, want true")
	}
	if stale.PrunableReason != "gitdir file points to non-existent location" {
		t.Errorf("PrunableReason = %q, want %q", stale.PrunableReason, "gitdir file points to non-existent location")
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

type seqFakeRunner struct {
	responses []fakeResponse
	idx       int
}

type fakeResponse struct {
	output []byte
	err    error
}

func (s *seqFakeRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	if s.idx >= len(s.responses) {
		return nil, errors.New("unexpected runner call")
	}
	r := s.responses[s.idx]
	s.idx++
	return r.output, r.err
}

func TestMergedBranches(t *testing.T) {
	ctx := context.Background()

	const baseSHA = "aaa111\n"

	tests := []struct {
		name      string
		responses []fakeResponse
		base      string
		want      []string
		wantErr   bool
	}{
		{
			name: "returns merged branches excluding base and fresh tips",
			responses: []fakeResponse{
				{output: []byte(baseSHA)},
				{output: []byte("main aaa111\nfeat bbb222\nfix/bug ccc333\n")},
			},
			base: "main",
			want: []string{"feat", "fix/bug"},
		},
		{
			name: "excludes branches whose tip equals base tip (fresh worktrees)",
			responses: []fakeResponse{
				{output: []byte(baseSHA)},
				{output: []byte("fresh aaa111\nreal bbb222\n")},
			},
			base: "main",
			want: []string{"real"},
		},
		{
			name: "blank lines ignored",
			responses: []fakeResponse{
				{output: []byte(baseSHA)},
				{output: []byte("\nfeat bbb222\n\n")},
			},
			base: "main",
			want: []string{"feat"},
		},
		{
			name: "nothing merged besides base",
			responses: []fakeResponse{
				{output: []byte(baseSHA)},
				{output: []byte("main aaa111\n")},
			},
			base: "main",
			want: nil,
		},
		{
			name: "rev-parse error",
			responses: []fakeResponse{
				{err: errors.New("unknown revision")},
			},
			base:    "main",
			wantErr: true,
		},
		{
			name: "for-each-ref error",
			responses: []fakeResponse{
				{output: []byte(baseSHA)},
				{err: errors.New("git for-each-ref failed")},
			},
			base:    "main",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergedBranches(ctx, &seqFakeRunner{responses: tt.responses}, tt.base)
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

func TestFindByBranch(t *testing.T) {
	wts := []Worktree{
		{Path: "/a", Branch: "main", IsMain: true},
		{Path: "/b", Branch: "feat", IsMain: false},
	}

	tests := []struct {
		name      string
		branch    string
		wantPath  string
		wantFound bool
	}{
		{"found first", "main", "/a", true},
		{"found second", "feat", "/b", true},
		{"not found", "missing", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FindByBranch(wts, tt.branch)
			if ok != tt.wantFound {
				t.Fatalf("found = %v, want %v", ok, tt.wantFound)
			}
			if ok && got.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", got.Path, tt.wantPath)
			}
		})
	}
}

func TestFindByBranch_empty(t *testing.T) {
	_, ok := FindByBranch(nil, "main")
	if ok {
		t.Fatal("expected not found on empty slice")
	}
}

func TestDirty(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		output  []byte
		err     error
		want    bool
		wantErr bool
	}{
		{
			name:   "clean worktree",
			output: []byte(""),
			want:   false,
		},
		{
			name:   "dirty worktree",
			output: []byte("M file.go\n"),
			want:   true,
		},
		{
			name:   "only whitespace",
			output: []byte("\n"),
			want:   false,
		},
		{
			name:    "runner error",
			err:     errors.New("exit status 128"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Dirty(ctx, fakeRunner{output: tt.output, err: tt.err}, "/repo")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Dirty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAheadBehind(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		responses []struct {
			output []byte
			err    error
		}
		wantAhead    int
		wantBehind   int
		wantUpstream bool
		wantErr      bool
	}{
		{
			name: "no upstream configured",
			responses: []struct {
				output []byte
				err    error
			}{
				{err: errors.New("fatal: no upstream configured")},
			},
			wantUpstream: false,
		},
		{
			name: "ahead 2, behind 1",
			responses: []struct {
				output []byte
				err    error
			}{
				{output: []byte("origin/main\n")},
				{output: []byte("2\t1\n")},
			},
			wantAhead:    2,
			wantBehind:   1,
			wantUpstream: true,
		},
		{
			name: "in sync",
			responses: []struct {
				output []byte
				err    error
			}{
				{output: []byte("origin/main\n")},
				{output: []byte("0\t0\n")},
			},
			wantAhead:    0,
			wantBehind:   0,
			wantUpstream: true,
		},
		{
			name: "rev-list error",
			responses: []struct {
				output []byte
				err    error
			}{
				{output: []byte("origin/main\n")},
				{err: errors.New("rev-list failed")},
			},
			wantUpstream: true,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &seqRunner{}
			runner.responses = append(runner.responses, tt.responses...)
			ahead, behind, hasUpstream, err := AheadBehind(ctx, runner, "/repo")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hasUpstream != tt.wantUpstream {
				t.Errorf("hasUpstream = %v, want %v", hasUpstream, tt.wantUpstream)
			}
			if ahead != tt.wantAhead {
				t.Errorf("ahead = %d, want %d", ahead, tt.wantAhead)
			}
			if behind != tt.wantBehind {
				t.Errorf("behind = %d, want %d", behind, tt.wantBehind)
			}
		})
	}
}

func TestLastCommit(t *testing.T) {
	ctx := context.Background()
	commitTime, _ := time.Parse(time.RFC3339, "2024-06-01T12:00:00Z")

	tests := []struct {
		name    string
		output  []byte
		err     error
		want    CommitInfo
		wantErr bool
	}{
		{
			name:   "happy path",
			output: []byte("abc1234\x00fix: correct thing\x002024-06-01T12:00:00Z\n"),
			want:   CommitInfo{ShortSHA: "abc1234", Subject: "fix: correct thing", Committed: commitTime},
		},
		{
			name:   "empty output (no commits)",
			output: []byte(""),
			want:   CommitInfo{},
		},
		{
			name:    "bad timestamp",
			output:  []byte("abc1234\x00subject\x00not-a-date\n"),
			wantErr: true,
		},
		{
			name:    "runner error",
			err:     errors.New("exit status 128"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LastCommit(ctx, fakeRunner{output: tt.output, err: tt.err}, "/repo")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ShortSHA != tt.want.ShortSHA {
				t.Errorf("ShortSHA = %q, want %q", got.ShortSHA, tt.want.ShortSHA)
			}
			if got.Subject != tt.want.Subject {
				t.Errorf("Subject = %q, want %q", got.Subject, tt.want.Subject)
			}
			if !got.Committed.Equal(tt.want.Committed) {
				t.Errorf("Committed = %v, want %v", got.Committed, tt.want.Committed)
			}
		})
	}
}
