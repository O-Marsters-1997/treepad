package treepad

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/config"
	"github.com/O-Marsters-1997/treepad/internal/gh"
	"github.com/O-Marsters-1997/treepad/internal/slug"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/lifecycle"
	"github.com/O-Marsters-1997/treepad/internal/treepad/repo"
	"github.com/O-Marsters-1997/treepad/internal/worktree"
)

type BatchListInput struct {
	JSON      bool
	OutputDir string
}

// BatchList prints every Batch, its Chains, and each member's ticket, ref,
// branch and base. It creates no worktrees and calls gh for nothing.
func BatchList(ctx context.Context, d deps.Deps, in BatchListInput) error {
	rc, err := repo.Load(ctx, d.Runner, in.OutputDir)
	if err != nil {
		return err
	}
	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	commonDir, err := batch.CommonDir(ctx, d.Runner)
	if err != nil {
		return err
	}

	manifests, err := batch.Load(commonDir)
	if err != nil {
		return err
	}

	members := make([]batch.Member, 0)
	for _, m := range manifests {
		chains, err := batch.Resolve(m, cfg.FromSpec.TicketURL)
		if err != nil {
			return err
		}
		for _, chain := range chains {
			members = append(members, chain...)
		}
	}

	if in.JSON {
		return json.NewEncoder(d.Out).Encode(members)
	}
	if len(manifests) == 0 {
		d.Log.Info("no Batches found under %s", filepath.Join(commonDir, "treepad", "batches"))
		return nil
	}
	return writeBatchTable(d, members)
}

func writeBatchTable(d deps.Deps, members []batch.Member) error {
	for _, line := range formatBatchRows(members) {
		_, _ = fmt.Fprintln(d.Out, line)
	}
	return nil
}

func formatBatchRows(members []batch.Member) []string {
	if len(members) == 0 {
		return nil
	}
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "BATCH\tCHAIN\tPOS\tTICKET\tREF\tBRANCH\tBASE")
	for _, m := range members {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\t%s\t%s\n",
			m.Batch, m.Chain, m.Position, m.Ticket, m.Ref, m.Branch, m.Base)
	}
	_ = w.Flush()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// Action values a ReportEntry can carry.
const (
	ActionCreated     = "created"      // worktree created this tick
	ActionSkipped     = "skipped"      // branch already existed; nothing to do
	ActionWouldCreate = "would-create" // --dry-run: would have been created
	ActionBlocked     = "blocked"      // gh is available but the parent has no open pull request yet
	ActionGHRequired  = "gh-required"  // gh absent, unauthenticated, or --offline: parent's PR state is unknown
	ActionError       = "error"        // materialise failed; this Chain stops here
)

// ReportEntry pairs a resolved Member with what Reconcile did about it this tick.
type ReportEntry struct {
	batch.Member
	WorktreePath string `json:"worktree_path,omitempty"`
	Action       string `json:"action"`
	Error        string `json:"error,omitempty"`
	PRNumber     int    `json:"pr_number,omitempty"`
	PRState      string `json:"pr_state,omitempty"`
	// PRStale is true when --offline was passed, or gh is absent or failed
	// this tick, so PRNumber/PRState are last-known rather than fresh. Its
	// zero value must never be mistaken for "no PR exists" — that case has
	// PRStale false and PRNumber/PRState empty.
	PRStale bool `json:"pr_stale,omitempty"`
}

// Report is the JSON shape for `--json` and the row source for the TUI: one
// entry per Member with the action Reconcile took on it this tick.
type Report struct {
	Members []ReportEntry `json:"members"`
}

// ReconcileInput parameterises Reconcile, called by both `tp batch sync` and
// (later) `tp ui`'s tick.
type ReconcileInput struct {
	Batch     string // narrows to one Manifest by name; empty means every Manifest
	DryRun    bool
	Offline   bool // skip the gh call entirely; PR fields report last-known plus staleness
	OutputDir string
}

