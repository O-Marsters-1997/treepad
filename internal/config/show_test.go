package config

import (
	"strings"
	"testing"
)

func TestShow(t *testing.T) {
	t.Run("no config anywhere reports built-in defaults", func(t *testing.T) {
		t.Setenv("TREEPAD_CONFIG", "")
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		out, err := Show(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "built-in defaults") {
			t.Errorf("want built-in defaults, got:\n%s", out)
		}
	})

	t.Run("hooks-only local config is reported as local, not defaults", func(t *testing.T) {
		t.Setenv("TREEPAD_CONFIG", "")
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		dir := t.TempDir()
		writeFile(t, dir+"/"+configFileName, "[[hooks.post_config_init]]\ncommand = \"marker\"\n")

		out, err := Show(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "local:") {
			t.Errorf("want a local source, got:\n%s", out)
		}
		if strings.Contains(out, "built-in defaults") {
			t.Errorf("hooks-only config should not be reported as built-in defaults, got:\n%s", out)
		}
	})
}
