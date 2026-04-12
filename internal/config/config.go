// Package config loads optional per-repo configuration from .treepad.json.
// All fields have sane defaults so the file is never required.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const configFileName = ".treepad.json"

// DefaultSyncFiles is the baseline list of files synced across worktrees when
// no .treepad.json is present or when sync.files is unset. Includes .vscode/*
// patterns which work for VS Code, Cursor, and Windsurf out of the box.
var DefaultSyncFiles = []string{
	".claude/settings.local.json",
	".env",
	".env.docker-compose",
	".vscode/settings.json",
	".vscode/tasks.json",
	".vscode/launch.json",
	".vscode/extensions.json",
	".vscode/*.code-snippets",
}

type SyncConfig struct {
	// Files replaces DefaultSyncFiles entirely when non-empty.
	Files []string `json:"files"`
}

type Config struct {
	Sync SyncConfig `json:"sync"`
}

// Load reads .treepad.json from repoRoot. Returns defaults when the file is absent.
func Load(repoRoot string) (Config, error) {
	cfg := Config{
		Sync: SyncConfig{Files: DefaultSyncFiles},
	}

	data, err := os.ReadFile(filepath.Join(repoRoot, configFileName))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("reading %s: %w", configFileName, err)
	}

	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", configFileName, err)
	}

	if len(fileCfg.Sync.Files) > 0 {
		cfg.Sync.Files = fileCfg.Sync.Files
	}

	return cfg, nil
}
