package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestRenderFilename(t *testing.T) {
	t.Run("renders slug and branch", func(t *testing.T) {
		spec := Spec{FilenameTemplate: `{{.Slug}}-{{.Branch}}.code-workspace`}
		data := TemplateData{Slug: "myrepo", Branch: "feature-auth"}

		got, err := RenderFilename(spec, data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "myrepo-feature-auth.code-workspace" {
			t.Errorf("got %q, want %q", got, "myrepo-feature-auth.code-workspace")
		}
	})

	t.Run("malformed template returns error", func(t *testing.T) {
		spec := Spec{FilenameTemplate: "{{.Unclosed"}
		_, err := RenderFilename(spec, TemplateData{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestRenderContent(t *testing.T) {
	t.Run("renders worktree fields into content", func(t *testing.T) {
		spec := Spec{
			FilenameTemplate: `{{.Slug}}-{{.Branch}}.txt`,
			ContentTemplate:  `path={{(index .Worktrees 0).RelPath}}`,
		}
		data := TemplateData{
			Slug:   "repo",
			Branch: "feat",
			Worktrees: []Worktree{
				{RelPath: "../repo-feat", Branch: "feat"},
			},
		}

		got, err := RenderContent(spec, data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "path=../repo-feat" {
			t.Errorf("got %q, want %q", string(got), "path=../repo-feat")
		}
	})

	t.Run("malformed template returns error", func(t *testing.T) {
		spec := Spec{FilenameTemplate: "ok.txt", ContentTemplate: "{{.Unclosed"}
		_, err := RenderContent(spec, TemplateData{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestPath(t *testing.T) {
	t.Run("zero spec returns empty path and false", func(t *testing.T) {
		path, ok, err := Path(Spec{}, "/some/dir", TemplateData{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("expected ok=false for zero spec")
		}
		if path != "" {
			t.Errorf("expected empty path, got %q", path)
		}
	})

	t.Run("non-zero spec returns joined path and true", func(t *testing.T) {
		spec := Spec{FilenameTemplate: `{{.Slug}}-{{.Branch}}.code-workspace`}
		data := TemplateData{Slug: "repo", Branch: "feat"}

		path, ok, err := Path(spec, "/out", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("expected ok=true for non-zero spec")
		}
		want := "/out/repo-feat.code-workspace"
		if path != want {
			t.Errorf("got %q, want %q", path, want)
		}
	})

	t.Run("template error is propagated", func(t *testing.T) {
		spec := Spec{FilenameTemplate: "{{.Unclosed"}
		_, _, err := Path(spec, "/out", TemplateData{})
		if err == nil || !strings.Contains(err.Error(), "render artifact filename") {
			t.Fatalf("got error %v, want error containing %q", err, "render artifact filename")
		}
	})
}

func TestWrite(t *testing.T) {
	t.Run("zero spec writes no file and returns empty path", func(t *testing.T) {
		dir := t.TempDir()

		path, err := Write(Spec{}, dir, TemplateData{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "" {
			t.Errorf("expected empty path for zero spec, got %q", path)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Error("no files should be written for zero spec")
		}
	})

	t.Run("writes file and returns its path", func(t *testing.T) {
		dir := t.TempDir()
		spec := Spec{
			FilenameTemplate: `{{.Slug}}-{{.Branch}}.txt`,
			ContentTemplate:  `branch={{.Branch}}`,
		}
		data := TemplateData{Slug: "repo", Branch: "feat"}

		got, err := Write(spec, dir, data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(dir, "repo-feat.txt")
		if got != want {
			t.Errorf("got path %q, want %q", got, want)
		}
		content, err := os.ReadFile(got)
		if err != nil {
			t.Fatalf("read written file: %v", err)
		}
		if string(content) != "branch=feat" {
			t.Errorf("file content %q, want %q", string(content), "branch=feat")
		}
	})

	t.Run("creates output dir when missing", func(t *testing.T) {
		base := t.TempDir()
		outputDir := filepath.Join(base, "new", "nested")
		spec := Spec{FilenameTemplate: "a.txt", ContentTemplate: ""}

		if _, err := Write(spec, outputDir, TemplateData{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(outputDir); err != nil {
			t.Errorf("output dir not created: %v", err)
		}
	})
}

func TestToWorktree(t *testing.T) {
	t.Run("sanitizes branch for Name and computes RelPath", func(t *testing.T) {
		outputDir := t.TempDir()
		worktreeDir := filepath.Join(filepath.Dir(outputDir), "repo-feature-auth")

		wt := ToWorktree("feature/auth", worktreeDir, outputDir)

		if wt.Branch != "feature/auth" {
			t.Errorf("Branch = %q, want %q", wt.Branch, "feature/auth")
		}
		if wt.Name != "feature-auth" {
			t.Errorf("Name = %q, want %q", wt.Name, "feature-auth")
		}
		if wt.Path != worktreeDir {
			t.Errorf("Path = %q, want %q", wt.Path, worktreeDir)
		}
		if wt.RelPath == "" || wt.RelPath == worktreeDir {
			t.Errorf("RelPath = %q, want a relative path", wt.RelPath)
		}
	})
}
