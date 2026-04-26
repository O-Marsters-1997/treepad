package treepad

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"
	"time"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/worktree"
)

type StatusInput struct {
	JSON      bool
	OutputDir string
}

type StatusRow struct {
	Branch         string              `json:"branch"`
	Path           string              `json:"path"`
	IsMain         bool                `json:"is_main"`
	Dirty          bool                `json:"dirty"`
	Ahead          int                 `json:"ahead"`
	Behind         int                 `json:"behind"`
	HasUpstream    bool                `json:"has_upstream"`
	LastCommit     worktree.CommitInfo `json:"last_commit"`
	ArtifactPath   string              `json:"artifact_path,omitempty"`
	LastTouched    time.Time           `json:"last_touched"`
	Prunable       bool                `json:"prunable,omitempty"`
	PrunableReason string              `json:"prunable_reason,omitempty"`
}

func refreshStatus(ctx context.Context, d Deps, in StatusInput) ([]StatusRow, error) {
	rc, err := loadRepoContext(ctx, d, in.OutputDir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return collectStatusRows(ctx, d, rc, artifactSpec(cfg.Artifact))
}

func Status(ctx context.Context, d Deps, in StatusInput) error {
	rows, err := refreshStatus(ctx, d, in)
	if err != nil {
		return err
	}
	if in.JSON {
		return json.NewEncoder(d.Out).Encode(rows)
	}
	return writeStatusTable(d, rows)
}

func collectStatusRows(ctx context.Context, d Deps, rc RepoContext, spec artifact.Spec) ([]StatusRow, error) {
	rows := make([]StatusRow, 0, len(rc.Worktrees))
	for _, wt := range rc.Worktrees {
		row := StatusRow{
			Branch:         wt.Branch,
			Path:           wt.Path,
			IsMain:         wt.IsMain,
			Prunable:       wt.Prunable,
			PrunableReason: wt.PrunableReason,
		}

		if wt.Prunable {
			rows = append(rows, row)
			continue
		}

		var err error
		row.Dirty, err = worktree.Dirty(ctx, d.Runner, wt.Path)
		if err != nil {
			return nil, err
		}

		row.Ahead, row.Behind, row.HasUpstream, err = worktree.AheadBehind(ctx, d.Runner, wt.Path)
		if err != nil {
			return nil, err
		}

		row.LastCommit, err = worktree.LastCommit(ctx, d.Runner, wt.Path)
		if err != nil {
			return nil, err
		}

		artifactPath, ok, err := resolveArtifactPath(spec, rc.Slug, wt.Branch, wt.Path, rc.OutputDir)
		if err != nil {
			return nil, err
		}
		if ok {
			row.ArtifactPath = artifactPath
			if info, statErr := os.Stat(artifactPath); statErr == nil {
				row.LastTouched = info.ModTime()
			}
		}

		rows = append(rows, row)
	}
	return rows, nil
}

func writeStatusTable(d Deps, rows []StatusRow) error {
	for _, line := range formatStatusRows(rows) {
		_, _ = fmt.Fprintln(d.Out, line)
	}
	if hasPrunable(rows) {
		_, _ = fmt.Fprintln(d.Out,
			"\nnote: stale worktree metadata detected — run 'tp prune' or 'git worktree prune' to clean up",
		)
	}
	return nil
}

func formatStatusRows(rows []StatusRow) []string {
	if len(rows) == 0 {
		return nil
	}
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

func hasPrunable(rows []StatusRow) bool {
	for _, r := range rows {
		if r.Prunable {
			return true
		}
	}
	return false
}

func since(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func collapsePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || !strings.HasPrefix(path, home) {
		return path
	}
	return "~" + path[len(home):]
}

// healthFlags carries per-worktree diagnostic signals beyond the base StatusRow
// fields. Used only by the TUI's richer status column.
type healthFlags struct {
	Merged  bool
	Drifted bool
}

const uiStaleThreshold = 14 * 24 * time.Hour

// computeHealth derives health flags for each non-main, non-prunable worktree.
// Runs only local git/file-IO checks; no network calls are made.
func computeHealth(ctx context.Context, d Deps, rows []StatusRow) (map[string]healthFlags, error) {
	var mainPath string
	for _, r := range rows {
		if r.IsMain {
			mainPath = r.Path
			break
		}
	}

	base := resolveDiffBaseFromMainPath(mainPath)
	var mainCfg config.Config
	if mainPath != "" {
		if cfg, err := config.Load(mainPath); err == nil {
			mainCfg = cfg
		}
	}

	mergedBranches, err := worktree.MergedBranches(ctx, d.Runner, base)
	if err != nil {
		return nil, err
	}
	mergedSet := make(map[string]bool, len(mergedBranches))
	for _, b := range mergedBranches {
		mergedSet[b] = true
	}

	health := make(map[string]healthFlags, len(rows))
	for _, r := range rows {
		if r.Prunable || r.IsMain {
			continue
		}
		flags := healthFlags{Merged: mergedSet[r.Branch]}
		if mainPath != "" {
			if wtCfg, cfgErr := config.Load(r.Path); cfgErr == nil {
				flags.Drifted = !reflect.DeepEqual(wtCfg, mainCfg)
			}
		}
		health[r.Branch] = flags
	}
	return health, nil
}

// deriveStatus returns a human-readable label and a category key for r,
// incorporating health flags. Priority: broken → detached → merged → dirty →
// diverged → ahead → behind → stale → local → clean.
func deriveStatus(r StatusRow, h healthFlags) (label, key string) {
	switch {
	case r.Prunable:
		return "broken", "broken"
	case r.Branch == "(detached)":
		return "detached", "detached"
	case h.Merged && !r.IsMain:
		label, key = "merged (safe rm)", "merged"
	case r.Dirty:
		label = "dirty"
		switch {
		case r.Ahead > 0 && r.Behind > 0:
			label += fmt.Sprintf(" · ↑%d ↓%d", r.Ahead, r.Behind)
		case r.Ahead > 0:
			label += fmt.Sprintf(" · ↑%d", r.Ahead)
		case r.Behind > 0:
			label += fmt.Sprintf(" · ↓%d", r.Behind)
		}
		key = "dirty"
	case r.HasUpstream && r.Ahead > 0 && r.Behind > 0:
		label, key = fmt.Sprintf("diverged · ↑%d ↓%d", r.Ahead, r.Behind), "diverged"
	case r.HasUpstream && r.Ahead > 0:
		label, key = fmt.Sprintf("ahead · ↑%d", r.Ahead), "ahead"
	case r.HasUpstream && r.Behind > 0:
		label, key = fmt.Sprintf("behind · ↓%d", r.Behind), "behind"
	case !r.LastCommit.Committed.IsZero() && time.Since(r.LastCommit.Committed) > uiStaleThreshold:
		label, key = "stale", "stale"
	case !r.HasUpstream:
		label, key = "local", "local"
	default:
		label, key = "clean", "clean"
	}
	if h.Drifted {
		label += " · drift"
	}
	return label, key
}

func formatUIRows(rows []StatusRow, health map[string]healthFlags) []string {
	if len(rows) == 0 {
		return nil
	}
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "BRANCH\tSTATUS\tLAST COMMIT\tTOUCHED\tPATH")

	for _, r := range rows {
		branch := r.Branch
		if r.IsMain {
			branch += " *"
		}

		label, _ := deriveStatus(r, health[r.Branch])

		if r.Prunable {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				branch, label, r.PrunableReason, "—", collapsePath(r.Path))
			continue
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

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			branch, label, lastCommit, touched, collapsePath(r.Path))
	}

	_ = w.Flush()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

func uiBuildSummary(rows []StatusRow, health map[string]healthFlags) string {
	if len(rows) == 0 {
		return ""
	}
	counts := make(map[string]int, 10)
	driftCount := 0
	for _, r := range rows {
		h := health[r.Branch]
		_, key := deriveStatus(r, h)
		counts[key]++
		if h.Drifted {
			driftCount++
		}
	}
	order := []string{"clean", "dirty", "ahead", "behind", "diverged", "merged", "stale", "local", "detached", "broken"}
	parts := make([]string, 0, 12)
	parts = append(parts, fmt.Sprintf("%d worktrees", len(rows)))
	for _, k := range order {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, n))
		}
	}
	if driftCount > 0 {
		parts = append(parts, fmt.Sprintf("drift %d", driftCount))
	}
	return strings.Join(parts, " · ")
}
