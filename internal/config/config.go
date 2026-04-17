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

func defaultSyncInclude() []string {
	return []string{
		".claude/",
		"node_modules/",
		".env",
		".env.docker-compose",
		".vscode/settings.json",
		".vscode/tasks.json",
		".vscode/launch.json",
		".vscode/extensions.json",
		".vscode/*.code-snippets",
	}
}

// SyncConfig holds gitignore-style patterns controlling which files are copied across worktrees.
type SyncConfig struct {
	// Include replaces defaultSyncInclude entirely when non-empty.
	// Patterns use gitignore syntax: ** crosses directories, trailing / matches
	// a directory and all its contents, ! prefix negates a pattern.
	Include []string `toml:"include"`
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

// FromSpecConfig configures `tp from-spec`.
type FromSpecConfig struct {
	// PromptTemplate is a text/template string rendered into the worktree.
	// Data: .Spec, .Skills, .Branch, .Slug, .WorktreePath, .PromptPath.
	// .Prompt is additionally available in AgentCommand templates.
	PromptTemplate string `toml:"prompt_template"`
	// PromptFilename is the file written inside the worktree root. Default "PROMPT.md".
	PromptFilename string `toml:"prompt_filename"`
	// Skills is a list of skill names exposed to the template as .Skills.
	Skills []string `toml:"skills"`
	// AgentCommand is invoked after the prompt is written. Each element is a
	// text/template string. Empty means write the prompt and exit 0.
	AgentCommand []string `toml:"agent_command"`
}

// IsZero reports whether no from-spec configuration is present.
func (f FromSpecConfig) IsZero() bool {
	return f.PromptTemplate == "" && f.PromptFilename == "" &&
		len(f.Skills) == 0 && len(f.AgentCommand) == 0
}

// Config is the full resolved configuration for a repo.
type Config struct {
	Sync     SyncConfig     `toml:"sync"`
	Artifact ArtifactConfig `toml:"artifact"`
	Open     OpenConfig     `toml:"open"`
	Hooks    hook.Config    `toml:"hooks"`
	Exec     ExecConfig     `toml:"exec"`
	FromSpec FromSpecConfig `toml:"from_spec"`
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

	// An explicit empty include array is treated as unset — defaults apply.
	if len(fileCfg.Sync.Include) > 0 {
		cfg.Sync.Include = fileCfg.Sync.Include
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
	if !fileCfg.FromSpec.IsZero() {
		cfg.FromSpec = fileCfg.FromSpec
	}

	slog.Debug("loaded .treepad.toml", "dir", repoRoot, "syncInclude", cfg.Sync.Include)
	return cfg, nil
}

func defaults() Config {
	return Config{
		Sync: SyncConfig{Include: defaultSyncInclude()},
		Artifact: ArtifactConfig{
			FilenameTemplate: defaultArtifactFilenameTemplate,
			ContentTemplate:  defaultArtifactContentTemplate,
		},
		Open: OpenConfig{Command: []string{"open", "{{.ArtifactPath}}"}},
	}
}
