package treepad

import (
	"fmt"

	"treepad/internal/artifact"
	"treepad/internal/config"
)

func artifactSpec(c config.ArtifactConfig) artifact.Spec {
	return artifact.Spec{
		FilenameTemplate: c.FilenameTemplate,
		ContentTemplate:  c.ContentTemplate,
	}
}

func templateData(repoSlug, branch, worktreePath, outputDir string) artifact.TemplateData {
	wt := artifact.ToWorktree(branch, worktreePath, outputDir)
	return artifact.TemplateData{
		Slug:      repoSlug,
		Branch:    wt.Name,
		Worktrees: []artifact.Worktree{wt},
		OutputDir: outputDir,
	}
}

func resolveArtifactPath(spec artifact.Spec, repoSlug, branch, wtPath, outputDir string) (string, bool, error) {
	data := templateData(repoSlug, branch, wtPath, outputDir)
	path, ok, err := artifact.Path(spec, outputDir, data)
	if err != nil {
		return "", false, fmt.Errorf("resolve artifact path: %w", err)
	}
	return path, ok, nil
}
