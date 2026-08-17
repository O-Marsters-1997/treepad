package fromspec

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
	"github.com/O-Marsters-1997/treepad/internal/ui"
)

const bulkTOML = `
[from_spec]
agent_command = []
ticket_url = "https://linear.app/acme/issue/{{.Ref}}"
`

// bulkSeqResponses builds seqRunner responses for the up-front config load
// plus N happy-path tickets. Per ticket: git worktree list, git worktree add
// --no-checkout, git checkout.
func bulkSeqResponses(mainPath string, n int) []treepadtest.RunResponse {
	porcelain := treepadtest.MainWorktreePorcelain(mainPath)
	responses := []treepadtest.RunResponse{{Output: porcelain}} // up-front config load
	for range n {
		responses = append(responses,
			treepadtest.RunResponse{Output: porcelain}, // git worktree list (lifecycle)
			treepadtest.RunResponse{Output: nil},       // git worktree add --no-checkout
			treepadtest.RunResponse{Output: nil},       // git checkout
		)
	}
	return responses
}

func TestDeriveBranch(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		ref    string
		want   string
	}{
		{name: "prefix with bare ref", prefix: "feat/", ref: "ENG-123", want: "feat/eng-123"},
		{name: "no prefix", prefix: "", ref: "42", want: "42"},
		{name: "prefix with title slug ref", prefix: "feat/", ref: "silent-refresh", want: "feat/silent-refresh"},
		{name: "duplicate refs derive the same branch", prefix: "feat/", ref: "ENG-123", want: "feat/eng-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveBranch(tt.prefix, tt.ref); got != tt.want {
				t.Errorf("deriveBranch(%q, %q) = %q, want %q", tt.prefix, tt.ref, got, tt.want)
			}
		})
	}
}

func TestFromSpecBulk(t *testing.T) {
	mainPath := makeMainWorktree(t)
	outputDir := t.TempDir()

	t.Run("happy path: 3 tickets creates 3 worktrees", func(t *testing.T) {
		writeTOML(t, mainPath, bulkTOML)

		tickets := []string{"ENG-12", "ENG-14", "https://github.com/acme/widgets/issues/19"}
		rr := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: bulkSeqResponses(mainPath, 3)}}
		pt := &treepadtest.FakePassthroughRunner{}
		var logBuf bytes.Buffer
		deps := deps.Deps{Runner: rr, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = pt
		deps.Log = treepadtest.NewPrinter(&logBuf)

		results, failed, err := FromSpecBulk(context.Background(), deps, FromSpecBulkInput{
			Tickets:      tickets,
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
		wantURLs := []string{
			"https://linear.app/acme/issue/ENG-12",
			"https://linear.app/acme/issue/ENG-14",
			"https://github.com/acme/widgets/issues/19",
		}
		for i, r := range results {
			if r.Err != nil {
				t.Errorf("results[%d].Err = %v, want nil", i, r.Err)
			}
			if r.WorktreePath == "" {
				t.Errorf("results[%d].WorktreePath is empty", i)
			}
			if r.TicketURL != wantURLs[i] {
				t.Errorf("results[%d].TicketURL = %q, want %q", i, r.TicketURL, wantURLs[i])
			}
		}
		if results[0].Branch != "feat/eng-12" {
			t.Errorf("branch[0] = %q, want feat/eng-12", results[0].Branch)
		}
		if results[2].Branch != "feat/19" {
			t.Errorf("branch[2] = %q, want feat/19", results[2].Branch)
		}

		for _, call := range rr.Calls {
			if len(call) > 0 && call[0] == "gh" {
				t.Errorf("gh should not be invoked; got call %v", call)
			}
		}

		// No agent invoked.
		if len(pt.Calls) != 0 {
			t.Errorf("PTRunner called %d times, want 0", len(pt.Calls))
		}

		// Summary printed, no leftover issue-number formatting.
		summary := logBuf.String()
		if !strings.Contains(summary, "3 succeeded") {
			t.Errorf("summary missing '3 succeeded'; got: %s", summary)
		}
		if strings.Contains(summary, "#") {
			t.Errorf("summary should not contain '#'; got: %s", summary)
		}
	})

	t.Run("middle ticket is unresolvable: continues, records failure", func(t *testing.T) {
		writeTOML(t, mainPath, `
[from_spec]
agent_command = []
`) // no ticket_url configured: a bare ref cannot resolve
		porcelain := treepadtest.MainWorktreePorcelain(mainPath)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain}, // up-front config load
			{Output: porcelain}, // git worktree list for ticket 1
			{Output: nil},       // git worktree add --no-checkout
			{Output: nil},       // git checkout
			// ticket 2 fails resolution before any runner call
			{Output: porcelain}, // git worktree list for ticket 3
			{Output: nil},       // git worktree add --no-checkout
			{Output: nil},       // git checkout
		}}
		var logBuf bytes.Buffer
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}
		deps.Log = treepadtest.NewPrinter(&logBuf)

		results, failed, err := FromSpecBulk(context.Background(), deps, FromSpecBulkInput{
			Tickets: []string{
				"https://github.com/acme/widgets/issues/12",
				"ENG-14",
				"https://github.com/acme/widgets/issues/19",
			},
			Base:      "main",
			OutputDir: outputDir,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if failed != 1 {
			t.Errorf("failed = %d, want 1", failed)
		}
		if results[1].Err == nil || !strings.Contains(results[1].Err.Error(), "no ticket_url configured") {
			t.Errorf("results[1].Err = %v, want no ticket_url configured error", results[1].Err)
		}
		if results[0].Err != nil || results[2].Err != nil {
			t.Errorf("expected surrounding tickets to succeed")
		}
		if !strings.Contains(logBuf.String(), "1 failed") {
			t.Errorf("summary missing '1 failed'")
		}
	})

	t.Run("no agent is ever invoked", func(t *testing.T) {
		writeTOML(t, mainPath, bulkTOML)

		runner := &treepadtest.SeqRunner{Responses: bulkSeqResponses(mainPath, 1)}
		pt := &treepadtest.FakePassthroughRunner{}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = pt
		deps.Log = ui.New(&bytes.Buffer{})

		_, _, _ = FromSpecBulk(context.Background(), deps, FromSpecBulkInput{
			Tickets:   []string{"ENG-42"},
			Base:      "main",
			OutputDir: outputDir,
		})

		if len(pt.Calls) != 0 {
			t.Errorf("PTRunner called %d times, want 0", len(pt.Calls))
		}
	})

	t.Run("__TREEPAD_CD__ never emitted", func(t *testing.T) {
		writeTOML(t, mainPath, bulkTOML)

		runner := &treepadtest.SeqRunner{Responses: bulkSeqResponses(mainPath, 1)}
		var stdout bytes.Buffer
		var logBuf bytes.Buffer
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}
		deps.Out = &stdout
		deps.Log = ui.New(&logBuf)

		_, _, _ = FromSpecBulk(context.Background(), deps, FromSpecBulkInput{
			Tickets:   []string{"ENG-42"},
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