// Reconcile is the single function driving Batch orchestration. Its five
// steps run in this fixed order on every tick; only materialise is
// implemented so far, the rest land as no-op stubs for later tickets.
func Reconcile(ctx context.Context, d deps.Deps, in ReconcileInput) (Report, error) {
	rc, err := repo.Load(ctx, d.Runner, in.OutputDir)
	if err != nil {
		return Report{}, err
	}
	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return Report{}, fmt.Errorf("load config: %w", err)
	}
	commonDir, err := batch.CommonDir(ctx, d.Runner)
	if err != nil {
		return Report{}, err
	}
	manifests, err := batch.Load(commonDir)
	if err != nil {
		return Report{}, err
	}

	prs, stale := loadPRs(ctx, d, in.Offline, commonDir)

	var entries []ReportEntry
	for _, m := range manifests {
		if in.Batch != "" && m.Name != in.Batch {
			continue
		}
		chains, err := batch.Resolve(m, cfg.FromSpec.TicketURL)
		if err != nil {
			return Report{}, err
		}
		for _, chain := range chains {
			entries = append(entries, materialise(ctx, d, in, rc, chain, prs, stale)...)
		}
	}
	report := Report{Members: entries}
	attachPRState(report.Members, prs, stale)

	launch(ctx, d, in, report)
	link(ctx, d, in, report)
	restack(ctx, d, in, report)
	retire(ctx, d, in, report)

	return report, nil
}

// materialise walks one Chain in order, creating a stacked worktree per
// member via lifecycle.CreateWorktreeWithSync — which already creates a
// stacked worktree when base is a sibling's branch. The gate for
// materialising a member beyond position 0 is batch.ReadyToMaterialise: the
// parent must have an open (or merged) pull request, not merely an existing
// branch. Skipping is by branch existence, so re-running is idempotent. A
// member that fails stops this Chain only — other Chains are unaffected. A
// member past the ready prefix is reported, never materialised: "blocked"
// when gh is working but the parent has no pull request yet, "gh-required"
// when gh is absent, unauthenticated, or --offline made its state unknown.
func materialise(
	ctx context.Context, d deps.Deps, in ReconcileInput, rc repo.Context, chain []batch.Member,
	prs map[string]batch.PR, stale bool,
) []ReportEntry {
	entries := make([]ReportEntry, 0, len(chain))
	existing := make(map[string]bool, len(chain))
	for _, m := range chain {
		exists, err := branchExists(ctx, d.Runner, m.Branch)
		if err != nil {
			return append(entries, ReportEntry{
				Member: m, WorktreePath: worktreePathFor(rc, m.Branch),
				Action: ActionError, Error: err.Error(),
			})
		}
		existing[m.Branch] = exists
	}

	ready := batch.ReadyToMaterialise(chain, existing, prs)

	for i, m := range chain {
		entry := ReportEntry{Member: m, WorktreePath: worktreePathFor(rc, m.Branch)}

		if i >= len(ready) {
			if stale {
				entry.Action = ActionGHRequired
				entry.Error = fmt.Sprintf("gh required to check whether parent branch %q has an open pull request", m.Base)
			} else {
				entry.Action = ActionBlocked
				entry.Error = fmt.Sprintf("parent branch %q has no open pull request", m.Base)
			}
			entries = append(entries, entry)
			continue
		}

		if existing[m.Branch] {
			entry.Action = ActionSkipped
			entries = append(entries, entry)
			continue
		}

		if in.DryRun {
			entry.Action = ActionWouldCreate
			entries = append(entries, entry)
			continue
		}

		res, err := lifecycle.CreateWorktreeWithSync(ctx, d, m.Branch, m.Base, in.OutputDir)
		if err != nil {
			entry.Action, entry.Error = ActionError, err.Error()
			entries = append(entries, entry)
			break
		}
		entry.Action = ActionCreated
		entry.WorktreePath = res.WorktreePath
		entries = append(entries, entry)
	}

	return entries
}

// launch starts an agent for a materialised member that has none yet.
// Not built in this ticket — a later ticket fills this in.
func launch(_ context.Context, _ deps.Deps, _ ReconcileInput, _ Report) {}

// link runs `gh stack link` across each Chain's ready prefix.
// Not built in this ticket — a later ticket fills this in.
func link(_ context.Context, _ deps.Deps, _ ReconcileInput, _ Report) {}

// restack repairs worktrees left behind by a merged member.
// Not built in this ticket — a later ticket fills this in.
func restack(_ context.Context, _ deps.Deps, _ ReconcileInput, _ Report) {}

// retire marks a merged member's worktree removable.
// Not built in this ticket — a later ticket fills this in.
func retire(_ context.Context, _ deps.Deps, _ ReconcileInput, _ Report) {}

