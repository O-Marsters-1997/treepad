//go:build e2e

package script

import (
	"context"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"treepad/internal/treepad"
)

// Run drives the tp ui model headlessly by replaying a comma-separated key
// script through Update, draining produced commands synchronously, and
// writing a deterministic frame to d.Out.
func Run(ctx context.Context, d treepad.Deps, in treepad.StatusInput, keys string) error {
	h := treepad.NewHeadlessUI(ctx, d, in)
	if quit := drainInto(h, h.Init()); !quit {
		for _, ev := range parseKeyScript(keys) {
			if quit := drainInto(h, h.Update(ev)); quit {
				break
			}
		}
	}
	_, _ = io.WriteString(d.Out, h.View())
	h.EmitCD()
	return nil
}

// drainInto executes cmd synchronously and feeds produced messages back through
// h.Update, enqueuing returned commands FIFO.
// tea.BatchMsg children are expanded; tea.QuitMsg signals the caller to stop;
// IsDrainDiscardable messages are silently dropped.
func drainInto(h *treepad.HeadlessUI, cmd tea.Cmd) (quit bool) {
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if msg == nil {
			continue
		}
		switch v := msg.(type) {
		case tea.BatchMsg:
			queue = append(queue, v...)
		case tea.QuitMsg:
			quit = true
		default:
			_ = v
			if treepad.IsDrainDiscardable(msg) {
				continue
			}
			if next := h.Update(msg); next != nil {
				queue = append(queue, next)
			}
		}
	}
	return quit
}

// parseKeyScript splits a comma-separated key script into KeyMsg events.
// Single-character tokens become rune keys; multi-character tokens are mapped
// to named keys: enter, up, down, esc, space, ctrl-c.
func parseKeyScript(s string) []tea.KeyMsg {
	if s == "" {
		return nil
	}
	var out []tea.KeyMsg
	for tok := range strings.SplitSeq(s, ",") {
		switch tok {
		case "enter":
			out = append(out, tea.KeyMsg{Type: tea.KeyEnter})
		case "up":
			out = append(out, tea.KeyMsg{Type: tea.KeyUp})
		case "down":
			out = append(out, tea.KeyMsg{Type: tea.KeyDown})
		case "esc":
			out = append(out, tea.KeyMsg{Type: tea.KeyEsc})
		case "space":
			out = append(out, tea.KeyMsg{Type: tea.KeySpace})
		case "ctrl-c":
			out = append(out, tea.KeyMsg{Type: tea.KeyCtrlC})
		default:
			if runes := []rune(tok); len(runes) == 1 {
				out = append(out, tea.KeyMsg{Type: tea.KeyRunes, Runes: runes})
			}
		}
	}
	return out
}
