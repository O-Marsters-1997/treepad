package codeworkspace

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"treepad/internal/worktree"
)

type workspaceFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type workspaceFile struct {
	Folders    []workspaceFolder   `json:"folders"`
	Extensions map[string][]string `json:"extensions"`
}

// Generate writes one .code-workspace file per worktree into outputDir.
// Files are named <slug>-<branch>.code-workspace.
// The .code-workspace format is supported by VS Code, Cursor, and Windsurf.
func Generate(worktrees []worktree.Worktree, extensions []string, slug, outputDir string) error {
	slog.Debug("generating workspace files", "count", len(worktrees), "outputDir", outputDir)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	for _, wt := range worktrees {
		branchSlug := sanitizeBranch(wt.Branch)
		filename := fmt.Sprintf("%s-%s.code-workspace", slug, branchSlug)
		dest := filepath.Join(outputDir, filename)

		folderPath, err := filepath.Rel(outputDir, wt.Path)
		if err != nil {
			// cross-volume path (e.g. different drive on Windows) — fall back to absolute
			folderPath = wt.Path
		}

		wf := workspaceFile{
			Folders: []workspaceFolder{
				{
					Name: wt.Branch,
					Path: folderPath,
				},
			},
			Extensions: map[string][]string{
				"recommendations": extensions,
			},
		}

		data, err := json.MarshalIndent(wf, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal workspace for %s: %w", wt.Branch, err)
		}

		if err := os.WriteFile(dest, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("write workspace file %s: %w", dest, err)
		}

		slog.Debug("wrote workspace file", "path", dest, "branch", wt.Branch)
		fmt.Printf("  created %s\n", filename)
	}
	return nil
}

func sanitizeBranch(branch string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
	)
	return replacer.Replace(branch)
}
