package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestLinkerReconcile(t *testing.T) {
	write := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name        string
		setup       func(t *testing.T, src, dst string)
		tracked     map[string]bool
		entry       string
		wantCreated []string
		wantSkip    string
		wantLink    bool
	}{
		{
			name:        "links a file and creates parents",
			setup:       func(t *testing.T, src, _ string) { write(t, filepath.Join(src, "a/b.txt"), "x") },
			entry:       "a/b.txt",
			wantCreated: []string{"a/b.txt"},
			wantLink:    true,
		},
		{
			name:     "absent in source is skipped",
			setup:    func(*testing.T, string, string) {},
			entry:    "missing",
			wantSkip: "absent in source",
		},
		{
			name:     "tracked is skipped",
			setup:    func(t *testing.T, src, _ string) { write(t, filepath.Join(src, "t"), "x") },
			tracked:  map[string]bool{"t": true},
			entry:    "t",
			wantSkip: "tracked in git",
		},
		{
			name: "existing regular file is left alone",
			setup: func(t *testing.T, src, dst string) {
				write(t, filepath.Join(src, "f"), "x")
				write(t, filepath.Join(dst, "f"), "local")
			},
			entry:    "f",
			wantSkip: "path exists and is not a symlink",
		},
		{
			name: "foreign symlink is left alone",
			setup: func(t *testing.T, src, dst string) {
				write(t, filepath.Join(src, "f"), "x")
				if err := os.Symlink("/elsewhere", filepath.Join(dst, "f")); err != nil {
					t.Fatal(err)
				}
			},
			entry:    "f",
			wantSkip: "path is a symlink not owned by treepad",
		},
		{
			name: "owned symlink is unchanged",
			setup: func(t *testing.T, src, dst string) {
				write(t, filepath.Join(src, "f"), "x")
				if err := os.Symlink(filepath.Join(src, "f"), filepath.Join(dst, "f")); err != nil {
					t.Fatal(err)
				}
			},
			entry:    "f",
			wantLink: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			tt.setup(t, src, dst)
			l := Linker{Tracked: func(rel string) bool { return tt.tracked[rel] }}

			res, err := l.Reconcile([]string{tt.entry}, Config{SourceDir: src, TargetDir: dst})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(res.Created, tt.wantCreated) {
				t.Errorf("Created = %v, want %v", res.Created, tt.wantCreated)
			}
			if tt.wantSkip != "" && (len(res.Skipped) != 1 || res.Skipped[0].Reason != tt.wantSkip) {
				t.Errorf("Skipped = %v, want reason %q", res.Skipped, tt.wantSkip)
			}
			if tt.wantSkip == "" && len(res.Skipped) != 0 {
				t.Errorf("Skipped = %v, want none", res.Skipped)
			}
			if tt.wantLink {
				got, err := os.Readlink(filepath.Join(dst, tt.entry))
				if err != nil || got != filepath.Join(src, tt.entry) {
					t.Errorf("Readlink = %q, %v; want %q", got, err, filepath.Join(src, tt.entry))
				}
			}
		})
	}

	t.Run("writing through the link changes the source", func(t *testing.T) {
		src, dst := t.TempDir(), t.TempDir()
		write(t, filepath.Join(src, "f"), "old")
		if _, err := (Linker{}).Reconcile([]string{"f"}, Config{SourceDir: src, TargetDir: dst}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, "f"), []byte("new"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(filepath.Join(src, "f"))
		if string(got) != "new" {
			t.Errorf("source = %q, want new", got)
		}
	})
}

func TestLinkerExpand(t *testing.T) {
	tests := []struct {
		name        string
		files       []string
		tracked     map[string]bool
		entries     []string
		wantCreated []string
		wantSkipped []string
	}{
		{
			name:        "directory becomes one link",
			files:       []string{".codemap/a", ".codemap/b"},
			entries:     []string{".codemap/"},
			wantCreated: []string{".codemap"},
		},
		{
			name:        "glob links each match",
			files:       []string{".vscode/a.code-snippets", ".vscode/b.code-snippets", ".vscode/c.json"},
			entries:     []string{".vscode/*.code-snippets"},
			wantCreated: []string{".vscode/a.code-snippets", ".vscode/b.code-snippets"},
		},
		{
			name:        "glob drops tracked matches",
			files:       []string{".vscode/a.code-snippets", ".vscode/b.code-snippets"},
			tracked:     map[string]bool{".vscode/a.code-snippets": true},
			entries:     []string{".vscode/*.code-snippets"},
			wantCreated: []string{".vscode/b.code-snippets"},
		},
		{
			name:        "nested entry is covered by parent",
			files:       []string{".claude/settings.local.json"},
			entries:     []string{".claude/", ".claude/settings.local.json"},
			wantCreated: []string{".claude"},
			wantSkipped: []string{".claude/settings.local.json"},
		},
		{
			name:        "glob match under linked directory is covered",
			files:       []string{".claude/a.md"},
			entries:     []string{".claude/*.md", ".claude/"},
			wantCreated: []string{".claude"},
			wantSkipped: []string{".claude/a.md"},
		},
		{
			name:        "literal file",
			files:       []string{"x"},
			entries:     []string{"x"},
			wantCreated: []string{"x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			for _, f := range tt.files {
				p := filepath.Join(src, f)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			l := Linker{Tracked: func(rel string) bool { return tt.tracked[rel] }}
			res, err := l.Reconcile(tt.entries, Config{SourceDir: src, TargetDir: dst})
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(res.Created)
			if !reflect.DeepEqual(res.Created, tt.wantCreated) {
				t.Errorf("Created = %v, want %v", res.Created, tt.wantCreated)
			}
			var skipped []string
			for _, s := range res.Skipped {
				skipped = append(skipped, s.Path)
			}
			if !reflect.DeepEqual(skipped, tt.wantSkipped) {
				t.Errorf("Skipped = %v, want %v", skipped, tt.wantSkipped)
			}
		})
	}

	t.Run("files created in a linked directory appear in source", func(t *testing.T) {
		src, dst := t.TempDir(), t.TempDir()
		if err := os.Mkdir(filepath.Join(src, "d"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := (Linker{}).Reconcile([]string{"d/"}, Config{SourceDir: src, TargetDir: dst}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, "d", "new"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(src, "d", "new")); err != nil {
			t.Error(err)
		}
	})
}
