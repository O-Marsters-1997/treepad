package fromspec

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		data    agentData
		want    string
		wantErr string
	}{
		{
			name: "renders Branch TicketURL WorktreePath",
			tmpl: "branch={{.Branch}} url={{.TicketURL}} wt={{.WorktreePath}}",
			data: agentData{
				Branch:       "feat/login",
				TicketURL:    "https://github.com/acme/widgets/issues/42",
				WorktreePath: "/wt",
				Slug:         "widgets",
			},
			want: "branch=feat/login url=https://github.com/acme/widgets/issues/42 wt=/wt",
		},
		{
			name:    "parse error wraps agent_command template",
			tmpl:    "{{.Unclosed",
			wantErr: "parse agent_command template",
		},
		{
			name:    "retired PromptPath fails loudly",
			tmpl:    "{{.PromptPath}}",
			wantErr: "execute agent_command template",
		},
		{
			name:    "retired Skills fails loudly",
			tmpl:    "{{.Skills}}",
			wantErr: "execute agent_command template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderTemplate(tt.tmpl, tt.data)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunAgent(t *testing.T) {
	ctx := context.Background()

	t.Run("empty agent_command returns 0 and skips PTRunner", func(t *testing.T) {
		pt := &treepadtest.FakePassthroughRunner{}
		d := deps.Deps{PTRunner: pt, Out: &bytes.Buffer{}}
		code, err := runAgent(ctx, d, nil, agentData{WorktreePath: "/wt"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 0 {
			t.Errorf("code = %d, want 0", code)
		}
		if len(pt.Calls) != 0 {
			t.Errorf("PTRunner called %d times, want 0", len(pt.Calls))
		}
	})

	t.Run("renders each element and invokes PTRunner with worktree dir", func(t *testing.T) {
		pt := &treepadtest.FakePassthroughRunner{}
		d := deps.Deps{PTRunner: pt}
		data := agentData{WorktreePath: "/wt", TicketURL: "https://github.com/acme/widgets/issues/42"}
		code, err := runAgent(ctx, d, []string{"claude", "{{.TicketURL}}"}, data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 0 {
			t.Errorf("code = %d, want 0", code)
		}
		if len(pt.Calls) != 1 {
			t.Fatalf("PTRunner called %d times, want 1", len(pt.Calls))
		}
		if pt.Calls[0].Dir != "/wt" {
			t.Errorf("dir = %q, want /wt", pt.Calls[0].Dir)
		}
		if pt.Calls[0].Name != "claude" {
			t.Errorf("name = %q, want claude", pt.Calls[0].Name)
		}
		if len(pt.Calls[0].Args) != 1 || pt.Calls[0].Args[0] != data.TicketURL {
			t.Errorf("args = %v, want [%s]", pt.Calls[0].Args, data.TicketURL)
		}
	})

	t.Run("template error surfaces with index", func(t *testing.T) {
		d := deps.Deps{PTRunner: &treepadtest.FakePassthroughRunner{}}
		_, err := runAgent(ctx, d, []string{"ok", "{{.NoSuchField}}"}, agentData{})
		if err == nil || !strings.Contains(err.Error(), "agent_command[1]") {
			t.Errorf("got error %v, want error containing agent_command[1]", err)
		}
	})

	t.Run("propagates PTRunner exit code", func(t *testing.T) {
		pt := &treepadtest.FakePassthroughRunner{ExitCode: 42}
		d := deps.Deps{PTRunner: pt}
		code, err := runAgent(ctx, d, []string{"claude"}, agentData{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 42 {
			t.Errorf("code = %d, want 42", code)
		}
	})
}

func TestFromSpec(t *testing.T) {
	mainPath := makeMainWorktree(t)
	outputDir := t.TempDir()
	porcelain := treepadtest.MainWorktreePorcelain(mainPath)

	const fromSpecTOML = `
[from_spec]
agent_command = []
`

	const ticketURL = "https://github.com/acme/widgets/issues/42"

	t.Run("ticket URL reaches agent_command, no gh invoked", func(t *testing.T) {
		writeTOML(t, mainPath, `
[from_spec]
agent_command = ["echo", "{{.TicketURL}}"]
`)

		rr := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain}, // git worktree list (ticket resolve)
			{Output: porcelain}, // git worktree list (lifecycle)
			{Output: nil},       // git worktree add --no-checkout
			{Output: nil},       // git checkout
		}}}
		pt := &treepadtest.FakePassthroughRunner{}
		var buf bytes.Buffer
		deps := deps.Deps{
			Runner: rr,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    &buf,
			Log:    treepadtest.NewPrinter(&bytes.Buffer{}),
			// A configured agent_command reaches MaybeWarnStaleWrapper, which
			// dereferences IsTerminal.
			IsTerminal: func(io.Writer) bool { return true },
		}
		deps.PTRunner = pt

		_, err := FromSpec(context.Background(), deps, FromSpecInput{
			Ticket:    ticketURL,
			Branch:    "feat/oauth-cite",
			Base:      "main",
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, call := range rr.Calls {
			if len(call) > 0 && call[0] == "gh" {
				t.Errorf("gh should not be invoked; got call %v", call)
			}
		}

		if len(pt.Calls) != 1 {
			t.Fatalf("PTRunner called %d times, want 1", len(pt.Calls))
		}
		if got := pt.Calls[0].Args; len(got) != 1 || got[0] != ticketURL {
			t.Errorf("args = %v, want [%s]", got, ticketURL)
		}
	})

	t.Run("empty agent_command skips passthrough", func(t *testing.T) {
		writeTOML(t, mainPath, fromSpecTOML)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain}, // git worktree list (ticket resolve)
			{Output: porcelain}, // git worktree list (lifecycle)
			{Output: nil},       // git worktree add --no-checkout
			{Output: nil},       // git checkout
		}}
		pt := &treepadtest.FakePassthroughRunner{}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = pt

		var logBuf bytes.Buffer
		deps.Out = &logBuf

		code, err := FromSpec(context.Background(), deps, FromSpecInput{
			Ticket:    ticketURL,
			Branch:    "feat/oauth-empty-agent",
			Base:      "main",
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		if len(pt.Calls) != 0 {
			t.Errorf("PTRunner called %d times, want 0", len(pt.Calls))
		}
	})

	t.Run("fires pre_new and post_new hooks", func(t *testing.T) {
		toml := "[[hooks.pre_new]]\ncommand = \"marker-pre\"\n\n" +
			"[[hooks.post_new]]\ncommand = \"marker-post\"\n\n" +
			fromSpecTOML
		writeTOML(t, mainPath, toml)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain}, // git worktree list (ticket resolve)
			{Output: porcelain}, // git worktree list (lifecycle)
			{Output: nil},       // git worktree add --no-checkout
			{Output: nil},       // git checkout
		}}
		hr := &treepadtest.FakeHookRunner{}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.HookRunner = hr
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}

		if _, err := FromSpec(context.Background(), deps, FromSpecInput{
			Ticket:    ticketURL,
			Branch:    "feat/oauth-hooks",
			Base:      "main",
			OutputDir: outputDir,
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hr.Calls) != 2 {
			t.Fatalf("hook runner called %d times, want 2", len(hr.Calls))
		}
		if got := hr.Calls[0].Data.HookType; got != "pre_new" {
			t.Errorf("calls[0].HookType = %q, want pre_new", got)
		}
		if got := hr.Calls[1].Data.HookType; got != "post_new" {
			t.Errorf("calls[1].HookType = %q, want post_new", got)
		}
	})

	t.Run("pre_new failure aborts before worktree add", func(t *testing.T) {
		toml := "[[hooks.pre_new]]\ncommand = \"fail\"\n\n" + fromSpecTOML
		writeTOML(t, mainPath, toml)

		rr := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain}, // git worktree list (ticket resolve)
			{Output: porcelain}, // git worktree list (lifecycle)
		}}}
		hr := &treepadtest.FakeHookRunner{Err: errors.New("hook aborted")}
		deps := deps.Deps{Runner: rr, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.HookRunner = hr
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}

		_, err := FromSpec(context.Background(), deps, FromSpecInput{
			Ticket:    ticketURL,
			Branch:    "feat/oauth-hook-fail",
			Base:      "main",
			OutputDir: outputDir,
		})
		if err == nil || !strings.Contains(err.Error(), "hook aborted") {
			t.Errorf("got error %v, want error containing 'hook aborted'", err)
		}
		for _, call := range rr.Calls {
			if len(call) >= 3 && call[1] == "worktree" && call[2] == "add" {
				t.Error("git worktree add should not be called when pre_new hook fails")
			}
		}
	})

	t.Run("emits __TREEPAD_CD__ when Current is false", func(t *testing.T) {
		writeTOML(t, mainPath, fromSpecTOML)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain}, // git worktree list (ticket resolve)
			{Output: porcelain}, // git worktree list (lifecycle)
			{Output: nil},       // git worktree add --no-checkout
			{Output: nil},       // git checkout
		}}
		var buf bytes.Buffer
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}
		deps.Out = &buf

		if _, err := FromSpec(context.Background(), deps, FromSpecInput{
			Ticket:    ticketURL,
			Branch:    "feat/oauth-cd",
			Base:      "main",
			Current:   false,
			OutputDir: outputDir,
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "__TREEPAD_CD__") {
			t.Errorf("expected __TREEPAD_CD__ in output; got: %s", buf.String())
		}
	})
}
