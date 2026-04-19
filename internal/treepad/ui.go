package treepad

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	uiCursorStyle   = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
	uiHeaderStyle   = lipgloss.NewStyle().Bold(true)
	uiToastOKStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	uiToastErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// ErrNotTTY is returned by UI when stdout is not an interactive terminal.
var ErrNotTTY = fmt.Errorf("tp ui requires an interactive terminal")

type (
	uiTickMsg         struct{}
	uiToastExpiredMsg struct{}
	uiRefreshMsg      struct {
		rows []StatusRow
		err  error
	}
	uiSyncDoneMsg struct {
		branch string // empty = fleet sync
		err    error
	}
	uiRemoveDoneMsg struct {
		branch string
		err    error
	}
	uiPruneDoneMsg struct {
		err error
	}
)

// uiMode tracks whether the confirm modal is active.
type uiMode int

const (
	uiModeNormal        uiMode = iota
	uiModeConfirmRemove        // r pressed — awaiting y/cancel
	uiModeConfirmPrune         // p pressed — awaiting y/cancel
)

// uiToast holds a transient message shown below the table.
type uiToast struct {
	msg   string
	isErr bool // error toasts stick until any key is pressed
}

type uiModel struct {
	ctx            context.Context
	d              Deps
	in             StatusInput
	rows           []StatusRow
	cursor         int
	width          int
	height         int
	loading        bool
	err            error
	selectedPath   string
	actionInFlight bool   // sync in progress — pauses auto-refresh
	syncBranch     string // branch being synced; empty = fleet sync
	toast          *uiToast
	spinner        spinner.Model
	mode           uiMode
	confirmBranch  string // branch name shown in the remove confirm modal
}

func (m uiModel) Init() tea.Cmd {
	return tea.Batch(m.doRefresh(), m.doTick())
}

func (m uiModel) doRefresh() tea.Cmd {
	return func() tea.Msg {
		rows, err := refreshStatus(m.ctx, m.d, m.in)
		return uiRefreshMsg{rows: rows, err: err}
	}
}

func (m uiModel) doTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return uiTickMsg{}
	})
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
			if len(m.rows) > 0 && m.cursor >= len(m.rows) {
				m.cursor = len(m.rows) - 1
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
		// Dismiss sticky error toast on first key press.
		if m.toast != nil && m.toast.isErr {
			m.toast = nil
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

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if len(m.rows) > 0 {
				m.selectedPath = m.rows[m.cursor].Path
				return m, tea.Quit
			}
		case "s":
			if !m.actionInFlight && len(m.rows) > 0 {
				branch := m.rows[m.cursor].Branch
				m.actionInFlight = true
				m.syncBranch = branch
				m.toast = nil
				return m, tea.Batch(m.doSync(branch), m.spinner.Tick)
			}
		case "S":
			if !m.actionInFlight {
				m.actionInFlight = true
				m.syncBranch = ""
				m.toast = nil
				return m, tea.Batch(m.doSyncFleet(), m.spinner.Tick)
			}
		case "r":
			if !m.actionInFlight && len(m.rows) > 0 {
				m.mode = uiModeConfirmRemove
				m.confirmBranch = m.rows[m.cursor].Branch
				return m, nil
			}
		case "p":
			if !m.actionInFlight {
				m.mode = uiModeConfirmPrune
				return m, nil
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m uiModel) handleConfirm() (tea.Model, tea.Cmd) {
	switch m.mode {
	case uiModeConfirmRemove:
		branch := m.confirmBranch
		m.mode = uiModeNormal
		m.confirmBranch = ""
		m.actionInFlight = true
		return m, tea.Batch(m.doRemove(branch), m.spinner.Tick)
	case uiModeConfirmPrune:
		m.mode = uiModeNormal
		m.actionInFlight = true
		return m, tea.Batch(m.doPrune(), m.spinner.Tick)
	case uiModeNormal:
	}
	return m, nil
}

func (m uiModel) doRemove(branch string) tea.Cmd {
	return func() tea.Msg {
		err := Remove(m.ctx, m.d, RemoveInput{Branch: branch, OutputDir: m.in.OutputDir})
		return uiRemoveDoneMsg{branch: branch, err: err}
	}
}

func (m uiModel) doPrune() tea.Cmd {
	return func() tea.Msg {
		err := Prune(m.ctx, m.d, PruneInput{Yes: true, Base: "main", OutputDir: m.in.OutputDir})
		return uiPruneDoneMsg{err: err}
	}
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
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return uiToastExpiredMsg{}
	})
}

