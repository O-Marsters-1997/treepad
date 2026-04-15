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
		if !reflect.DeepEqual(cfg.Sync.Files, defaultSyncFiles()) {
			t.Errorf("Sync.Files = %v, want defaults", cfg.Sync.Files)
		}
		if cfg.Artifact.IsZero() {
			t.Error("default Artifact should not be zero")
		}
		if cfg.Open.IsZero() {
			t.Error("default Open should not be zero")
		}
	})

	t.Run("valid config with custom sync files", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.toml"), `
[sync]
files = ["custom.txt"]
`)
		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(cfg.Sync.Files, []string{"custom.txt"}) {
			t.Errorf("Sync.Files = %v, want [custom.txt]", cfg.Sync.Files)
		}
	})

	t.Run("empty files array falls back to defaults", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".treepad.toml"), `
[sync]
files = []
`)
		cfg, err := Load(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(cfg.Sync.Files, defaultSyncFiles()) {
			t.Errorf("Sync.Files = %v, want defaults", cfg.Sync.Files)
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
files = ["a.txt"]
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
		writeFile(t, filepath.Join(dir, ".treepad.json"), `{"sync":{"files":["a.txt"]}}`)

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

func TestDefaultSyncFiles(t *testing.T) {
	files := defaultSyncFiles()
	if len(files) != 8 {
		t.Errorf("len(defaultSyncFiles()) = %d, want 8", len(files))
	}
	for _, want := range []string{".env", ".vscode/settings.json"} {
		if !slices.Contains(files, want) {
			t.Errorf("defaultSyncFiles() missing %q", want)
		}
	}
}
