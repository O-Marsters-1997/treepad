package exec

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// --- Tier 1: pure parser tests ---

func TestParseJustRecipes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "basic recipes",
			input: "build:\n\tgo build ./...\ntest:\n\tgo test ./...\n",
			want:  []string{"build", "test"},
		},
		{
			name:  "private recipe excluded",
			input: "build:\n\tgo build\n_hidden:\n\techo secret\n",
			want:  []string{"build"},
		},
		{
			name:  "variable assignment with space excluded",
			input: "X := 1\nbuild:\n\tgo build\n",
			want:  []string{"build"},
		},
		{
			name:  "variable assignment without space excluded",
			input: "x:=value\nbuild:\n\tgo build\n",
			want:  []string{"build"},
		},
		{
			name:  "recipe with default args",
			input: "build target='debug':\n\tgo build -tags {{target}}\ntest:\n\tgo test\n",
			want:  []string{"build", "test"},
		},
		{
			name:  "sorted output",
			input: "z:\n\tz\na:\n\ta\nm:\n\tm\n",
			want:  []string{"a", "m", "z"},
		},
		{
			name:  "empty file",
			input: "",
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseJustRecipes([]byte(tt.input))
			if !equalStrSlice(got, tt.want) {
				t.Errorf("parseJustRecipes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePackageScripts(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "basic scripts sorted",
			input: `{"scripts":{"build":"tsc","test":"jest"}}`,
			want:  []string{"build", "test"},
		},
		{
			name:  "empty scripts map",
			input: `{"scripts":{}}`,
			want:  []string{},
		},
		{
			name:  "no scripts field",
			input: `{}`,
			want:  []string{},
		},
		{
			name:    "invalid JSON",
			input:   `{broken`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePackageScripts([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalStrSlice(got, tt.want) {
				t.Errorf("parsePackageScripts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePyprojectScripts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "poetry scripts",
			input: "[tool.poetry]\nname = \"myapp\"\n\n[tool.poetry.scripts]\nmyapp = \"myapp.cli:main\"\n",
			want:  []string{"myapp"},
		},
		{
			name:  "project scripts",
			input: "[project]\nname = \"myapp\"\n\n[project.scripts]\nrun = \"myapp.main:main\"\n",
			want:  []string{"run"},
		},
		{
			name: "project scripts take precedence over poetry scripts",
			input: "[project]\nname = \"x\"\n\n[project.scripts]\nserve = \"x:serve\"\n\n" +
				"[tool.poetry.scripts]\nbuild = \"x:build\"\n",
			want: []string{"serve"},
		},
		{
			name:  "empty file",
			input: "",
			want:  []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePyprojectScripts([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalStrSlice(got, tt.want) {
				t.Errorf("parsePyprojectScripts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePackageManagerField(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"pnpm with version", `{"packageManager":"pnpm@9.0.0"}`, "pnpm"},
		{"yarn with version", `{"packageManager":"yarn@4.0.0"}`, "yarn"},
		{"bun with version", `{"packageManager":"bun@1.0.0"}`, "bun"},
		{"npm with version", `{"packageManager":"npm@10.0.0"}`, "npm"},
		{"unknown manager defaults to npm", `{"packageManager":"deno@1.0.0"}`, "npm"},
		{"empty field defaults to npm", `{"packageManager":""}`, "npm"},
		{"no field defaults to npm", `{}`, "npm"},
		{"invalid JSON defaults to npm", `{broken`, "npm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePackageManagerField([]byte(tt.input))
			if got != tt.want {
				t.Errorf("parsePackageManagerField() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Tier 2: detection tests via fstest.MapFS ---

func TestPickRunner(t *testing.T) {
	tests := []struct {
		name     string
		files    fstest.MapFS
		override string
		want     string
		wantErr  bool
	}{
		{
			name:  "justfile",
			files: fstest.MapFS{"justfile": {}},
			want:  "just",
		},
		{
			name:  "Justfile capitalised",
			files: fstest.MapFS{"Justfile": {}},
			want:  "just",
		},
		{
			name:  "package.json defaults to npm",
			files: fstest.MapFS{"package.json": {Data: []byte(`{}`)}},
			want:  "npm",
		},
		{
			name:  "Makefile",
			files: fstest.MapFS{"Makefile": {}},
			want:  "make",
		},
		{
			name:  "pyproject.toml uv",
			files: fstest.MapFS{"pyproject.toml": {Data: []byte("[project]\nname = \"x\"\n")}},
			want:  "uv",
		},
		{
			name:  "pyproject.toml poetry",
			files: fstest.MapFS{"pyproject.toml": {Data: []byte("[tool.poetry]\nname = \"x\"\n")}},
			want:  "poetry",
		},
		{
			name: "ambiguous runners error",
			files: fstest.MapFS{
				"justfile":     {},
				"package.json": {Data: []byte(`{}`)},
			},
			wantErr: true,
		},
		{
			name:    "no runner error",
			files:   fstest.MapFS{},
			wantErr: true,
		},
		{
			name: "override bypasses ambiguity",
			files: fstest.MapFS{
				"justfile":     {},
				"package.json": {Data: []byte(`{}`)},
			},
			override: "just",
			want:     "just",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pickRunner(tt.files, tt.override)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("pickRunner() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectJSManager(t *testing.T) {
	tests := []struct {
		name  string
		files fstest.MapFS
		want  string
	}{
		{
			name:  "bun lockfile",
			files: fstest.MapFS{"bun.lockb": {}, "package.json": {}},
			want:  "bun",
		},
		{
			name:  "pnpm lockfile",
			files: fstest.MapFS{"pnpm-lock.yaml": {}, "package.json": {}},
			want:  "pnpm",
		},
		{
			name:  "yarn lockfile",
			files: fstest.MapFS{"yarn.lock": {}, "package.json": {}},
			want:  "yarn",
		},
		{
			name:  "package-lock.json",
			files: fstest.MapFS{"package-lock.json": {}, "package.json": {}},
			want:  "npm",
		},
		{
			name:  "packageManager field",
			files: fstest.MapFS{"package.json": {Data: []byte(`{"packageManager":"pnpm@9"}`)}},
			want:  "pnpm",
		},
		{
			name:  "lockfile takes precedence over packageManager field",
			files: fstest.MapFS{"yarn.lock": {}, "package.json": {Data: []byte(`{"packageManager":"pnpm@9"}`)}},
			want:  "yarn",
		},
		{
			name:  "no lockfile no field defaults to npm",
			files: fstest.MapFS{"package.json": {Data: []byte(`{}`)}},
			want:  "npm",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectJSManager(tt.files)
			if got != tt.want {
				t.Errorf("detectJSManager() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Tier 3: Resolve integration tests (pins os.DirFS wiring) ---

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		override    string
		wantName    string
		wantScripts []string
		wantErr     bool
	}{
		{
			name:        "just",
			files:       map[string]string{"justfile": "build:\n  go build ./...\ntest:\n  go test ./...\n"},
			wantName:    "just",
			wantScripts: []string{"build", "test"},
		},
		{
			name:        "npm",
			files:       map[string]string{"package.json": `{"scripts":{"build":"tsc","start":"node index.js"}}`},
			wantName:    "npm",
			wantScripts: []string{"build", "start"},
		},
		{
			name:     "make",
			files:    map[string]string{"Makefile": "build:\n\tgo build\n"},
			wantName: "make",
		},
		{
			name: "poetry",
			files: map[string]string{
				"pyproject.toml": "[tool.poetry]\nname = \"x\"\n\n[tool.poetry.scripts]\nmyapp = \"myapp.cli:main\"\n",
			},
			wantName:    "poetry",
			wantScripts: []string{"myapp"},
		},
		{
			name:     "override selects runner despite ambiguity",
			files:    map[string]string{"justfile": "build:\n  go build\n", "package.json": `{"scripts":{}}`},
			override: "just",
			wantName: "just",
		},
		{
			name:    "no runner error",
			files:   map[string]string{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				writeFile(t, dir, name, content)
			}
			r, err := Resolve(dir, tt.override)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if r.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", r.Name, tt.wantName)
			}
			if tt.wantScripts != nil && !equalStrSlice(r.Scripts, tt.wantScripts) {
				t.Errorf("Scripts = %v, want %v", r.Scripts, tt.wantScripts)
			}
		})
	}
}

// --- Shared helpers ---

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
