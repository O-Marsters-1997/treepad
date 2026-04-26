package config

import (
	"fmt"

	"treepad/internal/artifact"
)

// Spec converts ArtifactConfig to an artifact.Spec.
func (c ArtifactConfig) Spec() artifact.Spec {
	return artifact.Spec{
		FilenameTemplate: c.FilenameTemplate,
		ContentTemplate:  c.ContentTemplate,
	}
}

// MakeTemplateData builds the artifact.TemplateData for a single worktree.
func MakeTemplateData(repoSlug, branch, worktreePath, outputDir string) artifact.TemplateData {
	wt := artifact.ToWorktree(branch, worktreePath, outputDir)
	return artifact.TemplateData{
		Slug:      repoSlug,
		Branch:    wt.Name,
		Worktrees: []artifact.Worktree{wt},
		OutputDir: outputDir,
	}
}

// ResolveArtifactPath returns the absolute path the artifact would be written
// to, or ("", false, nil) when no filename template is configured.
func ResolveArtifactPath(c ArtifactConfig, repoSlug, branch, wtPath, outputDir string) (string, bool, error) {
	data := MakeTemplateData(repoSlug, branch, wtPath, outputDir)
	path, ok, err := artifact.Path(c.Spec(), outputDir, data)
	if err != nil {
		return "", false, fmt.Errorf("resolve artifact path: %w", err)
	}
	return path, ok, nil
}
