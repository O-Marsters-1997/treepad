package treepad

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/config"
	"github.com/O-Marsters-1997/treepad/internal/launcher"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/repo"
)

// ghTickInterval is tp ui's slower cadence for the gh-backed Reconcile call.
// Local work (worktree list, dirty, ahead/behind, Activity mtime) stays on
// the faster uiTickInterval; only gh gets the slow one.
const ghTickInterval = 60 * time.Second

// applyRunStates sets RunState on every Batch-managed row from its Activity
// file's mtime — a local stat call, safe on tp ui's fast tick. Non-Batch rows
// are left with RunState empty: there is no launch concept for a worktree no
// Manifest resolved.
func applyRunStates(rows []StatusRow, commonDir string, now time.Time) []StatusRow {
	for i := range rows {
		if rows[i].Batch == "" {
			continue
		}
		state, err := launcher.State(commonDir, rows[i].Branch, now)
		if err != nil {
			continue
		}
		rows[i].RunState = string(state)
	}
	return rows
}

// applyPRState overlays the last Reconcile Report's PR fields onto rows
// sharing a branch with a Report entry. A nil report (no gh tick has
// completed yet) or a row outside any Batch is left with PRState empty and
// PRStale false — "not applicable", distinct from "stale".
func applyPRState(rows []StatusRow, report *Report) []StatusRow {
	if report == nil {
		return rows
	}
	byBranch := make(map[string]ReportEntry, len(report.Members))
	for _, e := range report.Members {
		byBranch[e.Branch] = e
	}
	for i := range rows {
		if e, ok := byBranch[rows[i].Branch]; ok {
			rows[i].PRNumber = e.PRNumber
			rows[i].PRState = e.PRState
			rows[i].PRStale = e.PRStale
		}
	}
	return rows
}

// groupRows orders rows for display: the main worktree first, then every
// Batch's Chains — Batches ordered by name, Chains by number, and each
// Chain's members by descending Position so it reads bottom (0, closest to
// main) to top — then every worktree no Manifest resolved, in its original
// order. tp ui remains the whole fleet's view: an ungrouped worktree still
// renders.
func groupRows(rows []StatusRow) []StatusRow {
	var main, ungrouped []StatusRow
	members := make(map[string]map[int][]StatusRow)
	var batchOrder []string
	chainOrder := make(map[string][]int)
	seenBatch := make(map[string]bool)
	seenChain := make(map[string]map[int]bool)

	for _, r := range rows {
		switch {
		case r.IsMain:
			main = append(main, r)
		case r.Batch == "":
			ungrouped = append(ungrouped, r)
		default:
			if !seenBatch[r.Batch] {
				seenBatch[r.Batch] = true
				batchOrder = append(batchOrder, r.Batch)
				members[r.Batch] = make(map[int][]StatusRow)
				seenChain[r.Batch] = make(map[int]bool)
			}
			if !seenChain[r.Batch][r.Chain] {
				seenChain[r.Batch][r.Chain] = true
				chainOrder[r.Batch] = append(chainOrder[r.Batch], r.Chain)
			}
			members[r.Batch][r.Chain] = append(members[r.Batch][r.Chain], r)
		}
	}
	sort.Strings(batchOrder)

	out := make([]StatusRow, 0, len(rows))
	out = append(out, main...)
	for _, b := range batchOrder {
		chains := chainOrder[b]
		sort.Ints(chains)
		for _, c := range chains {
			chainMembers := members[b][c]
			sort.SliceStable(chainMembers, func(i, j int) bool {
				return chainMembers[i].Position > chainMembers[j].Position
			})
			out = append(out, chainMembers...)
		}
	}
	return append(out, ungrouped...)
}

// launchContext loads the repo/config state a launch key needs, on demand
// rather than caching it on uiModel: launch keys are rare, so re-loading
// costs nothing and can never go stale.
func (m uiModel) launchContext() (config.Config, repo.Context, string, error) {
	rc, err := repo.Load(m.ctx, m.d.Runner, m.in.OutputDir)
	if err != nil {
		return config.Config{}, repo.Context{}, "", err
	}
	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return config.Config{}, repo.Context{}, "", err
	}
	commonDir, err := batch.CommonDir(m.ctx, m.d.Runner)
	if err != nil {
		return config.Config{}, repo.Context{}, "", err
	}
	return cfg, rc, commonDir, nil
}

// launchBranch launches exactly one member, found in the last Reconcile
// Report — the same Report tp ui's gh tick and `tp batch sync` both produce,
// so a manual launch key and Reconcile's own (disabled, on tp ui) launch step
// agree on what "ready to launch" means.
func (m uiModel) launchBranch(branch string) error {
	if m.report == nil {
		return fmt.Errorf("batch data not loaded yet")
	}
	cfg, rc, commonDir, err := m.launchContext()
	if err != nil {
		return err
	}
	for i := range m.report.Members {
		e := &m.report.Members[i]
		if e.Branch != branch {
			continue
		}
		if !readyToLaunch(commonDir, *e) {
			return fmt.Errorf("%s is not launchable", branch)
		}
		return launchEntry(m.d, cfg, commonDir, rc, e)
	}
	return fmt.Errorf("no Batch member for branch %q", branch)
}

// launchAllPending launches every member the last Reconcile Report marked
// materialised with no Activity file yet, returning how many succeeded.
func (m uiModel) launchAllPending() (int, error) {
	if m.report == nil {
		return 0, fmt.Errorf("batch data not loaded yet")
	}
	cfg, rc, commonDir, err := m.launchContext()
	if err != nil {
		return 0, err
	}
	count := 0
	for i := range m.report.Members {
		e := &m.report.Members[i]
		if !readyToLaunch(commonDir, *e) {
			continue
		}
		if err := launchEntry(m.d, cfg, commonDir, rc, e); err == nil {
			count++
		}
	}
	return count, nil
}

// activityExists reports whether branch already has an Activity file,
// resolving commonDir itself. Used by the log key to avoid opening a pager
// on a file that doesn't exist yet.
func activityExists(ctx context.Context, d deps.Deps, branch string) bool {
	commonDir, err := batch.CommonDir(ctx, d.Runner)
	if err != nil {
		return false
	}
	return launcher.Exists(commonDir, branch)
}
