package sync

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
			name: "copies matching file",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, ".env"), "SECRET=123")
			},
			patterns: []string{".env"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if got := readFile(t, filepath.Join(dst, ".env")); got != "SECRET=123" {
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
			name: "trailing slash pattern copies entire directory",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, ".claude", "settings.local.json"), `{"key":"val"}`)
				writeFile(t, filepath.Join(src, ".claude", "agents", "foo.md"), "# Foo")
				writeFile(t, filepath.Join(src, "other.txt"), "ignore me")
			},
			patterns: []string{".claude/"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if readFile(t, filepath.Join(dst, ".claude", "settings.local.json")) != `{"key":"val"}` {
					t.Error("settings.local.json content mismatch")
				}
				if readFile(t, filepath.Join(dst, ".claude", "agents", "foo.md")) != "# Foo" {
					t.Error("agents/foo.md content mismatch")
				}
				if _, err := os.Stat(filepath.Join(dst, "other.txt")); !os.IsNotExist(err) {
					t.Error("other.txt should not be synced")
				}
			},
		},
		{
			name: "double-star matches recursively",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, "docs", "a.md"), "a")
				writeFile(t, filepath.Join(src, "docs", "sub", "b.md"), "b")
				writeFile(t, filepath.Join(src, "docs", "sub", "c.txt"), "c")
			},
			patterns: []string{"docs/**/*.md"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if readFile(t, filepath.Join(dst, "docs", "a.md")) != "a" {
					t.Error("a.md content mismatch")
				}
				if readFile(t, filepath.Join(dst, "docs", "sub", "b.md")) != "b" {
					t.Error("sub/b.md content mismatch")
				}
				if _, err := os.Stat(filepath.Join(dst, "docs", "sub", "c.txt")); !os.IsNotExist(err) {
					t.Error("c.txt should not be synced")
				}
			},
		},
		{
			name: "negation pattern excludes file from matched directory",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, ".claude", "settings.local.json"), "ok")
				writeFile(t, filepath.Join(src, ".claude", "secret.md"), "secret")
			},
			patterns: []string{".claude/", "!.claude/secret.md"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if readFile(t, filepath.Join(dst, ".claude", "settings.local.json")) != "ok" {
					t.Error("settings.local.json should be synced")
				}
				if _, err := os.Stat(filepath.Join(dst, ".claude", "secret.md")); !os.IsNotExist(err) {
					t.Error("secret.md should be excluded by ! pattern")
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
			name:     "invalid pattern returns error",
			setup:    func(src string) {},
			patterns: []string{"[invalid"},
			wantErr:  "invalid pattern",
		},
		{
			name: "symlink to directory is recreated as symlink",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, ".claude", "settings.local.json"), `{"key":"val"}`)
				external := t.TempDir()
				writeFile(t, filepath.Join(external, "agent.md"), "# Agent")
				if err := os.Symlink(external, filepath.Join(src, ".claude", "agents")); err != nil {
					t.Fatal(err)
				}
			},
			patterns: []string{".claude/"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if readFile(t, filepath.Join(dst, ".claude", "settings.local.json")) != `{"key":"val"}` {
					t.Error("settings.local.json content mismatch")
				}
				info, err := os.Lstat(filepath.Join(dst, ".claude", "agents"))
				if err != nil {
					t.Fatalf("lstat agents: %v", err)
				}
				if info.Mode()&fs.ModeSymlink == 0 {
					t.Error("agents should be a symlink, got regular entry")
				}
				srcTarget, _ := os.Readlink(filepath.Join(src, ".claude", "agents"))
				dstTarget, _ := os.Readlink(filepath.Join(dst, ".claude", "agents"))
				if srcTarget != dstTarget {
					t.Errorf("symlink target mismatch: src=%q dst=%q", srcTarget, dstTarget)
				}
			},
		},
		{
			name: "symlink to file is recreated as symlink",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, "real.txt"), "content")
				if err := os.Symlink(filepath.Join(src, "real.txt"), filepath.Join(src, "link.txt")); err != nil {
					t.Fatal(err)
				}
			},
			patterns: []string{"*.txt"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if readFile(t, filepath.Join(dst, "real.txt")) != "content" {
					t.Error("real.txt content mismatch")
				}
				info, err := os.Lstat(filepath.Join(dst, "link.txt"))
				if err != nil {
					t.Fatalf("lstat link.txt: %v", err)
				}
				if info.Mode()&fs.ModeSymlink == 0 {
					t.Error("link.txt should be a symlink, got regular entry")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			dst := t.TempDir()
			tt.setup(src)

			_, err := FileSyncer{}.Sync(tt.patterns, Config{SourceDir: src, TargetDir: dst})

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

func TestFileSyncerSyncFastPath(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(src string)
		patterns []string
		check    func(t *testing.T, src, dst string)
	}{
		{
			name: "whole-dir clone includes all contents",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, "node_modules", "pkg", "index.js"), "module.exports={}")
				writeFile(t, filepath.Join(src, "node_modules", "pkg", "package.json"), `{"name":"pkg"}`)
				writeFile(t, filepath.Join(src, "src", "app.ts"), "unrelated")
			},
			patterns: []string{"node_modules/"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if readFile(t, filepath.Join(dst, "node_modules", "pkg", "index.js")) != "module.exports={}" {
					t.Error("index.js content mismatch")
				}
				if readFile(t, filepath.Join(dst, "node_modules", "pkg", "package.json")) != `{"name":"pkg"}` {
					t.Error("package.json content mismatch")
				}
				if _, err := os.Stat(filepath.Join(dst, "src")); !os.IsNotExist(err) {
					t.Error("src/ should not be synced")
				}
			},
		},
		{
			name: "whole-dir clone skips when exclude intersects",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, ".claude", "settings.json"), "settings")
				writeFile(t, filepath.Join(src, ".claude", "secret.md"), "secret")
			},
			patterns: []string{".claude/", "!.claude/secret.md"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if readFile(t, filepath.Join(dst, ".claude", "settings.json")) != "settings" {
					t.Error("settings.json should be synced")
				}
				if _, err := os.Stat(filepath.Join(dst, ".claude", "secret.md")); !os.IsNotExist(err) {
					t.Error("secret.md should be excluded")
				}
			},
		},
		{
			name: "early prune skips unmatched directories",
			setup: func(src string) {
				writeFile(t, filepath.Join(src, ".env"), "KEY=val")
				writeFile(t, filepath.Join(src, "src", "app.ts"), "irrelevant")
				writeFile(t, filepath.Join(src, "node_modules", "pkg", "index.js"), "irrelevant")
			},
			patterns: []string{".env"},
			check: func(t *testing.T, src, dst string) {
				t.Helper()
				if readFile(t, filepath.Join(dst, ".env")) != "KEY=val" {
					t.Error(".env content mismatch")
				}
				if _, err := os.Stat(filepath.Join(dst, "src")); !os.IsNotExist(err) {
					t.Error("src/ should not be synced")
				}
				if _, err := os.Stat(filepath.Join(dst, "node_modules")); !os.IsNotExist(err) {
					t.Error("node_modules/ should not be synced")
				}
			},
		},
		{
			name:     "whole-dir clone with missing source is a no-op",
			setup:    func(src string) {},
			patterns: []string{"node_modules/"},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			dst := t.TempDir()
			tt.setup(src)

			_, err := FileSyncer{}.Sync(tt.patterns, Config{SourceDir: src, TargetDir: dst})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, src, dst)
		})
	}
}

