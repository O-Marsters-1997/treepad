// Package artifact renders per-worktree files from config-supplied templates.
// No editor names appear here — callers supply templates via .treepad.toml.
package artifact

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// Spec describes how to generate a per-worktree artifact file.
// Both fields are text/template strings evaluated against TemplateData.
// When FilenameTemplate is empty, artifact generation is skipped.
type Spec struct {
	FilenameTemplate string
	ContentTemplate  string
}

// IsZero reports whether the spec has no artifact configuration.
func (s Spec) IsZero() bool {
	return s.FilenameTemplate == ""
}

// Worktree is the template-friendly view of a single git worktree.
type Worktree struct {
	Name    string // branch name sanitized for use in filenames
	Path    string // absolute path on disk
	RelPath string // path relative to the artifact output directory
	Branch  string // raw branch name
}

// TemplateData is the context available in filename and content templates.
type TemplateData struct {
	Slug      string
	Branch    string     // branch for this artifact; empty when not applicable
	Worktrees []Worktree // worktrees to include in this artifact
	OutputDir string
}

// RenderFilename executes the filename template and returns the result.
func RenderFilename(spec Spec, data TemplateData) (string, error) {
	return renderString("filename", spec.FilenameTemplate, data)
}

// RenderContent executes the content template and returns the rendered bytes.
func RenderContent(spec Spec, data TemplateData) ([]byte, error) {
	s, err := renderString("content", spec.ContentTemplate, data)
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

// Path returns the absolute path the artifact would be written to.
// Returns ("", false, nil) when spec is zero.
func Path(spec Spec, outputDir string, data TemplateData) (string, bool, error) {
	if spec.IsZero() {
		return "", false, nil
	}
	filename, err := RenderFilename(spec, data)
	if err != nil {
		return "", false, fmt.Errorf("render artifact filename: %w", err)
	}
	return filepath.Join(outputDir, filename), true, nil
}

// Write renders and writes the artifact to outputDir.
// Returns the absolute path written, or "" when spec is zero.
func Write(spec Spec, outputDir string, data TemplateData) (string, error) {
	if spec.IsZero() {
		return "", nil
	}
	filename, err := RenderFilename(spec, data)
	if err != nil {
		return "", fmt.Errorf("render artifact filename: %w", err)
	}
	content, err := RenderContent(spec, data)
	if err != nil {
		return "", fmt.Errorf("render artifact content: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	dest := filepath.Join(outputDir, filename)
	if err := os.WriteFile(dest, content, 0o644); err != nil {
		return "", fmt.Errorf("write artifact %s: %w", dest, err)
	}
	return dest, nil
}

// ToWorktree builds the template-friendly Worktree view from raw path and branch,
// computing RelPath relative to outputDir.
func ToWorktree(branch, path, outputDir string) Worktree {
	relPath, err := filepath.Rel(outputDir, path)
	if err != nil {
		// cross-volume path — fall back to absolute
		relPath = path
	}
	return Worktree{
		Name:    sanitizeBranch(branch),
		Path:    path,
		RelPath: relPath,
		Branch:  branch,
	}
}

func sanitizeBranch(branch string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		default:
			return r
		}
	}, branch)
}

func renderString(name, tmpl string, data any) (string, error) {
	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse %s template: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute %s template: %w", name, err)
	}
	return buf.String(), nil
}
