package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDefault(t *testing.T) {
	t.Run("writes built-in defaults", func(t *testing.T) {
		dir := t.TempDir()
		path, err := WriteDefault(dir, InitOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != filepath.Join(dir, configFileName) {
			t.Errorf("path = %q, want %s/%s", path, dir, configFileName)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != defaultTOML {
			t.Error("written content does not match defaultTOML")
		}
	})

	t.Run("inherit copies global config verbatim", func(t *testing.T) {
		globalPath := filepath.Join(t.TempDir(), "config.toml")
		writeFile(t, globalPath, "# a comment worth keeping\n[[hooks.post_config_init]]\ncommand = \"marker\"\n")
		t.Setenv("TREEPAD_CONFIG", globalPath)

		dir := t.TempDir()
		path, err := WriteDefault(dir, InitOptions{Inherit: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), "# a comment worth keeping") {
			t.Errorf("expected comment to survive verbatim inherit, got:\n%s", got)
		}
	})

	t.Run("inherit hooks-only keeps defaults and lifts hooks", func(t *testing.T) {
		globalPath := filepath.Join(t.TempDir(), "config.toml")
		writeFile(t, globalPath, "[[hooks.post_config_init]]\ncommand = \"marker\"\n")
		t.Setenv("TREEPAD_CONFIG", globalPath)

		dir := t.TempDir()
		path, err := WriteDefault(dir, InitOptions{Inherit: true, HooksOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), "[[hooks.post_config_init]]") {
			t.Errorf("expected [hooks] lifted from global config, got:\n%s", got)
		}
		if !strings.Contains(string(got), "[artifact]") {
			t.Errorf("expected built-in [artifact] section to survive, got:\n%s", got)
		}
	})

	t.Run("inherit without a global config errors", func(t *testing.T) {
		t.Setenv("TREEPAD_CONFIG", filepath.Join(t.TempDir(), "missing.toml"))
		_, err := WriteDefault(t.TempDir(), InitOptions{Inherit: true})
		if err == nil {
			t.Fatal("want error, got nil")
		}
	})

	t.Run("refuses to overwrite an existing file without --force", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, configFileName)
		writeFile(t, path, "sentinel")

		_, err := WriteDefault(dir, InitOptions{})
		if err == nil {
			t.Fatal("want error, got nil")
		}
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(got) != "sentinel" {
			t.Error("existing config was modified despite missing --force")
		}
	})

	t.Run("force overwrites an existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, configFileName)
		writeFile(t, path, "sentinel")

		_, err := WriteDefault(dir, InitOptions{Force: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != defaultTOML {
			t.Error("force did not overwrite the existing config")
		}
	})
}
