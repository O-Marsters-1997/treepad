package codeworkspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type extensionsFile struct {
	Recommendations []string `json:"recommendations"`
}

// ResolveExtensions reads .vscode/extensions.json if present, otherwise
// auto-detects recommended extensions by walking the directory tree.
func ResolveExtensions(dir string) ([]string, error) {
	exts, err := ReadExtensions(dir)
	if err != nil {
		return nil, err
	}
	if exts != nil {
		return exts, nil
	}
	return DetectExtensions(dir)
}

// ReadExtensions returns (nil, nil) when extensions.json is absent — caller should fall through to DetectExtensions.
func ReadExtensions(dir string) ([]string, error) {
	path := filepath.Join(dir, ".vscode", "extensions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read extensions.json: %w", err)
	}

	var ef extensionsFile
	if err := json.Unmarshal(data, &ef); err != nil {
		return nil, fmt.Errorf("parse extensions.json: %w", err)
	}
	return ef.Recommendations, nil
}

func DetectExtensions(dir string) ([]string, error) {
	type probe struct {
		ext       string // file extension to match; empty means match by filename instead
		filename  string // exact filename; takes priority over ext when both are set
		extension string
	}

	probes := []probe{
		{ext: ".go", extension: "golang.go"},
		{ext: ".ts", extension: "ms-vscode.vscode-typescript-next"},
		{ext: ".tsx", extension: "ms-vscode.vscode-typescript-next"},
		{ext: ".js", extension: "ms-vscode.vscode-typescript-next"},
		{ext: ".jsx", extension: "ms-vscode.vscode-typescript-next"},
		{filename: "package.json", extension: "esbenp.prettier-vscode"},
		{ext: ".py", extension: "ms-python.python"},
		{ext: ".rs", extension: "rust-lang.rust-analyzer"},
		{filename: "Dockerfile", extension: "ms-azuretools.vscode-docker"},
		{filename: "docker-compose.yml", extension: "ms-azuretools.vscode-docker"},
	}

	found := make(map[string]bool)
	var extensions []string

	add := func(id string) {
		if !found[id] {
			found[id] = true
			extensions = append(extensions, id)
		}
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than aborting the walk
		}
		if d.IsDir() {
			name := d.Name()
			// skip hidden dirs and common dependency directories
			if name != "." && (name[0] == '.' || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		ext := filepath.Ext(name)

		for _, p := range probes {
			switch {
			case p.filename != "" && name == p.filename:
				add(p.extension)
			case p.ext != "" && ext == p.ext:
				add(p.extension)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("detect extensions: %w", err)
	}

	return extensions, nil
}
