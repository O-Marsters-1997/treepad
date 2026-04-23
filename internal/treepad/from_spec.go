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

	"treepad/internal/config"
)

// FromSpecInput parameterises a tp from-spec invocation.
// Issue must be set to a valid GitHub issue number.
type FromSpecInput struct {
	Issue     int
	Branch    string
	Base      string
	Current   bool
	OutputDir string
	// Prompt is optional user-supplied instructions appended to the prompt body.
	// When empty, the body ends with "Implement the ticket."
	Prompt string
}

// promptData is the template context for each agent_command element.
type promptData struct {
	Spec         string
	Skills       []string
	Branch       string
	Slug         string
	WorktreePath string
	PromptPath   string
	// Prompt holds the rendered prompt body.
	Prompt string
}

// FromSpec creates a worktree seeded from a GitHub issue,
// writes PROMPT.md into the worktree, and hands off to a configured agent.
// Returns the agent's exit code (0 when no agent_command is configured).
func FromSpec(ctx context.Context, d Deps, in FromSpecInput) (int, error) {
	if in.Issue == 0 {
		return 0, errors.New("issue is required")
	}
	spec, err := resolveIssueSpec(ctx, d, in.Issue)
	if err != nil {
		return 0, err
	}

	res, err := createWorktreeWithSync(ctx, d, in.Branch, in.Base, in.OutputDir)
	if err != nil {
		return 0, err
	}

	promptPath, rendered, err := resolveOrBuildPrompt(d, res, in.Branch, spec, in.Prompt)
	if err != nil {
		return 0, err
	}

	data := promptData{
		Spec:         spec,
		Skills:       res.Cfg.FromSpec.Skills,
		Branch:       in.Branch,
		Slug:         res.RC.Slug,
		WorktreePath: res.WorktreePath,
		PromptPath:   promptPath,
		Prompt:       rendered,
	}
	maybeWarnStaleWrapper(d, len(res.Cfg.FromSpec.AgentCommand) > 0)
	code, err := runAgent(ctx, d, res.Cfg.FromSpec.AgentCommand, data)
	if err != nil {
		return code, err
	}
	if !in.Current {
		emitCD(d, res.WorktreePath)
	}
	return code, nil
}

// resolveOrBuildPrompt builds the prompt from the spec, skills, and optional user prompt,
// then written into the worktree as PROMPT.md.
func resolveOrBuildPrompt(
	d Deps,
	res createWorktreeResult,
	branch, spec, userPrompt string,
) (path, rendered string, err error) {
	promptPath := filepath.Join(res.WorktreePath, "PROMPT.md")

	if existing, readErr := os.ReadFile(promptPath); readErr == nil {
		d.Log.Info("using existing prompt at %s", promptPath)
		return promptPath, string(existing), nil
	}

	body := buildPrompt(res.Cfg.FromSpec, branch, spec, userPrompt)
	path, err = writePromptFile(d, res.WorktreePath, body)
	return path, body, err
}

// buildPrompt constructs the prompt body from fixed structure + config skills + optional user instructions.
func buildPrompt(cfg config.FromSpecConfig, branch, spec, userPrompt string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n## Spec\n%s\n", branch, spec)
	if len(cfg.Skills) > 0 {
		b.WriteString("\n## Skills\n")
		for _, s := range cfg.Skills {
			fmt.Fprintf(&b, "- /%s\n", s)
		}
	}
	if userPrompt != "" {
		fmt.Fprintf(&b, "\nImplement the ticket according to the following instructions:\n\n%s\n", userPrompt)
	} else {
		b.WriteString("\nImplement the ticket.\n")
	}
	return b.String()
}

// writePromptFile writes body to PROMPT.md inside worktreePath and returns the absolute path.
func writePromptFile(d Deps, worktreePath, body string) (string, error) {
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		return "", fmt.Errorf("create worktree dir: %w", err)
	}
	promptPath := filepath.Join(worktreePath, "PROMPT.md")
	if err := os.WriteFile(promptPath, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	d.Log.OK("wrote prompt to %s", promptPath)
	return promptPath, nil
}

// resolveIssueSpec fetches the body of a single GitHub issue.
func resolveIssueSpec(ctx context.Context, d Deps, issue int) (string, error) {
	out, err := d.Runner.Run(ctx, "gh", "issue", "view", strconv.Itoa(issue), "--json", "body", "-q", ".body")
	if err != nil {
		return "", fmt.Errorf("gh issue view %d: %w", issue, err)
	}
	body := strings.TrimSpace(string(out))
	if body == "" {
		return "", fmt.Errorf("issue %d has an empty body", issue)
	}
	return body, nil
}

// renderPrompt executes a text/template string with the given promptData.
func renderPrompt(tmpl string, data promptData) (string, error) {
	t, err := template.New("prompt").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse agent_command template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute agent_command template: %w", err)
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
