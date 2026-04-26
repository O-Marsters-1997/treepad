package treepad

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"treepad/internal/config"
	"treepad/internal/treepad/lifecycle"
	"treepad/internal/treepad/repo"
)

func (m uiModel) Init() tea.Cmd {
	return tea.Batch(m.doRefresh(), m.doTick())
}

func (m uiModel) doRefresh() tea.Cmd {
	return func() tea.Msg {
		rows, err := refreshStatus(m.ctx, m.d, m.in)
		if err != nil {
			return uiRefreshMsg{err: err}
		}
		health, _ := computeHealth(m.ctx, m.d, rows)
		return uiRefreshMsg{rows: rows, health: health}
	}
}

func (m uiModel) doTick() tea.Cmd {
	if m.tickCmd == nil {
		return nil
	}
	return m.tickCmd()
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		if !m.actionInFlight {
			return m, nil // action done — stop animation
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case uiTickMsg:
		if m.actionInFlight {
			return m, m.doTick() // skip refresh, reschedule tick
		}
		return m, tea.Batch(m.doRefresh(), m.doTick())

	case uiRefreshMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.rows = msg.rows
			if msg.health != nil {
				m.health = msg.health
			}
			if vr := m.visibleRows(); len(vr) > 0 && m.cursor >= len(vr) {
				m.cursor = len(vr) - 1
			} else if len(vr) == 0 {
				m.cursor = 0
			}
		}

	case uiSyncDoneMsg:
		m.actionInFlight = false
		m.syncBranch = ""
		if msg.err != nil {
			m.toast = &uiToast{msg: fmt.Sprintf("%v", msg.err), isErr: true}
			return m, nil
		}
		label := "fleet"
		if msg.branch != "" {
			label = msg.branch
		}
		m.toast = &uiToast{msg: fmt.Sprintf("✓ synced %s", label)}
		return m, tea.Batch(m.doRefresh(), m.doTick(), m.doToastTimer())

	case uiToastExpiredMsg:
		if m.toast != nil && !m.toast.isErr {
			m.toast = nil
		}

	case uiOpenDoneMsg:
		m.actionInFlight = false
		if msg.err != nil {
			m.toast = &uiToast{msg: fmt.Sprintf("%v", msg.err), isErr: true}
			return m, nil
		}
		m.toast = &uiToast{msg: fmt.Sprintf("✓ opened %s", msg.path)}
		return m, m.doToastTimer()

	case uiDiffDoneMsg:
		m.actionInFlight = false
		if msg.err != nil {
			m.toast = &uiToast{msg: fmt.Sprintf("%v", msg.err), isErr: true}
			return m, nil
		}
		m.toast = &uiToast{msg: fmt.Sprintf("✓ diffed %s", msg.branch)}
		return m, m.doToastTimer()

	case uiShellDoneMsg:
		m.actionInFlight = false
		var exitErr *exec.ExitError
		if msg.err != nil && !errors.As(msg.err, &exitErr) {
			m.toast = &uiToast{msg: fmt.Sprintf("%v", msg.err), isErr: true}
			return m, tea.Batch(m.doRefresh(), m.doTick())
		}
		m.toast = &uiToast{msg: fmt.Sprintf("✓ shell exited (%s)", msg.branch)}
		return m, tea.Batch(m.doRefresh(), m.doTick(), m.doToastTimer())

	case uiYankClearMsg:
		m.yankPath = ""

	case uiRemoveDoneMsg:
		m.actionInFlight = false
		if msg.err != nil {
			m.toast = &uiToast{msg: fmt.Sprintf("%v", msg.err), isErr: true}
			return m, nil
		}
		m.toast = &uiToast{msg: fmt.Sprintf("✓ removed %s", msg.branch)}
		return m, tea.Batch(m.doRefresh(), m.doTick(), m.doToastTimer())

	case uiPruneDoneMsg:
		m.actionInFlight = false
		if msg.err != nil {
			m.toast = &uiToast{msg: fmt.Sprintf("%v", msg.err), isErr: true}
			return m, nil
		}
		m.toast = &uiToast{msg: "✓ pruned merged worktrees"}
		return m, tea.Batch(m.doRefresh(), m.doTick(), m.doToastTimer())

	case tea.KeyMsg:
		// Filter mode intercepts all keystrokes for query editing.
		if m.mode == uiModeFilter {
			switch msg.Type {
			case tea.KeyEnter:
				m.mode = uiModeNormal
				m.filterActive = m.filterStr != ""
				m.cursor = 0
				return m, nil
			case tea.KeyEsc:
				m.mode = uiModeNormal
				m.filterStr = ""
				m.filterActive = false
				m.cursor = 0
				return m, nil
			case tea.KeyBackspace:
				if runes := []rune(m.filterStr); len(runes) > 0 {
					m.filterStr = string(runes[:len(runes)-1])
					m.cursor = 0
				}
				return m, nil
			case tea.KeyRunes:
				m.filterStr += string(msg.Runes)
				m.cursor = 0
				return m, nil
			case tea.KeySpace:
				m.filterStr += " "
				m.cursor = 0
				return m, nil
			case tea.KeyCtrlC:
				return m, tea.Quit
			default:
				// ignore unhandled keys in filter mode
			}
			return m, nil
		}

		// Dismiss sticky error toast on first key press.
		if m.toast != nil && m.toast.isErr {
			m.toast = nil
			return m, nil
		}

		// Help overlay: any key dismisses.
		if m.mode == uiModeHelp {
			m.mode = uiModeNormal
			return m, nil
		}

		// Confirm modal intercepts all keys except q/ctrl+c.
		if m.mode != uiModeNormal {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "y":
				return m.handleConfirm()
			default:
				m.mode = uiModeNormal
				m.confirmBranch = ""
			}
			return m, nil
		}

		vr := m.visibleRows()
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if row, ok := m.visibleCursorRow(); ok {
				m.selectedPath = row.Path
				return m, tea.Quit
			}
		case "s":
			if !m.actionInFlight {
				if row, ok := m.visibleCursorRow(); ok {
					m.actionInFlight = true
					m.syncBranch = row.Branch
					m.toast = nil
					return m, tea.Batch(m.doSync(row.Branch), m.spinner.Tick)
				}
			}
		case "S":
			if !m.actionInFlight {
				m.actionInFlight = true
				m.syncBranch = ""
				m.toast = nil
				return m, tea.Batch(m.doSyncFleet(), m.spinner.Tick)
			}
		case "r":
			if !m.actionInFlight {
				if row, ok := m.visibleCursorRow(); ok {
					m.mode = uiModeConfirmRemove
					m.confirmBranch = row.Branch
					return m, nil
				}
			}
		case "R":
			if !m.actionInFlight {
				if row, ok := m.visibleCursorRow(); ok {
					m.mode = uiModeConfirmForceRemove
					m.confirmBranch = row.Branch
					return m, nil
				}
			}
		case "p":
			if !m.actionInFlight {
				m.mode = uiModeConfirmPrune
				return m, nil
			}
		case "o":
			if !m.actionInFlight {
				if row, ok := m.visibleCursorRow(); ok {
					m.actionInFlight = true
					m.toast = nil
					return m, m.doOpen(row)
				}
			}
		case "d":
			if !m.actionInFlight {
				if row, ok := m.visibleCursorRow(); ok && !row.Prunable {
					m.actionInFlight = true
					m.toast = nil
					return m, m.doDiff(row)
				}
			}
		case "e":
			if !m.actionInFlight {
				if row, ok := m.visibleCursorRow(); ok {
					m.mode = uiModeConfirmShell
					m.confirmBranch = row.Branch
					m.confirmShellPath = row.Path
					return m, nil
				}
			}
		case "y":
			if row, ok := m.visibleCursorRow(); ok {
				m.yankPath = row.Path
				m.toast = &uiToast{msg: "📋 yanked " + row.Path}
				return m, tea.Batch(m.doToastTimer(), func() tea.Msg { return uiYankClearMsg{} })
			}
		case "?":
			m.mode = uiModeHelp
			return m, nil
		case "/":
			m.mode = uiModeFilter
			m.filterActive = false
			return m, nil
		case "esc":
			if m.filterActive {
				m.filterStr = ""
				m.filterActive = false
				m.cursor = 0
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(vr)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m uiModel) visibleRows() []StatusRow {
	if !m.filterActive && m.mode != uiModeFilter {
		return m.rows
	}
	return filterRows(m.rows, m.filterStr)
}

