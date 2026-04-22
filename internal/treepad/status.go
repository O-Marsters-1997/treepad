package treepad

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	v, err := d.NewRepoView(ctx, in.OutputDir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(v.Main().Path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	snaps, err := v.Snapshots(ctx, ProbeAll)
	if err != nil {
		return nil, err
	}
	return collectStatusRows(snaps, v.Slug(), v.OutputDir(), artifactSpec(cfg.Artifact))
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

func collectStatusRows(snaps []Snapshot, repoSlug, outputDir string, spec artifact.Spec) ([]StatusRow, error) {
	rows := make([]StatusRow, 0, len(snaps))
	for _, s := range snaps {
		row := StatusRow{
			Branch:         s.Branch,
			Path:           s.Path,
			IsMain:         s.IsMain,
			Prunable:       s.Prunable,
			PrunableReason: s.PrunableReason,
			Dirty:          s.Dirty,
			Ahead:          s.Ahead,
			Behind:         s.Behind,
			HasUpstream:    s.HasUpstream,
			LastCommit:     s.LastCommit,
		}
		if s.Prunable {
			rows = append(rows, row)
			continue
		}
		data := templateData(repoSlug, s.Branch, s.Path, outputDir)
		artifactPath, ok, err := artifact.Path(spec, outputDir, data)
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
