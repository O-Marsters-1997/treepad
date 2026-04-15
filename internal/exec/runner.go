// Package exec detects project task runners and enumerates their scripts.
package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Runner describes a detected task runner with its script-invocation prefix
// and the set of known script names.
type Runner struct {
	Name      string   // e.g. "just", "pnpm"
	ScriptCmd []string // prefix before script name, e.g. ["pnpm", "run"]
	Scripts   []string // enumerated script names (sorted)
}

// Detect identifies the task runner in worktreePath and returns it.
// override, if non-empty, forces use of the named runner (from .treepad.toml [exec] runner).
// Returns an error if multiple runners are detected and override is empty.
func Detect(worktreePath, override string) (Runner, error) {
	if override != "" {
		scripts, err := listScripts(worktreePath, override)
		if err != nil {
			return Runner{}, err
		}
		return Runner{
			Name:      override,
			ScriptCmd: scriptCmd(override),
			Scripts:   scripts,
		}, nil
	}

	var detected []string

	if hasFile(worktreePath, "justfile") || hasFile(worktreePath, "Justfile") {
		detected = append(detected, "just")
	}
	if hasFile(worktreePath, "package.json") {
		jsRunner, err := detectJSManager(worktreePath)
		if err != nil {
			return Runner{}, err
		}
		detected = append(detected, jsRunner)
	}
	if hasFile(worktreePath, "Makefile") {
		detected = append(detected, "make")
	}
	if hasFile(worktreePath, "pyproject.toml") {
		detected = append(detected, detectPythonRunner(worktreePath))
	}

	if len(detected) == 0 {
		return Runner{}, fmt.Errorf("no task runner detected; check that a justfile, package.json, Makefile, or pyproject.toml is present")
	}
	if len(detected) > 1 {
		return Runner{}, fmt.Errorf("multiple task runners detected (%s); set [exec]\nrunner = %q (or another) in .treepad.toml to disambiguate", strings.Join(detected, ", "), detected[0])
	}

	name := detected[0]
	scripts, err := listScripts(worktreePath, name)
	if err != nil {
		return Runner{}, err
	}
	return Runner{
		Name:      name,
		ScriptCmd: scriptCmd(name),
		Scripts:   scripts,
	}, nil
}

// listScripts returns the known script names for the named runner in worktreePath.
// Returns nil for runners that do not support script enumeration (make, pip).
func listScripts(worktreePath, runnerName string) ([]string, error) {
	switch runnerName {
	case "just":
		return listJustRecipes(worktreePath)
	case "npm", "pnpm", "yarn", "bun":
		return listPackageJSONScripts(worktreePath)
	case "poetry", "uv", "pip":
		return listPyprojectScripts(worktreePath)
	case "make":
		return nil, nil
	default:
		return nil, nil
	}
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

func hasFile(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// detectJSManager selects npm/pnpm/yarn/bun based on lockfile presence,
// then the packageManager field in package.json, defaulting to npm.
func detectJSManager(dir string) (string, error) {
	switch {
	case hasFile(dir, "bun.lockb"):
		return "bun", nil
	case hasFile(dir, "pnpm-lock.yaml"):
		return "pnpm", nil
	case hasFile(dir, "yarn.lock"):
		return "yarn", nil
	case hasFile(dir, "package-lock.json"):
		return "npm", nil
	}

	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "npm", nil
	}
	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	if jsonErr := json.Unmarshal(data, &pkg); jsonErr == nil && pkg.PackageManager != "" {
		name, _, _ := strings.Cut(pkg.PackageManager, "@")
		switch name {
		case "npm", "pnpm", "yarn", "bun":
			return name, nil
		}
	}
	return "npm", nil
}

// detectPythonRunner picks poetry or uv based on pyproject.toml contents.
func detectPythonRunner(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		return "uv"
	}
	if strings.Contains(string(data), "[tool.poetry]") {
		return "poetry"
	}
	return "uv"
}

// justRecipeRe matches lines that look like recipe definitions (starts with an
// identifier then anything up to a colon). Post-filtered to exclude ":="
// variable assignments since Go's regexp doesn't support negative lookaheads.
var justRecipeRe = regexp.MustCompile(`(?m)^([a-zA-Z0-9][a-zA-Z0-9_-]*)[^:\n]*:`)

func listJustRecipes(dir string) ([]string, error) {
	for _, name := range []string{"justfile", "Justfile"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var recipes []string
		for _, m := range justRecipeRe.FindAllStringSubmatch(string(data), -1) {
			recipe := m[1]
			fullMatch := m[0]
			// Exclude variable assignments (name := value) and private recipes.
			if strings.Contains(fullMatch, ":=") || strings.HasPrefix(recipe, "_") {
				continue
			}
			recipes = append(recipes, recipe)
		}
		sort.Strings(recipes)
		return recipes, nil
	}
	return nil, nil
}

func listPackageJSONScripts(dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, fmt.Errorf("read package.json: %w", err)
	}
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

func listPyprojectScripts(dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		return nil, fmt.Errorf("read pyproject.toml: %w", err)
	}
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
