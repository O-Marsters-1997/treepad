package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Show returns a human-readable summary of the resolved config and which
// source contributed (local .treepad.toml, global config, or built-in defaults).
func Show(repoRoot string) (string, error) {
	globalPath, err := GlobalConfigPath()
	if err != nil {
		return "", err
	}

	localPath := filepath.Join(repoRoot, configFileName)
	localCfg, localFound, err := loadFile(localPath)
	if err != nil {
		return "", err
	}

	globalCfg, globalFound, err := loadFile(globalPath)
	if err != nil {
		return "", err
	}

	var source string
	var cfg Config
	switch {
	case localFound:
		cfg = localCfg
		source = fmt.Sprintf("local:  %s", localPath)
	case globalFound:
		cfg = globalCfg
		source = fmt.Sprintf("global: %s", globalPath)
	default:
		cfg = defaults()
		source = "built-in defaults"
	}

	var sb strings.Builder
	sb.WriteString("Sources:\n  ")
	sb.WriteString(source)
	sb.WriteString("\n\nConfig:\n")
	if err := toml.NewEncoder(&sb).Encode(cfg); err != nil {
		return "", fmt.Errorf("encode config: %w", err)
	}

	return sb.String(), nil
}

// loadFile reads and parses a single .treepad.toml file.
// Returns (cfg, true, nil) if found with non-empty sync.include.
// Returns (zero, false, nil) if missing or sync.include is empty.
// Returns (zero, false, err) if the file exists but cannot be read or parsed.
func loadFile(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(cfg.Sync.Include) == 0 {
		return Config{}, false, nil
	}
	return cfg, true, nil
}
