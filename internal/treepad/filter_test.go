package treepad

import (
	"testing"
)

func TestFilterRows(t *testing.T) {
	rows := []StatusRow{
		{Branch: "main", Path: "/repo/main"},
		{Branch: "feat/foo-bar", Path: "/repo/foo-bar"},
		{Branch: "fix/baz", Path: "/repo/baz"},
	}

	t.Run("empty returns all", func(t *testing.T) {
		got := filterRows(rows, "")
		if len(got) != len(rows) {
			t.Fatalf("got %d rows, want %d", len(got), len(rows))
		}
	})

	t.Run("exact branch match", func(t *testing.T) {
		got := filterRows(rows, "main")
		assertBranches(t, got, []string{"main"})
	})

	t.Run("substring in branch", func(t *testing.T) {
		got := filterRows(rows, "foo")
		assertBranches(t, got, []string{"feat/foo-bar"})
	})

	t.Run("fuzzy subsequence fb matches both branches", func(t *testing.T) {
		// "fb" is a valid subsequence for both feat/foo-bar and fix/baz.
		// Exact order is scoring-algorithm-dependent; assert both are present.
		got := filterRows(rows, "fb")
		if len(got) != 2 {
			t.Fatalf("got %d rows for 'fb', want 2: %v", len(got), branchNames(got))
		}
		set := map[string]bool{got[0].Branch: true, got[1].Branch: true}
		if !set["feat/foo-bar"] || !set["fix/baz"] {
			t.Errorf("got %v, want feat/foo-bar and fix/baz", branchNames(got))
		}
	})

	t.Run("matches path basename", func(t *testing.T) {
		// "baz" matches via path basename "baz" even though branch is "fix/baz".
		got := filterRows(rows, "baz")
		assertBranches(t, got, []string{"fix/baz"})
	})

	t.Run("no matches", func(t *testing.T) {
		got := filterRows(rows, "zzz")
		if len(got) != 0 {
			t.Errorf("got %v, want no results", branchNames(got))
		}
	})
}

func assertBranches(t *testing.T, got []StatusRow, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d rows %v, want %d %v", len(got), branchNames(got), len(want), want)
	}
	for i, w := range want {
		if got[i].Branch != w {
			t.Errorf("row[%d].Branch = %q, want %q", i, got[i].Branch, w)
		}
	}
}

func TestFilterRowsMatchesBatchAndChain(t *testing.T) {
	rows := []StatusRow{
		{Branch: "feat/eng-12", Path: "/repo/eng-12", Batch: "silent-refresh", Chain: 0},
		{Branch: "feat/eng-14", Path: "/repo/eng-14", Batch: "silent-refresh", Chain: 1},
		{Branch: "feat/other", Path: "/repo/other", Batch: "other-batch", Chain: 0},
		{Branch: "unrelated", Path: "/repo/unrelated"},
	}

	t.Run("matches Batch name across both Chains", func(t *testing.T) {
		got := filterRows(rows, "silent-refresh")
		set := map[string]bool{}
		for _, r := range got {
			set[r.Branch] = true
		}
		if !set["feat/eng-12"] || !set["feat/eng-14"] {
			t.Errorf("got %v, want feat/eng-12 and feat/eng-14", branchNames(got))
		}
		if set["feat/other"] {
			t.Errorf("got %v, want other-batch excluded", branchNames(got))
		}
	})

	t.Run("a row outside any Batch never matches on the Batch field", func(t *testing.T) {
		got := filterRows(rows, "silent-refresh")
		for _, r := range got {
			if r.Branch == "unrelated" {
				t.Error("unrelated row matched a Batch-name query")
			}
		}
	})
}

func TestFilterRowsNoCrossFieldMatch(t *testing.T) {
	// Verify that characters from the path cannot complete a branch-only
	// subsequence. "feat" should NOT match "bug-fix-b" even if the path
	// happens to contain 'e', 'a', 't' after the 'f' in "fix".
	rows := []StatusRow{
		{Branch: "bug-fix-b", Path: "/var/folders/feat-like/path/repo-bug-fix-b"},
		{Branch: "feature-a", Path: "/repo/feature-a"},
	}
	got := filterRows(rows, "feat")
	if len(got) != 1 || got[0].Branch != "feature-a" {
		t.Errorf("filterRows(\"feat\") = %v, want only feature-a", branchNames(got))
	}
}

func branchNames(rows []StatusRow) []string {
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Branch
	}
	return names
}
