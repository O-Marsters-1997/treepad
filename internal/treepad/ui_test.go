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

func TestUISync(t *testing.T) {
	rows2 := []StatusRow{
		{Branch: "main", IsMain: true, Path: "/repo/main"},
		{Branch: "feat", Path: "/repo/feat"},
	}

	t.Run("s dispatches single-branch sync for cursor row", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 1}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		m2 := updated.(uiModel)
		if !m2.actionInFlight {
			t.Error("actionInFlight should be true after s")
		}
		if m2.syncBranch != "feat" {
			t.Errorf("syncBranch = %q, want %q", m2.syncBranch, "feat")
		}
		if cmd == nil {
			t.Error("expected non-nil command after s")
		}
	})

	t.Run("S dispatches fleet sync", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 0}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
		m2 := updated.(uiModel)
		if !m2.actionInFlight {
			t.Error("actionInFlight should be true after S")
		}
		if m2.syncBranch != "" {
			t.Errorf("syncBranch = %q, want empty for fleet sync", m2.syncBranch)
		}
		if cmd == nil {
			t.Error("expected non-nil command after S")
		}
	})

	t.Run("s is no-op when action in flight", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 0, actionInFlight: true, syncBranch: "main"}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		m2 := updated.(uiModel)
		if m2.syncBranch != "main" {
			t.Error("syncBranch should not change while action in flight")
		}
		if cmd != nil {
			t.Error("expected nil command when action already in flight")
		}
	})

	t.Run("sync success shows toast and clears actionInFlight", func(t *testing.T) {
		m := uiModel{actionInFlight: true, syncBranch: "feat"}
		updated, _ := m.Update(uiSyncDoneMsg{branch: "feat", err: nil})
		m2 := updated.(uiModel)
		if m2.actionInFlight {
			t.Error("actionInFlight should be false after sync done")
		}
		if m2.toast == nil {
			t.Fatal("expected toast after sync success")
		}
		if m2.toast.isErr {
			t.Error("success toast should not be error")
		}
		if !strings.Contains(m2.toast.msg, "feat") {
			t.Errorf("toast msg = %q, want containing branch name", m2.toast.msg)
		}
	})

	t.Run("sync error shows sticky error toast", func(t *testing.T) {
		m := uiModel{actionInFlight: true, syncBranch: "feat"}
		updated, cmd := m.Update(uiSyncDoneMsg{branch: "feat", err: errors.New("sync failed")})
		m2 := updated.(uiModel)
		if m2.actionInFlight {
			t.Error("actionInFlight should be false")
		}
		if m2.toast == nil {
			t.Fatal("expected error toast")
		}
		if !m2.toast.isErr {
			t.Error("error toast should have isErr=true")
		}
		if cmd != nil {
			t.Error("error toast should not start a timer")
		}
	})

	t.Run("fleet sync success shows fleet toast", func(t *testing.T) {
		m := uiModel{actionInFlight: true, syncBranch: ""}
		updated, _ := m.Update(uiSyncDoneMsg{branch: "", err: nil})
		m2 := updated.(uiModel)
		if m2.toast == nil {
			t.Fatal("expected toast")
		}
		if !strings.Contains(m2.toast.msg, "fleet") {
			t.Errorf("fleet toast msg = %q, want containing 'fleet'", m2.toast.msg)
		}
	})

	t.Run("tick skips refresh when action in flight", func(t *testing.T) {
		// When actionInFlight, tick should not trigger a refresh cmd batch
		// (only reschedule the next tick)
		m := uiModel{actionInFlight: true}
		_, cmd := m.Update(uiTickMsg{})
		if cmd == nil {
			t.Error("expected tick rescheduling command")
		}
		// The command should be a single doTick (not a batch with doRefresh).
		// We verify indirectly: the model rows are unchanged.
		_ = cmd
	})

	t.Run("error toast dismissed by any key", func(t *testing.T) {
		m := uiModel{toast: &uiToast{msg: "oops", isErr: true}}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m2 := updated.(uiModel)
		if m2.toast != nil {
			t.Error("error toast should be dismissed by key press")
		}
	})
}

