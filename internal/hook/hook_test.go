package hook_test

import (
	"context"
	"errors"
	"testing"

	"treepad/internal/hook"
)

type fakeRunner struct {
	calls [][]string
	err   error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	entry := append([]string{name}, args...)
	f.calls = append(f.calls, entry)
	return nil, f.err
}

var testData = hook.Data{
	Branch:       "feat/foo",
	WorktreePath: "/repo/feat-foo",
	Slug:         "myrepo",
	HookType:     "post_new",
	OutputDir:    "/tmp/workspaces",
}

func TestExecRunnerRun(t *testing.T) {
	t.Run("renders template and runs via sh -c", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		if err := runner.Run(context.Background(), []hook.HookEntry{{Command: "echo {{.Branch}}"}}, testData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 1 {
			t.Fatalf("got %d calls, want 1", len(r.calls))
		}
		got := r.calls[0]
		if got[0] != "sh" || got[1] != "-c" || got[2] != "echo feat/foo" {
			t.Errorf("unexpected call args: %v", got)
		}
	})

	t.Run("empty list is no-op", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		if err := runner.Run(context.Background(), nil, testData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 0 {
			t.Errorf("got %d calls, want 0", len(r.calls))
		}
	})

	t.Run("stops on first error", func(t *testing.T) {
		r := &fakeRunner{err: errors.New("exit status 1")}
		runner := hook.ExecRunner{Runner: r}

		hooks := []hook.HookEntry{{Command: "cmd1"}, {Command: "cmd2"}}
		if err := runner.Run(context.Background(), hooks, testData); err == nil {
			t.Fatal("expected error, got nil")
		}
		if len(r.calls) != 1 {
			t.Errorf("got %d calls, want 1 (should stop after first failure)", len(r.calls))
		}
	})

	t.Run("runs multiple hooks sequentially", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		hooks := []hook.HookEntry{{Command: "cmd1"}, {Command: "cmd2"}}
		if err := runner.Run(context.Background(), hooks, testData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 2 {
			t.Errorf("got %d calls, want 2", len(r.calls))
		}
	})

	t.Run("invalid template returns error before exec", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		if err := runner.Run(context.Background(), []hook.HookEntry{{Command: "echo {{.Invalid"}}, testData); err == nil {
			t.Fatal("expected error for bad template, got nil")
		}
		if len(r.calls) != 0 {
			t.Errorf("got %d runner calls, want 0 (should fail before exec)", len(r.calls))
		}
	})

	t.Run("all template fields available", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		cmd := "{{.Branch}} {{.WorktreePath}} {{.Slug}} {{.HookType}} {{.OutputDir}}"
		if err := runner.Run(context.Background(), []hook.HookEntry{{Command: cmd}}, testData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "feat/foo /repo/feat-foo myrepo post_new /tmp/workspaces"
		if got := r.calls[0][2]; got != want {
			t.Errorf("rendered = %q, want %q", got, want)
		}
	})

	t.Run("skips entry when branch does not match only filter", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		hooks := []hook.HookEntry{{Command: "cmd1", Only: []string{"fix/*"}}}
		if err := runner.Run(context.Background(), hooks, testData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 0 {
			t.Errorf("got %d calls, want 0 (branch feat/foo should not match fix/*)", len(r.calls))
		}
	})

	t.Run("runs entry when branch matches only filter", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		hooks := []hook.HookEntry{{Command: "cmd1", Only: []string{"feat/*"}}}
		if err := runner.Run(context.Background(), hooks, testData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 1 {
			t.Errorf("got %d calls, want 1", len(r.calls))
		}
	})

	t.Run("skips entry when branch matches except filter", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		hooks := []hook.HookEntry{{Command: "cmd1", Except: []string{"feat/*"}}}
		if err := runner.Run(context.Background(), hooks, testData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 0 {
			t.Errorf("got %d calls, want 0 (feat/foo matches except filter)", len(r.calls))
		}
	})

	t.Run("only and except AND semantics: skips when only matches but except also matches", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		hooks := []hook.HookEntry{{Command: "cmd1", Only: []string{"feat/**"}, Except: []string{"feat/foo"}}}
		if err := runner.Run(context.Background(), hooks, testData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 0 {
			t.Errorf("got %d calls, want 0 (feat/foo excluded by except)", len(r.calls))
		}
	})

	t.Run("doublestar pattern matches nested branch", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		deepData := hook.Data{Branch: "feat/JIRA-123/my-thing", HookType: "post_new"}
		hooks := []hook.HookEntry{{Command: "cmd1", Only: []string{"feat/**"}}}
		if err := runner.Run(context.Background(), hooks, deepData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 1 {
			t.Errorf("got %d calls, want 1 (feat/** should match feat/JIRA-123/my-thing)", len(r.calls))
		}
	})
}

func TestConfigFor(t *testing.T) {
	e := func(cmd string) hook.HookEntry { return hook.HookEntry{Command: cmd} }
	cfg := hook.Config{
		PreNew:     []hook.HookEntry{e("a")},
		PostNew:    []hook.HookEntry{e("b"), e("c")},
		PreRemove:  []hook.HookEntry{e("d")},
		PostRemove: []hook.HookEntry{e("e")},
		PreSync:    []hook.HookEntry{e("f")},
		PostSync:   []hook.HookEntry{e("g")},
	}

	tests := []struct {
		event hook.Event
		want  int
	}{
		{hook.PreNew, 1},
		{hook.PostNew, 2},
		{hook.PreRemove, 1},
		{hook.PostRemove, 1},
		{hook.PreSync, 1},
		{hook.PostSync, 1},
		{"unknown", 0},
	}
	for _, tt := range tests {
		got := cfg.For(tt.event)
		if len(got) != tt.want {
			t.Errorf("For(%q): got %d entries, want %d", tt.event, len(got), tt.want)
		}
	}
}

func TestConfigIsZero(t *testing.T) {
	if !(hook.Config{}).IsZero() {
		t.Error("empty Config.IsZero() = false, want true")
	}
	if (hook.Config{PreNew: []hook.HookEntry{{Command: "x"}}}).IsZero() {
		t.Error("non-empty Config.IsZero() = true, want false")
	}
}
