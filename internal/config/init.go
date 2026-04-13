package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteDefault writes a config file populated with the default sync files.
// If global is true, writes to the global config path (XDG or $TREEPAD_CONFIG).
// Otherwise writes .treepad.json in dir.
// Returns the path written.
func WriteDefault(dir string, global bool) (string, error) {
	var path string
	if global {
		p, err := GlobalConfigPath()
		if err != nil {
			return "", err
		}
		path = p
	} else {
		path = filepath.Join(dir, configFileName)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	cfg := Config{Sync: SyncConfig{Files: defaultSyncFiles()}}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}

	return path, nil
}
