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
			wantSkip: "differs from source; use --force to back up and link",
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

func TestLinkerUnlink(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	for _, f := range []string{"owned", "regular", "foreign"} {
		if err := os.WriteFile(filepath.Join(src, f), []byte("main"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(src, "owned"), filepath.Join(dst, "owned")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "regular"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("elsewhere", filepath.Join(dst, "foreign")); err != nil {
		t.Fatal(err)
	}

	entries := []string{"owned", "regular", "foreign", "absent"}
	removed, err := Linker{}.Unlink(entries, Config{SourceDir: src, TargetDir: dst})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(removed, []string{"owned"}) {
		t.Errorf("removed = %v, want [owned]", removed)
	}
	if _, err := os.Lstat(filepath.Join(dst, "owned")); err == nil {
		t.Error("owned link should be gone")
	}
	for _, f := range []string{"regular", "foreign"} {
		if _, err := os.Lstat(filepath.Join(dst, f)); err != nil {
			t.Errorf("%s should be untouched: %v", f, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(src, "owned")); string(b) != "main" {
		t.Errorf("source content = %q, want main", b)
	}
}

func TestLinkerInspect(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, src, dst string)
		ignored  bool
		wantKind string
	}{
		{
			name: "healthy link",
			setup: func(t *testing.T, src, dst string) {
				mustLink(t, src, dst, "f")
			},
			ignored: true,
		},
		{
			name: "broken link when source removed",
			setup: func(t *testing.T, src, dst string) {
				mustLink(t, src, dst, "f")
				_ = os.Remove(filepath.Join(src, "f"))
			},
			ignored:  true,
			wantKind: "broken",
		},
		{
			name: "foreign symlink is replaced",
			setup: func(t *testing.T, src, dst string) {
				mustWrite(t, filepath.Join(src, "f"))
				if err := os.Symlink("elsewhere", filepath.Join(dst, "f")); err != nil {
					t.Fatal(err)
				}
			},
			ignored:  true,
			wantKind: "replaced",
		},
		{
			name: "replaced by regular file",
			setup: func(t *testing.T, src, dst string) {
				mustLink(t, src, dst, "f")
				_ = os.Remove(filepath.Join(dst, "f"))
				if err := os.WriteFile(filepath.Join(dst, "f"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			ignored:  true,
			wantKind: "replaced",
		},
		{
			name:     "unignored link",
			setup:    func(t *testing.T, src, dst string) { mustLink(t, src, dst, "f") },
			ignored:  false,
			wantKind: "unignored",
		},
		{
			name:  "not yet linked is not an issue",
			setup: func(t *testing.T, src, _ string) { mustWrite(t, filepath.Join(src, "f")) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			tt.setup(t, src, dst)
			l := Linker{Ignored: func(string) bool { return tt.ignored }}
			issues := l.Inspect([]string{"f"}, Config{SourceDir: src, TargetDir: dst})
			if tt.wantKind == "" {
				if len(issues) != 0 {
					t.Fatalf("issues = %v, want none", issues)
				}
				return
			}
			if len(issues) != 1 || issues[0].Kind != tt.wantKind {
				t.Fatalf("issues = %v, want one %q", issues, tt.wantKind)
			}
		})
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustLink(t *testing.T, src, dst, rel string) {
	t.Helper()
	mustWrite(t, filepath.Join(src, rel))
	if err := os.Symlink(filepath.Join(src, rel), filepath.Join(dst, rel)); err != nil {
		t.Fatal(err)
	}
}

func TestLinkerReconcileUnignored(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(src, "f"))
	l := Linker{Ignored: func(string) bool { return false }}
	res, err := l.Reconcile([]string{"f"}, Config{SourceDir: src, TargetDir: dst})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Unignored, []string{"f"}) {
		t.Errorf("Unignored = %v, want [f]", res.Unignored)
	}
}

func TestLinkerTransitions(t *testing.T) {
	write := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	isLink := func(path string) bool {
		fi, err := os.Lstat(path)
		return err == nil && fi.Mode()&os.ModeSymlink != 0
	}

	t.Run("identical copy becomes a link", func(t *testing.T) {
		src, dst := t.TempDir(), t.TempDir()
		write(t, filepath.Join(src, "f"), "x")
		write(t, filepath.Join(dst, "f"), "x")
		res, err := (Linker{}).Reconcile([]string{"f"}, Config{SourceDir: src, TargetDir: dst})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(res.Replaced, []string{"f"}) || !isLink(filepath.Join(dst, "f")) {
			t.Errorf("Replaced = %v, link = %v", res.Replaced, isLink(filepath.Join(dst, "f")))
		}
	})

	t.Run("directory copy is refused without force", func(t *testing.T) {
		src, dst := t.TempDir(), t.TempDir()
		write(t, filepath.Join(src, "d/a"), "x")
		write(t, filepath.Join(dst, "d/a"), "x")
		res, err := (Linker{}).Reconcile([]string{"d/"}, Config{SourceDir: src, TargetDir: dst})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Skipped) != 1 || isLink(filepath.Join(dst, "d")) {
			t.Errorf("Skipped = %v, want one skip and no link", res.Skipped)
		}
	})

	t.Run("force backs up a divergent file", func(t *testing.T) {
		src, dst := t.TempDir(), t.TempDir()
		write(t, filepath.Join(src, "f"), "main")
		write(t, filepath.Join(dst, "f"), "local")
		res, err := (Linker{Force: true}).Reconcile([]string{"f"}, Config{SourceDir: src, TargetDir: dst})
		if err != nil {
			t.Fatal(err)
		}
		bak, _ := os.ReadFile(filepath.Join(dst, "f.treepad-bak"))
		if string(bak) != "local" || !isLink(filepath.Join(dst, "f")) || !reflect.DeepEqual(res.Replaced, []string{"f"}) {
			t.Errorf("bak = %q, link = %v, Replaced = %v", bak, isLink(filepath.Join(dst, "f")), res.Replaced)
		}
	})

	t.Run("force backs up a directory", func(t *testing.T) {
		src, dst := t.TempDir(), t.TempDir()
		write(t, filepath.Join(src, "d/a"), "x")
		write(t, filepath.Join(dst, "d/a"), "x")
		if _, err := (Linker{Force: true}).Reconcile([]string{"d/"}, Config{SourceDir: src, TargetDir: dst}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dst, "d.treepad-bak", "a")); err != nil || !isLink(filepath.Join(dst, "d")) {
			t.Errorf("backup missing (%v) or not linked", err)
		}
	})

	t.Run("owned link no longer wanted is removed, foreign link kept", func(t *testing.T) {
		src, dst := t.TempDir(), t.TempDir()
		write(t, filepath.Join(src, "owned"), "x")
		write(t, filepath.Join(src, "sub/owned2"), "x")
		for _, p := range []string{"owned", "sub/owned2"} {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dst, p)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(src, p), filepath.Join(dst, p)); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Symlink("/elsewhere", filepath.Join(dst, "foreign")); err != nil {
			t.Fatal(err)
		}
		res, err := (Linker{}).Reconcile(nil, Config{SourceDir: src, TargetDir: dst})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(res.Removed)
		if !reflect.DeepEqual(res.Removed, []string{"owned", "sub/owned2"}) || !isLink(filepath.Join(dst, "foreign")) {
			t.Errorf("Removed = %v, foreign kept = %v", res.Removed, isLink(filepath.Join(dst, "foreign")))
		}
	})

	t.Run("second run changes nothing", func(t *testing.T) {
		src, dst := t.TempDir(), t.TempDir()
		write(t, filepath.Join(src, "a"), "x")
		write(t, filepath.Join(dst, "a"), "x")
		cfg := Config{SourceDir: src, TargetDir: dst}
		if _, err := (Linker{}).Reconcile([]string{"a"}, cfg); err != nil {
			t.Fatal(err)
		}
		res, err := (Linker{}).Reconcile([]string{"a"}, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Created)+len(res.Replaced)+len(res.Removed)+len(res.Skipped) != 0 {
			t.Errorf("second run = %+v, want no changes", res)
		}
	})
}