// attachPRState fills in PRNumber, PRState and PRStale on each entry from
// the tick's already-loaded PR data. It never fails Reconcile: --offline, gh
// being absent, or the call itself failing all degrade to the last cached
// result (nil on a cold cache) with PRStale set, rather than blocking the
// tick.
func attachPRState(entries []ReportEntry, prs map[string]batch.PR, stale bool) {
	for i := range entries {
		if pr, ok := prs[entries[i].Branch]; ok {
			entries[i].PRNumber = pr.Number
			entries[i].PRState = pr.State
		}
		entries[i].PRStale = stale
	}
}

func loadPRs(ctx context.Context, d deps.Deps, offline bool, commonDir string) (map[string]batch.PR, bool) {
	cachePath := prCachePath(commonDir)
	if offline || !gh.Available(ctx, d.Runner) {
		return loadPRCache(cachePath), true
	}
	prs, err := gh.PRList(ctx, d.Runner)
	if err != nil {
		return loadPRCache(cachePath), true
	}
	savePRCache(cachePath, prs)
	return prs, false
}

// prCachePath is where the last successful `gh pr list` result is cached, so
// an offline or failing tick can report last-known PR state instead of a
// blank one.
func prCachePath(commonDir string) string {
	return filepath.Join(commonDir, "treepad", "pr-cache.json")
}

func loadPRCache(path string) map[string]batch.PR {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var prs map[string]batch.PR
	if err := json.Unmarshal(data, &prs); err != nil {
		return nil
	}
	return prs
}

// savePRCache writes best-effort: a failed write degrades the next stale
// tick's data, not this one's, so it is not an error Reconcile need surface.
func savePRCache(path string, prs map[string]batch.PR) {
	data, err := json.Marshal(prs)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// branchExists reports whether branch is a known local branch. `git branch
// --list` always exits 0, so existence is read from output emptiness rather
// than the exit code.
func branchExists(ctx context.Context, r worktree.CommandRunner, branch string) (bool, error) {
	out, err := r.Run(ctx, "git", "branch", "--list", branch)
	if err != nil {
		return false, fmt.Errorf("git branch --list %s: %w", branch, err)
	}
	return len(bytes.TrimSpace(out)) > 0, nil
}

// worktreePathFor mirrors lifecycle.CreateWorktreeWithSync's path derivation,
// so a Report can name a Member's worktree path before it exists (--dry-run,
// or a member already skipped).
func worktreePathFor(rc repo.Context, branch string) string {
	return filepath.Join(filepath.Dir(rc.Main.Path), rc.Slug+"-"+slug.Slug(branch))
}

// BatchSyncInput parameterises the `tp batch sync` command.
type BatchSyncInput struct {
	JSON      bool
	DryRun    bool
	Offline   bool
	Batch     string
	OutputDir string
}

// BatchSync runs Reconcile and renders the Report for `tp batch sync`. It
// returns the count of members whose materialisation failed this tick, for
// the caller to translate into a process exit code.
func BatchSync(ctx context.Context, d deps.Deps, in BatchSyncInput) (int, error) {
	report, err := Reconcile(ctx, d, ReconcileInput{
		Batch:     in.Batch,
		DryRun:    in.DryRun,
		Offline:   in.Offline,
		OutputDir: in.OutputDir,
	})
	if err != nil {
		return 0, err
	}

	if in.JSON {
		if err := json.NewEncoder(d.Out).Encode(report); err != nil {
			return 0, err
		}
	} else if len(report.Members) == 0 {
		d.Log.Info("no Batches found to reconcile")
	} else {
		writeReconcileTable(d, report.Members)
	}

	failed := 0
	for _, e := range report.Members {
		if e.Action == ActionError {
			failed++
		}
	}
	return failed, nil
}

func writeReconcileTable(d deps.Deps, entries []ReportEntry) {
	for _, line := range formatReconcileRows(entries) {
		_, _ = fmt.Fprintln(d.Out, line)
	}
}

func formatReconcileRows(entries []ReportEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "BATCH\tCHAIN\tPOS\tBRANCH\tACTION\tDETAIL")
	for _, e := range entries {
		detail := e.WorktreePath
		if e.Error != "" {
			detail = e.Error
		}
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\t%s\n", e.Batch, e.Chain, e.Position, e.Branch, e.Action, detail)
	}
	_ = w.Flush()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}
