// Package gh is the entire gh CLI surface for Batch orchestration (ADR
// 0003): `gh auth status`, via Available, and `gh pr list`, via PRList. It
// must never grow a call to `gh stack init`, `add`, `submit`, `modify`,
// `rebase`, or `sync` — see forbidden_test.go.
package gh

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/worktree"
)

// Available reports whether gh is installed and authenticated. `gh auth
// status` exits non-zero both when gh is absent (the exec itself fails) and
// when it is installed but unauthenticated, so one call answers both.
func Available(ctx context.Context, r worktree.CommandRunner) bool {
	_, err := r.Run(ctx, "gh", "auth", "status")
	return err == nil
}

type prListEntry struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	State       string `json:"state"`
	URL         string `json:"url"`
}

// PRList issues exactly one `gh pr list` call for the whole repo, keyed by
// head branch. --state all includes merged and closed pull requests, which
// drive the retire step and the link prefix filter (later tickets).
func PRList(ctx context.Context, r worktree.CommandRunner) (map[string]batch.PR, error) {
	out, err := r.Run(ctx, "gh", "pr", "list",
		"--json", "number,headRefName,baseRefName,state,url",
		"--state", "all", "--limit", "200")
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}

	var entries []prListEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse gh pr list output: %w", err)
	}

	prs := make(map[string]batch.PR, len(entries))
	for _, e := range entries {
		prs[e.HeadRefName] = batch.PR{
			Number:      e.Number,
			HeadRefName: e.HeadRefName,
			BaseRefName: e.BaseRefName,
			State:       e.State,
			URL:         e.URL,
		}
	}
	return prs, nil
}
