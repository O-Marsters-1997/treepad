package treepad

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	uiCursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230"))
	uiHeaderStyle = lipgloss.NewStyle().Bold(true)
)

// ErrNotTTY is returned by UI when stdout is not an interactive terminal.
var ErrNotTTY = fmt.Errorf("tp ui requires an interactive terminal")

type (
	uiTickMsg    struct{}
	uiRefreshMsg struct {
		rows []StatusRow
		err  error
	}
)

type uiModel struct {
	ctx     context.Context
	d       Deps
	in      StatusInput
	rows    []StatusRow
	cursor  int
	width   int
	height  int
	loading bool
	err     error
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
	case uiTickMsg:
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
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
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

func (m uiModel) View() string {
	var sb strings.Builder

	sb.WriteString("tp ui  ·  ")
	sb.WriteString(time.Now().Format("15:04:05"))
	sb.WriteString("  ·  q quit  ·  ↓j / ↑k navigate\n\n")

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
			if rowIdx == m.cursor {
				sb.WriteString(uiCursorStyle.Render("▶ " + line))
			} else {
				sb.WriteString("  ")
				sb.WriteString(line)
			}
		}
		sb.WriteString("\n")
	}

	if m.err != nil {
		fmt.Fprintf(&sb, "\nerror: %v\n", m.err)
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
	m := uiModel{ctx: ctx, d: d, in: in, loading: true}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}
