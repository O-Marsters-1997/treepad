package treepad

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
	"github.com/O-Marsters-1997/treepad/internal/ui"
)

func TestSkillInstall(t *testing.T) {
	t.Run("installs into the canonical .agents/skills by default", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		var logBuf bytes.Buffer
		d := deps.Deps{Log: ui.New(&logBuf)}

		if err := SkillInstall(context.Background(), d, SkillInstallInput{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := filepath.Join(home, ".agents", "skills", "treepad", "SKILL.md")
		if _, err := os.Stat(got); err != nil {
			t.Errorf("expected %s to exist: %v", got, err)
		}
		if !strings.Contains(logBuf.String(), "installed skill treepad") {
			t.Errorf("expected OK log line, got:\n%s", logBuf.String())
		}
	})

	t.Run("no harness detected logs a hint instead of silently doing nothing", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		var logBuf bytes.Buffer
		d := deps.Deps{Log: ui.New(&logBuf)}

		if err := SkillInstall(context.Background(), d, SkillInstallInput{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(logBuf.String(), "no known agent harness detected") {
			t.Errorf("expected a no-harness hint, got:\n%s", logBuf.String())
		}
	})

	t.Run("links a compat symlink for a detected claude-code install", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
			t.Fatalf("mkdir .claude: %v", err)
		}
		var logBuf bytes.Buffer
		d := deps.Deps{Log: ui.New(&logBuf)}

		if err := SkillInstall(context.Background(), d, SkillInstallInput{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		link := filepath.Join(home, ".claude", "skills", "treepad")
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("lstat %s: %v", link, err)
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			t.Fatalf("%s is not a symlink", link)
		}
		target, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("readlink: %v", err)
		}
		if want := filepath.Join("..", "..", ".agents", "skills", "treepad"); target != want {
			t.Errorf("link target = %q, want %q", target, want)
		}
		if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
			t.Errorf("expected symlink to resolve to a readable SKILL.md: %v", err)
		}
		if !strings.Contains(logBuf.String(), "linked skill treepad for claude-code") {
			t.Errorf("expected link log line, got:\n%s", logBuf.String())
		}
	})

	t.Run("refuses to overwrite an existing skill without --force", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		d := deps.Deps{Log: ui.New(&bytes.Buffer{})}

		if err := SkillInstall(context.Background(), d, SkillInstallInput{}); err != nil {
			t.Fatalf("first install: %v", err)
		}
		err := SkillInstall(context.Background(), d, SkillInstallInput{})
		if err == nil || !strings.Contains(err.Error(), "already exists; pass --force") {
			t.Fatalf("got %v, want already-exists error", err)
		}
	})

	t.Run("force replaces a stale claude-code entry with a symlink", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
			t.Fatalf("mkdir .claude: %v", err)
		}
		d := deps.Deps{Log: ui.New(&bytes.Buffer{})}

		if err := SkillInstall(context.Background(), d, SkillInstallInput{}); err != nil {
			t.Fatalf("first install: %v", err)
		}

		link := filepath.Join(home, ".claude", "skills", "treepad")
		if err := os.RemoveAll(link); err != nil {
			t.Fatalf("remove link: %v", err)
		}
		if err := os.MkdirAll(link, 0o755); err != nil {
			t.Fatalf("mkdir stale: %v", err)
		}
		if err := os.WriteFile(filepath.Join(link, "stale.md"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write stale file: %v", err)
		}

		if err := SkillInstall(context.Background(), d, SkillInstallInput{Force: true}); err != nil {
			t.Fatalf("force install: %v", err)
		}
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("lstat %s: %v", link, err)
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("expected stale directory to be replaced with a symlink")
		}
	})

	t.Run("unknown skill name errors listing available skills", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		d := deps.Deps{Log: ui.New(&bytes.Buffer{})}

		err := SkillInstall(context.Background(), d, SkillInstallInput{Names: []string{"nope"}})
		if err == nil || !strings.Contains(err.Error(), "unknown skill") {
			t.Fatalf("got %v, want unknown-skill error", err)
		}
	})

	t.Run("--local installs into the main worktree and links compat harnesses found there", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		if err := os.MkdirAll(filepath.Join(mainPath, ".claude"), 0o755); err != nil {
			t.Fatalf("mkdir .claude: %v", err)
		}
		d := deps.Deps{
			Runner: &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: treepadtest.MainWorktreePorcelain(mainPath)},
			}},
			Log: ui.New(&bytes.Buffer{}),
		}

		if err := SkillInstall(context.Background(), d, SkillInstallInput{Local: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		canonical := filepath.Join(mainPath, ".agents", "skills", "treepad", "SKILL.md")
		if _, err := os.Stat(canonical); err != nil {
			t.Errorf("expected %s to exist: %v", canonical, err)
		}
		link := filepath.Join(mainPath, ".claude", "skills", "treepad")
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("lstat %s: %v", link, err)
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("expected %s to be a symlink", link)
		}
	})
}

func TestSkillList(t *testing.T) {
	var out bytes.Buffer
	d := deps.Deps{Out: &out, Log: ui.New(&bytes.Buffer{})}

	if err := SkillList(context.Background(), d, SkillListInput{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "treepad") {
		t.Errorf("output = %q, want it to contain %q", got, "treepad")
	}
}
