package exec

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestDetect_just(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "justfile", "build:\n  go build ./...\ntest:\n  go test ./...\n_private:\n  echo secret\n")

	r, err := Detect(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "just" {
		t.Errorf("Name = %q, want %q", r.Name, "just")
	}
	want := []string{"build", "test"}
	if !equalStrSlice(r.Scripts, want) {
		t.Errorf("Scripts = %v, want %v", r.Scripts, want)
	}
}

func TestDetect_justfileCapitalised(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Justfile", "lint:\n  golangci-lint run\n")

	r, err := Detect(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "just" {
		t.Errorf("Name = %q, want %q", r.Name, "just")
	}
}

func TestDetect_packageJSON_lockfileBun(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"scripts":{"dev":"vite","build":"vite build"}}`)
	writeFile(t, dir, "bun.lockb", "")

	r, err := Detect(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "bun" {
		t.Errorf("Name = %q, want %q", r.Name, "bun")
	}
	want := []string{"build", "dev"}
	if !equalStrSlice(r.Scripts, want) {
		t.Errorf("Scripts = %v, want %v", r.Scripts, want)
	}
}

func TestDetect_packageJSON_lockfilePnpm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"scripts":{"start":"node index.js"}}`)
	writeFile(t, dir, "pnpm-lock.yaml", "")

	r, err := Detect(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "pnpm" {
		t.Errorf("Name = %q, want %q", r.Name, "pnpm")
	}
}

func TestDetect_packageJSON_packageManagerField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"packageManager":"yarn@4.0.0","scripts":{"build":"tsc"}}`)

	r, err := Detect(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "yarn" {
		t.Errorf("Name = %q, want %q", r.Name, "yarn")
	}
}

func TestDetect_packageJSON_defaultNPM(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"scripts":{"test":"jest"}}`)

	r, err := Detect(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "npm" {
		t.Errorf("Name = %q, want %q", r.Name, "npm")
	}
}

func TestDetect_pyprojectPoetry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[tool.poetry]\nname = \"myapp\"\n\n[tool.poetry.scripts]\nmyapp = \"myapp.cli:main\"\n")

	r, err := Detect(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "poetry" {
		t.Errorf("Name = %q, want %q", r.Name, "poetry")
	}
	want := []string{"myapp"}
	if !equalStrSlice(r.Scripts, want) {
		t.Errorf("Scripts = %v, want %v", r.Scripts, want)
	}
}

func TestDetect_pyprojectUV(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[project]\nname = \"myapp\"\n\n[project.scripts]\nrun = \"myapp.main:main\"\n")

	r, err := Detect(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "uv" {
		t.Errorf("Name = %q, want %q", r.Name, "uv")
	}
}

func TestDetect_make(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", "build:\n\tgo build ./...\n")

	r, err := Detect(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "make" {
		t.Errorf("Name = %q, want %q", r.Name, "make")
	}
	if len(r.Scripts) != 0 {
		t.Errorf("make Scripts should be empty, got %v", r.Scripts)
	}
}

func TestDetect_ambiguous_error(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "justfile", "build:\n  go build\n")
	writeFile(t, dir, "package.json", `{"scripts":{"build":"tsc"}}`)

	_, err := Detect(dir, "")
	if err == nil {
		t.Fatal("expected error for ambiguous runners")
	}
}

func TestDetect_noneFound_error(t *testing.T) {
	dir := t.TempDir()

	_, err := Detect(dir, "")
	if err == nil {
		t.Fatal("expected error when no runners detected")
	}
}

func TestDetect_override(t *testing.T) {
	dir := t.TempDir()
	// Has package.json but we force just via override
	writeFile(t, dir, "package.json", `{"scripts":{"test":"jest"}}`)
	writeFile(t, dir, "justfile", "check:\n  go vet ./...\n")

	r, err := Detect(dir, "just")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "just" {
		t.Errorf("Name = %q, want %q", r.Name, "just")
	}
}

func TestListScripts_justRecipeWithArgs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "justfile", "build target='debug':\n  go build -tags {{target}}\ntest:\n  go test\n")

	scripts, err := ListScripts(dir, "just")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"build", "test"}
	if !equalStrSlice(scripts, want) {
		t.Errorf("Scripts = %v, want %v", scripts, want)
	}
}

func TestScriptCmd(t *testing.T) {
	cases := []struct {
		runner string
		want   []string
	}{
		{"just", []string{"just"}},
		{"npm", []string{"npm", "run"}},
		{"pnpm", []string{"pnpm", "run"}},
		{"yarn", []string{"yarn"}},
		{"bun", []string{"bun", "run"}},
		{"poetry", []string{"poetry", "run"}},
		{"uv", []string{"uv", "run"}},
		{"make", []string{"make"}},
	}
	for _, tc := range cases {
		got := scriptCmd(tc.runner)
		if !equalStrSlice(got, tc.want) {
			t.Errorf("scriptCmd(%q) = %v, want %v", tc.runner, got, tc.want)
		}
	}
}

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
