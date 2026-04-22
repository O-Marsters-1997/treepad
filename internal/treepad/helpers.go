package treepad

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"treepad/internal/artifact"
	"treepad/internal/config"
	"treepad/internal/hook"
	"treepad/internal/slug"
	internalsync "treepad/internal/sync"
)

// createWorktreeResult carries post-creation state shared by New and FromSpec.
type createWorktreeResult struct {
	RC           repoContext
	Cfg          config.Config
	WorktreePath string
	ArtifactPath string
}

// createWorktreeWithSync is the shared prologue for tp new and tp from-spec:
// resolve repo context → derive worktree path → load config → fire pre_new hook
// → git worktree add → sync configs → write artifact → fire post_new hook.
// Callers own --open and emitCD.
func createWorktreeWithSync(ctx context.Context, d Deps, branch, base, outputDir string) (createWorktreeResult, error) {
	rc, err := loadRepoContext(ctx, d, outputDir)
	if err != nil {
		return createWorktreeResult{}, err
	}

	worktreePath := filepath.Join(filepath.Dir(rc.Main.Path), rc.Slug+"-"+slug.Slug(branch))
	slog.Debug("derived worktree path", "path", worktreePath)

	hookCfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return createWorktreeResult{}, fmt.Errorf("load config: %w", err)
	}
	hData := hookData(rc.Slug, branch, worktreePath, rc.OutputDir)
	if err := runHook(ctx, d, hookCfg.Hooks, hook.PreNew, hData); err != nil {
		return createWorktreeResult{}, fmt.Errorf("pre_new hook: %w", err)
	}

	if _, err := d.Runner.Run(ctx, "git", "worktree", "add", "-b", branch, worktreePath, base); err != nil {
		return createWorktreeResult{}, fmt.Errorf("git worktree add: %w", err)
	}
	d.Log.OK("created worktree at %s", worktreePath)

	cfg, err := loadAndSync(ctx, d, rc.Main.Path, nil, []syncTarget{{path: worktreePath, branch: branch}}, rc.Slug, rc.OutputDir)
	if err != nil {
		return createWorktreeResult{}, err
	}

	artData := templateData(rc.Slug, branch, worktreePath, rc.OutputDir)
	artifactPath, err := artifact.Write(artifactSpec(cfg.Artifact), rc.OutputDir, artData)
	if err != nil {
		return createWorktreeResult{}, fmt.Errorf("write artifact: %w", err)
	}
	slog.Debug("wrote artifact", "outputDir", rc.OutputDir, "branch", branch)

	if err := runHook(ctx, d, cfg.Hooks, hook.PostNew, hData); err != nil {
		d.Log.Warn("post_new hook failed: %v", err)
	}

	return createWorktreeResult{
		RC:           rc,
		Cfg:          cfg,
		WorktreePath: worktreePath,
		ArtifactPath: artifactPath,
	}, nil
}

type syncTarget struct {
	path   string
	branch string
}

// loadAndSync loads config from sourceDir, syncs it to targets, and returns the config.
// Pass a nil targets slice to skip syncing and only load the config.
func loadAndSync(ctx context.Context, d Deps, sourceDir string, extraPatterns []string, targets []syncTarget, repoSlug, outputDir string) (config.Config, error) {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}

	if len(targets) == 0 {
		return cfg, nil
	}

	patterns := slices.Concat(cfg.Sync.Include, extraPatterns)
	slog.Debug("sync patterns", "patterns", patterns)

	d.Log.Step("syncing configs to worktrees...")
	for _, t := range targets {
		d.Log.Info("→ %s (%s)", t.branch, t.path)
		hData := hookData(repoSlug, t.branch, t.path, outputDir)
		if err := runHook(ctx, d, cfg.Hooks, hook.PreSync, hData); err != nil {
			return config.Config{}, fmt.Errorf("pre_sync hook: %w", err)
		}
		if err := d.Syncer.Sync(patterns, internalsync.Config{
			SourceDir: sourceDir,
			TargetDir: t.path,
		}); err != nil {
			return config.Config{}, fmt.Errorf("sync configs to %s: %w", t.branch, err)
		}
		slog.Debug("synced worktree", "branch", t.branch, "target", t.path)
		if err := runHook(ctx, d, cfg.Hooks, hook.PostSync, hData); err != nil {
			d.Log.Warn("post_sync hook failed: %v", err)
		}
	}
	return cfg, nil
}

// hookData constructs a hook.Data value from operation context.
// The HookType field is set by runHook when the event is known.
func hookData(slug, branch, wtPath, outputDir string) hook.Data {
	return hook.Data{
		Branch:       branch,
		WorktreePath: wtPath,
		Slug:         slug,
		OutputDir:    outputDir,
	}
}

// runHook fires the hooks for the given event. It is a no-op when no hooks are
// configured for the event. Callers control the failure semantics: pre-hooks
// should return the error to abort; post-hooks should log and continue.
func runHook(ctx context.Context, d Deps, cfg hook.Config, event hook.Event, data hook.Data) error {
	entries := cfg.For(event)
	if len(entries) == 0 {
		return nil
	}
	data.HookType = string(event)
	return d.HookRunner.Run(ctx, entries, data)
}

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

func emitCD(d Deps, path string) {
	if w := cdSentinelWriter(d); w != nil {
		_, _ = io.WriteString(w, path+"\n")
		return
	}
	_, _ = fmt.Fprintf(d.Out, "__TREEPAD_CD__\t%s\n", path)
}

// cdSentinelWriter returns a writer for the __TREEPAD_CD__ sentinel.
// The new tp shell wrapper sets TREEPAD_CD_FD=3 and redirects fd 3 into
// its $(...) capture, letting tp's stdout go to the real terminal. When the
// env var is absent (stale wrapper, direct binary invocation, tests) it
// returns nil so emitCD falls back to d.Out.
func cdSentinelWriter(d Deps) io.Writer {
	if d.CDSentinel != nil {
		return d.CDSentinel()
	}
	fdStr := os.Getenv("TREEPAD_CD_FD")
	if fdStr == "" {
		return nil
	}
	fd, err := strconv.Atoi(fdStr)
	if err != nil || fd < 0 {
		return nil
	}
	return os.NewFile(uintptr(fd), "treepad-cd")
}

// maybeWarnStaleWrapper prints a one-line stderr hint when an agent_command is
// configured but the new shell wrapper (which sets TREEPAD_CD_FD) has not been
// installed. Fires only when TREEPAD_CD_FD is unset AND stdout is not a TTY
// (i.e. inside the old wrapper's $(...) capture). No-op in all other contexts
// (new wrapper active, CI, direct binary invocation).
func maybeWarnStaleWrapper(d Deps, hasAgentCommand bool) {
	if !hasAgentCommand {
		return
	}
	if os.Getenv("TREEPAD_CD_FD") != "" {
		return
	}
	if d.IsTerminal(d.Out) {
		return
	}
	d.Log.Warn("stale shell wrapper detected — re-run: eval \"$(tp shell-init)\"")
	d.Log.Warn("Your agent will still start interactively via /dev/tty.")
}
