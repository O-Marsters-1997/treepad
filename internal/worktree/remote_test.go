package worktree

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRemoteBranchExists(t *testing.T) {
	tests := []struct {
		name         string
		responses    []struct{ output []byte; err error }
		wantExists   bool
		wantUpstream bool
		wantErrStr   string
	}{
		{
			name:         "no upstream configured",
			responses:    []struct{ output []byte; err error }{{err: errors.New("fatal: no upstream configured")}},
			wantExists:   false,
			wantUpstream: false,
		},
		{
			name: "branch exists on remote",
			responses: []struct{ output []byte; err error }{
				{output: []byte("origin/feat\n")},
				{output: []byte("abc123\trefs/heads/feat\n")},
			},
			wantExists:   true,
			wantUpstream: true,
		},
		{
			name: "branch gone from remote",
			responses: []struct{ output []byte; err error }{
				{output: []byte("origin/feat\n")},
				{output: []byte("")},
			},
			wantExists:   false,
			wantUpstream: true,
		},
		{
			name: "extracts remote name from upstream ref",
			responses: []struct{ output []byte; err error }{
				{output: []byte("upstream-remote/feat\n")},
				{output: []byte("abc123\trefs/heads/feat\n")},
			},
			wantExists:   true,
			wantUpstream: true,
		},
		{
			name: "ls-remote network failure",
			responses: []struct{ output []byte; err error }{
				{output: []byte("origin/feat\n")},
				{err: errors.New("network unreachable")},
			},
			wantExists:   false,
			wantUpstream: true,
			wantErrStr:   "git ls-remote",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &seqRunner{}
			runner.responses = append(runner.responses, tt.responses...)

			exists, hasUpstream, err := RemoteBranchExists(context.Background(), runner, "/repo", "feat")

			if tt.wantErrStr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrStr) {
					t.Fatalf("got error %v, want error containing %q", err, tt.wantErrStr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if exists != tt.wantExists {
				t.Errorf("exists = %v, want %v", exists, tt.wantExists)
			}
			if hasUpstream != tt.wantUpstream {
				t.Errorf("hasUpstream = %v, want %v", hasUpstream, tt.wantUpstream)
			}
		})
	}
}
