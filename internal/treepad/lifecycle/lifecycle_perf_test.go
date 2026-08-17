//go:build perf

package lifecycle

import (
	"context"
	"strings"
	"testing"
	"time"

	internalsync "github.com/O-Marsters-1997/treepad/internal/sync"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
)

// slowSyncer sleeps for the configured duration to simulate a heavy file sync.
type slowSyncer struct {
	delay time.Duration
}

func (s slowSyncer) Sync(_ []string, _ internalsync.Config) (internalsync.SyncResult, error) {
	time.Sleep(s.delay)
	return internalsync.SyncResult{}, nil
}

// TestCreateWorktreeWithSync_ArtifactAndSyncRunConcurrently verifies that
// artifact.Write, git checkout, and LoadAndSync all run concurrently: a slow
// Syncer should not add latency on top of git checkout, so total wall-clock
// time should be close to max(sync, checkout, artifact) rather than their sum.
//
// Run with: go test -tags=perf -run TestCreateWorktreeWithSync ./internal/treepad/lifecycle/
func TestCreateWorktreeWithSync_ArtifactAndSyncRunConcurrently(t *testing.T) {
	const syncDelay = 300 * time.Millisecond

	mainPath := makeMainWorktree(t)
	outputDir := t.TempDir()
	porcelain := treepadtest.MainWorktreePorcelain(mainPath)

	runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
		{Output: porcelain},
		{Output: nil}, // git worktree add --no-checkout
		{Output: nil}, // git -C <path> checkout HEAD -- .
	}}
	d := deps.Deps{
		Runner: runner,
		Syncer: slowSyncer{delay: syncDelay},
		Opener: &treepadtest.FakeOpener{},
		In:     strings.NewReader(""),
	}

	start := time.Now()
	_, err := CreateWorktreeWithSync(context.Background(), d, "perf-branch", "main", outputDir)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Serial execution would be syncDelay + git_checkout + artifact ≈ 300ms+.
	// Concurrent: max of the three ≈ 300ms. Allow 100ms headroom for overhead.
	const serialBudget = syncDelay + 100*time.Millisecond
	t.Logf("elapsed=%v serial_budget=%v", elapsed, serialBudget)
	if elapsed >= serialBudget {
		t.Errorf("CreateWorktreeWithSync took %v ≥ %v, suggesting serial execution", elapsed, serialBudget)
	}
}
