package treepad

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
	"github.com/O-Marsters-1997/treepad/internal/ui"
	"github.com/O-Marsters-1997/treepad/internal/worktree"
)

func restackTestDeps(runner worktree.CommandRunner) deps.Deps {
	return deps.Deps{Runner: runner, Log: ui.New(io.Discard)}
}

func TestRestackOne(t *testing.T) {
	t.Run("clean behind-only fast-forwards", func(t *testing.T) {
		runner := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: []byte("")},              // fetch
			{Output: []byte("")},              // dirty (clean)
			{Output: []byte("origin/feat\n")}, // rev-parse @{upstream}
			{Output: []byte("0\t3\n")},        // rev-list: 0 ahead, 3 behind
			{Output: []byte("")},              // merge --ff-only
		}}}
		e := &ReportEntry{Member: batch.Member{Branch: "feat"}, WorktreePath: "/repo/feat"}
		restackOne(context.Background(), restackTestDeps(runner), e)

		if e.StackStale {
			t.Error("expected no stack-stale mark for a plain fast-forward")
		}
		if !containsCall(runner.Calls, "merge", "--ff-only") {
			t.Errorf("expected a merge --ff-only call, got: %v", runner.Calls)
		}
		if containsArg(runner.Calls, "cherry") {
			t.Errorf("git cherry must not run for a non-diverged branch: %v", runner.Calls)
		}
	})

	t.Run("clean diverged patch-equivalent resets", func(t *testing.T) {
		runner := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: []byte("")},                // fetch
			{Output: []byte("")},                // dirty (clean)
			{Output: []byte("origin/feat\n")},   // rev-parse @{upstream}
			{Output: []byte("2\t3\n")},          // rev-list: diverged
			{Output: []byte("- abc1234 msg\n")}, // cherry: patch-equivalent
			{Output: []byte("")},                // reset --hard
		}}}
		e := &ReportEntry{Member: batch.Member{Branch: "feat"}, WorktreePath: "/repo/feat"}
		restackOne(context.Background(), restackTestDeps(runner), e)

		if e.StackStale {
			t.Error("expected no stack-stale mark for a patch-equivalent reset")
		}
		if !containsCall(runner.Calls, "reset", "--hard") {
			t.Errorf("expected a reset --hard call, got: %v", runner.Calls)
		}
	})

	t.Run("dirty diverged never auto-repairs, even when patch-equivalent", func(t *testing.T) {
		runner := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: []byte("")},                // fetch
			{Output: []byte("M file.go\n")},     // dirty
			{Output: []byte("origin/feat\n")},   // rev-parse @{upstream}
			{Output: []byte("2\t3\n")},          // rev-list: diverged
			{Output: []byte("- abc1234 msg\n")}, // cherry: patch-equivalent
		}}}
		e := &ReportEntry{Member: batch.Member{Branch: "feat"}, WorktreePath: "/repo/feat"}
		restackOne(context.Background(), restackTestDeps(runner), e)

		if !e.StackStale {
			t.Error("expected stack-stale: a dirty worktree never auto-repairs")
		}
		if containsCall(runner.Calls, "merge", "--ff-only") || containsCall(runner.Calls, "reset", "--hard") {
			t.Errorf("dirty worktree must never be repaired: %v", runner.Calls)
		}
	})

	t.Run("clean diverged non-patch-equivalent never auto-repairs", func(t *testing.T) {
		runner := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: []byte("")},                // fetch
			{Output: []byte("")},                // dirty (clean)
			{Output: []byte("origin/feat\n")},   // rev-parse @{upstream}
			{Output: []byte("1\t1\n")},          // rev-list: diverged
			{Output: []byte("+ abc1234 msg\n")}, // cherry: genuinely local commit
		}}}
		e := &ReportEntry{Member: batch.Member{Branch: "feat"}, WorktreePath: "/repo/feat"}
		restackOne(context.Background(), restackTestDeps(runner), e)

		if !e.StackStale {
			t.Error("expected stack-stale: a genuinely local commit never auto-repairs")
		}
		if containsCall(runner.Calls, "merge", "--ff-only") || containsCall(runner.Calls, "reset", "--hard") {
			t.Errorf("a non-patch-equivalent diverged branch must never be repaired: %v", runner.Calls)
		}
	})

	t.Run("no upstream: nothing to repair, cherry never runs", func(t *testing.T) {
		runner := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: []byte("")},             // fetch
			{Output: []byte("")},             // dirty (clean)
			{Err: errors.New("no upstream")}, // rev-parse @{upstream}
		}}}
		e := &ReportEntry{Member: batch.Member{Branch: "feat"}, WorktreePath: "/repo/feat"}
		restackOne(context.Background(), restackTestDeps(runner), e)

		if e.StackStale {
			t.Error("a branch with no upstream has nothing to restack, should not be stack-stale")
		}
		if containsArg(runner.Calls, "cherry") {
			t.Errorf("git cherry must not run when there is no upstream: %v", runner.Calls)
		}
	})

	t.Run("ahead-only never runs cherry and is not stack-stale", func(t *testing.T) {
		runner := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: []byte("")},              // fetch
			{Output: []byte("")},              // dirty (clean)
			{Output: []byte("origin/feat\n")}, // rev-parse @{upstream}
			{Output: []byte("4\t0\n")},        // rev-list: 4 ahead, 0 behind
		}}}
		e := &ReportEntry{Member: batch.Member{Branch: "feat"}, WorktreePath: "/repo/feat"}
		restackOne(context.Background(), restackTestDeps(runner), e)

		if e.StackStale {
			t.Error("ahead-only (unpushed local work, no divergence) should not be stack-stale")
		}
		if containsArg(runner.Calls, "cherry") {
			t.Errorf("git cherry must not run for an ahead-only branch: %v", runner.Calls)
		}
		if containsCall(runner.Calls, "merge", "--ff-only") || containsCall(runner.Calls, "reset", "--hard") {
			t.Errorf("ahead-only has nothing to repair: %v", runner.Calls)
		}
	})
}