// TestFileSyncerSyncBudget asserts that syncing a node_modules-sized directory
// tree completes within a usable time budget. On APFS (Darwin) the fast-clone
// path makes this near-instant; on Linux the budget covers kernel-copy overhead.
func TestFileSyncerSyncBudget(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("fast-clone path (clonefile) is Darwin/APFS only")
	}
	const (
		pkgCount    = 20
		filesPerPkg = 1000
		budget      = 5 * time.Second
	)

	src := t.TempDir()
	dst := t.TempDir()

	for i := range pkgCount {
		for j := range filesPerPkg {
			path := filepath.Join(src, "node_modules", fmt.Sprintf("pkg%d", i), fmt.Sprintf("file%d.js", j))
			writeFile(t, path, "module.exports={}")
		}
	}

	start := time.Now()
	res, err := FileSyncer{}.Sync([]string{"node_modules/"}, Config{SourceDir: src, TargetDir: dst})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("synced %d files in %v", res.Files, elapsed)
	if elapsed > budget {
		t.Errorf("sync took %v, want < %v — possible regression to slow copy path", elapsed, budget)
	}
}

func TestDirCouldMatch(t *testing.T) {
	tests := []struct {
		dir      string
		includes []string
		want     bool
	}{
		{"node_modules", []string{".claude/"}, false},
		{".claude", []string{".claude/"}, true},
		{".claude/agents", []string{".claude/"}, true},
		{".claude/agents/sub", []string{".claude/"}, true},
		{"node_modules/pkg", []string{"node_modules/"}, true},
		{".vscode", []string{".vscode/settings.json"}, true},
		{".vscode", []string{".vscode/*.json"}, true},
		{".vscode", []string{".env"}, false},
		{"docs", []string{"docs/**/*.md"}, true},
		{"src", []string{"docs/**/*.md"}, false},
		{"docs/api", []string{"docs/**/*.md"}, true},
		{".claude", []string{}, false},
	}
	for _, tt := range tests {
		if got := dirCouldMatch(tt.dir, tt.includes); got != tt.want {
			t.Errorf("dirCouldMatch(%q, %v) = %v, want %v", tt.dir, tt.includes, got, tt.want)
		}
	}
}