func (m uiModel) View() string {
	var sb strings.Builder

	header := "tp ui  ·  " + time.Now().Format("15:04:05") + "  ·  q quit  ·  ↓j / ↑k  ·  s sync  ·  S sync fleet"
	if m.actionInFlight && m.syncBranch == "" {
		header += "  " + m.spinner.View()
	}
	sb.WriteString(header + "\n\n")

	if m.loading && len(m.rows) == 0 {
		sb.WriteString("  loading...\n")
		return sb.String()
	}

	lines := uiBuildTableLines(m.rows)
	for i, line := range lines {
		if i == 0 {
			sb.WriteString("  ")
			sb.WriteString(uiHeaderStyle.Render(line))
		} else {
			rowIdx := i - 1
			isSyncing := m.actionInFlight && m.syncBranch != "" &&
				rowIdx < len(m.rows) && m.rows[rowIdx].Branch == m.syncBranch

			var prefix string
			switch {
			case isSyncing:
				prefix = m.spinner.View() + " "
			case rowIdx == m.cursor:
				prefix = "▶ "
			default:
				prefix = "  "
			}

			if rowIdx == m.cursor {
				sb.WriteString(uiCursorStyle.Render(prefix + line))
			} else {
				sb.WriteString(prefix + line)
			}
		}
		sb.WriteString("\n")
	}

	if m.err != nil {
		fmt.Fprintf(&sb, "\nerror: %v\n", m.err)
	}

	if m.mode != uiModeNormal {
		sb.WriteString("\n")
		sb.WriteString(uiRenderModal(m))
		sb.WriteString("\n")
	}

	if m.toast != nil {
		if m.toast.isErr {
			fmt.Fprintf(&sb, "\n%s  (press any key to dismiss)\n", uiToastErrStyle.Render("✗ "+m.toast.msg))
		} else {
			fmt.Fprintf(&sb, "\n%s\n", uiToastOKStyle.Render(m.toast.msg))
		}
	}

	if uiHasPrunable(m.rows) {
		sb.WriteString("\nnote: stale worktree metadata detected — run 'tp prune' or 'git worktree prune' to clean up\n")
	}

	return sb.String()
}

func uiBuildTableLines(rows []StatusRow) []string {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "BRANCH\tSTATUS\tAHEAD/BEHIND\tLAST COMMIT\tTOUCHED\tPATH")

	for _, r := range rows {
		branch := r.Branch
		if r.IsMain {
			branch += " *"
		}
		if r.Prunable {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				branch, "prunable", "—", r.PrunableReason, "—", collapsePath(r.Path))
			continue
		}

		status := "clean"
		if r.Dirty {
			status = "dirty"
		}

		aheadBehind := "—"
		if r.HasUpstream {
			aheadBehind = fmt.Sprintf("↑%d ↓%d", r.Ahead, r.Behind)
		}

		lastCommit := "—"
		if r.LastCommit.ShortSHA != "" {
			subject := r.LastCommit.Subject
			if len(subject) > 35 {
				subject = subject[:35] + "…"
			}
			lastCommit = fmt.Sprintf("%s %s · %s", r.LastCommit.ShortSHA, subject, since(r.LastCommit.Committed))
		}

		touched := "—"
		if !r.LastTouched.IsZero() {
			touched = since(r.LastTouched)
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			branch, status, aheadBehind, lastCommit, touched, collapsePath(r.Path))
	}

	_ = w.Flush()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

var uiModalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(0, 2)

func uiRenderModal(m uiModel) string {
	var title, detail string
	switch m.mode {
	case uiModeConfirmRemove:
		title = fmt.Sprintf("Remove worktree: %s", m.confirmBranch)
		detail = "This will permanently delete the worktree and its branch."
	case uiModeConfirmPrune:
		title = "Prune merged worktrees"
		detail = "All worktrees whose branches are merged into main will be removed."
	case uiModeNormal:
	}
	body := fmt.Sprintf("%s\n%s\n\n[y] confirm  ·  [any other key] cancel", title, detail)
	return uiModalStyle.Render(body)
}

func uiHasPrunable(rows []StatusRow) bool {
	for _, r := range rows {
		if r.Prunable {
			return true
		}
	}
	return false
}

// UI opens a full-screen fleet view. It returns ErrNotTTY when d.Out is not
// an interactive terminal.
func UI(ctx context.Context, d Deps, in StatusInput) error {
	if !d.IsTerminal(d.Out) {
		return ErrNotTTY
	}
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	m := uiModel{ctx: ctx, d: d, in: in, loading: true, spinner: sp}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	final, err := p.Run()
	if err != nil {
		return err
	}
	// After alt-screen tears down, emit cd sentinel if Enter was pressed.
	if fm, ok := final.(uiModel); ok && fm.selectedPath != "" {
		uiEmitCD(d, fm.selectedPath)
	}
	return nil
}

// uiEmitCD writes the cd sentinel (consumed by the shell wrapper) and a
// human-readable fallback line so users without the wrapper see where they'd land.
func uiEmitCD(d Deps, path string) {
	emitCD(d, path)
	_, _ = fmt.Fprintf(d.Out, "→ cd: %s\n", path)
}
