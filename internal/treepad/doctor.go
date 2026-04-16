package treepad

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/tabwriter"
	"time"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/slug"
	"treepad/internal/worktree"
)

type DoctorInput struct {
	JSON      bool
	StaleDays int
	Base      string
	Offline   bool
	Strict    bool
	OutputDir string
}

type DoctorFinding struct {
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

func Doctor(ctx context.Context, d Deps, in DoctorInput) error {
	worktrees, err := listWorktrees(ctx, d)
	if err != nil {
		return err
	}

	mainWT, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return err
	}

	repoSlug := slug.Slug(filepath.Base(mainWT.Path))

	outputDir, err := resolveOutputDir(in.OutputDir, repoSlug)
	if err != nil {
		return err
	}

	mainCfg, err := config.Load(mainWT.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	spec := artifactSpec(mainCfg.Artifact)

	mergedBranches, err := worktree.MergedBranches(ctx, d.Runner, in.Base)
	if err != nil {
		return fmt.Errorf("merged branches: %w", err)
	}
	mergedSet := make(map[string]bool, len(mergedBranches))
	for _, b := range mergedBranches {
		mergedSet[b] = true
	}

	staleThreshold := time.Duration(in.StaleDays) * 24 * time.Hour

	var findings []DoctorFinding

	for _, wt := range worktrees {
		if wt.Branch == "(detached)" {
			continue
		}

		commit, err := worktree.LastCommit(ctx, d.Runner, wt.Path)
		if err != nil {
			return err
		}

		dirty, err := worktree.Dirty(ctx, d.Runner, wt.Path)
		if err != nil {
			return err
		}

		if commit.ShortSHA != "" {
			age := time.Since(commit.Committed)
			if dirty && age > staleThreshold {
				findings = append(findings, DoctorFinding{
					Branch: wt.Branch,
					Path:   wt.Path,
					Kind:   "dirty-old",
					Detail: fmt.Sprintf("uncommitted changes, last commit %s ago", since(commit.Committed)),
				})
			} else if age > staleThreshold {
				findings = append(findings, DoctorFinding{
					Branch: wt.Branch,
					Path:   wt.Path,
					Kind:   "stale",
					Detail: fmt.Sprintf("last commit %s ago", since(commit.Committed)),
				})
			}
		}

		if !wt.IsMain && mergedSet[wt.Branch] {
			findings = append(findings, DoctorFinding{
				Branch: wt.Branch,
				Path:   wt.Path,
				Kind:   "merged-present",
				Detail: fmt.Sprintf("branch already merged into %s", in.Base),
			})
		}

		if !in.Offline {
			exists, hasUpstream, err := worktree.RemoteBranchExists(ctx, d.Runner, wt.Path, wt.Branch)
			if err != nil {
				return err
			}
			if hasUpstream && !exists {
				findings = append(findings, DoctorFinding{
					Branch: wt.Branch,
					Path:   wt.Path,
					Kind:   "remote-gone",
					Detail: "branch no longer exists on remote",
				})
			}
		}

		data := templateData(repoSlug, wt.Branch, wt.Path, outputDir)
		artifactPath, ok, err := artifact.Path(spec, outputDir, data)
		if err != nil {
			return fmt.Errorf("resolve artifact path: %w", err)
		}
		if ok {
			if _, statErr := os.Stat(artifactPath); os.IsNotExist(statErr) {
				findings = append(findings, DoctorFinding{
					Branch: wt.Branch,
					Path:   wt.Path,
					Kind:   "artifact-missing",
					Detail: fmt.Sprintf("expected artifact at %s", collapsePath(artifactPath)),
				})
			}
		}

		if !wt.IsMain {
			wtCfg, cfgErr := config.Load(wt.Path)
			if cfgErr != nil {
				findings = append(findings, DoctorFinding{
					Branch: wt.Branch,
					Path:   wt.Path,
					Kind:   "config-drift",
					Detail: fmt.Sprintf("could not load .treepad.toml: %s", cfgErr),
				})
			} else if !reflect.DeepEqual(wtCfg, mainCfg) {
				findings = append(findings, DoctorFinding{
					Branch: wt.Branch,
					Path:   wt.Path,
					Kind:   "config-drift",
					Detail: configDriftDetail(mainCfg, wtCfg),
				})
			}
		}
	}

	if in.JSON {
		return json.NewEncoder(d.Out).Encode(findings)
	}
	writeDoctorTable(d.Out, findings)

	if in.Strict && len(findings) > 0 {
		return fmt.Errorf("%d finding(s) reported", len(findings))
	}
	return nil
}

func writeDoctorTable(out io.Writer, findings []DoctorFinding) {
	if len(findings) == 0 {
		_, _ = fmt.Fprintln(out, "no issues found")
		return
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "KIND\tBRANCH\tDETAIL\tPATH")
	for _, f := range findings {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", f.Kind, f.Branch, f.Detail, collapsePath(f.Path))
	}
	_ = w.Flush()
}

func configDriftDetail(main, wt config.Config) string {
	var sections []string
	if !reflect.DeepEqual(main.Sync, wt.Sync) {
		sections = append(sections, "sync")
	}
	if !reflect.DeepEqual(main.Artifact, wt.Artifact) {
		sections = append(sections, "artifact")
	}
	if !reflect.DeepEqual(main.Open, wt.Open) {
		sections = append(sections, "open")
	}
	return "differs in: " + strings.Join(sections, ", ")
}
