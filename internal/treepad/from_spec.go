package treepad

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

// FromSpecInput parameterises a tp from-spec invocation.
// Exactly one of Issue or File must be set.
type FromSpecInput struct {
	Issue     int
	File      string
	Branch    string
	Base      string
	Current   bool
	OutputDir string
}

// promptData is the template context for both the prompt body and each
// agent_command element.
type promptData struct {
	Spec         string
	Skills       []string
	Branch       string
	Slug         string
	WorktreePath string
	PromptPath   string
	// Prompt holds the rendered prompt body; populated only for agent_command templates.
	Prompt string
}

// FromSpec creates a worktree seeded from a spec (GitHub issue or local file),
// renders a prompt into it, and hands off to a configured agent.
// Returns the agent's exit code (0 when no agent_command is configured).
func FromSpec(ctx context.Context, d Deps, in FromSpecInput) (int, error) {
	spec, err := resolveSpec(ctx, d, in.Issue, in.File)
	if err != nil {
		return 0, err
	}

	res, err := createWorktreeWithSync(ctx, d, in.Branch, in.Base, in.OutputDir)
	if err != nil {
		return 0, err
	}

	filename := res.Cfg.FromSpec.PromptFilename
	if filename == "" {
		filename = "PROMPT.md"
	}
	promptPath := filepath.Join(res.WorktreePath, filename)

	if res.Cfg.FromSpec.PromptTemplate == "" {
		return 0, errors.New("from_spec.prompt_template not set in .treepad.toml; run `tp config init` to scaffold a default")
	}

	data := promptData{
		Spec:         spec,
		Skills:       res.Cfg.FromSpec.Skills,
		Branch:       in.Branch,
		Slug:         res.RC.Slug,
		WorktreePath: res.WorktreePath,
		PromptPath:   promptPath,
	}
	rendered, err := renderPrompt(res.Cfg.FromSpec.PromptTemplate, data)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(res.WorktreePath, 0o755); err != nil {
		return 0, fmt.Errorf("create worktree dir: %w", err)
	}
	if err := os.WriteFile(promptPath, []byte(rendered), 0o644); err != nil {
		return 0, fmt.Errorf("write prompt: %w", err)
	}
	d.Log.OK("wrote prompt to %s", promptPath)

	data.Prompt = rendered
	code, err := runAgent(ctx, d, res.Cfg.FromSpec.AgentCommand, data)
	if err != nil {
		return code, err
	}
	if !in.Current {
		emitCD(d, res.WorktreePath)
	}
	return code, nil
}

// resolveSpec returns the raw spec body from either a GitHub issue or a local file.
func resolveSpec(ctx context.Context, d Deps, issue int, file string) (string, error) {
	switch {
	case issue > 0:
		out, err := d.Runner.Run(ctx, "gh", "issue", "view", strconv.Itoa(issue), "--json", "body", "-q", ".body")
		if err != nil {
			return "", fmt.Errorf("gh issue view %d: %w", issue, err)
		}
		body := strings.TrimSpace(string(out))
		if body == "" {
			return "", fmt.Errorf("issue %d has an empty body", issue)
		}
		return body, nil
	case file != "":
		path := file
		if !filepath.IsAbs(path) {
			abs, err := filepath.Abs(path)
			if err != nil {
				return "", fmt.Errorf("resolve spec path: %w", err)
			}
			path = abs
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read spec %s: %w", path, err)
		}
		body := strings.TrimSpace(string(data))
		if body == "" {
			return "", fmt.Errorf("spec file %s is empty", path)
		}
		return body, nil
	default:
		return "", errors.New("either --issue or --file is required")
	}
}

// renderPrompt executes a text/template string with the given promptData.
func renderPrompt(tmpl string, data promptData) (string, error) {
	t, err := template.New("prompt").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse prompt_template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute prompt_template: %w", err)
	}
	return buf.String(), nil
}

// runAgent renders agent_command templates and invokes the passthrough runner.
// Returns 0 with no error when agent_command is empty.
func runAgent(ctx context.Context, d Deps, cmdTmpls []string, data promptData) (int, error) {
	if len(cmdTmpls) == 0 {
		d.Log.Info("no agent_command configured; prompt written to %s", data.PromptPath)
		return 0, nil
	}
	rendered := make([]string, len(cmdTmpls))
	for i, t := range cmdTmpls {
		s, err := renderPrompt(t, data)
		if err != nil {
			return 0, fmt.Errorf("render agent_command[%d]: %w", i, err)
		}
		rendered[i] = s
	}
	return d.PTRunner.Run(ctx, data.WorktreePath, rendered[0], rendered[1:]...)
}
