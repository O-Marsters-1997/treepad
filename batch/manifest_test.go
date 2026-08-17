package batch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest %s: %v", name, err)
	}
}

func TestLoad(t *testing.T) {
	t.Run("missing batches dir returns nil, not an error", func(t *testing.T) {
		commonDir := t.TempDir()
		got, err := Load(commonDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("unions every file under the batches dir", func(t *testing.T) {
		commonDir := t.TempDir()
		batchesDir := filepath.Join(commonDir, "treepad", "batches")
		if err := os.MkdirAll(batchesDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeManifest(t, batchesDir, "one.toml", `
name = "one"
[[chain]]
tickets = ["ENG-1"]
`)
		writeManifest(t, batchesDir, "two.toml", `
name = "two"
[[chain]]
tickets = ["ENG-2"]
`)

		got, err := Load(commonDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
		names := map[string]bool{got[0].Name: true, got[1].Name: true}
		if !names["one"] || !names["two"] {
			t.Errorf("got names %v, want one and two", names)
		}
	})

	t.Run("filename stem defaults the name; feat/ and main default the rest", func(t *testing.T) {
		commonDir := t.TempDir()
		batchesDir := filepath.Join(commonDir, "treepad", "batches")
		if err := os.MkdirAll(batchesDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeManifest(t, batchesDir, "silent-refresh.toml", `
[[chain]]
tickets = ["ENG-1"]
`)

		got, err := Load(commonDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("len(got) = %d, want 1", len(got))
		}
		m := got[0]
		if m.Name != "silent-refresh" {
			t.Errorf("Name = %q, want silent-refresh", m.Name)
		}
		if m.BranchPrefix != "feat/" {
			t.Errorf("BranchPrefix = %q, want feat/", m.BranchPrefix)
		}
		if m.Base != "main" {
			t.Errorf("Base = %q, want main", m.Base)
		}
	})

	t.Run("malformed file errors naming the file", func(t *testing.T) {
		commonDir := t.TempDir()
		batchesDir := filepath.Join(commonDir, "treepad", "batches")
		if err := os.MkdirAll(batchesDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeManifest(t, batchesDir, "broken.toml", `this is not valid toml [[[`)

		_, err := Load(commonDir)
		if err == nil || !strings.Contains(err.Error(), "broken.toml") {
			t.Fatalf("got error %v, want error naming broken.toml", err)
		}
	})
}