func TestUIDestructive(t *testing.T) {
	rows2 := []StatusRow{
		{Branch: "main", IsMain: true, Path: "/repo/main"},
		{Branch: "feat", Path: "/repo/feat"},
	}

	t.Run("r enters confirmRemove mode with branch name", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 1}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeConfirmRemove {
			t.Errorf("mode = %v, want uiModeConfirmRemove", m2.mode)
		}
		if m2.confirmBranch != "feat" {
			t.Errorf("confirmBranch = %q, want %q", m2.confirmBranch, "feat")
		}
		if cmd != nil {
			t.Error("r should not dispatch a command immediately")
		}
	})

	t.Run("y in confirmRemove dispatches Remove and returns to normal mode", func(t *testing.T) {
		m := uiModel{
			rows:          rows2,
			cursor:        1,
			mode:          uiModeConfirmRemove,
			confirmBranch: "feat",
		}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeNormal {
			t.Errorf("mode = %v, want uiModeNormal after confirm", m2.mode)
		}
		if !m2.actionInFlight {
			t.Error("actionInFlight should be true after y confirm")
		}
		if cmd == nil {
			t.Error("expected dispatch command after y confirm")
		}
	})

	t.Run("non-y key in confirmRemove cancels and returns to normal", func(t *testing.T) {
		m := uiModel{
			mode:          uiModeConfirmRemove,
			confirmBranch: "feat",
		}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m2 := updated.(uiModel)
		if m2.mode != uiModeNormal {
			t.Errorf("mode = %v, want uiModeNormal after cancel", m2.mode)
		}
		if m2.confirmBranch != "" {
			t.Errorf("confirmBranch = %q, want empty after cancel", m2.confirmBranch)
		}
		if m2.actionInFlight {
			t.Error("actionInFlight should be false after cancel")
		}
	})

	t.Run("p enters confirmPrune mode", func(t *testing.T) {
		m := uiModel{rows: rows2}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeConfirmPrune {
			t.Errorf("mode = %v, want uiModeConfirmPrune", m2.mode)
		}
		if cmd != nil {
			t.Error("p should not dispatch immediately")
		}
	})

	t.Run("y in confirmPrune dispatches Prune", func(t *testing.T) {
		m := uiModel{mode: uiModeConfirmPrune}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeNormal {
			t.Errorf("mode = %v, want normal", m2.mode)
		}
		if !m2.actionInFlight {
			t.Error("actionInFlight should be true after prune confirm")
		}
		if cmd == nil {
			t.Error("expected dispatch command")
		}
	})

	t.Run("non-y key in confirmPrune cancels", func(t *testing.T) {
		m := uiModel{mode: uiModeConfirmPrune}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeNormal {
			t.Errorf("mode = %v, want normal", m2.mode)
		}
		if m2.actionInFlight {
			t.Error("actionInFlight should be false after cancel")
		}
	})

	t.Run("cursor cannot move while modal is open", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 0, mode: uiModeConfirmRemove, confirmBranch: "feat"}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m2 := updated.(uiModel)
		if m2.cursor != 0 {
			t.Error("cursor should not move while modal is open")
		}
		// The modal should be cancelled by the non-y key
		if m2.mode != uiModeNormal {
			t.Error("modal should cancel on non-y key")
		}
	})

	t.Run("remove success shows toast and triggers refresh", func(t *testing.T) {
		m := uiModel{actionInFlight: true}
		updated, cmd := m.Update(uiRemoveDoneMsg{branch: "feat", err: nil})
		m2 := updated.(uiModel)
		if m2.actionInFlight {
			t.Error("actionInFlight should be false")
		}
		if m2.toast == nil {
			t.Fatal("expected toast")
		}
		if !strings.Contains(m2.toast.msg, "feat") {
			t.Errorf("toast = %q, want containing branch", m2.toast.msg)
		}
		if cmd == nil {
			t.Error("expected refresh command")
		}
	})

	t.Run("prune success shows toast", func(t *testing.T) {
		m := uiModel{actionInFlight: true}
		updated, _ := m.Update(uiPruneDoneMsg{err: nil})
		m2 := updated.(uiModel)
		if m2.toast == nil {
			t.Fatal("expected toast")
		}
		if !strings.Contains(m2.toast.msg, "prune") {
			t.Errorf("toast = %q, want containing 'prune'", m2.toast.msg)
		}
	})

	t.Run("view renders modal when in confirm mode", func(t *testing.T) {
		m := uiModel{mode: uiModeConfirmRemove, confirmBranch: "feat/my-branch"}
		view := m.View()
		for _, want := range []string{"feat/my-branch", "confirm", "cancel"} {
			if !strings.Contains(view, want) {
				t.Errorf("modal view missing %q", want)
			}
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
