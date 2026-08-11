package treepad

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/treepadtest"
	"treepad/internal/ui"
)

// writeGlobalConfig writes content to a new global config file and points
// $TREEPAD_CONFIG at it for the duration of the test.
func writeGlobalConfig(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("TREEPAD_CONFIG", path)
}

func TestConfigInit(t *testing.T) {
	t.Run("fires PostConfigInit hook from an inherited global config", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		writeGlobalConfig(t, "[[hooks.post_config_init]]\ncommand = \"marker\"\n")

		hr := &treepadtest.FakeHookRunner{}
		d := deps.Deps{
			Runner:     &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: treepadtest.MainWorktreePorcelain(mainPath)}}},
			HookRunner: hr,
			Log:        ui.New(&bytes.Buffer{}),
		}

		err := ConfigInit(context.Background(), d, ConfigInitInput{Inherit: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hr.Calls) != 1 {
			t.Fatalf("hook runner called %d times, want 1", len(hr.Calls))
		}
		if got := hr.Calls[0].Data.HookType; got != "post_config_init" {
			t.Errorf("HookType = %q, want post_config_init", got)
		}
		if got := hr.Calls[0].Data.Branch; got != "main" {
			t.Errorf("Branch = %q, want main", got)
		}
		if got := hr.Calls[0].Data.WorktreePath; got != mainPath {
			t.Errorf("WorktreePath = %q, want %q", got, mainPath)
		}
	})

	t.Run("no hooks configured is a no-op", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		hr := &treepadtest.FakeHookRunner{}
		d := deps.Deps{
			Runner:     &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: treepadtest.MainWorktreePorcelain(mainPath)}}},
			HookRunner: hr,
			Log:        ui.New(&bytes.Buffer{}),
		}

		err := ConfigInit(context.Background(), d, ConfigInitInput{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hr.Calls) != 0 {
			t.Errorf("hook runner called %d times, want 0", len(hr.Calls))
		}
	})

	t.Run("PostConfigInit failure logs warning but does not abort", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		writeGlobalConfig(t, "[[hooks.post_config_init]]\ncommand = \"fail\"\n")

		var logBuf bytes.Buffer
		d := deps.Deps{
			Runner:     &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{{Output: treepadtest.MainWorktreePorcelain(mainPath)}}},
			HookRunner: &treepadtest.FakeHookRunner{Err: errors.New("boom")},
			Log:        ui.New(&logBuf),
		}

		err := ConfigInit(context.Background(), d, ConfigInitInput{Inherit: true})
		if err != nil {
			t.Fatalf("PostConfigInit hook failure should not abort operation, got error: %v", err)
		}
		if !strings.Contains(logBuf.String(), "post_config_init hook failed") {
			t.Errorf("expected warning in log output, got:\n%s", logBuf.String())
		}
	})
}
