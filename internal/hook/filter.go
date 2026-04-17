package hook

import "github.com/bmatcuk/doublestar/v4"

// shouldRun reports whether a hook entry should execute for the given branch.
// If Only is set, the branch must match at least one pattern.
// If Except is set, the branch must not match any pattern.
// Both conditions apply when both are set (AND semantics).
func shouldRun(entry HookEntry, branch string) bool {
	if len(entry.Only) > 0 {
		matched := false
		for _, pattern := range entry.Only {
			if ok, _ := doublestar.Match(pattern, branch); ok {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, pattern := range entry.Except {
		if ok, _ := doublestar.Match(pattern, branch); ok {
			return false
		}
	}
	return true
}
