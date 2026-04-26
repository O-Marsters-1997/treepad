package fromspec

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/treepadtest"
	"treepad/internal/ui"
)

const bulkTOML = `
[from_spec]
agent_command = []
`

// issueJSON builds a fake gh issue view JSON response.
func fakeIssueJSON(title, body string) []byte {
	return []byte(`{"title":"` + title + `","body":"` + body + `"}`)
}

// bulkSeqResponses builds seqRunner responses for N happy-path issues.
// Per issue: gh response, git worktree list, git worktree add.
func bulkSeqResponses(mainPath string, issues []struct{ title, body string }) []treepadtest.RunResponse {
	porcelain := treepadtest.MainWorktreePorcelain(mainPath)
	var responses []treepadtest.RunResponse
	for _, issue := range issues {
		responses = append(responses,
			treepadtest.RunResponse{Output: fakeIssueJSON(issue.title, issue.body)},
			treepadtest.RunResponse{Output: porcelain},
			treepadtest.RunResponse{Output: nil},
		)
	}
	return responses
}

func TestFromSpecBulk(t *testing.T) {
	mainPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(mainPath, ".git"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	outputDir := t.TempDir()

	writeBulkTOML := func(t *testing.T) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(bulkTOML), 0o644); err != nil {
			t.Fatalf("write toml: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(mainPath, ".treepad.toml")) })
	}

	t.Run("happy path: 3 issues creates 3 worktrees with PROMPT.md", func(t *testing.T) {
		writeBulkTOML(t)

		issues := []struct{ title, body string }{
			{"Add retry to sync", "implement retry logic"},
			{"Cache cleanup", "remove stale cache"},
			{"Fix auth flow", "patch oauth handler"},
		}
		runner := &treepadtest.SeqRunner{Responses: bulkSeqResponses(mainPath, issues)}
		pt := &treepadtest.FakePassthroughRunner{}
		var logBuf bytes.Buffer
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = pt
		deps.Log = treepadtest.NewPrinter(&logBuf)

		results, failed, err := FromSpecBulk(context.Background(), deps, FromSpecBulkInput{
			Issues:       []int{12, 14, 19},
			BranchPrefix: "feat/",
			Base:         "main",
			OutputDir:    outputDir,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if failed != 0 {
			t.Errorf("failed = %d, want 0", failed)
		}
		if len(results) != 3 {
			t.Fatalf("len(results) = %d, want 3", len(results))
		}
		for i, r := range results {
			if r.Err != nil {
				t.Errorf("results[%d].Err = %v, want nil", i, r.Err)
			}
			if r.PromptPath == "" {
				t.Errorf("results[%d].PromptPath is empty", i)
			} else {
				content, err := os.ReadFile(r.PromptPath)
				if err != nil {
					t.Errorf("results[%d]: read PROMPT.md: %v", i, err)
				}
				if !strings.Contains(string(content), issues[i].body) {
					t.Errorf("results[%d]: PROMPT.md does not contain spec body", i)
				}
				if !strings.Contains(string(content), "Implement the ticket") {
					t.Errorf("results[%d]: PROMPT.md does not contain default ending", i)
				}
			}
		}
		if !strings.Contains(results[0].Branch, "add-retry-to-sync") {
			t.Errorf("branch[0] = %q, want to contain add-retry-to-sync", results[0].Branch)
		}

		// No agent invoked.
		if len(pt.Calls) != 0 {
			t.Errorf("PTRunner called %d times, want 0", len(pt.Calls))
		}

		// Summary printed.
		summary := logBuf.String()
		if !strings.Contains(summary, "3 succeeded") {
			t.Errorf("summary missing '3 succeeded'; got: %s", summary)
		}
	})

	t.Run("middle issue has empty body: continues, records failure", func(t *testing.T) {
		writeBulkTOML(t)
		porcelain := treepadtest.MainWorktreePorcelain(mainPath)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: fakeIssueJSON("Add retry", "implement retry")},
			{Output: porcelain},
			{Output: nil},
			{Output: fakeIssueJSON("Empty issue", "")}, // empty body
			{Output: fakeIssueJSON("Fix auth", "patch oauth")},
			{Output: porcelain},
			{Output: nil},
		}}
		var logBuf bytes.Buffer
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}
		deps.Log = treepadtest.NewPrinter(&logBuf)

		results, failed, err := FromSpecBulk(context.Background(), deps, FromSpecBulkInput{
			Issues:    []int{12, 14, 19},
			Base:      "main",
			OutputDir: outputDir,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if failed != 1 {
			t.Errorf("failed = %d, want 1", failed)
		}
		if results[1].Err == nil || !strings.Contains(results[1].Err.Error(), "empty body") {
			t.Errorf("results[1].Err = %v, want empty body error", results[1].Err)
		}
		if results[0].Err != nil || results[2].Err != nil {
			t.Errorf("expected surrounding issues to succeed")
		}
		if !strings.Contains(logBuf.String(), "1 failed") {
			t.Errorf("summary missing '1 failed'")
		}
	})

	t.Run("middle issue gh exits non-zero: continues, records failure", func(t *testing.T) {
		writeBulkTOML(t)
		porcelain := treepadtest.MainWorktreePorcelain(mainPath)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: fakeIssueJSON("Add retry", "implement retry")},
			{Output: porcelain},
			{Output: nil},
			{Output: nil, Err: treepadtest.ErrExitNonZero},
			{Output: fakeIssueJSON("Fix auth", "patch oauth")},
			{Output: porcelain},
			{Output: nil},
		}}
		var logBuf bytes.Buffer
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.Log = ui.New(&logBuf)

		results, failed, err := FromSpecBulk(context.Background(), deps, FromSpecBulkInput{
			Issues:    []int{12, 14, 19},
			Base:      "main",
			OutputDir: outputDir,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if failed != 1 {
			t.Errorf("failed = %d, want 1", failed)
		}
		if results[1].Err == nil || !strings.Contains(results[1].Err.Error(), "gh issue view 14") {
			t.Errorf("results[1].Err = %v, want gh error for issue 14", results[1].Err)
		}
	})

	t.Run("branch name collision: second issue gets -N suffix", func(t *testing.T) {
		writeBulkTOML(t)
		porcelain := treepadtest.MainWorktreePorcelain(mainPath)

		// Both issues have the same title.
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: fakeIssueJSON("Duplicate Title", "spec body one")},
			{Output: porcelain},
			{Output: nil},
			{Output: fakeIssueJSON("Duplicate Title", "spec body two")},
			{Output: porcelain},
			{Output: nil},
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		var logBuf bytes.Buffer
		deps.Log = ui.New(&logBuf)

		results, failed, err := FromSpecBulk(context.Background(), deps, FromSpecBulkInput{
			Issues:    []int{10, 11},
			Base:      "main",
			OutputDir: outputDir,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if failed != 0 {
			t.Errorf("failed = %d, want 0", failed)
		}
		if results[0].Branch == results[1].Branch {
			t.Errorf("branch names should be distinct; both are %q", results[0].Branch)
		}
		if !strings.HasSuffix(results[1].Branch, "-11") {
			t.Errorf("second branch %q should end with -11", results[1].Branch)
		}
	})

	t.Run("no agent is ever invoked", func(t *testing.T) {
		writeBulkTOML(t)
		porcelain := treepadtest.MainWorktreePorcelain(mainPath)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: fakeIssueJSON("Some feature", "do the thing")},
			{Output: porcelain},
			{Output: nil},
		}}
		pt := &treepadtest.FakePassthroughRunner{}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = pt
		deps.Log = ui.New(&bytes.Buffer{})

		_, _, _ = FromSpecBulk(context.Background(), deps, FromSpecBulkInput{
			Issues:    []int{42},
			Base:      "main",
			OutputDir: outputDir,
		})

		if len(pt.Calls) != 0 {
			t.Errorf("PTRunner called %d times, want 0", len(pt.Calls))
		}
	})

	t.Run("__TREEPAD_CD__ never emitted", func(t *testing.T) {
		writeBulkTOML(t)
		porcelain := treepadtest.MainWorktreePorcelain(mainPath)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: fakeIssueJSON("Some feature", "do the thing")},
			{Output: porcelain},
			{Output: nil},
		}}
		var stdout bytes.Buffer
		var logBuf bytes.Buffer
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}
		deps.Out = &stdout
		deps.Log = ui.New(&logBuf)

		_, _, _ = FromSpecBulk(context.Background(), deps, FromSpecBulkInput{
			Issues:    []int{42},
			Base:      "main",
			OutputDir: outputDir,
		})

		if strings.Contains(stdout.String(), "__TREEPAD_CD__") {
			t.Errorf("stdout should not contain __TREEPAD_CD__; got: %s", stdout.String())
		}
		if strings.Contains(logBuf.String(), "__TREEPAD_CD__") {
			t.Errorf("log should not contain __TREEPAD_CD__; got: %s", logBuf.String())
		}
	})
}
