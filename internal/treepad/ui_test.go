package treepad

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUI(t *testing.T) {
	t.Run("non-TTY returns ErrNotTTY", func(t *testing.T) {
		deps := testDeps(&fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		err := UI(context.Background(), deps, StatusInput{})
		if !errors.Is(err, ErrNotTTY) {
			t.Errorf("got error %v, want ErrNotTTY", err)
		}
	})
}

func TestUIModel(t *testing.T) {
	rows2 := []StatusRow{
		{Branch: "main", IsMain: true},
		{Branch: "feat"},
	}

	t.Run("init starts in loading state", func(t *testing.T) {
		m := uiModel{
			ctx:     context.Background(),
			d:       testDeps(&fakeRunner{}, &fakeSyncer{}, &fakeOpener{}),
			loading: true,
		}
		if !m.loading {
			t.Error("expected loading=true on init")
		}
		if m.Init() == nil {
			t.Error("Init should return a non-nil command")
		}
	})

	t.Run("cursor moves down on down arrow", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 0}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m2 := updated.(uiModel)
		if m2.cursor != 1 {
			t.Errorf("cursor = %d, want 1", m2.cursor)
		}
	})

	t.Run("cursor moves down on j", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 0}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m2 := updated.(uiModel)
		if m2.cursor != 1 {
			t.Errorf("cursor = %d, want 1", m2.cursor)
		}
	})

	t.Run("cursor moves up on up arrow", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 1}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m2 := updated.(uiModel)
		if m2.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m2.cursor)
		}
	})

	t.Run("cursor moves up on k", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 1}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		m2 := updated.(uiModel)
		if m2.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m2.cursor)
		}
	})

	t.Run("cursor does not go below zero", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 0}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m2 := updated.(uiModel)
		if m2.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m2.cursor)
		}
	})

	t.Run("cursor does not exceed last row", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 1}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m2 := updated.(uiModel)
		if m2.cursor != 1 {
			t.Errorf("cursor = %d, want 1", m2.cursor)
		}
	})

	t.Run("q quits", func(t *testing.T) {
		m := uiModel{}
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		if cmd == nil {
			t.Fatal("expected quit command")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("got msg %T, want QuitMsg", msg)
		}
	})

	t.Run("ctrl+c quits", func(t *testing.T) {
		m := uiModel{}
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if cmd == nil {
			t.Fatal("expected quit command")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("got msg %T, want QuitMsg", msg)
		}
	})

	t.Run("refresh message updates rows and clears loading", func(t *testing.T) {
		m := uiModel{loading: true}
		updated, _ := m.Update(uiRefreshMsg{rows: rows2})
		m2 := updated.(uiModel)
		if m2.loading {
			t.Error("loading should be false after refresh")
		}
		if len(m2.rows) != 2 {
			t.Errorf("rows = %d, want 2", len(m2.rows))
		}
		if m2.err != nil {
			t.Errorf("unexpected error: %v", m2.err)
		}
	})

	t.Run("refresh error is stored and rows preserved", func(t *testing.T) {
		existing := []StatusRow{{Branch: "main"}}
		m := uiModel{rows: existing}
		updated, _ := m.Update(uiRefreshMsg{err: errors.New("git failure")})
		m2 := updated.(uiModel)
		if m2.err == nil {
			t.Error("expected error to be stored")
		}
		if len(m2.rows) != 1 {
			t.Errorf("rows should be preserved on error, got %d", len(m2.rows))
		}
	})

	t.Run("cursor clamped when rows shrink after refresh", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 1}
		updated, _ := m.Update(uiRefreshMsg{rows: []StatusRow{{Branch: "main"}}})
		m2 := updated.(uiModel)
		if m2.cursor != 0 {
			t.Errorf("cursor = %d, want 0 after shrink", m2.cursor)
		}
	})

	t.Run("view contains expected columns when rows present", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 0}
		view := m.View()
		for _, want := range []string{"BRANCH", "STATUS", "main", "feat"} {
			if !strings.Contains(view, want) {
				t.Errorf("view missing %q", want)
			}
		}
	})

	t.Run("view shows loading when no rows yet", func(t *testing.T) {
		m := uiModel{loading: true}
		view := m.View()
		if !strings.Contains(view, "loading") {
			t.Errorf("view missing 'loading': %s", view)
		}
	})

	t.Run("enter sets selectedPath and quits", func(t *testing.T) {
		m := uiModel{
			rows:   []StatusRow{{Branch: "main", Path: "/repo/main"}},
			cursor: 0,
		}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := updated.(uiModel)
		if m2.selectedPath != "/repo/main" {
			t.Errorf("selectedPath = %q, want %q", m2.selectedPath, "/repo/main")
		}
		if cmd == nil {
			t.Fatal("expected quit command after enter")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("got msg %T, want QuitMsg", msg)
		}
	})

	t.Run("enter with no rows does not set selectedPath", func(t *testing.T) {
		m := uiModel{}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := updated.(uiModel)
		if m2.selectedPath != "" {
			t.Errorf("selectedPath = %q, want empty on no rows", m2.selectedPath)
		}
	})

	t.Run("q quit does not set selectedPath", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 0}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		m2 := updated.(uiModel)
		if m2.selectedPath != "" {
			t.Errorf("q should not set selectedPath, got %q", m2.selectedPath)
		}
	})
}

func TestUIEmitCD(t *testing.T) {
	t.Run("emits sentinel and human line", func(t *testing.T) {
		var buf strings.Builder
		deps := testDeps(&fakeRunner{}, &fakeSyncer{}, &fakeOpener{})
		deps.Out = &buf

		uiEmitCD(deps, "/some/path")

		out := buf.String()
		if !strings.Contains(out, "__TREEPAD_CD__\t/some/path") {
			t.Errorf("missing sentinel in %q", out)
		}
		if !strings.Contains(out, "→ cd: /some/path") {
			t.Errorf("missing human line in %q", out)
		}
		// Sentinel must come before human line
		sentinelIdx := strings.Index(out, "__TREEPAD_CD__")
		humanIdx := strings.Index(out, "→ cd:")
		if sentinelIdx > humanIdx {
			t.Error("sentinel must appear before human line")
		}
	})
}
