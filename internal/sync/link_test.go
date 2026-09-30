package sync

import (
	"os"
	"path/filepath"
	"reflect"
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
