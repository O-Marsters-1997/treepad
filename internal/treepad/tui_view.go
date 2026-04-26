package treepad

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var uiModalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(0, 2)

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
		lines := formatUIRows(vr, m.health)
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

				isActive := m.activePath != "" && rowIdx < len(vr) &&
					cwdInside(m.activePath, vr[rowIdx].Path)
				switch {
				case rowIdx == m.cursor:
					sb.WriteString(uiCursorStyle.Render(prefix + line))
				case isActive:
					sb.WriteString(uiActiveStyle.Render(prefix + line))
				default:
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

	switch m.mode {
	case uiModeHelp:
		sb.WriteString("\n")
		sb.WriteString(uiRenderHelp())
		sb.WriteString("\n")
	case uiModeConfirmRemove, uiModeConfirmForceRemove, uiModeConfirmPrune, uiModeConfirmShell:
		sb.WriteString("\n")
		sb.WriteString(uiRenderModal(m))
		sb.WriteString("\n")
	case uiModeNormal, uiModeFilter:
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

	if summary := uiBuildSummary(vr, m.health); summary != "" {
		sb.WriteString("\n")
		sb.WriteString(uiSummaryStyle.Render(summary))
		sb.WriteString("\n")
	}

	return sb.String()
}

func uiRenderHelp() string {
	body := "Key Bindings\n\n" +
		"↑ / k       Move cursor up\n" +
		"↓ / j       Move cursor down\n" +
		"Enter       cd into selected worktree\n" +
		"s           Sync selected worktree (config + artifact)\n" +
		"S           Sync all worktrees\n" +
		"o           Open artifact for selected worktree\n" +
		"d           Diff selected worktree against base (default origin/main)\n" +
		"e           Open shell in selected worktree (with confirmation)\n" +
		"y           Yank path of selected worktree (OSC-52)\n" +
		"r           Remove selected worktree (with confirmation)\n" +
		"R           Force-remove selected worktree (discards unmerged work, with confirmation)\n" +
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
	case uiModeConfirmForceRemove:
		title = fmt.Sprintf("Force-remove worktree: %s", m.confirmBranch)
		detail = "This will force-delete the worktree and its branch, discarding uncommitted changes and unmerged commits."
	case uiModeConfirmPrune:
		title = "Prune merged worktrees"
		detail = "All worktrees whose branches are merged into main will be removed."
	case uiModeConfirmShell:
		title = fmt.Sprintf("Open shell in %s", m.confirmBranch)
		detail = m.confirmShellPath
	}
	body := fmt.Sprintf("%s\n%s\n\n[y] confirm  ·  [any other key] cancel", title, detail)
	return uiModalStyle.Render(body)
}
