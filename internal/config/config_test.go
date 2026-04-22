package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

func TestLoad(t *testing.T) {
	t.Run("no config file returns defaults", func(t *testing.T) {
		cfg, err := Load(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(cfg.Sync.Include, defaultSyncInclude()) {
			t.Errorf("Sync.Include = %v, want defaults", cfg.Sync.Include)
		}
		if cfg.Artifact.IsZero() {
			t.Error("default Artifact should not be zero")
		}
		if cfg.Open.IsZero() {
			t.Error("default Open should not be zero")
		}
	})

	t.Run("valid config with custom sync include", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.toml"), `
[sync]
include = ["custom.txt"]
`)
		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(cfg.Sync.Include, []string{"custom.txt"}) {
			t.Errorf("Sync.Include = %v, want [custom.txt]", cfg.Sync.Include)
		}
	})

	t.Run("empty include array falls back to defaults", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.toml"), `
[sync]
include = []
`)
		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(cfg.Sync.Include, defaultSyncInclude()) {
			t.Errorf("Sync.Include = %v, want defaults", cfg.Sync.Include)
		}
	})

	t.Run("custom artifact section is loaded", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.toml"), `
[artifact]
filename = "{{.Slug}}.sublime-project"
content = "{}"
`)
		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Artifact.FilenameTemplate != "{{.Slug}}.sublime-project" {
			t.Errorf("Artifact.FilenameTemplate = %q", cfg.Artifact.FilenameTemplate)
		}
	})

	t.Run("omitting artifact section retains default", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.toml"), `
[sync]
include = ["a.txt"]
`)
		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Artifact.IsZero() {
			t.Error("omitting [artifact] should retain the default template")
		}
	})

	t.Run("legacy .treepad.json returns migration error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.json"), `{"sync":{"include":["a.txt"]}}`)

		_, err := Load(dir)
		if err == nil || !strings.Contains(err.Error(), ".treepad.json") {
			t.Fatalf("got error %v, want error mentioning .treepad.json", err)
		}
	})

	t.Run("invalid TOML returns error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.toml"), `{invalid`)

		_, err := Load(dir)
		if err == nil || !strings.Contains(err.Error(), "parsing") {
			t.Fatalf("got error %v, want error containing %q", err, "parsing")
		}
	})

	t.Run("unreadable file returns error", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".treepad.toml"), 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := Load(dir)
		if err == nil || !strings.Contains(err.Error(), "reading") {
			t.Fatalf("got error %v, want error containing %q", err, "reading")
		}
	})

	t.Run("from_spec section round-trips", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.toml"), `
[from_spec]
prompt_filename = "AGENT.md"
skills = ["go", "testing"]
agent_command = ["claude", "{{.PromptPath}}"]
prompt_template = "spec: {{.Spec}}"
`)
		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.FromSpec.PromptFilename != "AGENT.md" {
			t.Errorf("PromptFilename = %q, want %q", cfg.FromSpec.PromptFilename, "AGENT.md")
		}
		if !reflect.DeepEqual(cfg.FromSpec.Skills, []string{"go", "testing"}) {
			t.Errorf("Skills = %v, want [go testing]", cfg.FromSpec.Skills)
		}
		if cfg.FromSpec.PromptTemplate != "spec: {{.Spec}}" {
			t.Errorf("PromptTemplate = %q", cfg.FromSpec.PromptTemplate)
		}
		if !reflect.DeepEqual(cfg.FromSpec.AgentCommand, []string{"claude", "{{.PromptPath}}"}) {
			t.Errorf("AgentCommand = %v", cfg.FromSpec.AgentCommand)
		}
	})

	t.Run("omitted from_spec section is zero", func(t *testing.T) {
		cfg, err := Load(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.FromSpec.IsZero() {
			t.Error("expected zero FromSpec when section omitted")
		}
	})

	t.Run("default diff base is origin/main", func(t *testing.T) {
		cfg, err := Load(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Diff.Base != "origin/main" {
			t.Errorf("Diff.Base = %q, want %q", cfg.Diff.Base, "origin/main")
		}
	})

	t.Run("custom diff base is loaded", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.toml"), "[diff]\nbase = \"master\"\n")
		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Diff.Base != "master" {
			t.Errorf("Diff.Base = %q, want %q", cfg.Diff.Base, "master")
		}
	})

	t.Run("omitting diff section retains origin/main default", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.toml"), "[sync]\ninclude = [\"a.txt\"]\n")
		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Diff.Base != "origin/main" {
			t.Errorf("Diff.Base = %q, want %q", cfg.Diff.Base, "origin/main")
		}
	})
}

func TestDefaultSyncInclude(t *testing.T) {
	patterns := defaultSyncInclude()
	if len(patterns) != 9 {
		t.Errorf("len(defaultSyncInclude()) = %d, want 9", len(patterns))
	}
	for _, want := range []string{".claude/", "node_modules/", ".env", ".vscode/settings.json"} {
		if !slices.Contains(patterns, want) {
			t.Errorf("defaultSyncInclude() missing %q", want)
		}
	}
}
