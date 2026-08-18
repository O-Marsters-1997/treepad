package treepad

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/launcher"
)

func TestGroupRows(t *testing.T) {
	t.Run("main first, then Batches by name, Chains by number, position descending", func(t *testing.T) {
		rows := []StatusRow{
			{Branch: "b-chain0-pos1", Batch: "bbb", Chain: 0, Position: 1},
			{Branch: "a-chain1-pos0", Batch: "aaa", Chain: 1, Position: 0},
			{Branch: "main", IsMain: true},
			{Branch: "b-chain0-pos0", Batch: "bbb", Chain: 0, Position: 0},
			{Branch: "a-chain0-pos0", Batch: "aaa", Chain: 0, Position: 0},
			{Branch: "ungrouped"},
		}
		got := groupRows(rows)
		want := []string{
			"main",
			"a-chain0-pos0", "a-chain1-pos0",
			"b-chain0-pos1", "b-chain0-pos0",
			"ungrouped",
		}
		if len(got) != len(want) {
			t.Fatalf("len(got) = %d, want %d: %v", len(got), len(want), branchNames(got))
		}
		for i, w := range want {
			if got[i].Branch != w {
				t.Errorf("got[%d].Branch = %q, want %q (order: %v)", i, got[i].Branch, w, branchNames(got))
			}
		}
	})

	t.Run("ungrouped rows keep their original relative order", func(t *testing.T) {
		rows := []StatusRow{
			{Branch: "z"},
			{Branch: "a"},
			{Branch: "m"},
		}
		got := groupRows(rows)
		want := []string{"z", "a", "m"}
		for i, w := range want {
			if got[i].Branch != w {
				t.Errorf("got[%d].Branch = %q, want %q", i, got[i].Branch, w)
			}
		}
	})

	t.Run("a worktree with no Manifest entry still renders, ungrouped", func(t *testing.T) {
		rows := []StatusRow{
			{Branch: "solo"},
		}
		got := groupRows(rows)
		if len(got) != 1 || got[0].Branch != "solo" {
			t.Errorf("groupRows(solo-only) = %v, want [solo]", branchNames(got))
		}
	})
}

func TestApplyRunStates(t *testing.T) {
	commonDir := t.TempDir()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	path := launcher.ActivityPath(commonDir, "feat/working")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	rows := []StatusRow{
		{Branch: "feat/pending", Batch: "b"},
		{Branch: "feat/working", Batch: "b"},
		{Branch: "not-batch"},
	}
	got := applyRunStates(rows, commonDir, now)

	if got[0].RunState != string(launcher.StatePending) {
		t.Errorf("pending row RunState = %q, want %q", got[0].RunState, launcher.StatePending)
	}
	if got[1].RunState != string(launcher.StateWorking) {
		t.Errorf("working row RunState = %q, want %q", got[1].RunState, launcher.StateWorking)
	}
	if got[2].RunState != "" {
		t.Errorf("non-Batch row RunState = %q, want empty", got[2].RunState)
	}
}

func TestApplyPRState(t *testing.T) {
	t.Run("nil report leaves rows unchanged", func(t *testing.T) {
		rows := []StatusRow{{Branch: "feat", Batch: "b"}}
		got := applyPRState(rows, nil)
		if got[0].PRNumber != 0 || got[0].PRState != "" || got[0].PRStale {
			t.Errorf("row mutated by nil report: %+v", got[0])
		}
	})

	t.Run("overlays PR fields from a matching Report entry", func(t *testing.T) {
		rows := []StatusRow{
			{Branch: "feat/eng-12", Batch: "b"},
			{Branch: "not-in-report", Batch: "b"},
		}
		report := &Report{Members: []ReportEntry{
			{Member: batch.Member{Branch: "feat/eng-12"}, PRNumber: 42, PRState: "OPEN", PRStale: false},
		}}
		got := applyPRState(rows, report)
		if got[0].PRNumber != 42 || got[0].PRState != "OPEN" || got[0].PRStale {
			t.Errorf("row[0] = %+v, want PR 42/OPEN not stale", got[0])
		}
		if got[1].PRNumber != 0 || got[1].PRState != "" {
			t.Errorf("row[1] should be untouched, got %+v", got[1])
		}
	})
}

func TestFormatBatchAndRunAndPRCells(t *testing.T) {
	t.Run("non-Batch row: all three cells are the never-blank dash", func(t *testing.T) {
		r := StatusRow{Branch: "solo"}
		if got := formatBatchCell(r); got != "—" {
			t.Errorf("formatBatchCell = %q, want —", got)
		}
		if got := formatRunState(r); got != "—" {
			t.Errorf("formatRunState = %q, want —", got)
		}
		if got := formatPRState(r); got != "—" {
			t.Errorf("formatPRState = %q, want —", got)
		}
	})

	t.Run("Batch row renders batch/chain#position", func(t *testing.T) {
		r := StatusRow{Batch: "silent-refresh", Chain: 1, Position: 2}
		if got := formatBatchCell(r); got != "silent-refresh/1#2" {
			t.Errorf("formatBatchCell = %q, want silent-refresh/1#2", got)
		}
	})

	t.Run("run state renders the launcher state verbatim", func(t *testing.T) {
		r := StatusRow{Batch: "b", RunState: "working"}
		if got := formatRunState(r); got != "working" {
			t.Errorf("formatRunState = %q, want working", got)
		}
	})

	t.Run("PR state never blank: no PR yet", func(t *testing.T) {
		r := StatusRow{Batch: "b"}
		if got := formatPRState(r); got != "none" {
			t.Errorf("formatPRState = %q, want none", got)
		}
	})

	t.Run("PR state never blank: gh has never answered", func(t *testing.T) {
		r := StatusRow{Batch: "b", PRStale: true}
		if got := formatPRState(r); got != "stale" {
			t.Errorf("formatPRState = %q, want stale", got)
		}
	})

	t.Run("PR state: live PR", func(t *testing.T) {
		r := StatusRow{Batch: "b", PRNumber: 42, PRState: "OPEN"}
		if got := formatPRState(r); got != "#42 OPEN" {
			t.Errorf("formatPRState = %q, want #42 OPEN", got)
		}
	})

	t.Run("PR state: last-known plus staleness marker, never blank", func(t *testing.T) {
		r := StatusRow{Batch: "b", PRNumber: 42, PRState: "OPEN", PRStale: true}
		if got := formatPRState(r); got != "#42 OPEN (stale)" {
			t.Errorf("formatPRState = %q, want #42 OPEN (stale)", got)
		}
	})
}