func (m uiModel) visibleCursorRow() (StatusRow, bool) {
	vr := m.visibleRows()
	if len(vr) == 0 || m.cursor >= len(vr) {
		return StatusRow{}, false
	}
	return vr[m.cursor], true
}

func (m uiModel) handleConfirm() (tea.Model, tea.Cmd) {
	switch m.mode {
	case uiModeNormal, uiModeHelp:
		return m, nil
	case uiModeConfirmRemove:
		branch := m.confirmBranch
		m.mode = uiModeNormal
		m.confirmBranch = ""
		m.actionInFlight = true
		return m, tea.Batch(m.doRemove(branch), m.spinner.Tick)
	case uiModeConfirmForceRemove:
		branch := m.confirmBranch
		m.mode = uiModeNormal
		m.confirmBranch = ""
		m.actionInFlight = true
		return m, tea.Batch(m.doForceRemove(branch), m.spinner.Tick)
	case uiModeConfirmPrune:
		m.mode = uiModeNormal
		m.actionInFlight = true
		return m, tea.Batch(m.doPrune(), m.spinner.Tick)
	case uiModeConfirmShell:
		path := m.confirmShellPath
		branch := m.confirmBranch
		m.mode = uiModeNormal
		m.confirmBranch = ""
		m.confirmShellPath = ""
		m.actionInFlight = true
		return m, m.doShell(StatusRow{Branch: branch, Path: path})
	case uiModeFilter:
		return m, nil
	}
	return m, nil
}

