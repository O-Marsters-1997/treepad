package treepad

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/config"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/repo"
	"github.com/O-Marsters-1997/treepad/internal/worktree"
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
	// Batch, Chain and Position identify this worktree's place in a Batch
	// Manifest, when its branch is a resolved Chain member; zero-value
	// (Batch empty) means it is not Batch-managed.
	Batch    string `json:"batch,omitempty"`
	Chain    int    `json:"chain,omitempty"`
	Position int    `json:"position,omitempty"`

	// RunState mirrors launcher.RunState (pending/working/idle), derived from
	// the Activity file's mtime. Empty for a non-Batch row: `tp status` never
	// populates it, since deriving it costs nothing but a stat call, the same
	// as tp ui's local ~5s tick.
	RunState string `json:"run_state,omitempty"`
	// PRNumber, PRState and PRStale mirror ReportEntry's fields of the same
	// name, filled in by tp ui's slower gh tick from the last Reconcile
	// Report. Zero-value on a non-Batch row or before the first gh tick
	// completes.
	PRNumber int    `json:"pr_number,omitempty"`
	PRState  string `json:"pr_state,omitempty"`
	PRStale  bool   `json:"pr_stale,omitempty"`
}

func refreshStatus(ctx context.Context, d deps.Deps, in StatusInput) ([]StatusRow, error) {
	rc, err := repo.Load(ctx, d.Runner, in.OutputDir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	members, err := batchMembersByBranch(ctx, d, cfg)
	if err != nil {
		return nil, err
	}
	return collectStatusRows(ctx, d, rc, cfg.Artifact, members)
}

// batchMembersByBranch resolves every Batch Manifest's Members, keyed by
// branch, so collectStatusRows can label a worktree with its Batch/Chain/
// Position regardless of whether Reconcile has run this session. A repo with
// no Manifests returns an empty map, not an error.
func batchMembersByBranch(ctx context.Context, d deps.Deps, cfg config.Config) (map[string]batch.Member, error) {
	commonDir, err := batch.CommonDir(ctx, d.Runner)
	if err != nil {
		return nil, err
	}
	manifests, err := batch.Load(commonDir)
	if err != nil {
		return nil, err
	}
	members := make(map[string]batch.Member)
	for _, m := range manifests {
		chains, err := batch.Resolve(m, cfg.FromSpec.TicketURL)
		if err != nil {
			return nil, err
		}
		for _, chain := range chains {
			for _, mem := range chain {
				members[mem.Branch] = mem
			}
		}
	}
	return members, nil
}

func Status(ctx context.Context, d deps.Deps, in StatusInput) error {
	rows, err := refreshStatus(ctx, d, in)
	if err != nil {
		return err
	}
	if in.JSON {
		return json.NewEncoder(d.Out).Encode(rows)
	}
	return writeStatusTable(d, rows)
}

func collectStatusRows(
	ctx context.Context, d deps.Deps, rc repo.Context, artCfg config.ArtifactConfig,
	members map[string]batch.Member,
) ([]StatusRow, error) {
	rows := make([]StatusRow, 0, len(rc.Worktrees))
	for _, wt := range rc.Worktrees {
		row := StatusRow{
			Branch:         wt.Branch,
			Path:           wt.Path,
			IsMain:         wt.IsMain,
			Prunable:       wt.Prunable,
			PrunableReason: wt.PrunableReason,
		}
		if mem, ok := members[wt.Branch]; ok {
			row.Batch = mem.Batch
			row.Chain = mem.Chain
			row.Position = mem.Position
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

		artifactPath, ok, err := config.ResolveArtifactPath(artCfg, rc.Slug, wt.Branch, wt.Path, rc.OutputDir)
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
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].IsMain && !rows[j].IsMain })
	return rows, nil
}

func writeStatusTable(d deps.Deps, rows []StatusRow) error {
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
	// StackStale mirrors batch.RestackStale: this worktree diverged from
	// origin/<branch> in a way treepad cannot safely repair — dirty, or
	// holding a commit git cherry says is not yet upstream — and it is
	// waiting for a human.
	StackStale bool
}

const uiStaleThreshold = 14 * 24 * time.Hour

// computeHealth derives health flags for each non-main, non-prunable worktree.
// Runs only local git/file-IO checks; no network calls are made.
func computeHealth(ctx context.Context, d deps.Deps, rows []StatusRow) (map[string]healthFlags, error) {
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
		// git cherry is the expensive check, so it runs only once divergence
		// is already established from ahead/behind counts already in hand.
		if !flags.Merged && r.HasUpstream && r.Ahead > 0 && r.Behind > 0 {
			patchEquivalent, err := patchEquivalentToOrigin(ctx, d.Runner, r.Path, r.Branch)
			if err != nil {
				return nil, err
			}
			flags.StackStale = batch.RestackDecision(!r.Dirty, r.Ahead, r.Behind, patchEquivalent) == batch.RestackStale
		}
		health[r.Branch] = flags
	}
	return health, nil
}

// deriveStatus returns a human-readable label and a category key for r,
// incorporating health flags. Priority: broken → detached → merged → dirty →
// stack-stale → diverged → ahead → behind → stale → local → clean.
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
	case h.StackStale:
		label, key = fmt.Sprintf("stack-stale · ↑%d ↓%d", r.Ahead, r.Behind), "stack-stale"
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
	_, _ = fmt.Fprintln(w, "BATCH\tBRANCH\tRUN\tPR\tSTATUS\tLAST COMMIT\tTOUCHED\tPATH")

	for _, r := range rows {
		branch := r.Branch
		if r.IsMain {
			branch += " *"
		}

		label, _ := deriveStatus(r, health[r.Branch])
		batchCell := formatBatchCell(r)
		runCell := formatRunState(r)
		prCell := formatPRState(r)

		if r.Prunable {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				batchCell, branch, runCell, prCell, label, r.PrunableReason, "—", collapsePath(r.Path))
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

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			batchCell, branch, runCell, prCell, label, lastCommit, touched, collapsePath(r.Path))
	}

	_ = w.Flush()
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// formatBatchCell renders a row's place in a Batch Manifest as
// "<batch>/<chain>#<position>", or "—" for a worktree no Manifest resolved —
// tp ui renders the whole fleet, Batch-managed or not.
func formatBatchCell(r StatusRow) string {
	if r.Batch == "" {
		return "—"
	}
	return fmt.Sprintf("%s/%d#%d", r.Batch, r.Chain, r.Position)
}

// formatRunState renders launcher.RunState, or "—" for a non-Batch row: there
// is no launch concept for a worktree no Manifest resolved.
func formatRunState(r StatusRow) string {
	if r.RunState == "" {
		return "—"
	}
	return r.RunState
}

// formatPRState renders a Batch member's pull request state, always
// non-blank per #142's degradation requirement: "none" (no PR yet) and
// "stale" (gh has never answered) are distinct from each other and from a
// live "#N STATE".
func formatPRState(r StatusRow) string {
	switch {
	case r.Batch == "":
		return "—"
	case r.PRNumber == 0 && r.PRStale:
		return "stale"
	case r.PRNumber == 0:
		return "none"
	case r.PRStale:
		return fmt.Sprintf("#%d %s (stale)", r.PRNumber, r.PRState)
	default:
		return fmt.Sprintf("#%d %s", r.PRNumber, r.PRState)
	}
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
	order := []string{
		"clean", "dirty", "stack-stale", "ahead", "behind", "diverged",
		"merged", "stale", "local", "detached", "broken",
	}
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
