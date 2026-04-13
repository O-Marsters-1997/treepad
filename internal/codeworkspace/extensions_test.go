package codeworkspace

import (
	"slices"
	"strings"
	"testing"
)

func TestReadExtensions(t *testing.T) {
	t.Run("reads valid extensions.json", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/.vscode/extensions.json", `{"recommendations":["golang.go","ms-python.python"]}`)

		got, err := ReadExtensions(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0] != "golang.go" || got[1] != "ms-python.python" {
			t.Errorf("got %v, want [golang.go ms-python.python]", got)
		}
	})

	t.Run("no file returns nil nil", func(t *testing.T) {
		got, err := ReadExtensions(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/.vscode/extensions.json", `{invalid`)

		_, err := ReadExtensions(dir)
		if err == nil || !strings.Contains(err.Error(), "parse extensions.json") {
			t.Fatalf("got error %v, want error containing %q", err, "parse extensions.json")
		}
	})

	t.Run("unreadable file returns error", func(t *testing.T) {
		dir := t.TempDir()
		// Create as a directory so os.ReadFile fails with non-ErrNotExist error.
		writeFile(t, dir+"/.vscode/extensions.json/placeholder", "")

		_, err := ReadExtensions(dir)
		if err == nil || !strings.Contains(err.Error(), "read extensions.json") {
			t.Fatalf("got error %v, want error containing %q", err, "read extensions.json")
		}
	})
}

func TestDetectExtensions(t *testing.T) {
	t.Run("detects Go files", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/main.go", "package main")

		got, err := DetectExtensions(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Contains(got, "golang.go") {
			t.Errorf("got %v, want to contain golang.go", got)
		}
	})

	t.Run("detects multiple types", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/main.go", "package main")
		writeFile(t, dir+"/app.py", "print('hello')")

		got, err := DetectExtensions(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{"golang.go", "ms-python.python"} {
			if !slices.Contains(got, want) {
				t.Errorf("got %v, want to contain %s", got, want)
			}
		}
	})

	t.Run("detects by filename", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/Dockerfile", "FROM ubuntu")

		got, err := DetectExtensions(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Contains(got, "ms-azuretools.vscode-docker") {
			t.Errorf("got %v, want to contain ms-azuretools.vscode-docker", got)
		}
	})

	t.Run("deduplicates extensions for ts and tsx", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/a.ts", "")
		writeFile(t, dir+"/b.tsx", "")

		got, err := DetectExtensions(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count := 0
		for _, g := range got {
			if g == "ms-vscode.vscode-typescript-next" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("ms-vscode.vscode-typescript-next appeared %d times, want 1", count)
		}
	})

	t.Run("skips hidden directories", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/.hidden/main.go", "package main")

		got, err := DetectExtensions(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if slices.Contains(got, "golang.go") {
			t.Errorf("got %v, should not contain golang.go from hidden dir", got)
		}
	})

	t.Run("skips node_modules", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/node_modules/pkg/index.js", "module.exports = {}")

		got, err := DetectExtensions(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if slices.Contains(got, "ms-vscode.vscode-typescript-next") {
			t.Errorf("got %v, should not contain JS extension from node_modules", got)
		}
	})

	t.Run("empty directory returns empty slice", func(t *testing.T) {
		got, err := DetectExtensions(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})
}

func TestResolveExtensions(t *testing.T) {
	t.Run("prefers extensions.json over detection", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/.vscode/extensions.json", `{"recommendations":["custom.ext"]}`)
		writeFile(t, dir+"/main.go", "package main")

		got, err := ResolveExtensions(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0] != "custom.ext" {
			t.Errorf("got %v, want [custom.ext]", got)
		}
	})

	t.Run("falls back to detection when no extensions.json", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/main.go", "package main")

		got, err := ResolveExtensions(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Contains(got, "golang.go") {
			t.Errorf("got %v, want to contain golang.go", got)
		}
	})

	t.Run("propagates read error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir+"/.vscode/extensions.json/placeholder", "")

		_, err := ResolveExtensions(dir)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
