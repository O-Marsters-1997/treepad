package treepad

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
	"github.com/O-Marsters-1997/treepad/internal/ui"
)

// verbatimBody deliberately carries a template action, a heading, leading
// whitespace and a trailing blank line: any composition, interpolation or
// trimming by PlaybookNew fails the byte-identity assertion.
const verbatimBody = "# Task dashboard\n\n  Use /impeccable for {{.Ref}}.\n\n"

func playbookDeps(t *testing.T, mainPath, body string) (deps.Deps, *bytes.Buffer) {
	t.Helper()
	var logBuf bytes.Buffer
	return deps.Deps{
		Runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: treepadtest.MainWorktreePorcelain(mainPath)},
		}},
		Log: ui.New(&logBuf),
		In:  strings.NewReader(body),
	}, &logBuf
}

func TestPlaybookNew(t *testing.T) {
	t.Run("writes the body verbatim to .claude/playbooks/<name>.md", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		d, logBuf := playbookDeps(t, mainPath, verbatimBody)

		if err := PlaybookNew(context.Background(), d, PlaybookNewInput{Name: "task-dashboard"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		path := filepath.Join(mainPath, ".claude", "playbooks", "task-dashboard.md")
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read playbook: %v", err)
		}
		if string(got) != verbatimBody {
			t.Errorf("content = %q, want byte-identical %q", got, verbatimBody)
		}
		if !strings.Contains(logBuf.String(), path) {
			t.Errorf("expected the written path in the log, got:\n%s", logBuf.String())
		}
	})

	t.Run("refuses to overwrite without --force", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		d, _ := playbookDeps(t, mainPath, verbatimBody)
		if err := PlaybookNew(context.Background(), d, PlaybookNewInput{Name: "dupe"}); err != nil {
			t.Fatalf("first write: %v", err)
		}

		d2, _ := playbookDeps(t, mainPath, verbatimBody)
		err := PlaybookNew(context.Background(), d2, PlaybookNewInput{Name: "dupe"})
		if err == nil || !strings.Contains(err.Error(), "already exists; pass --force to overwrite") {
			t.Fatalf("got %v, want already-exists error", err)
		}
	})

	t.Run("--force replaces the existing playbook", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		d, _ := playbookDeps(t, mainPath, "old\n")
		if err := PlaybookNew(context.Background(), d, PlaybookNewInput{Name: "dupe"}); err != nil {
			t.Fatalf("first write: %v", err)
		}

		d2, _ := playbookDeps(t, mainPath, verbatimBody)
		if err := PlaybookNew(context.Background(), d2, PlaybookNewInput{Name: "dupe", Force: true}); err != nil {
			t.Fatalf("force write: %v", err)
		}
		got, err := os.ReadFile(filepath.Join(mainPath, ".claude", "playbooks", "dupe.md"))
		if err != nil {
			t.Fatalf("read playbook: %v", err)
		}
		if string(got) != verbatimBody {
			t.Errorf("content = %q, want %q", got, verbatimBody)
		}
	})

	t.Run("a name containing a path separator errors", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		d, _ := playbookDeps(t, mainPath, verbatimBody)

		err := PlaybookNew(context.Background(), d, PlaybookNewInput{Name: "../escape"})
		if err == nil || !strings.Contains(err.Error(), "invalid playbook name") {
			t.Fatalf("got %v, want invalid-name error", err)
		}
	})

	t.Run("empty stdin errors instead of writing an empty file", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		d, _ := playbookDeps(t, mainPath, "")

		err := PlaybookNew(context.Background(), d, PlaybookNewInput{Name: "empty"})
		if err == nil || !strings.Contains(err.Error(), "empty") {
			t.Fatalf("got %v, want empty-body error", err)
		}
		if _, statErr := os.Stat(filepath.Join(mainPath, ".claude", "playbooks", "empty.md")); statErr == nil {
			t.Error("an empty playbook file should not be written")
		}
	})
}
