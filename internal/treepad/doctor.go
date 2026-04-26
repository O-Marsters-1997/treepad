package treepad

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"
	"time"

	"treepad/internal/artifact"
	"treepad/internal/config"
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
	rc, err := loadRepoContext(ctx, d, in.OutputDir)
	if err != nil {
		return err
	}

	mainCfg, err := config.Load(rc.Main.Path)
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

	for _, wt := range rc.Worktrees {
		if wt.Prunable {
			findings = append(findings, doctorCheckPrunable(wt)...)
			continue
		}
		if wt.Branch == "(detached)" {
			continue
		}

		ageFindings, err := doctorCheckAge(ctx, d, wt, staleThreshold)
		if err != nil {
			return err
		}
		findings = append(findings, ageFindings...)

		findings = append(findings, doctorCheckMerged(wt, mergedSet, in.Base)...)

		if !in.Offline {
			remoteFindings, err := doctorCheckRemoteGone(ctx, d, wt)
			if err != nil {
				return err
			}
			findings = append(findings, remoteFindings...)
		}

		artifactFindings, err := doctorCheckArtifact(spec, rc.Slug, wt, rc.OutputDir)
		if err != nil {
			return err
		}
		findings = append(findings, artifactFindings...)

		if !wt.IsMain {
			findings = append(findings, doctorCheckConfigDrift(wt, mainCfg)...)
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

func doctorCheckPrunable(wt worktree.Worktree) []DoctorFinding {
	return []DoctorFinding{{
		Branch: wt.Branch,
		Path:   wt.Path,
		Kind:   "prunable",
		Detail: wt.PrunableReason + " — run 'tp prune' or 'git worktree prune' to clean up",
	}}
}

func doctorCheckAge(
	ctx context.Context, d Deps, wt worktree.Worktree, threshold time.Duration,
) ([]DoctorFinding, error) {
	commit, err := worktree.LastCommit(ctx, d.Runner, wt.Path)
	if err != nil {
		return nil, err
	}
	dirty, err := worktree.Dirty(ctx, d.Runner, wt.Path)
	if err != nil {
		return nil, err
	}
	if commit.ShortSHA == "" {
		return nil, nil
	}
	age := time.Since(commit.Committed)
	if dirty && age > threshold {
		return []DoctorFinding{{
			Branch: wt.Branch,
			Path:   wt.Path,
			Kind:   "dirty-old",
			Detail: fmt.Sprintf("uncommitted changes, last commit %s ago", since(commit.Committed)),
		}}, nil
	}
	if age > threshold {
		return []DoctorFinding{{
			Branch: wt.Branch,
			Path:   wt.Path,
			Kind:   "stale",
			Detail: fmt.Sprintf("last commit %s ago", since(commit.Committed)),
		}}, nil
	}
	return nil, nil
}

func doctorCheckMerged(wt worktree.Worktree, mergedSet map[string]bool, base string) []DoctorFinding {
	if !wt.IsMain && mergedSet[wt.Branch] {
		return []DoctorFinding{{
			Branch: wt.Branch,
			Path:   wt.Path,
			Kind:   "merged-present",
			Detail: fmt.Sprintf("branch already merged into %s", base),
		}}
	}
	return nil
}

func doctorCheckRemoteGone(ctx context.Context, d Deps, wt worktree.Worktree) ([]DoctorFinding, error) {
	exists, hasUpstream, err := worktree.RemoteBranchExists(ctx, d.Runner, wt.Path, wt.Branch)
	if err != nil {
		return nil, err
	}
	if hasUpstream && !exists {
		return []DoctorFinding{{
			Branch: wt.Branch,
			Path:   wt.Path,
			Kind:   "remote-gone",
			Detail: "branch no longer exists on remote",
		}}, nil
	}
	return nil, nil
}

func doctorCheckArtifact(
	spec artifact.Spec, repoSlug string, wt worktree.Worktree, outputDir string,
) ([]DoctorFinding, error) {
	artifactPath, ok, err := resolveArtifactPath(spec, repoSlug, wt.Branch, wt.Path, outputDir)
	if err != nil {
		return nil, err
	}
	if ok {
		if _, statErr := os.Stat(artifactPath); os.IsNotExist(statErr) {
			return []DoctorFinding{{
				Branch: wt.Branch,
				Path:   wt.Path,
				Kind:   "artifact-missing",
				Detail: fmt.Sprintf("expected artifact at %s", collapsePath(artifactPath)),
			}}, nil
		}
	}
	return nil, nil
}

func doctorCheckConfigDrift(wt worktree.Worktree, mainCfg config.Config) []DoctorFinding {
	wtCfg, cfgErr := config.Load(wt.Path)
	if cfgErr != nil {
		return []DoctorFinding{{
			Branch: wt.Branch,
			Path:   wt.Path,
			Kind:   "config-drift",
			Detail: fmt.Sprintf("could not load .treepad.toml: %s", cfgErr),
		}}
	}
	if !reflect.DeepEqual(wtCfg, mainCfg) {
		return []DoctorFinding{{
			Branch: wt.Branch,
			Path:   wt.Path,
			Kind:   "config-drift",
			Detail: configDriftDetail(mainCfg, wtCfg),
		}}
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
