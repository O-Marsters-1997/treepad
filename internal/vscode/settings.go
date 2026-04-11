package vscode

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

type Settings map[string]any

const (
	keySearchUseIgnoreFiles = "search.useIgnoreFiles"
	keySearchFollowSymlinks = "search.followSymlinks"
	keyWindowTitle          = "window.title"
	windowTitleValue        = "${activeEditorShort} — ${rootName} — VS Code"
)

// ReadSettings returns empty Settings (not an error) when settings.json is absent.
func ReadSettings(dir string) (Settings, error) {
	path := filepath.Join(dir, ".vscode", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Settings{}, nil
		}
		return nil, fmt.Errorf("read settings.json: %w", err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse settings.json: %w", err)
	}
	return s, nil
}

// FilterSettings removes search.exclude and files.exclude entries whose keys start with "../"
// (cross-worktree path references), injects isolation settings, and returns a new map without
// mutating the input.
func FilterSettings(s Settings) Settings {
	out := make(Settings, len(s))
	maps.Copy(out, s)

	for _, field := range []string{"search.exclude", "files.exclude"} {
		raw, ok := out[field]
		if !ok {
			continue
		}
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		filtered := make(map[string]any, len(m))
		for k, v := range m {
			if !strings.HasPrefix(k, "../") {
				filtered[k] = v
			}
		}
		out[field] = filtered
	}

	out[keySearchUseIgnoreFiles] = true
	out[keySearchFollowSymlinks] = false
	out[keyWindowTitle] = windowTitleValue

	return out
}
