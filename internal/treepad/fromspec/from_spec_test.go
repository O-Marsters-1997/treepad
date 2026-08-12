package fromspec

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/config"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/lifecycle"
	"treepad/internal/treepad/repo"
	"treepad/internal/treepad/treepadtest"
)

func TestRenderPrompt(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		data    promptData
		want    string
		wantErr string
	}{
		{
			name: "renders Spec Skills Branch PromptPath",
			tmpl: "branch={{.Branch}} spec={{.Spec}} path={{.PromptPath}} skills={{range .Skills}}{{.}},{{end}}",
			data: promptData{
				Branch:     "feat/login",
				Spec:       "add login",
				PromptPath: "/repo/PROMPT.md",
				Skills:     []string{"go", "testing"},
			},
			want: "branch=feat/login spec=add login path=/repo/PROMPT.md skills=go,testing,",
		},
		{
			name:    "parse error wraps agent_command template",
			tmpl:    "{{.Unclosed",
			wantErr: "parse agent_command template",
		},
		{
			name:    "execute error wraps agent_command template",
			tmpl:    "{{.NoSuchField}}",
			wantErr: "execute agent_command template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderPrompt(tt.tmpl, tt.data)
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
		code, err := runAgent(ctx, d, nil, promptData{PromptPath: "/p"})
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
		data := promptData{WorktreePath: "/wt", PromptPath: "/wt/PROMPT.md", Prompt: "do the thing"}
		code, err := runAgent(ctx, d, []string{"claude", "{{.PromptPath}}"}, data)
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
		if len(pt.Calls[0].Args) != 1 || pt.Calls[0].Args[0] != "/wt/PROMPT.md" {
			t.Errorf("args = %v, want [/wt/PROMPT.md]", pt.Calls[0].Args)
		}
	})

	t.Run("template error surfaces with index", func(t *testing.T) {
		d := deps.Deps{PTRunner: &treepadtest.FakePassthroughRunner{}}
		_, err := runAgent(ctx, d, []string{"ok", "{{.NoSuchField}}"}, promptData{})
		if err == nil || !strings.Contains(err.Error(), "agent_command[1]") {
			t.Errorf("got error %v, want error containing agent_command[1]", err)
		}
	})

	t.Run("propagates PTRunner exit code", func(t *testing.T) {
		pt := &treepadtest.FakePassthroughRunner{ExitCode: 42}
		d := deps.Deps{PTRunner: pt}
		code, err := runAgent(ctx, d, []string{"claude"}, promptData{})
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

	const specBody = "implement OAuth flow"
	const fromSpecTOML = `
[from_spec]
agent_command = []
`

	t.Run("uses existing PROMPT.md from worktree without rendering template", func(t *testing.T) {
		wt := t.TempDir()
		existingContent := "my custom prompt"
		promptFilePath := filepath.Join(wt, "PROMPT.md")
		if err := os.WriteFile(promptFilePath, []byte(existingContent), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		res := lifecycle.CreateResult{
			WorktreePath: wt,
			RC:           repo.Context{Slug: "treepad"},
			Cfg:          config.Config{FromSpec: config.FromSpecConfig{}},
		}
		deps := deps.Deps{
			Runner: &treepadtest.SeqRunner{},
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
		}

		path, rendered, err := resolveOrBuildPrompt(deps, res, "feat/test", specBody, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != promptFilePath {
			t.Errorf("path = %q, want %q", path, promptFilePath)
		}
		if rendered != existingContent {
			t.Errorf("rendered = %q, want existing file content %q", rendered, existingContent)
		}
	})

	const ticketURL = "https://github.com/acme/widgets/issues/42"

	t.Run("ticket URL is cited verbatim in PROMPT.md, no gh invoked", func(t *testing.T) {
		writeTOML(t, mainPath, fromSpecTOML)

		rr := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain}, // git worktree list (ticket resolve)
			{Output: porcelain}, // git worktree list (lifecycle)
			{Output: nil},       // git worktree add --no-checkout
			{Output: nil},       // git checkout
		}}}
		var buf bytes.Buffer
		deps := deps.Deps{
			Runner: rr,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    &buf,
		}
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}

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

		promptPath := filepath.Join(worktreePathFromCD(t, buf.String()), "PROMPT.md")
		content, err := os.ReadFile(promptPath)
		if err != nil {
			t.Fatalf("read PROMPT.md: %v", err)
		}
		if !strings.Contains(string(content), ticketURL) {
			t.Errorf("PROMPT.md does not cite ticket URL; got: %s", content)
		}
	})

	t.Run("empty agent_command skips passthrough but writes PROMPT.md", func(t *testing.T) {
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

	t.Run("--prompt flag appends user instructions to body", func(t *testing.T) {
		writeTOML(t, mainPath, fromSpecTOML)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain}, // git worktree list (ticket resolve)
			{Output: porcelain}, // git worktree list (lifecycle)
			{Output: nil},       // git worktree add --no-checkout
			{Output: nil},       // git checkout
		}}
		var buf bytes.Buffer
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}, Out: &buf}
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}

		_, err := FromSpec(context.Background(), deps, FromSpecInput{
			Ticket:    ticketURL,
			Branch:    "feat/oauth-prompt",
			Base:      "main",
			OutputDir: outputDir,
			Prompt:    "use the new auth library",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		promptPath := filepath.Join(worktreePathFromCD(t, buf.String()), "PROMPT.md")
		content, err := os.ReadFile(promptPath)
		if err != nil {
			t.Fatalf("read PROMPT.md: %v", err)
		}
		if !strings.Contains(string(content), "use the new auth library") {
			t.Errorf("prompt does not contain user instructions; got: %s", content)
		}
		if strings.Contains(string(content), "Implement the ticket.\n") {
			t.Errorf("prompt should not contain default ending when --prompt is set; got: %s", content)
		}
	})

	t.Run("empty skills produces no Skills section", func(t *testing.T) {
		res := lifecycle.CreateResult{
			WorktreePath: t.TempDir(),
			RC:           repo.Context{Slug: "treepad"},
			Cfg:          config.Config{FromSpec: config.FromSpecConfig{Skills: nil}},
		}
		body := buildPrompt(res.Cfg.FromSpec, "feat/test", specBody, "")
		if strings.Contains(body, "## Skills") {
			t.Errorf("body should not contain '## Skills' when skills is empty; got: %s", body)
		}
		if !strings.Contains(body, "Implement the ticket.") {
			t.Errorf("body should contain default ending; got: %s", body)
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
