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

		if err := runner.Run(context.Background(), []string{"echo {{.Branch}}"}, testData); err != nil {
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

		if err := runner.Run(context.Background(), []string{"cmd1", "cmd2"}, testData); err == nil {
			t.Fatal("expected error, got nil")
		}
		if len(r.calls) != 1 {
			t.Errorf("got %d calls, want 1 (should stop after first failure)", len(r.calls))
		}
	})

	t.Run("runs multiple hooks sequentially", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		if err := runner.Run(context.Background(), []string{"cmd1", "cmd2"}, testData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r.calls) != 2 {
			t.Errorf("got %d calls, want 2", len(r.calls))
		}
	})

	t.Run("invalid template returns error before exec", func(t *testing.T) {
		r := &fakeRunner{}
		runner := hook.ExecRunner{Runner: r}

		if err := runner.Run(context.Background(), []string{"echo {{.Invalid"}, testData); err == nil {
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
		if err := runner.Run(context.Background(), []string{cmd}, testData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "feat/foo /repo/feat-foo myrepo post_new /tmp/workspaces"
		if got := r.calls[0][2]; got != want {
			t.Errorf("rendered = %q, want %q", got, want)
		}
	})
}

func TestConfigFor(t *testing.T) {
	cfg := hook.Config{
		PreNew:     []string{"a"},
		PostNew:    []string{"b", "c"},
		PreRemove:  []string{"d"},
		PostRemove: []string{"e"},
		PreSync:    []string{"f"},
		PostSync:   []string{"g"},
	}

	tests := []struct {
		event hook.Event
		want  []string
	}{
		{hook.PreNew, []string{"a"}},
		{hook.PostNew, []string{"b", "c"}},
		{hook.PreRemove, []string{"d"}},
		{hook.PostRemove, []string{"e"}},
		{hook.PreSync, []string{"f"}},
		{hook.PostSync, []string{"g"}},
		{"unknown", nil},
	}
	for _, tt := range tests {
		got := cfg.For(tt.event)
		if len(got) != len(tt.want) {
			t.Errorf("For(%q): got %v, want %v", tt.event, got, tt.want)
		}
	}
}

func TestConfigIsZero(t *testing.T) {
	if !(hook.Config{}).IsZero() {
		t.Error("empty Config.IsZero() = false, want true")
	}
	if (hook.Config{PreNew: []string{"x"}}).IsZero() {
		t.Error("non-empty Config.IsZero() = true, want false")
	}
}
