package treepad

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
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

func Status(ctx context.Context, d Deps, in StatusInput) error {
	rc, err := loadRepoContext(ctx, d, in.OutputDir)
	if err != nil {
		return err
	}

	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	rows, err := collectStatusRows(ctx, d, rc, artifactSpec(cfg.Artifact))
	if err != nil {
		return err
	}

	if in.JSON {
		return json.NewEncoder(d.Out).Encode(rows)
	}
	return writeStatusTable(d, rows)
}

func collectStatusRows(ctx context.Context, d Deps, rc repoContext, spec artifact.Spec) ([]StatusRow, error) {
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

		data := templateData(rc.Slug, wt.Branch, wt.Path, rc.OutputDir)
		artifactPath, ok, err := artifact.Path(spec, rc.OutputDir, data)
		if err != nil {
			return nil, fmt.Errorf("resolve artifact path: %w", err)
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

func StatusWatch(ctx context.Context, d Deps, in StatusInput) error {
	if !d.IsTerminal(d.Out) {
		return fmt.Errorf("--watch requires a TTY")
	}

	rc, err := loadRepoContext(ctx, d, in.OutputDir)
	if err != nil {
		return fmt.Errorf("status watch: %w", err)
	}
	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return fmt.Errorf("status watch: load config: %w", err)
	}
	spec := artifactSpec(cfg.Artifact)

	watchCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	_, _ = fmt.Fprint(d.Out, "\x1b[?1049h\x1b[?25l")                    // enter alt-screen, hide cursor
	defer func() { _, _ = fmt.Fprint(d.Out, "\x1b[?25h\x1b[?1049l") }() // show cursor, exit alt-screen

	for {
		_, _ = fmt.Fprint(d.Out, "\x1b[2J\x1b[H")
		_, _ = fmt.Fprintf(d.Out, "tp status --watch · every 2s · %s · Ctrl-C to exit\n\n",
			time.Now().Format("2006-01-02 15:04:05"))

		rows, err := collectStatusRows(watchCtx, d, rc, spec)
		if err != nil {
			_, _ = fmt.Fprintf(d.Out, "error: %v\n", err)
		} else {
			_ = writeStatusTable(d, rows)
		}

		select {
		case <-watchCtx.Done():
			return nil
		case <-d.Sleep(2 * time.Second):
		}
	}
}

func writeStatusTable(d Deps, rows []StatusRow) error {
	w := tabwriter.NewWriter(d.Out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "BRANCH\tSTATUS\tAHEAD/BEHIND\tLAST COMMIT\tTOUCHED\tPATH")

	hasPrunable := false
	for _, r := range rows {
		branch := r.Branch
		if r.IsMain {
			branch += " *"
		}

		if r.Prunable {
			hasPrunable = true
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

	if err := w.Flush(); err != nil {
		return err
	}
	if hasPrunable {
		_, _ = fmt.Fprintln(d.Out, "\nnote: stale worktree metadata detected — run 'tp prune' or 'git worktree prune' to clean up")
	}
	return nil
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
