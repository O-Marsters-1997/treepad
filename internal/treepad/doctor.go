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
	v, err := d.NewRepoView(ctx, in.OutputDir)
	if err != nil {
		return err
	}

	mainCfg, err := config.Load(v.Main().Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	spec := artifactSpec(mainCfg.Artifact)

	merged, err := v.MergedInto(ctx, in.Base)
	if err != nil {
		return fmt.Errorf("merged branches: %w", err)
	}

	snaps, err := v.Snapshots(ctx, Probe{Dirty: true, LastCommit: true})
	if err != nil {
		return err
	}

	staleThreshold := time.Duration(in.StaleDays) * 24 * time.Hour
	var findings []DoctorFinding

	for _, s := range snaps {
		if s.Prunable {
			findings = append(findings, DoctorFinding{
				Branch: s.Branch,
				Path:   s.Path,
				Kind:   "prunable",
				Detail: s.PrunableReason + " — run 'tp prune' or 'git worktree prune' to clean up",
			})
			continue
		}

		if s.Branch == "(detached)" {
			continue
		}

		if s.LastCommit.ShortSHA != "" {
			age := time.Since(s.LastCommit.Committed)
			if s.Dirty && age > staleThreshold {
				findings = append(findings, DoctorFinding{
					Branch: s.Branch,
					Path:   s.Path,
					Kind:   "dirty-old",
					Detail: fmt.Sprintf("uncommitted changes, last commit %s ago", since(s.LastCommit.Committed)),
				})
			} else if age > staleThreshold {
				findings = append(findings, DoctorFinding{
					Branch: s.Branch,
					Path:   s.Path,
					Kind:   "stale",
					Detail: fmt.Sprintf("last commit %s ago", since(s.LastCommit.Committed)),
				})
			}
		}

		if !s.IsMain && merged[s.Branch] {
			findings = append(findings, DoctorFinding{
				Branch: s.Branch,
				Path:   s.Path,
				Kind:   "merged-present",
				Detail: fmt.Sprintf("branch already merged into %s", in.Base),
			})
		}

		if !in.Offline {
			exists, hasUpstream, err := worktree.RemoteBranchExists(ctx, d.Runner, s.Path, s.Branch)
			if err != nil {
				return err
			}
			if hasUpstream && !exists {
				findings = append(findings, DoctorFinding{
					Branch: s.Branch,
					Path:   s.Path,
					Kind:   "remote-gone",
					Detail: "branch no longer exists on remote",
				})
			}
		}

		data := templateData(v.Slug(), s.Branch, s.Path, v.OutputDir())
		artifactPath, ok, err := artifact.Path(spec, v.OutputDir(), data)
		if err != nil {
			return fmt.Errorf("resolve artifact path: %w", err)
		}
		if ok {
			if _, statErr := os.Stat(artifactPath); os.IsNotExist(statErr) {
				findings = append(findings, DoctorFinding{
					Branch: s.Branch,
					Path:   s.Path,
					Kind:   "artifact-missing",
					Detail: fmt.Sprintf("expected artifact at %s", collapsePath(artifactPath)),
				})
			}
		}

		if !s.IsMain {
			wtCfg, cfgErr := config.Load(s.Path)
			if cfgErr != nil {
				findings = append(findings, DoctorFinding{
					Branch: s.Branch,
					Path:   s.Path,
					Kind:   "config-drift",
					Detail: fmt.Sprintf("could not load .treepad.toml: %s", cfgErr),
				})
			} else if !reflect.DeepEqual(wtCfg, mainCfg) {
				findings = append(findings, DoctorFinding{
					Branch: s.Branch,
					Path:   s.Path,
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
