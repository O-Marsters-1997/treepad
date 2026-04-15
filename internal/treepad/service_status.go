package treepad

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/slug"
	"treepad/internal/worktree"
)

type StatusInput struct {
	JSON      bool
	OutputDir string
}

type StatusRow struct {
	Branch       string           `json:"branch"`
	Path         string           `json:"path"`
	IsMain       bool             `json:"is_main"`
	Dirty        bool             `json:"dirty"`
	Ahead        int              `json:"ahead"`
	Behind       int              `json:"behind"`
	HasUpstream  bool             `json:"has_upstream"`
	LastCommit   worktree.CommitInfo `json:"last_commit"`
	ArtifactPath string           `json:"artifact_path,omitempty"`
	LastTouched  time.Time        `json:"last_touched"`
}

func (s *Service) Status(ctx context.Context, in StatusInput) error {
	worktrees, err := s.listWorktrees(ctx)
	if err != nil {
		return err
	}

	mainWT, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return err
	}

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))

	outputDir, err := s.resolveOutputDir(in.OutputDir, repoSlug)
	if err != nil {
		return err
	}

	cfg, err := config.Load(mainWT.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	spec := artifactSpec(cfg.Artifact)

	rows := make([]StatusRow, 0, len(worktrees))
	for _, wt := range worktrees {
		row := StatusRow{
			Branch: wt.Branch,
			Path:   wt.Path,
			IsMain: wt.IsMain,
		}

		row.Dirty, err = worktree.Dirty(ctx, s.runner, wt.Path)
		if err != nil {
			return err
		}

		row.Ahead, row.Behind, row.HasUpstream, err = worktree.AheadBehind(ctx, s.runner, wt.Path)
		if err != nil {
			return err
		}

		row.LastCommit, err = worktree.LastCommit(ctx, s.runner, wt.Path)
		if err != nil {
			return err
		}

		data := s.templateData(repoSlug, wt.Branch, wt.Path, outputDir)
		artifactPath, ok, err := artifact.Path(spec, outputDir, data)
		if err != nil {
			return fmt.Errorf("resolve artifact path: %w", err)
		}
		if ok {
			row.ArtifactPath = artifactPath
			if info, statErr := os.Stat(artifactPath); statErr == nil {
				row.LastTouched = info.ModTime()
			}
		}

		rows = append(rows, row)
	}

	if in.JSON {
		return json.NewEncoder(s.out).Encode(rows)
	}
	return s.writeStatusTable(rows)
}

func (s *Service) writeStatusTable(rows []StatusRow) error {
	w := tabwriter.NewWriter(s.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "BRANCH\tSTATUS\tAHEAD/BEHIND\tLAST COMMIT\tTOUCHED\tPATH")

	for _, r := range rows {
		branch := r.Branch
		if r.IsMain {
			branch += " *"
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

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			branch, status, aheadBehind, lastCommit, touched, collapsePath(r.Path))
	}

	return w.Flush()
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
