package treepad

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"treepad/internal/slug"
)

// FromSpecBulkInput parameterises a tp from-spec-bulk invocation.
type FromSpecBulkInput struct {
	Issues       []int
	BranchPrefix string
	Base         string
	OutputDir    string
}

// BulkResult records the outcome for one issue in a bulk run.
type BulkResult struct {
	Issue        int
	Branch       string
	WorktreePath string
	PromptPath   string
	Err          error
}

type issueJSON struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// FromSpecBulk creates one worktree per issue, writing PROMPT.md into each.
// It never launches an agent and never emits __TREEPAD_CD__. On partial
// failure it continues to the next issue and records the error in the result.
// Returns the per-issue results, a count of failures, and any fatal setup error.
func FromSpecBulk(ctx context.Context, d Deps, in FromSpecBulkInput) ([]BulkResult, int, error) {
	results := make([]BulkResult, 0, len(in.Issues))
	usedBranches := make(map[string]bool)
	failed := 0

	for _, issueNum := range in.Issues {
		res := BulkResult{Issue: issueNum}

		title, body, err := fetchIssue(ctx, d, issueNum)
		if err != nil {
			res.Err = err
			results = append(results, res)
			failed++
			continue
		}

		branch := deriveBranch(in.BranchPrefix, title, issueNum, usedBranches)
		usedBranches[branch] = true
		res.Branch = branch

		wtRes, err := createWorktreeWithSync(ctx, d, branch, in.Base, in.OutputDir)
		if err != nil {
			res.Err = fmt.Errorf("create worktree: %w", err)
			results = append(results, res)
			failed++
			continue
		}
		res.WorktreePath = wtRes.WorktreePath

		promptPath, _, err := renderAndWritePrompt(d, wtRes, branch, body)
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

func fetchIssue(ctx context.Context, d Deps, issue int) (title, body string, err error) {
	out, err := d.Runner.Run(ctx, "gh", "issue", "view", strconv.Itoa(issue), "--json", "title,body")
	if err != nil {
		return "", "", fmt.Errorf("gh issue view %d: %w", issue, err)
	}
	var data issueJSON
	if err := json.Unmarshal(out, &data); err != nil {
		return "", "", fmt.Errorf("parse issue %d: %w", issue, err)
	}
	data.Title = strings.TrimSpace(data.Title)
	data.Body = strings.TrimSpace(data.Body)
	if data.Body == "" {
		return "", "", fmt.Errorf("issue %d has an empty body", issue)
	}
	return data.Title, data.Body, nil
}

// deriveBranch computes a unique branch name from prefix + slug(title).
// If the result collides with an already-used branch, appends -<issueNum>.
func deriveBranch(prefix, title string, issueNum int, used map[string]bool) string {
	base := prefix + slug.Slug(title)
	if !used[base] {
		return base
	}
	return base + "-" + strconv.Itoa(issueNum)
}

func printBulkSummary(d Deps, results []BulkResult) {
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
			d.Log.OK("  #%d  %s   %s", r.Issue, r.Branch, r.WorktreePath)
		} else {
			d.Log.Warn("  #%d  %s", r.Issue, r.Err)
		}
	}
	d.Log.Info("%d succeeded, %d failed", succeeded, failed)
}
