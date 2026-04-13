package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestFileSyncerSync(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(src string)
		patterns []string
		check    func(t *testing.T, src, dst string)
		wantErr  string
	}{
		{
			name: "copies matching files",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, ".env"), "SECRET=123")
			},
			patterns: []string{".env"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				got := readFile(t, filepath.Join(dst, ".env"))
				if got != "SECRET=123" {
					t.Errorf("content = %q, want %q", got, "SECRET=123")
				}
			},
		},
		{
			name: "glob pattern matches multiple files",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, ".vscode", "a.json"), `{"a":1}`)
				writeFile(t, filepath.Join(src, ".vscode", "b.json"), `{"b":2}`)
			},
			patterns: []string{".vscode/*.json"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if readFile(t, filepath.Join(dst, ".vscode", "a.json")) != `{"a":1}` {
					t.Error("a.json content mismatch")
				}
				if readFile(t, filepath.Join(dst, ".vscode", "b.json")) != `{"b":2}` {
					t.Error("b.json content mismatch")
				}
			},
		},
		{
			name:     "non-matching pattern skips silently",
			setup:    func(src string) {},
			patterns: []string{"*.nonexistent"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				entries, err := os.ReadDir(dst)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 0 {
					t.Errorf("dst has %d entries, want 0", len(entries))
				}
			},
		},
		{
			name: "creates nested directories",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, "a", "b", "c", "deep.txt"), "deep")
			},
			patterns: []string{"a/b/c/deep.txt"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if readFile(t, filepath.Join(dst, "a", "b", "c", "deep.txt")) != "deep" {
					t.Error("deep.txt content mismatch")
				}
			},
		},
		{
			name:     "invalid glob returns error",
			setup:    func(src string) {},
			patterns: []string{"[invalid"},
			wantErr:  "invalid pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			dst := t.TempDir()
			tt.setup(src)

			err := FileSyncer{}.Sync(tt.patterns, Config{SourceDir: src, TargetDir: dst})

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, src, dst)
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	t.Run("copies content and creates parent dirs", func(t *testing.T) {
		src := t.TempDir()
		dst := t.TempDir()

		srcFile := filepath.Join(src, "file.txt")
		writeFile(t, srcFile, "hello")

		dstFile := filepath.Join(dst, "nested", "dir", "file.txt")
		if err := copyFile(srcFile, dstFile); err != nil {
			t.Fatalf("copyFile error: %v", err)
		}
		if readFile(t, dstFile) != "hello" {
			t.Error("content mismatch")
		}
	})

	t.Run("source not found returns error", func(t *testing.T) {
		err := copyFile("/nonexistent/path/file.txt", t.TempDir()+"/out.txt")
		if err == nil || !strings.Contains(err.Error(), "open source") {
			t.Fatalf("got error %v, want error containing %q", err, "open source")
		}
	})
}
