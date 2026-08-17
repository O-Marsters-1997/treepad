package treepad

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/treepadtest"
)

const batchTOML = `
[from_spec]
ticket_url = "https://linear.app/acme/issue/{{.Ref}}"
`

func writeBatchManifest(t *testing.T, commonDir, name, content string) {
	t.Helper()
	batchesDir := filepath.Join(commonDir, "treepad", "batches")
	if err := os.MkdirAll(batchesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(batchesDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBatchList(t *testing.T) {
	t.Run("prints every Batch, Chain, and member", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		commonDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(batchTOML), 0o644); err != nil {
			t.Fatal(err)
		}
		writeBatchManifest(t, commonDir, "silent-refresh.toml", `
name = "silent-refresh"
branch_prefix = "feat/"
base = "main"
[[chain]]
tickets = ["ENG-12", "ENG-13"]
[[chain]]
tickets = ["ENG-14"]
`)

		porcelain := treepadtest.MainWorktreePorcelain(mainPath)
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},                // repo.Load: git worktree list
			{Output: []byte(commonDir + "\n")}, // batch.CommonDir: git rev-parse --git-common-dir
		}}
		var out bytes.Buffer
		var logBuf bytes.Buffer
		d := deps.Deps{Runner: runner, Out: &out, Log: treepadtest.NewPrinter(&logBuf)}

		if err := BatchList(context.Background(), d, BatchListInput{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := out.String()
		wantRows := []string{
			"silent-refresh", "0", "1", "ENG-12", "ENG-13", "ENG-14",
			"feat/eng-12", "feat/eng-13", "feat/eng-14", "main",
		}
		for _, want := range wantRows {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q; got:\n%s", want, got)
			}
		}
	})

	t.Run("--json emits the same data", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		commonDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(batchTOML), 0o644); err != nil {
			t.Fatal(err)
		}
		writeBatchManifest(t, commonDir, "silent-refresh.toml", `
name = "silent-refresh"
[[chain]]
tickets = ["ENG-14"]
`)

		porcelain := treepadtest.MainWorktreePorcelain(mainPath)
		runner := &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
			{Output: porcelain},
			{Output: []byte(commonDir + "\n")},
		}}
		var out bytes.Buffer
		d := deps.Deps{Runner: runner, Out: &out}

		if err := BatchList(context.Background(), d, BatchListInput{JSON: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var members []batch.Member
		if err := json.Unmarshal(out.Bytes(), &members); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, out.String())
		}
		if len(members) != 1 {
			t.Fatalf("len(members) = %d, want 1", len(members))
		}
		want := batch.Member{
			Ticket: "ENG-14", Ref: "ENG-14", TicketURL: "https://linear.app/acme/issue/ENG-14",
			Branch: "feat/eng-14", Base: "main", Batch: "silent-refresh", Chain: 0, Position: 0,
		}
		if members[0] != want {
			t.Errorf("members[0] = %+v, want %+v", members[0], want)
		}
	})

	t.Run("no Manifests: json is [], table notes the common dir, exit 0", func(t *testing.T) {
		mainPath := makeMainWorktree(t)
		commonDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(mainPath, ".treepad.toml"), []byte(batchTOML), 0o644); err != nil {
			t.Fatal(err)
		}

		porcelain := treepadtest.MainWorktreePorcelain(mainPath)
		newRunner := func() *treepadtest.SeqRunner {
			return &treepadtest.SeqRunner{Responses: []treepadtest.RunResponse{
				{Output: porcelain},
				{Output: []byte(commonDir + "\n")},
			}}
		}

		var jsonOut bytes.Buffer
		jsonDeps := deps.Deps{Runner: newRunner(), Out: &jsonOut}
		if err := BatchList(context.Background(), jsonDeps, BatchListInput{JSON: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(jsonOut.String()) != "[]" {
			t.Errorf("json output = %q, want []", jsonOut.String())
		}

		var tableOut, logBuf bytes.Buffer
		tableDeps := deps.Deps{Runner: newRunner(), Out: &tableOut, Log: treepadtest.NewPrinter(&logBuf)}
		if err := BatchList(context.Background(), tableDeps, BatchListInput{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(logBuf.String(), commonDir) {
			t.Errorf("log missing common dir %q; got: %s", commonDir, logBuf.String())
		}
	})
}
