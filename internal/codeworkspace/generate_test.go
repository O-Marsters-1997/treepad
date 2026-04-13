package codeworkspace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"treepad/internal/worktree"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeBranch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main", "main"},
		{"feature/my-work", "feature-my-work"},
		{"a:b*c?d", "a-b-c-d"},
		{"a\\b", "a-b"},
		{"has<angle>brackets", "has-angle-brackets"},
		{"clean-branch", "clean-branch"},
		{`a"b`, "a-b"},
		{"a|b", "a-b"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeBranch(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeBranch(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerate(t *testing.T) {
	t.Run("single worktree creates correct file", func(t *testing.T) {
		outputDir := t.TempDir()
		wts := []worktree.Worktree{
			{Path: filepath.Join(outputDir, "../repo"), Branch: "main"},
		}
		extensions := []string{"golang.go"}

		err := Generate(wts, extensions, "myslug", outputDir, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("Generate error: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(outputDir, "myslug-main.code-workspace"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}

		var wf workspaceFile
		if err := json.Unmarshal(data, &wf); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(wf.Folders) != 1 {
			t.Fatalf("folders len = %d, want 1", len(wf.Folders))
		}
		if wf.Folders[0].Name != "main" {
			t.Errorf("folder name = %q, want %q", wf.Folders[0].Name, "main")
		}
		recs := wf.Extensions["recommendations"]
		if len(recs) != 1 || recs[0] != "golang.go" {
			t.Errorf("recommendations = %v, want [golang.go]", recs)
		}
	})

	t.Run("branch with slash is sanitized in filename", func(t *testing.T) {
		outputDir := t.TempDir()
		wts := []worktree.Worktree{
			{Path: filepath.Join(outputDir, "repo1"), Branch: "main"},
			{Path: filepath.Join(outputDir, "repo2"), Branch: "feature/x"},
		}

		err := Generate(wts, nil, "slug", outputDir, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("Generate error: %v", err)
		}

		for _, name := range []string{"slug-main.code-workspace", "slug-feature-x.code-workspace"} {
			if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
				t.Errorf("expected file %q not found: %v", name, err)
			}
		}
	})

	t.Run("writes filenames to out", func(t *testing.T) {
		outputDir := t.TempDir()
		wts := []worktree.Worktree{
			{Path: filepath.Join(outputDir, "repo"), Branch: "main"},
		}
		var buf bytes.Buffer

		if err := Generate(wts, nil, "slug", outputDir, &buf); err != nil {
			t.Fatalf("Generate error: %v", err)
		}

		if !strings.Contains(buf.String(), "slug-main.code-workspace") {
			t.Errorf("output %q does not mention created file", buf.String())
		}
	})

	t.Run("creates output dir if missing", func(t *testing.T) {
		base := t.TempDir()
		outputDir := filepath.Join(base, "new", "nested", "dir")
		wts := []worktree.Worktree{
			{Path: filepath.Join(base, "repo"), Branch: "main"},
		}

		err := Generate(wts, nil, "slug", outputDir, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("Generate error: %v", err)
		}
		if _, err := os.Stat(outputDir); err != nil {
			t.Errorf("output dir not created: %v", err)
		}
	})
}
