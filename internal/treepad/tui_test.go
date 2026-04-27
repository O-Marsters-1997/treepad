package treepad

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"treepad/internal/artifact"
	"treepad/internal/treepad/cd"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/treepadtest"
)

func TestUI(t *testing.T) {
	t.Run("non-TTY returns ErrNotTTY", func(t *testing.T) {
		deps := deps.Deps{Out: io.Discard}
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
			d:       deps.Deps{Out: io.Discard},
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
		// When actionInFlight, tick should reschedule without triggering a refresh.
		noopTick := func() tea.Cmd { return func() tea.Msg { return uiTickMsg{} } }
		m := uiModel{actionInFlight: true, tickCmd: noopTick}
		_, cmd := m.Update(uiTickMsg{})
		if cmd == nil {
			t.Error("expected tick rescheduling command")
		}
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

func TestUIPolish(t *testing.T) {
	rows2 := []StatusRow{
		{Branch: "main", IsMain: true, Path: "/repo/main", ArtifactPath: "/out/main.code-workspace"},
		{Branch: "feat", Path: "/repo/feat"},
	}

	t.Run("o dispatches open with artifact path", func(t *testing.T) {
		opener := artifact.ExecOpener{Runner: &treepadtest.Runner{}}
		deps := deps.Deps{Out: io.Discard, Opener: opener}
		m := uiModel{
			ctx:    context.Background(),
			d:      deps,
			rows:   rows2,
			cursor: 0,
		}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
		m2 := updated.(uiModel)
		if !m2.actionInFlight {
			t.Error("actionInFlight should be true while opening")
		}
		if cmd == nil {
			t.Error("expected open command")
		}
	})

	t.Run("o open success shows toast", func(t *testing.T) {
		m := uiModel{actionInFlight: true}
		updated, _ := m.Update(uiOpenDoneMsg{path: "/out/main.code-workspace", err: nil})
		m2 := updated.(uiModel)
		if m2.actionInFlight {
			t.Error("actionInFlight should be false after open")
		}
		if m2.toast == nil {
			t.Fatal("expected toast after open")
		}
		if !strings.Contains(m2.toast.msg, "opened") {
			t.Errorf("toast = %q, want containing 'opened'", m2.toast.msg)
		}
	})

	t.Run("y sets yankPath and shows toast", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 0}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		m2 := updated.(uiModel)
		if m2.yankPath != "/repo/main" {
			t.Errorf("yankPath = %q, want /repo/main", m2.yankPath)
		}
		if m2.toast == nil || !strings.Contains(m2.toast.msg, "/repo/main") {
			t.Errorf("toast = %v, want yank toast", m2.toast)
		}
		if cmd == nil {
			t.Error("expected yank command")
		}
	})

	t.Run("y view contains OSC-52 sequence", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 0, yankPath: "/repo/main"}
		view := m.View()
		if !strings.Contains(view, "\x1b]52;c;") {
			t.Error("view missing OSC-52 escape sequence")
		}
		// Verify base64 content
		encoded := base64.StdEncoding.EncodeToString([]byte("/repo/main"))
		if !strings.Contains(view, encoded) {
			t.Errorf("view missing base64 encoded path %q", encoded)
		}
	})

	t.Run("yankClearMsg clears yankPath", func(t *testing.T) {
		m := uiModel{yankPath: "/some/path"}
		updated, _ := m.Update(uiYankClearMsg{})
		m2 := updated.(uiModel)
		if m2.yankPath != "" {
			t.Errorf("yankPath = %q, want empty after clear", m2.yankPath)
		}
	})

	t.Run("? enters help mode", func(t *testing.T) {
		m := uiModel{}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeHelp {
			t.Errorf("mode = %v, want uiModeHelp", m2.mode)
		}
		if cmd != nil {
			t.Error("? should not dispatch a command")
		}
	})

	t.Run("any key dismisses help overlay", func(t *testing.T) {
		m := uiModel{mode: uiModeHelp}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeNormal {
			t.Errorf("mode = %v, want uiModeNormal after dismiss", m2.mode)
		}
	})

	t.Run("help view contains key bindings", func(t *testing.T) {
		m := uiModel{mode: uiModeHelp}
		view := m.View()
		for _, want := range []string{"Enter", "Sync", "Yank", "Remove", "Prune", "dismiss"} {
			if !strings.Contains(view, want) {
				t.Errorf("help view missing %q", want)
			}
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

	t.Run("R enters confirmForceRemove mode with branch name", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 1}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeConfirmForceRemove {
			t.Errorf("mode = %v, want uiModeConfirmForceRemove", m2.mode)
		}
		if m2.confirmBranch != "feat" {
			t.Errorf("confirmBranch = %q, want %q", m2.confirmBranch, "feat")
		}
		if cmd != nil {
			t.Error("R should not dispatch a command immediately")
		}
	})

	t.Run("y in confirmForceRemove dispatches ForceRemove and returns to normal mode", func(t *testing.T) {
		m := uiModel{
			rows:          rows2,
			cursor:        1,
			mode:          uiModeConfirmForceRemove,
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

	t.Run("non-y key in confirmForceRemove cancels and returns to normal", func(t *testing.T) {
		m := uiModel{
			mode:          uiModeConfirmForceRemove,
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

	t.Run("view renders force-remove modal with branch name and danger copy", func(t *testing.T) {
		m := uiModel{mode: uiModeConfirmForceRemove, confirmBranch: "feat/my-branch"}
		view := m.View()
		for _, want := range []string{"feat/my-branch", "Force-remove", "confirm", "cancel"} {
			if !strings.Contains(view, want) {
				t.Errorf("force-remove modal view missing %q", want)
			}
		}
	})
}

func TestUIFilter(t *testing.T) {
	rows3 := []StatusRow{
		{Branch: "main", IsMain: true, Path: "/repo/main"},
		{Branch: "feat/foo-bar", Path: "/repo/foo-bar"},
		{Branch: "fix/baz", Path: "/repo/baz"},
	}

	t.Run("slash enters filter mode", func(t *testing.T) {
		m := uiModel{rows: rows3}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeFilter {
			t.Errorf("mode = %v, want uiModeFilter", m2.mode)
		}
		if m2.filterStr != "" {
			t.Errorf("filterStr = %q, want empty on mode entry", m2.filterStr)
		}
		if cmd != nil {
			t.Error("/ should not dispatch a command")
		}
	})

	t.Run("typing appends to filterStr and narrows visible rows", func(t *testing.T) {
		m := uiModel{rows: rows3, mode: uiModeFilter}
		for _, r := range []rune{'f', 'e', 'a', 't'} {
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = updated.(uiModel)
		}
		if m.filterStr != "feat" {
			t.Errorf("filterStr = %q, want feat", m.filterStr)
		}
		vr := m.visibleRows()
		if len(vr) != 1 || vr[0].Branch != "feat/foo-bar" {
			t.Errorf("visibleRows = %v, want [feat/foo-bar]", vr)
		}
	})

	t.Run("backspace trims last rune", func(t *testing.T) {
		m := uiModel{rows: rows3, mode: uiModeFilter, filterStr: "feat"}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m2 := updated.(uiModel)
		if m2.filterStr != "fea" {
			t.Errorf("filterStr = %q, want fea after backspace", m2.filterStr)
		}
	})

	t.Run("backspace on empty filterStr is a no-op", func(t *testing.T) {
		m := uiModel{rows: rows3, mode: uiModeFilter, filterStr: ""}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m2 := updated.(uiModel)
		if m2.filterStr != "" {
			t.Errorf("filterStr = %q, want empty", m2.filterStr)
		}
	})

	t.Run("esc clears filter and exits mode", func(t *testing.T) {
		m := uiModel{rows: rows3, mode: uiModeFilter, filterStr: "feat"}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m2 := updated.(uiModel)
		if m2.mode != uiModeNormal {
			t.Errorf("mode = %v, want uiModeNormal", m2.mode)
		}
		if m2.filterStr != "" {
			t.Errorf("filterStr = %q, want empty after esc", m2.filterStr)
		}
		if m2.filterActive {
			t.Error("filterActive should be false after esc")
		}
	})

	t.Run("enter commits non-empty filter", func(t *testing.T) {
		m := uiModel{rows: rows3, mode: uiModeFilter, filterStr: "feat"}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := updated.(uiModel)
		if m2.mode != uiModeNormal {
			t.Errorf("mode = %v, want uiModeNormal", m2.mode)
		}
		if !m2.filterActive {
			t.Error("filterActive should be true after enter with non-empty filterStr")
		}
		if m2.filterStr != "feat" {
			t.Errorf("filterStr = %q, want feat preserved", m2.filterStr)
		}
	})

	t.Run("enter with empty filterStr does not activate filter", func(t *testing.T) {
		m := uiModel{rows: rows3, mode: uiModeFilter, filterStr: ""}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := updated.(uiModel)
		if m2.filterActive {
			t.Error("filterActive should remain false when entering empty query")
		}
	})

	t.Run("esc in normal mode clears committed filter", func(t *testing.T) {
		m := uiModel{rows: rows3, filterStr: "feat", filterActive: true}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m2 := updated.(uiModel)
		if m2.filterActive {
			t.Error("filterActive should be false after esc in normal mode")
		}
		if m2.filterStr != "" {
			t.Errorf("filterStr = %q, want empty", m2.filterStr)
		}
	})

	t.Run("action targets filtered row not original index", func(t *testing.T) {
		deps := deps.Deps{Out: io.Discard}
		m := uiModel{
			ctx:          context.Background(),
			d:            deps,
			rows:         rows3,
			filterStr:    "baz",
			filterActive: true,
			cursor:       0,
		}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		m2 := updated.(uiModel)
		if m2.syncBranch != "fix/baz" {
			t.Errorf("syncBranch = %q, want fix/baz (filtered row)", m2.syncBranch)
		}
	})

	t.Run("refresh preserves filter state", func(t *testing.T) {
		newRows := []StatusRow{
			{Branch: "main", Path: "/repo/main"},
			{Branch: "feat/foo-bar", Path: "/repo/foo-bar"},
		}
		m := uiModel{rows: rows3, filterStr: "feat", filterActive: true, cursor: 0}
		updated, _ := m.Update(uiRefreshMsg{rows: newRows})
		m2 := updated.(uiModel)
		if m2.filterStr != "feat" {
			t.Errorf("filterStr = %q, want feat preserved after refresh", m2.filterStr)
		}
		if !m2.filterActive {
			t.Error("filterActive should remain true after refresh")
		}
		vr := m2.visibleRows()
		if len(vr) != 1 || vr[0].Branch != "feat/foo-bar" {
			t.Errorf("visibleRows after refresh = %v, want [feat/foo-bar]", vr)
		}
	})

	t.Run("cursor clamps when filter narrows rows", func(t *testing.T) {
		m := uiModel{rows: rows3, mode: uiModeFilter, filterStr: "", cursor: 2}
		// type a query that narrows to 1 row
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
		m2 := updated.(uiModel)
		vr := m2.visibleRows()
		if len(vr) == 0 {
			t.Fatal("expected at least one visible row for query 'b'")
		}
		if m2.cursor >= len(vr) {
			t.Errorf("cursor = %d is out of bounds for %d visible rows", m2.cursor, len(vr))
		}
	})

	t.Run("empty match shows no-matches message in view", func(t *testing.T) {
		m := uiModel{rows: rows3, filterStr: "zzz", filterActive: true}
		view := m.View()
		if !strings.Contains(view, `no matches for "zzz"`) {
			t.Errorf("view missing no-matches message, got:\n%s", view)
		}
	})

	t.Run("footer shows typing indicator in filter mode", func(t *testing.T) {
		m := uiModel{rows: rows3, mode: uiModeFilter, filterStr: "fe"}
		view := m.View()
		if !strings.Contains(view, "/ fe") {
			t.Errorf("view missing filter prompt, got:\n%s", view)
		}
	})

	t.Run("footer shows committed filter with esc hint", func(t *testing.T) {
		m := uiModel{rows: rows3, filterStr: "feat", filterActive: true}
		view := m.View()
		if !strings.Contains(view, "(esc to clear)") {
			t.Errorf("view missing esc hint, got:\n%s", view)
		}
	})

	t.Run("help modal lists filter binding", func(t *testing.T) {
		m := uiModel{mode: uiModeHelp}
		view := m.View()
		if !strings.Contains(view, "/") || !strings.Contains(view, "Filter") {
			t.Errorf("help view missing filter binding, got:\n%s", view)
		}
	})

	t.Run("toast dismiss does not eat slash", func(t *testing.T) {
		m := uiModel{rows: rows3, toast: &uiToast{msg: "error msg", isErr: true}}
		// first key dismisses the sticky error toast
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m2 := updated.(uiModel)
		if m2.toast != nil {
			t.Error("first key should dismiss sticky error toast")
		}
		if m2.mode == uiModeFilter {
			t.Error("filter mode should not activate on toast dismiss key")
		}
		// subsequent / enters filter mode
		updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m3 := updated2.(uiModel)
		if m3.mode != uiModeFilter {
			t.Errorf("mode = %v, want uiModeFilter on second /", m3.mode)
		}
	})
}

func TestUIShell(t *testing.T) {
	rows2 := []StatusRow{
		{Branch: "main", IsMain: true, Path: "/repo/main"},
		{Branch: "feat", Path: "/repo/feat"},
	}

	t.Run("e enters confirmShell mode with branch and path", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 1}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeConfirmShell {
			t.Errorf("mode = %v, want uiModeConfirmShell", m2.mode)
		}
		if m2.confirmBranch != "feat" {
			t.Errorf("confirmBranch = %q, want feat", m2.confirmBranch)
		}
		if m2.confirmShellPath != "/repo/feat" {
			t.Errorf("confirmShellPath = %q, want /repo/feat", m2.confirmShellPath)
		}
		if cmd != nil {
			t.Error("e should not dispatch a command immediately")
		}
	})

	t.Run("e is no-op when action in flight", func(t *testing.T) {
		m := uiModel{rows: rows2, cursor: 1, actionInFlight: true}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeNormal {
			t.Errorf("mode = %v, want uiModeNormal while action in flight", m2.mode)
		}
		if cmd != nil {
			t.Error("expected nil command when action already in flight")
		}
	})

	t.Run("y in confirmShell dispatches shell and returns to normal mode", func(t *testing.T) {
		m := uiModel{
			rows:             rows2,
			cursor:           1,
			mode:             uiModeConfirmShell,
			confirmBranch:    "feat",
			confirmShellPath: "/repo/feat",
		}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		m2 := updated.(uiModel)
		if m2.mode != uiModeNormal {
			t.Errorf("mode = %v, want uiModeNormal after confirm", m2.mode)
		}
		if !m2.actionInFlight {
			t.Error("actionInFlight should be true after y confirm")
		}
		if m2.confirmBranch != "" {
			t.Errorf("confirmBranch = %q, want empty after confirm", m2.confirmBranch)
		}
		if m2.confirmShellPath != "" {
			t.Errorf("confirmShellPath = %q, want empty after confirm", m2.confirmShellPath)
		}
		if cmd == nil {
			t.Error("expected dispatch command after y confirm")
		}
	})

	t.Run("non-y key in confirmShell cancels and returns to normal", func(t *testing.T) {
		m := uiModel{
			mode:             uiModeConfirmShell,
			confirmBranch:    "feat",
			confirmShellPath: "/repo/feat",
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

	t.Run("shell done success shows toast and triggers refresh", func(t *testing.T) {
		m := uiModel{actionInFlight: true}
		updated, cmd := m.Update(uiShellDoneMsg{branch: "feat", err: nil})
		m2 := updated.(uiModel)
		if m2.actionInFlight {
			t.Error("actionInFlight should be false after shell done")
		}
		if m2.toast == nil {
			t.Fatal("expected toast after shell exits")
		}
		if m2.toast.isErr {
			t.Error("success toast should not be error")
		}
		if !strings.Contains(m2.toast.msg, "feat") {
			t.Errorf("toast = %q, want containing branch name", m2.toast.msg)
		}
		if cmd == nil {
			t.Error("expected refresh command after shell exits")
		}
	})

	t.Run("shell done with ExitError shows success toast and refreshes", func(t *testing.T) {
		m := uiModel{actionInFlight: true}
		updated, cmd := m.Update(uiShellDoneMsg{branch: "feat", err: &exec.ExitError{}})
		m2 := updated.(uiModel)
		if m2.toast == nil || m2.toast.isErr {
			t.Error("ExitError from shell should show success toast, not error toast")
		}
		if cmd == nil {
			t.Error("expected refresh command")
		}
	})

	t.Run("shell done with launch error shows error toast", func(t *testing.T) {
		m := uiModel{actionInFlight: true}
		updated, _ := m.Update(uiShellDoneMsg{branch: "feat", err: errors.New("exec: not found")})
		m2 := updated.(uiModel)
		if m2.toast == nil || !m2.toast.isErr {
			t.Error("launch failure should show error toast")
		}
	})

	t.Run("view renders shell modal with branch and path", func(t *testing.T) {
		m := uiModel{mode: uiModeConfirmShell, confirmBranch: "feat/my-branch", confirmShellPath: "/repo/feat"}
		view := m.View()
		for _, want := range []string{"feat/my-branch", "/repo/feat", "confirm", "cancel"} {
			if !strings.Contains(view, want) {
				t.Errorf("shell modal view missing %q", want)
			}
		}
	})

	t.Run("help view lists e binding", func(t *testing.T) {
		m := uiModel{mode: uiModeHelp}
		view := m.View()
		if !strings.Contains(view, "e") || !strings.Contains(view, "shell") {
			t.Errorf("help view missing shell binding, got:\n%s", view)
		}
	})
}

func TestUIEmitCD(t *testing.T) {
	t.Run("emits sentinel and human line", func(t *testing.T) {
		var buf strings.Builder
		deps := deps.Deps{Out: &buf}
		cd.EmitCD(deps, "/some/path")

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

func TestUINumberNavigation(t *testing.T) {
	rows4 := []StatusRow{
		{Branch: "main", IsMain: true, Path: "/repo/main"},
		{Branch: "feat-a", Path: "/repo/feat-a"},
		{Branch: "feat-b", Path: "/repo/feat-b"},
		{Branch: "fix", Path: "/repo/fix"},
	}

	t.Run("number key jumps cursor to that row", func(t *testing.T) {
		m := uiModel{rows: rows4, cursor: 0}
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
		m2 := updated.(uiModel)
		if m2.cursor != 2 {
			t.Errorf("cursor = %d, want 2", m2.cursor)
		}
		if cmd != nil {
			t.Error("in-range jump should not dispatch a command")
		}
	})

	t.Run("0 jumps to first row", func(t *testing.T) {
		m := uiModel{rows: rows4, cursor: 3}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
		m2 := updated.(uiModel)
		if m2.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m2.cursor)
		}
	})

	t.Run("out-of-range shows toast and does not move cursor", func(t *testing.T) {
		m := uiModel{rows: rows4, cursor: 1}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
		m2 := updated.(uiModel)
		if m2.cursor != 1 {
			t.Errorf("cursor = %d, want 1 (unchanged)", m2.cursor)
		}
		if m2.toast == nil {
			t.Fatal("expected out-of-range toast")
		}
		if !strings.Contains(m2.toast.msg, "out of range") {
			t.Errorf("toast = %q, want containing 'out of range'", m2.toast.msg)
		}
		if m2.toast.isErr {
			t.Error("out-of-range toast should not be sticky error")
		}
	})

	t.Run("out-of-range increments toastGen so stale ticks do not clear toast", func(t *testing.T) {
		m := uiModel{rows: rows4}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
		m1 := updated.(uiModel)
		gen1 := m1.toastGen

		// second press bumps gen again
		updated2, _ := m1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
		m2 := updated2.(uiModel)
		if m2.toastGen <= gen1 {
			t.Errorf("toastGen = %d, want > %d after second press", m2.toastGen, gen1)
		}

		// stale tick (gen1) must not clear the current toast
		updated3, _ := m2.Update(uiToastExpiredMsg{gen: gen1})
		m3 := updated3.(uiModel)
		if m3.toast == nil {
			t.Error("stale tick should not clear toast set by later press")
		}

		// current tick (m2.toastGen) must clear
		updated4, _ := m3.Update(uiToastExpiredMsg{gen: m2.toastGen})
		m4 := updated4.(uiModel)
		if m4.toast != nil {
			t.Error("current-gen tick should clear the toast")
		}
	})

	t.Run("number key in filter mode types character not navigates", func(t *testing.T) {
		m := uiModel{rows: rows4, mode: uiModeFilter, filterStr: ""}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
		m2 := updated.(uiModel)
		if m2.filterStr != "2" {
			t.Errorf("filterStr = %q, want '2'", m2.filterStr)
		}
		if m2.cursor != 0 {
			t.Errorf("cursor = %d, want 0 (unchanged in filter mode)", m2.cursor)
		}
	})

	t.Run("view shows row numbers as inline column prefix", func(t *testing.T) {
		m := uiModel{rows: rows4, cursor: 0}
		view := m.View()
		if !strings.Contains(view, "0 ") {
			t.Errorf("view missing '0 ' prefix: %s", view)
		}
		if !strings.Contains(view, "3 ") {
			t.Errorf("view missing '3 ' prefix: %s", view)
		}
	})

	t.Run("help text mentions number key navigation", func(t *testing.T) {
		m := uiModel{mode: uiModeHelp}
		view := m.View()
		if !strings.Contains(view, "0–9") {
			t.Errorf("help view missing '0–9' binding: %s", view)
		}
	})
}
