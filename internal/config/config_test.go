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
