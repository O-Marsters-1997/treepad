package gh

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/O-Marsters-1997/treepad/batch"
)

type staticRunner struct {
	out []byte
	err error
}

func (r staticRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return r.out, r.err
}

func TestAvailable(t *testing.T) {
	t.Run("gh absent", func(t *testing.T) {
		r := staticRunner{err: &exec.Error{Name: "gh", Err: exec.ErrNotFound}}
		if Available(context.Background(), r) {
			t.Error("Available() = true, want false when gh is absent")
		}
	})

	t.Run("gh present but unauthenticated", func(t *testing.T) {
		r := staticRunner{err: errors.New("exit status 1")}
		if Available(context.Background(), r) {
			t.Error("Available() = true, want false when gh is unauthenticated")
		}
	})

	t.Run("gh present and authenticated", func(t *testing.T) {
		r := staticRunner{}
		if !Available(context.Background(), r) {
			t.Error("Available() = false, want true")
		}
	})
}

// countingRunner asserts PRList issues exactly one gh invocation regardless
// of fleet size.
type countingRunner struct {
	out   []byte
	calls int
}

func (r *countingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls++
	return r.out, nil
}

const prListJSON = `[
	{"number": 12, "headRefName": "feat/eng-12", "baseRefName": "main", "state": "OPEN", "url": "https://x/12"},
	{"number": 13, "headRefName": "feat/eng-13", "baseRefName": "feat/eng-12", "state": "MERGED", "url": "https://x/13"},
	{"number": 14, "headRefName": "feat/eng-14", "baseRefName": "feat/eng-12", "state": "CLOSED", "url": "https://x/14"}
]`

func TestPRList(t *testing.T) {
	t.Run("issues exactly one gh invocation regardless of fleet size", func(t *testing.T) {
		r := &countingRunner{out: []byte(prListJSON)}
		if _, err := PRList(context.Background(), r); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.calls != 1 {
			t.Errorf("gh invoked %d times, want 1", r.calls)
		}
	})

	t.Run("returns a map keyed by head branch, including merged and closed PRs", func(t *testing.T) {
		r := &countingRunner{out: []byte(prListJSON)}
		prs, err := PRList(context.Background(), r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(prs) != 3 {
			t.Fatalf("len(prs) = %d, want 3", len(prs))
		}

		want := map[string]batch.PR{
			"feat/eng-12": {
				Number: 12, HeadRefName: "feat/eng-12", BaseRefName: "main", State: "OPEN", URL: "https://x/12",
			},
			"feat/eng-13": {
				Number: 13, HeadRefName: "feat/eng-13", BaseRefName: "feat/eng-12", State: "MERGED", URL: "https://x/13",
			},
			"feat/eng-14": {
				Number: 14, HeadRefName: "feat/eng-14", BaseRefName: "feat/eng-12", State: "CLOSED", URL: "https://x/14",
			},
		}
		for branch, wantPR := range want {
			got, ok := prs[branch]
			if !ok {
				t.Errorf("prs[%q] missing", branch)
				continue
			}
			if got != wantPR {
				t.Errorf("prs[%q] = %+v, want %+v", branch, got, wantPR)
			}
		}
	})

	t.Run("gh call failing returns an error", func(t *testing.T) {
		r := staticRunner{err: errors.New("gh: command not found")}
		if _, err := PRList(context.Background(), r); err == nil {
			t.Error("expected an error, got nil")
		}
	})
}
