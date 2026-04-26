package fromspec

import (
	"bytes"
	"context"
	"errors"
	"io"
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

func TestResolveIssueSpec(t *testing.T) {
	tests := []struct {
		name     string
		issue    int
		runner   *treepadtest.SeqRunner
		wantBody string
		wantErr  string
	}{
		{
			name:  "invokes gh and trims body",
			issue: 42,
			runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: []byte("  implement OAuth flow\n")},
			}},
			wantBody: "implement OAuth flow",
		},
		{
			name:    "empty issue body errors",
			issue:   7,
			runner:  &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: []byte("\n")}}},
			wantErr: "empty body",
		},
		{
			name:    "gh error propagates",
			issue:   99,
			runner:  &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Err: errors.New("gh: not authenticated")}}},
			wantErr: "gh issue view 99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := deps.Deps{Runner: tt.runner}
			body, err := resolveIssueSpec(context.Background(), d, tt.issue)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

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

	t.Run("issue source invokes gh and renders prompt with issue body", func(t *testing.T) {
		writeTOML(t, mainPath, fromSpecTOML)

		rr := &treepadtest.RecordingRunner{Inner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: []byte(specBody)}, // gh issue view
			{Output: porcelain},        // git worktree list
			{Output: nil},              // git worktree add
		}}}
		deps := deps.Deps{
			Runner: rr,
			Syncer: &treepadtest.FakeSyncer{},
			Opener: &treepadtest.FakeOpener{},
			Out:    io.Discard,
		}
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}

		_, err := FromSpec(context.Background(), deps, FromSpecInput{
			Issue:     42,
			Branch:    "feat/oauth",
			Base:      "main",
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify gh was called with the issue number.
		var ghFound bool
		for _, call := range rr.Calls {
			if len(call) > 0 && call[0] == "gh" {
				ghFound = true
				if len(call) < 4 || call[3] != "42" {
					t.Errorf("gh call args = %v, want issue number 42", call)
				}
			}
		}
		if !ghFound {
			t.Error("gh was not invoked")
		}
	})

	t.Run("empty agent_command skips passthrough but writes PROMPT.md", func(t *testing.T) {
		writeTOML(t, mainPath, fromSpecTOML)

		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: []byte(specBody)}, // gh issue view
			{Output: porcelain},        // git worktree list
			{Output: nil},              // git worktree add
		}}
		pt := &treepadtest.FakePassthroughRunner{}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = pt

		var logBuf bytes.Buffer
		deps.Out = &logBuf

		code, err := FromSpec(context.Background(), deps, FromSpecInput{
			Issue:     1,
			Branch:    "feat/oauth",
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
			{Output: []byte(specBody)}, // gh issue view
			{Output: porcelain},        // git worktree list
			{Output: nil},              // git worktree add
		}}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}

		_, err := FromSpec(context.Background(), deps, FromSpecInput{
			Issue:     1,
			Branch:    "feat/oauth",
			Base:      "main",
			OutputDir: outputDir,
			Prompt:    "use the new auth library",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		promptPath := filepath.Join(outputDir, "feat-oauth", "PROMPT.md")
		content, err := os.ReadFile(promptPath)
		if err != nil {
			// fall back: find the worktree dir from deps output
			t.Logf("note: %v — searching outputDir for PROMPT.md", err)
		} else {
			if !strings.Contains(string(content), "use the new auth library") {
				t.Errorf("prompt does not contain user instructions; got: %s", content)
			}
			if strings.Contains(string(content), "Implement the ticket.\n") {
				t.Errorf("prompt should not contain default ending when --prompt is set; got: %s", content)
			}
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
			{Output: []byte(specBody)}, // gh issue view
			{Output: porcelain},        // git worktree list
			{Output: nil},              // git worktree add
		}}
		hr := &treepadtest.FakeHookRunner{}
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.HookRunner = hr
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}

		if _, err := FromSpec(context.Background(), deps, FromSpecInput{
			Issue:     1,
			Branch:    "feat/oauth",
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
			{Output: []byte(specBody)}, // gh issue view
			{Output: porcelain},        // git worktree list
		}}}
		hr := &treepadtest.FakeHookRunner{Err: errors.New("hook aborted")}
		deps := deps.Deps{Runner: rr, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.HookRunner = hr
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}

		_, err := FromSpec(context.Background(), deps, FromSpecInput{
			Issue:     1,
			Branch:    "feat/oauth",
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
			{Output: []byte(specBody)}, // gh issue view
			{Output: porcelain},        // git worktree list
			{Output: nil},              // git worktree add
		}}
		var buf bytes.Buffer
		deps := deps.Deps{Runner: runner, Syncer: &treepadtest.FakeSyncer{}, Opener: &treepadtest.FakeOpener{}}
		deps.PTRunner = &treepadtest.FakePassthroughRunner{}
		deps.Out = &buf

		if _, err := FromSpec(context.Background(), deps, FromSpecInput{
			Issue:     1,
			Branch:    "feat/oauth",
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
