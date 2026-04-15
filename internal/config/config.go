// Package config loads optional per-repo configuration from .treepad.toml.
// All fields have sane defaults so the file is never required.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"treepad/internal/hook"
)

const configFileName = ".treepad.toml"

// legacyConfigFileName is the old JSON config name. Its presence triggers a
// migration hint so users know to update.
const legacyConfigFileName = ".treepad.json"

// defaultArtifactFilenameTemplate and defaultArtifactContentTemplate produce
// VS Code .code-workspace files — the default editor experience — when no
// [artifact] block is present in .treepad.toml.
const defaultArtifactFilenameTemplate = `{{.Slug}}-{{.Branch}}.code-workspace`
const defaultArtifactContentTemplate = `{
  "folders": [
    {{- range $i, $w := .Worktrees}}
    {{- if $i}},{{end}}
    {"name": "{{$w.Branch}}", "path": "{{$w.RelPath}}"}
    {{- end}}
  ]
}
`

func defaultSyncFiles() []string {
	return []string{
		".claude/settings.local.json",
		".env",
		".env.docker-compose",
		".vscode/settings.json",
		".vscode/tasks.json",
		".vscode/launch.json",
		".vscode/extensions.json",
		".vscode/*.code-snippets",
	}
}

// SyncConfig holds file patterns to copy across worktrees.
type SyncConfig struct {
	// Files replaces defaultSyncFiles entirely when non-empty.
	Files []string `toml:"files"`
}

// ArtifactConfig describes the per-worktree file to generate.
// Both fields are text/template strings. Leave FilenameTemplate empty to skip
// artifact generation.
type ArtifactConfig struct {
	FilenameTemplate string `toml:"filename"`
	ContentTemplate  string `toml:"content"`
}

// IsZero reports whether no artifact is configured.
func (a ArtifactConfig) IsZero() bool {
	return a.FilenameTemplate == ""
}

// OpenConfig describes the command used by --open.
// Each element of Command is a text/template string.
type OpenConfig struct {
	Command []string `toml:"command"`
}

// IsZero reports whether no open command is configured.
func (o OpenConfig) IsZero() bool {
	return len(o.Command) == 0
}

// ExecConfig controls the task runner used by `tp exec`.
type ExecConfig struct {
	// Runner overrides auto-detection. Valid values: just, npm, pnpm, yarn, bun,
	// make, pip, poetry, uv.
	Runner string `toml:"runner"`
}

// IsZero reports whether no exec runner is explicitly configured.
func (e ExecConfig) IsZero() bool {
	return e.Runner == ""
}

// Config is the full resolved configuration for a repo.
type Config struct {
	Sync     SyncConfig     `toml:"sync"`
	Artifact ArtifactConfig `toml:"artifact"`
	Open     OpenConfig     `toml:"open"`
	Hooks    hook.Config    `toml:"hooks"`
	Exec     ExecConfig     `toml:"exec"`
}

// GlobalConfigPath returns the path to the global config file.
// Resolution order: $TREEPAD_CONFIG → $XDG_CONFIG_HOME/treepad/config.toml → ~/.config/treepad/config.toml
func GlobalConfigPath() (string, error) {
	if envPath := os.Getenv("TREEPAD_CONFIG"); envPath != "" {
		return envPath, nil
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("determine home directory: %w", err)
		}
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "treepad", "config.toml"), nil
}

// Load reads .treepad.toml from repoRoot. Returns defaults when the file is absent.
// Returns an error with a migration hint when only the legacy .treepad.json is present.
func Load(repoRoot string) (Config, error) {
	cfg := defaults()

	tomlPath := filepath.Join(repoRoot, configFileName)
	data, err := os.ReadFile(tomlPath)
	if errors.Is(err, os.ErrNotExist) {
		if _, jsonErr := os.Stat(filepath.Join(repoRoot, legacyConfigFileName)); jsonErr == nil {
			return cfg, fmt.Errorf(
				"found %s but treepad now uses TOML; move your config to %s or re-run `treepad config init --local`",
				legacyConfigFileName, configFileName,
			)
		}
		slog.Debug("no .treepad.toml found, using defaults", "dir", repoRoot)
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("reading %s: %w", configFileName, err)
	}

	var fileCfg Config
	if _, err := toml.Decode(string(data), &fileCfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", configFileName, err)
	}

	// An explicit empty files array is treated as unset — defaults apply.
	if len(fileCfg.Sync.Files) > 0 {
		cfg.Sync.Files = fileCfg.Sync.Files
	}
	if !fileCfg.Artifact.IsZero() {
		cfg.Artifact = fileCfg.Artifact
	}
	if !fileCfg.Open.IsZero() {
		cfg.Open = fileCfg.Open
	}
	if !fileCfg.Hooks.IsZero() {
		cfg.Hooks = fileCfg.Hooks
	}
	if !fileCfg.Exec.IsZero() {
		cfg.Exec = fileCfg.Exec
	}

	slog.Debug("loaded .treepad.toml", "dir", repoRoot, "syncFiles", cfg.Sync.Files)
	return cfg, nil
}

func defaults() Config {
	return Config{
		Sync: SyncConfig{Files: defaultSyncFiles()},
		Artifact: ArtifactConfig{
			FilenameTemplate: defaultArtifactFilenameTemplate,
			ContentTemplate:  defaultArtifactContentTemplate,
		},
		Open: OpenConfig{Command: []string{"open", "{{.ArtifactPath}}"}},
	}
}