func (m uiModel) doRemove(branch string) tea.Cmd {
	return func() tea.Msg {
		err := lifecycle.Remove(m.ctx, m.d, lifecycle.RemoveInput{Branch: branch, OutputDir: m.in.OutputDir})
		return uiRemoveDoneMsg{branch: branch, err: err}
	}
}

func (m uiModel) doForceRemove(branch string) tea.Cmd {
	return func() tea.Msg {
		err := lifecycle.Remove(m.ctx, m.d, lifecycle.RemoveInput{Branch: branch, OutputDir: m.in.OutputDir, Force: true})
		return uiRemoveDoneMsg{branch: branch, err: err}
	}
}

func (m uiModel) doPrune() tea.Cmd {
	return func() tea.Msg {
		err := lifecycle.Prune(m.ctx, m.d, lifecycle.PruneInput{Yes: true, Base: "main", OutputDir: m.in.OutputDir})
		return uiPruneDoneMsg{err: err}
	}
}

func (m uiModel) doOpen(row StatusRow) tea.Cmd {
	return func() tea.Msg {
		rc, err := repo.Load(m.ctx, m.d.Runner, m.in.OutputDir)
		if err != nil {
			return uiOpenDoneMsg{err: err}
		}
		cfg, err := config.Load(rc.Main.Path)
		if err != nil {
			return uiOpenDoneMsg{err: err}
		}
		openPath := row.Path
		if row.ArtifactPath != "" {
			openPath = row.ArtifactPath
		}
		err = lifecycle.OpenWorktree(m.ctx, m.d, cfg.Open.Command, row.Branch, row.Path, row.ArtifactPath, m.in.OutputDir)
		return uiOpenDoneMsg{path: openPath, err: err}
	}
}

func (m uiModel) doShell(row StatusRow) tea.Cmd {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell)
	cmd.Dir = row.Path
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return uiShellDoneMsg{branch: row.Branch, err: err}
	})
}

func (m uiModel) doDiff(row StatusRow) tea.Cmd {
	var mainPath string
	for _, r := range m.rows {
		if r.IsMain {
			mainPath = r.Path
			break
		}
	}
	base := resolveDiffBaseFromMainPath(mainPath)
	cmd := exec.Command("git", "-C", row.Path, "diff", base+"...HEAD")
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return uiDiffDoneMsg{branch: row.Branch, err: err}
	})
}

func (m uiModel) doSync(branch string) tea.Cmd {
	return func() tea.Msg {
		err := Generate(m.ctx, m.d, GenerateInput{Branch: branch, OutputDir: m.in.OutputDir})
		return uiSyncDoneMsg{branch: branch, err: err}
	}
}

func (m uiModel) doSyncFleet() tea.Cmd {
	return func() tea.Msg {
		err := Generate(m.ctx, m.d, GenerateInput{OutputDir: m.in.OutputDir})
		return uiSyncDoneMsg{branch: "", err: err}
	}
}

func (m uiModel) doToastTimer() tea.Cmd {
	if m.toastTimerCmd == nil {
		return nil
	}
	return m.toastTimerCmd()
}