// TestRestackNeverStashes is the load-bearing assertion from issue #140:
// across every scenario restack can hit, no call ever names git stash.
// Treepad never stashes on an agent's behalf.
func TestRestackNeverStashes(t *testing.T) {
	scenarios := [][]treepadtest.RunResponse{
		{ // fast-forward
			{Output: []byte("")}, {Output: []byte("")}, {Output: []byte("origin/feat\n")},
			{Output: []byte("0\t3\n")}, {Output: []byte("")},
		},
		{ // reset
			{Output: []byte("")}, {Output: []byte("")}, {Output: []byte("origin/feat\n")},
			{Output: []byte("2\t3\n")}, {Output: []byte("- abc msg\n")}, {Output: []byte("")},
		},
		{ // dirty diverged: stale
			{Output: []byte("")}, {Output: []byte("M file.go\n")}, {Output: []byte("origin/feat\n")},
			{Output: []byte("2\t3\n")}, {Output: []byte("- abc msg\n")},
		},
		{ // clean diverged non-equivalent: stale
			{Output: []byte("")}, {Output: []byte("")}, {Output: []byte("origin/feat\n")},
			{Output: []byte("1\t1\n")}, {Output: []byte("+ abc msg\n")},
		},
	}

	var allCalls [][]string
	for i, responses := range scenarios {
		runner := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: responses}}
		e := &ReportEntry{Member: batch.Member{Branch: "feat"}, WorktreePath: "/repo/feat"}
		restackOne(context.Background(), restackTestDeps(runner), e)
		allCalls = append(allCalls, runner.Calls...)
		if containsArg(runner.Calls, "stash") {
			t.Errorf("scenario %d issued a git stash call: %v", i, runner.Calls)
		}
	}
	if containsArg(allCalls, "stash") {
		t.Errorf("git stash appeared somewhere across all restack scenarios: %v", allCalls)
	}
}

