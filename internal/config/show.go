package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Show returns a human-readable summary of the resolved config and which
// sources contributed (local .treepad.json, global config, or built-in defaults).
func Show(repoRoot string) (string, error) {
	var sources []string
	var cfg Config

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

	switch {
	case localFound:
		cfg = localCfg
		sources = append(sources, fmt.Sprintf("local:  %s", localPath))
	case globalFound:
		cfg = globalCfg
		sources = append(sources, fmt.Sprintf("global: %s", globalPath))
	default:
		cfg = Config{Sync: SyncConfig{Files: defaultSyncFiles()}}
		sources = append(sources, "built-in defaults")
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("Sources:\n")
	for _, s := range sources {
		sb.WriteString("  ")
		sb.WriteString(s)
		sb.WriteByte('\n')
	}
	sb.WriteString("\nConfig:\n")
	sb.Write(data)
	sb.WriteByte('\n')

	return sb.String(), nil
}

// loadFile reads and parses a single config JSON file.
// Returns (cfg, true, nil) if found and valid with non-empty sync.files.
// Returns (zero, false, nil) if file is missing or sync.files is empty.
// Returns (zero, false, err) if file exists but cannot be read or parsed.
func loadFile(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(cfg.Sync.Files) == 0 {
		return Config{}, false, nil
	}
	return cfg, true, nil
}
