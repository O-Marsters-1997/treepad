package script

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"treepad/internal/treepad"
	"treepad/internal/treepad/deps"
)

func TestParseKeyScript(t *testing.T) {
	tests := []struct {
		in   string
		want []tea.KeyMsg
	}{
		{"", nil},
		{"q", []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("q")}}},
		{"enter", []tea.KeyMsg{{Type: tea.KeyEnter}}},
		{"up", []tea.KeyMsg{{Type: tea.KeyUp}}},
		{"down", []tea.KeyMsg{{Type: tea.KeyDown}}},
		{"esc", []tea.KeyMsg{{Type: tea.KeyEsc}}},
		{"space", []tea.KeyMsg{{Type: tea.KeySpace}}},
		{"ctrl-c", []tea.KeyMsg{{Type: tea.KeyCtrlC}}},
		{"backspace", []tea.KeyMsg{{Type: tea.KeyBackspace}}},
		{"j,enter", []tea.KeyMsg{
			{Type: tea.KeyRunes, Runes: []rune("j")},
			{Type: tea.KeyEnter},
		}},
		{"S,q", []tea.KeyMsg{
			{Type: tea.KeyRunes, Runes: []rune("S")},
			{Type: tea.KeyRunes, Runes: []rune("q")},
		}},
		{"unknown_multi", nil},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := parseKeyScript(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i].Type != tt.want[i].Type {
					t.Errorf("[%d] Type = %v, want %v", i, got[i].Type, tt.want[i].Type)
				}
				if string(got[i].Runes) != string(tt.want[i].Runes) {
					t.Errorf("[%d] Runes = %q, want %q", i, got[i].Runes, tt.want[i].Runes)
				}
			}
		})
	}
}

func TestDrainInto(t *testing.T) {
	newH := func() *treepad.HeadlessUI {
		return treepad.NewHeadlessUI(context.Background(), deps.Deps{}, treepad.StatusInput{})
	}

	t.Run("nil cmd is a no-op", func(t *testing.T) {
		quit := drainInto(newH(), nil)
		if quit {
			t.Error("nil cmd should not signal quit")
		}
	})

	t.Run("QuitMsg returns true", func(t *testing.T) {
		quit := drainInto(newH(), tea.Quit)
		if !quit {
			t.Error("QuitMsg should return quit=true")
		}
	})

	t.Run("BatchMsg children are processed", func(t *testing.T) {
		quit := drainInto(newH(), tea.Batch(tea.Quit))
		if !quit {
			t.Error("QuitMsg inside BatchMsg should signal quit")
		}
	})
}
