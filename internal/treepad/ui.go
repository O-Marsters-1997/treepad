package treepad

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"treepad/internal/artifact"
	"treepad/internal/config"
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
	uiOpenDoneMsg struct {
		path string
		err  error
	}
	uiDiffDoneMsg struct {
		branch string
		err    error
	}
	uiYankClearMsg struct{}
)

// uiMode tracks the current interaction mode.
type uiMode int

const (
	uiModeNormal        uiMode = iota
	uiModeConfirmRemove        // r pressed — awaiting y/cancel
	uiModeConfirmPrune         // p pressed — awaiting y/cancel
	uiModeHelp                 // ? pressed — any key dismisses
	uiModeFilter               // / pressed — typing filter query
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
	yankPath       string // emits OSC-52 in View() then cleared on next cycle
	filterStr      string // query typed while in uiModeFilter; retained after commit
	filterActive   bool   // true once Enter commits a non-empty filter

	// Injectable behaviour. Nil means the feature is disabled; see UI() for
	// production defaults and NewHeadlessUI for the headless/e2e overrides.
	tickCmd       func() tea.Cmd // auto-refresh tick; nil → no tick
	toastTimerCmd func() tea.Cmd // toast-expiry timer; nil → toasts persist
	headerClock   func() string  // header timestamp; nil → time.Now()
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

// visibleRows applies the active filter and returns the rows to display.
// When no filter is active it returns m.rows unchanged.
func (m uiModel) visibleRows() []StatusRow {
	if !m.filterActive && m.mode != uiModeFilter {
		return m.rows
	}
	return filterRows(m.rows, m.filterStr)
}

// visibleCursorRow returns the row under the cursor within the visible set.
// Returns the zero value and false when the visible set is empty.
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
	case uiModeConfirmPrune:
		m.mode = uiModeNormal
		m.actionInFlight = true
		return m, tea.Batch(m.doPrune(), m.spinner.Tick)
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

func (m uiModel) doOpen(row StatusRow) tea.Cmd {
	return func() tea.Msg {
		rc, err := loadRepoContext(m.ctx, m.d, m.in.OutputDir)
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
		spec := artifact.OpenSpec{Command: cfg.Open.Command}
		data := artifact.OpenData{
			ArtifactPath: openPath,
			Worktree:     artifact.ToWorktree(row.Branch, row.Path, m.in.OutputDir),
		}
		err = m.d.Opener.Open(m.ctx, spec, data)
		return uiOpenDoneMsg{path: openPath, err: err}
	}
}

func (m uiModel) doDiff(row StatusRow) tea.Cmd {
	var mainPath string
	for _, r := range m.rows {
		if r.IsMain {
			mainPath = r.Path
			break
		}
	}
	base := "origin/main"
	if mainPath != "" {
		if cfg, err := config.Load(mainPath); err == nil {
			base = cfg.Diff.Base
		}
	}
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

func (m uiModel) View() string {
	var sb strings.Builder

	// OSC-52: write clipboard sequence before anything visual; cleared next cycle.
	if m.yankPath != "" {
		encoded := base64.StdEncoding.EncodeToString([]byte(m.yankPath))
		fmt.Fprintf(&sb, "\x1b]52;c;%s\x07", encoded)
	}

	ts := time.Now().Format("15:04:05")
	if m.headerClock != nil {
		ts = m.headerClock()
	}
	header := "tp ui  ·  " + ts + "  ·  q quit  ·  ? help  ·  ↓j / ↑k  ·  s sync  ·  S sync fleet"
	if m.actionInFlight && m.syncBranch == "" {
		header += "  " + m.spinner.View()
	}
	sb.WriteString(header + "\n\n")

	if m.loading && len(m.rows) == 0 {
		sb.WriteString("  loading...\n")
		return sb.String()
	}

	vr := m.visibleRows()
	if len(vr) == 0 && m.filterStr != "" {
		fmt.Fprintf(&sb, "  no matches for %q\n", m.filterStr)
	} else {
		lines := uiBuildTableLines(vr)
		for i, line := range lines {
			if i == 0 {
				sb.WriteString("  ")
				sb.WriteString(uiHeaderStyle.Render(line))
			} else {
				rowIdx := i - 1
				isSyncing := m.actionInFlight && m.syncBranch != "" &&
					rowIdx < len(vr) && vr[rowIdx].Branch == m.syncBranch

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
	}

	if m.err != nil {
		fmt.Fprintf(&sb, "\nerror: %v\n", m.err)
	}

	if m.mode == uiModeFilter {
		fmt.Fprintf(&sb, "\n/ %s█\n", m.filterStr)
	} else if m.filterActive {
		fmt.Fprintf(&sb, "\n/ %s  (esc to clear)\n", m.filterStr)
	}

	if m.mode == uiModeHelp {
		sb.WriteString("\n")
		sb.WriteString(uiRenderHelp())
		sb.WriteString("\n")
	} else if m.mode != uiModeNormal && m.mode != uiModeFilter {
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

	if hasPrunable(m.rows) {
		sb.WriteString("\nnote: stale worktree metadata detected — run 'tp prune' or 'git worktree prune' to clean up\n")
	}

	return sb.String()
}

func uiBuildTableLines(rows []StatusRow) []string {
	return formatStatusRows(rows)
}

var uiModalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(0, 2)

func uiRenderHelp() string {
	body := "Key Bindings\n\n" +
		"↑ / k       Move cursor up\n" +
		"↓ / j       Move cursor down\n" +
		"Enter       cd into selected worktree\n" +
		"s           Sync selected worktree (config + artifact)\n" +
		"S           Sync all worktrees\n" +
		"o           Open artifact for selected worktree\n" +
		"d           Diff selected worktree against base (default origin/main)\n" +
		"y           Yank path of selected worktree (OSC-52)\n" +
		"r           Remove selected worktree (with confirmation)\n" +
		"p           Prune merged worktrees (with confirmation)\n" +
		"/           Filter / search worktrees\n" +
		"?           Show this help\n" +
		"q / Ctrl-C  Quit\n\n" +
		"Press any key to dismiss"
	return uiModalStyle.Render(body)
}

func uiRenderModal(m uiModel) string {
	var title, detail string
	switch m.mode {
	case uiModeNormal, uiModeHelp, uiModeFilter:
		return ""
	case uiModeConfirmRemove:
		title = fmt.Sprintf("Remove worktree: %s", m.confirmBranch)
		detail = "This will permanently delete the worktree and its branch."
	case uiModeConfirmPrune:
		title = "Prune merged worktrees"
		detail = "All worktrees whose branches are merged into main will be removed."
	}
	body := fmt.Sprintf("%s\n%s\n\n[y] confirm  ·  [any other key] cancel", title, detail)
	return uiModalStyle.Render(body)
}

// UI opens a full-screen fleet view. It returns ErrNotTTY when d.Out is not
// an interactive terminal.
func UI(ctx context.Context, d Deps, in StatusInput) error {
	if !d.IsTerminal(d.Out) {
		return ErrNotTTY
	}
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	m := uiModel{
		ctx: ctx, d: d, in: in, loading: true, spinner: sp,
		tickCmd: func() tea.Cmd { return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return uiTickMsg{} }) },
		toastTimerCmd: func() tea.Cmd {
			return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return uiToastExpiredMsg{} })
		},
		headerClock: func() string { return time.Now().Format("15:04:05") },
	}
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

// HeadlessUI drives uiModel synchronously without a TTY or tea.Program.
// Use NewHeadlessUI to construct; Init / Update / View follow the tea.Model
// protocol but mutate state in place rather than returning new models.
type HeadlessUI struct {
	model uiModel
}

// NewHeadlessUI constructs a HeadlessUI ready for headless key replay.
// The header clock is fixed at "STATIC" for deterministic output; auto-refresh
// and toast timers are disabled (nil factories) so no time-based cmds are emitted.
func NewHeadlessUI(ctx context.Context, d Deps, in StatusInput) *HeadlessUI {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	return &HeadlessUI{model: uiModel{
		ctx:         ctx,
		d:           d,
		in:          in,
		loading:     true,
		spinner:     sp,
		headerClock: func() string { return "STATIC" },
	}}
}

// Init follows tea.Model; call once before the first Update.
func (h *HeadlessUI) Init() tea.Cmd {
	return h.model.Init()
}

// Update feeds msg through the model, stores the updated state, and returns
// the next command.
func (h *HeadlessUI) Update(msg tea.Msg) tea.Cmd {
	updated, cmd := h.model.Update(msg)
	h.model = updated.(uiModel)
	return cmd
}

// View renders the current frame.
func (h *HeadlessUI) View() string {
	return h.model.View()
}

// SelectedPath returns the worktree path chosen via Enter, or "".
func (h *HeadlessUI) SelectedPath() string {
	return h.model.selectedPath
}

// EmitCD writes the cd sentinel and human-readable line to d.Out.
// No-op when SelectedPath is empty.
func (h *HeadlessUI) EmitCD() {
	if h.model.selectedPath != "" {
		uiEmitCD(h.model.d, h.model.selectedPath)
	}
}

// IsDrainDiscardable reports whether msg should be skipped by a headless drain
// loop. Discards uiYankClearMsg (so yankPath survives until View renders the
// OSC-52 sequence) and spinner.TickMsg (timer-driven, would chain infinitely).
// Exposed for the e2e/script harness; not intended for production use.
func IsDrainDiscardable(msg tea.Msg) bool {
	switch msg.(type) {
	case uiYankClearMsg, spinner.TickMsg:
		return true
	}
	return false
}
