package fromspec

import (
	"context"
	"fmt"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/profile"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/lifecycle"
)

// FromSpecBulkInput parameterises a tp from-spec-bulk invocation.
type FromSpecBulkInput struct {
	Tickets      []string
	BranchPrefix string
	Base         string
	OutputDir    string
	// Prompt is optional user-supplied instructions appended to each prompt body.
	Prompt string
}

// BulkResult records the outcome for one ticket in a bulk run.
type BulkResult struct {
	Ticket       string
	TicketURL    string
	Branch       string
	WorktreePath string
	PromptPath   string
	Err          error
}

// FromSpecBulk creates one worktree per ticket, writing PROMPT.md into each.
// It never launches an agent and never emits __TREEPAD_CD__. On partial
// failure it continues to the next ticket and records the error in the result.
// Returns the per-ticket results, a count of failures, and any fatal setup error.
func FromSpecBulk(ctx context.Context, d deps.Deps, in FromSpecBulkInput) ([]BulkResult, int, error) {
	p := profile.OrDisabled(d.Profiler)

	// Loaded once, ahead of the loop: batch.DeriveBranch needs each ticket's
	// Ref, which comes from batch.ResolveTicket, which needs this config.
	fsCfg, err := loadFromSpecConfig(ctx, d, in.OutputDir)
	if err != nil {
		return nil, 0, err
	}

	results := make([]BulkResult, 0, len(in.Tickets))
	failed := 0

	for _, ticket := range in.Tickets {
		res := BulkResult{Ticket: ticket}

		resolveDone := p.Stage("ticket.resolve")
		ticketURL, ref, err := batch.ResolveTicket(fsCfg.TicketURL, ticket)
		resolveDone()
		if err != nil {
			res.Err = err
			results = append(results, res)
			failed++
			continue
		}
		res.TicketURL = ticketURL

		branch := batch.DeriveBranch(in.BranchPrefix, ref)
		res.Branch = branch

		wtRes, err := lifecycle.CreateWorktreeWithSync(ctx, d, branch, in.Base, in.OutputDir)
		if err != nil {
			res.Err = fmt.Errorf("create worktree: %w", err)
			results = append(results, res)
			failed++
			continue
		}
		res.WorktreePath = wtRes.WorktreePath

		promptBody := buildPrompt(wtRes.Cfg.FromSpec, branch, specCitation(ticketURL), in.Prompt)
		promptDone := p.Stage("prompt.write")
		promptPath, err := writePromptFile(d, wtRes.WorktreePath, promptBody)
		promptDone()
		if err != nil {
			res.Err = fmt.Errorf("render prompt: %w", err)
			results = append(results, res)
			failed++
			continue
		}
		res.PromptPath = promptPath

		results = append(results, res)
	}

	printBulkSummary(d, results)
	return results, failed, nil
}

func printBulkSummary(d deps.Deps, results []BulkResult) {
	succeeded := 0
	for _, r := range results {
		if r.Err == nil {
			succeeded++
		}
	}
	failed := len(results) - succeeded

	d.Log.Step("RESULTS")
	for _, r := range results {
		if r.Err == nil {
			d.Log.OK("  %s  %s   %s", r.Ticket, r.Branch, r.WorktreePath)
		} else {
			d.Log.Warn("  %s  %s", r.Ticket, r.Err)
		}
	}
	d.Log.Info("%d succeeded, %d failed", succeeded, failed)
}
