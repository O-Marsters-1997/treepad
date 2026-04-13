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
	tests := []struct {
		name      string
		setup     func(dir string)
		wantFiles []string
		wantErr   string
	}{
		{
			name:      "no config file returns defaults",
			setup:     func(dir string) {},
			wantFiles: defaultSyncFiles(),
		},
		{
			name: "valid config with custom files",
			setup: func(dir string) {
				writeFile(t, filepath.Join(dir, ".treepad.json"), `{"sync":{"files":["custom.txt"]}}`)
			},
			wantFiles: []string{"custom.txt"},
		},
		{
			name: "empty files array falls back to defaults",
			setup: func(dir string) {
				writeFile(t, filepath.Join(dir, ".treepad.json"), `{"sync":{"files":[]}}`)
			},
			wantFiles: defaultSyncFiles(),
		},
		{
			name: "invalid JSON returns error",
			setup: func(dir string) {
				writeFile(t, filepath.Join(dir, ".treepad.json"), `{invalid`)
			},
			wantErr: "parsing",
		},
		{
			name: "unreadable file returns error",
			setup: func(dir string) {
				if err := os.Mkdir(filepath.Join(dir, ".treepad.json"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "reading",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(dir)

			cfg, err := Load(dir)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(cfg.Sync.Files, tt.wantFiles) {
				t.Errorf("Sync.Files = %v, want %v", cfg.Sync.Files, tt.wantFiles)
			}
		})
	}
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