func containsCall(calls [][]string, verbs ...string) bool {
	for _, c := range calls {
		if hasSubsequence(c, verbs) {
			return true
		}
	}
	return false
}

func containsArg(calls [][]string, arg string) bool {
	for _, c := range calls {
		for _, a := range c {
			if a == arg {
				return true
			}
		}
	}
	return false
}

func hasSubsequence(call, verbs []string) bool {
	for i := 0; i+len(verbs) <= len(call); i++ {
		match := true
		for j, v := range verbs {
			if call[i+j] != v {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestRestackOneRealGitRewrittenUpstream is the acceptance-criterion fixture:
// a branch rebased and force-pushed server-side (simulating a lower pull
// request merging) resolves via `git reset --hard origin/<branch>` and lands
// the worktree exactly at origin/<branch>, losing no commit's content.
func TestRestackOneRealGitRewrittenUpstream(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	mainRepo := filepath.Join(root, "main")
	rebaseRepo := filepath.Join(root, "rebase-clone")
	featWT := filepath.Join(root, "feat-wt")

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v (in %s): %v\n%s", args, dir, err, out)
		}
	}
	writeFile := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runGit(root, "init", "--bare", "-b", "main", bare)

	runGit(root, "init", "-b", "main", mainRepo)
	writeFile(filepath.Join(mainRepo, "README.md"), "hello\n")
	runGit(mainRepo, "add", "README.md")
	runGit(mainRepo, "commit", "-m", "initial")
	runGit(mainRepo, "remote", "add", "origin", bare)
	runGit(mainRepo, "push", "-u", "origin", "main")

	runGit(mainRepo, "checkout", "-b", "feat")
	writeFile(filepath.Join(mainRepo, "feature.txt"), "feature work\n")
	runGit(mainRepo, "add", "feature.txt")
	runGit(mainRepo, "commit", "-m", "feat work")
	runGit(mainRepo, "push", "-u", "origin", "feat")
	runGit(mainRepo, "checkout", "main")

	// feat isn't checked out anywhere yet, so it's safe to worktree it here.
	runGit(mainRepo, "worktree", "add", featWT, "feat")

	// Advance main so the later server-side rebase has something to rebase onto.
	writeFile(filepath.Join(mainRepo, "later.txt"), "later\n")
	runGit(mainRepo, "add", "later.txt")
	runGit(mainRepo, "commit", "-m", "advance main")
	runGit(mainRepo, "push", "origin", "main")

	// Simulate the server-side rewrite from a separate clone, since feat is
	// checked out in featWT and git refuses to check it out elsewhere.
	runGit(root, "clone", bare, rebaseRepo)
	runGit(rebaseRepo, "checkout", "feat")
	runGit(rebaseRepo, "fetch", "origin")
	runGit(rebaseRepo, "rebase", "origin/main")
	runGit(rebaseRepo, "push", "--force", "origin", "feat")

	ctx := context.Background()
	runner := worktree.ExecRunner{}
	d := restackTestDeps(runner)
	e := &ReportEntry{Member: batch.Member{Branch: "feat"}, WorktreePath: featWT}

	restackOne(ctx, d, e)

	if e.StackStale {
		t.Fatal("expected the rewritten-upstream case to auto-repair via reset, not go stack-stale")
	}

	headOut, err := runner.Run(ctx, "git", "-C", featWT, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	originOut, err := runner.Run(ctx, "git", "-C", featWT, "rev-parse", "origin/feat")
	if err != nil {
		t.Fatalf("rev-parse origin/feat: %v", err)
	}
	if strings.TrimSpace(string(headOut)) != strings.TrimSpace(string(originOut)) {
		t.Errorf("HEAD = %s, want origin/feat = %s", headOut, originOut)
	}

	if _, err := os.Stat(filepath.Join(featWT, "feature.txt")); err != nil {
		t.Errorf("feature.txt (the branch's own commit) missing after restack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(featWT, "later.txt")); err != nil {
		t.Errorf("later.txt (from the rebased-onto main) missing after restack: %v", err)
	}
}
