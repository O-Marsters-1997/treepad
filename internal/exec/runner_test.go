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

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		override string
		wantName string
		wantErr  bool
		// wantScripts, if non-nil, asserts Scripts exactly.
		wantScripts []string
		// wantNoScripts asserts Scripts is empty (for make).
		wantNoScripts bool
	}{
		{
			name: "just",
			files: map[string]string{
				"justfile": "build:\n  go build ./...\ntest:\n  go test ./...\n_private:\n  echo secret\n",
			},
			wantName:    "just",
			wantScripts: []string{"build", "test"},
		},
		{
			name: "justfile capitalised",
			files: map[string]string{
				"Justfile": "lint:\n  golangci-lint run\n",
			},
			wantName: "just",
		},
		{
			name: "package.json with bun lockfile",
			files: map[string]string{
				"package.json": `{"scripts":{"dev":"vite","build":"vite build"}}`,
				"bun.lockb":    "",
			},
			wantName:    "bun",
			wantScripts: []string{"build", "dev"},
		},
		{
			name: "package.json with pnpm lockfile",
			files: map[string]string{
				"package.json":   `{"scripts":{"start":"node index.js"}}`,
				"pnpm-lock.yaml": "",
			},
			wantName: "pnpm",
		},
		{
			name: "package.json with packageManager field",
			files: map[string]string{
				"package.json": `{"packageManager":"yarn@4.0.0","scripts":{"build":"tsc"}}`,
			},
			wantName: "yarn",
		},
		{
			name: "package.json defaults to npm",
			files: map[string]string{
				"package.json": `{"scripts":{"test":"jest"}}`,
			},
			wantName: "npm",
		},
		{
			name: "pyproject.toml poetry",
			files: map[string]string{
				"pyproject.toml": "[tool.poetry]\nname = \"myapp\"\n\n[tool.poetry.scripts]\nmyapp = \"myapp.cli:main\"\n",
			},
			wantName:    "poetry",
			wantScripts: []string{"myapp"},
		},
		{
			name: "pyproject.toml uv",
			files: map[string]string{
				"pyproject.toml": "[project]\nname = \"myapp\"\n\n[project.scripts]\nrun = \"myapp.main:main\"\n",
			},
			wantName: "uv",
		},
		{
			name: "make",
			files: map[string]string{
				"Makefile": "build:\n\tgo build ./...\n",
			},
			wantName:      "make",
			wantNoScripts: true,
		},
		{
			name: "ambiguous runners error",
			files: map[string]string{
				"justfile":     "build:\n  go build\n",
				"package.json": `{"scripts":{"build":"tsc"}}`,
			},
			wantErr: true,
		},
		{
			name:    "no runner found error",
			files:   map[string]string{},
			wantErr: true,
		},
		{
			name: "override selects runner despite ambiguity",
			files: map[string]string{
				"package.json": `{"scripts":{"test":"jest"}}`,
				"justfile":     "check:\n  go vet ./...\n",
			},
			override: "just",
			wantName: "just",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				writeFile(t, dir, name, content)
			}

			r, err := Detect(dir, tt.override)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", r.Name, tt.wantName)
			}
			if tt.wantScripts != nil && !equalStrSlice(r.Scripts, tt.wantScripts) {
				t.Errorf("Scripts = %v, want %v", r.Scripts, tt.wantScripts)
			}
			if tt.wantNoScripts && len(r.Scripts) != 0 {
				t.Errorf("Scripts should be empty, got %v", r.Scripts)
			}
		})
	}
}

func TestListScripts_justRecipeWithArgs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "justfile", "build target='debug':\n  go build -tags {{target}}\ntest:\n  go test\n")

	scripts, err := listScripts(dir, "just")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"build", "test"}
	if !equalStrSlice(scripts, want) {
		t.Errorf("Scripts = %v, want %v", scripts, want)
	}
}

func TestScriptCmd(t *testing.T) {
	tests := []struct {
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
	for _, tt := range tests {
		t.Run(tt.runner, func(t *testing.T) {
			got := scriptCmd(tt.runner)
			if !equalStrSlice(got, tt.want) {
				t.Errorf("scriptCmd(%q) = %v, want %v", tt.runner, got, tt.want)
			}
		})
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
