package batch

// LinkArgs returns the longest prefix of chain whose branches all have an
// open pull request, bottom to top — the argument list for `gh stack link`.
// It is a prefix, not a filter: stopping at the first member without an open
// pull request, rather than skipping it, because `gh stack link` sets each
// argument's base to the one before it — skipping a member would silently
// base the next one on the wrong branch.
//
// gh stack link pushes every branch it is given and creates a pull request
// for any that lack one, so a member past the prefix must never be included
// — passing the whole Chain "for tidiness" would push work-in-progress and
// open unwanted pull requests. This is load-bearing: do not widen the filter
// to include members without an open pull request.
//
// A Chain of fewer than two PR-having members produces no Stack, so the
// result is nil in that case too.
func LinkArgs(chain []Member, prs map[string]PR) []string {
	var branches []string
	for _, m := range chain {
		pr, ok := prs[m.Branch]
		if !ok || pr.State != "OPEN" {
			break
		}
		branches = append(branches, m.Branch)
	}
	if len(branches) < 2 {
		return nil
	}
	return branches
}
