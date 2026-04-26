package treepad

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"treepad/internal/treepad/cd"
	"treepad/internal/treepad/deps"
)

var (
	uiCursorStyle   = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
	uiHeaderStyle   = lipgloss.NewStyle().Bold(true)
	uiActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	uiSummaryStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	uiToastOKStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	uiToastErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// ErrNotTTY is returned by UI when stdout is not an interactive terminal.
var ErrNotTTY = errors.New("tp ui requires an interactive terminal")

type (
	uiTickMsg         struct{}
	uiToastExpiredMsg struct{}
	uiRefreshMsg      struct {
		rows   []StatusRow
		health map[string]healthFlags
		err    error
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
	uiShellDoneMsg struct {
		branch string
		err    error
	}
	uiYankClearMsg struct{}
)

type uiMode int

const (
	uiModeNormal             uiMode = iota
	uiModeConfirmRemove             // r pressed — awaiting y/cancel
	uiModeConfirmForceRemove        // R pressed — awaiting y/cancel
	uiModeConfirmPrune              // p pressed — awaiting y/cancel
	uiModeConfirmShell              // e pressed — awaiting y/cancel
	uiModeHelp                      // ? pressed — any key dismisses
	uiModeFilter                    // / pressed — typing filter query
)

// uiToast holds a transient message shown below the table.
type uiToast struct {
	msg   string
	isErr bool // error toasts stick until any key is pressed
}

type uiModel struct {
	ctx              context.Context
	d                deps.Deps
	in               StatusInput
	rows             []StatusRow
	health           map[string]healthFlags // keyed by branch; nil until first refresh
	activePath       string                 // filepath.Clean(cwd) at UI launch; "" if unavailable
	cursor           int
	width            int
	height           int
	loading          bool
	err              error
	selectedPath     string
	actionInFlight   bool   // sync in progress — pauses auto-refresh
	syncBranch       string // branch being synced; empty = fleet sync
	toast            *uiToast
	spinner          spinner.Model
	mode             uiMode
	confirmBranch    string // branch name shown in confirm modals
	confirmShellPath string // worktree path stashed for the shell confirm
	yankPath         string // emits OSC-52 in View() then cleared on next cycle
	filterStr        string // query typed while in uiModeFilter; retained after commit
	filterActive     bool   // true once Enter commits a non-empty filter

	// Injectable behaviour. Nil means the feature is disabled; see UI() for
	// production defaults and NewHeadlessUI for the headless/e2e overrides.
	tickCmd       func() tea.Cmd // auto-refresh tick; nil → no tick
	toastTimerCmd func() tea.Cmd // toast-expiry timer; nil → toasts persist
	headerClock   func() string  // header timestamp; nil → time.Now()
}

// UI opens a full-screen fleet view. It returns ErrNotTTY when d.Out is not
// an interactive terminal.
func UI(ctx context.Context, d deps.Deps, in StatusInput) error {
	if d.IsTerminal == nil || !d.IsTerminal(d.Out) {
		return ErrNotTTY
	}
	curDir, _ := os.Getwd()
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	m := uiModel{
		ctx: ctx, d: d, in: in, loading: true, spinner: sp,
		activePath: filepath.Clean(curDir),
		tickCmd:    func() tea.Cmd { return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return uiTickMsg{} }) },
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
func uiEmitCD(d deps.Deps, path string) {
	cd.EmitCD(d, path)
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
func NewHeadlessUI(ctx context.Context, d deps.Deps, in StatusInput) *HeadlessUI {
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