func TestWholeDirPattern(t *testing.T) {
	tests := []struct {
		pattern string
		wantDir string
		wantOK  bool
	}{
		{"node_modules/", "node_modules", true},
		{".claude/", ".claude", true},
		{".claude/**", ".claude", true},
		{".vscode/settings.json", "", false},
		{".vscode/*.json", "", false},
		{"docs/**/*.md", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		dir, ok := wholeDirPattern(tt.pattern)
		if dir != tt.wantDir || ok != tt.wantOK {
			t.Errorf("wholeDirPattern(%q) = (%q, %v), want (%q, %v)", tt.pattern, dir, ok, tt.wantDir, tt.wantOK)
		}
	}
}

func TestClonePassDoesNotEnumerateClonedTree(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS clonefile required for fast-clone path")
	}
	src := t.TempDir()
	dst := t.TempDir()
	sub := filepath.Join(src, "node_modules")
	const leafCount = 5000
	for i := 0; i < leafCount; i++ {
		writeFile(t, filepath.Join(sub, fmt.Sprintf("pkg%d", i), "index.js"), "x")
	}

	stageDur := map[string]time.Duration{}
	res, err := FileSyncer{}.Sync([]string{"node_modules/"}, Config{
		SourceDir: src,
		TargetDir: dst,
		Stage: func(name string) func() {
			t0 := time.Now()
			return func() { stageDur[name] = time.Since(t0) }
		},
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Primary deterministic check: clone-pass must not enumerate every leaf.
	if res.Files >= leafCount {
		t.Errorf("SyncResult.Files = %d (>= leaf count %d): clone_pass is walking the source tree",
			res.Files, leafCount)
	}
	// Sanity: the clone actually happened.
	if _, err := os.Stat(filepath.Join(dst, "node_modules", "pkg0", "index.js")); err != nil {
		t.Errorf("expected cloned file present: %v", err)
	}
	// Soft perf guard: generous threshold relative to expected <10 ms.
	if d := stageDur["sync.clone_pass"]; d > 500*time.Millisecond {
		t.Errorf("sync.clone_pass = %v, expected < 500ms (stat-walk likely reintroduced)", d)
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
