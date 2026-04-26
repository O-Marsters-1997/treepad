// Package exec detects project task runners and enumerates their scripts.
package exec

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type Runner struct {
	Name      string   // e.g. "just", "pnpm"
	ScriptCmd []string // prefix before script name, e.g. ["pnpm", "run"]
	Scripts   []string // enumerated script names (sorted)
}

func Resolve(dir, override string) (Runner, error) {
	fsys := os.DirFS(dir)
	name, err := pickRunner(fsys, override)
	if err != nil {
		return Runner{}, err
	}
	scripts, err := enumerate(fsys, name)
	if err != nil {
		return Runner{}, err
	}
	return Runner{Name: name, ScriptCmd: scriptCmd(name), Scripts: scripts}, nil
}

func pickRunner(fsys fs.FS, override string) (string, error) {
	if override != "" {
		return override, nil
	}

	var detected []string

	if hasFile(fsys, "justfile") || hasFile(fsys, "Justfile") {
		detected = append(detected, "just")
	}
	if hasFile(fsys, "package.json") {
		detected = append(detected, detectJSManager(fsys))
	}
	if hasFile(fsys, "Makefile") {
		detected = append(detected, "make")
	}
	if hasFile(fsys, "pyproject.toml") {
		detected = append(detected, detectPythonRunner(fsys))
	}

	if len(detected) == 0 {
		return "", fmt.Errorf("no task runner detected; check that a justfile, " +
			"package.json, Makefile, or pyproject.toml is present")
	}
	if len(detected) > 1 {
		return "", fmt.Errorf(
			"multiple task runners detected (%s); set [exec]\nrunner = %q (or another) in .treepad.toml to disambiguate",
			strings.Join(detected, ", "), detected[0])
	}
	return detected[0], nil
}

func enumerate(fsys fs.FS, name string) ([]string, error) {
	switch name {
	case "just":
		for _, fname := range []string{"justfile", "Justfile"} {
			data, err := fs.ReadFile(fsys, fname)
			if err == nil {
				return parseJustRecipes(data), nil
			}
		}
		return nil, nil
	case "npm", "pnpm", "yarn", "bun":
		data, err := fs.ReadFile(fsys, "package.json")
		if err != nil {
			return nil, fmt.Errorf("read package.json: %w", err)
		}
		return parsePackageScripts(data)
	case "poetry", "uv", "pip":
		data, err := fs.ReadFile(fsys, "pyproject.toml")
		if err != nil {
			return nil, fmt.Errorf("read pyproject.toml: %w", err)
		}
		return parsePyprojectScripts(data)
	default:
		return nil, nil
	}
}

func hasFile(fsys fs.FS, name string) bool {
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func detectJSManager(fsys fs.FS) string {
	switch {
	case hasFile(fsys, "bun.lockb"):
		return "bun"
	case hasFile(fsys, "pnpm-lock.yaml"):
		return "pnpm"
	case hasFile(fsys, "yarn.lock"):
		return "yarn"
	case hasFile(fsys, "package-lock.json"):
		return "npm"
	}
	data, err := fs.ReadFile(fsys, "package.json")
	if err != nil {
		return "npm"
	}
	return parsePackageManagerField(data)
}

func detectPythonRunner(fsys fs.FS) string {
	data, err := fs.ReadFile(fsys, "pyproject.toml")
	if err != nil {
		return "uv"
	}
	if strings.Contains(string(data), "[tool.poetry]") {
		return "poetry"
	}
	return "uv"
}

// justRecipeRe matches lines that begin with an identifier followed by a colon.
// Variable assignments (name := value) are excluded post-match by checking the
// character immediately after the colon.
var justRecipeRe = regexp.MustCompile(`(?m)^([a-zA-Z0-9][a-zA-Z0-9_-]*)[^:\n]*:`)

func parseJustRecipes(data []byte) []string {
	var recipes []string
	for _, idx := range justRecipeRe.FindAllSubmatchIndex(data, -1) {
		// idx[1] is the position after the colon; skip if followed by '=' (:= assignment).
		if idx[1] < len(data) && data[idx[1]] == '=' {
			continue
		}
		recipe := string(data[idx[2]:idx[3]])
		if strings.HasPrefix(recipe, "_") {
			continue
		}
		recipes = append(recipes, recipe)
	}
	sort.Strings(recipes)
	return recipes
}

func parsePackageScripts(data []byte) ([]string, error) {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse package.json: %w", err)
	}
	scripts := make([]string, 0, len(pkg.Scripts))
	for name := range pkg.Scripts {
		scripts = append(scripts, name)
	}
	sort.Strings(scripts)
	return scripts, nil
}

func parsePackageManagerField(data []byte) string {
	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.PackageManager == "" {
		return "npm"
	}
	name, _, _ := strings.Cut(pkg.PackageManager, "@")
	switch name {
	case "npm", "pnpm", "yarn", "bun":
		return name
	}
	return "npm"
}

type pyprojectTOML struct {
	Project struct {
		Scripts map[string]string `toml:"scripts"`
	} `toml:"project"`
	Tool struct {
		Poetry struct {
			Scripts map[string]string `toml:"scripts"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

func parsePyprojectScripts(data []byte) ([]string, error) {
	var py pyprojectTOML
	if _, err := toml.Decode(string(data), &py); err != nil {
		return nil, fmt.Errorf("parse pyproject.toml: %w", err)
	}
	scriptMap := py.Project.Scripts
	if len(scriptMap) == 0 {
		scriptMap = py.Tool.Poetry.Scripts
	}
	scripts := make([]string, 0, len(scriptMap))
	for name := range scriptMap {
		scripts = append(scripts, name)
	}
	sort.Strings(scripts)
	return scripts, nil
}

func scriptCmd(runner string) []string {
	switch runner {
	case "just":
		return []string{"just"}
	case "npm":
		return []string{"npm", "run"}
	case "pnpm":
		return []string{"pnpm", "run"}
	case "yarn":
		return []string{"yarn"}
	case "bun":
		return []string{"bun", "run"}
	case "poetry":
		return []string{"poetry", "run"}
	case "uv":
		return []string{"uv", "run"}
	case "make":
		return []string{"make"}
	default:
		return []string{runner}
	}
}
