package fromspec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"treepad/internal/config"
	"treepad/internal/profile"
	"treepad/internal/treepad/cd"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/lifecycle"
	"treepad/internal/treepad/repo"
)

// FromSpecInput parameterises a tp from-spec invocation.
// Ticket must be set to a Ticket URL or, when the repo configures ticket_url, a bare Ref.
type FromSpecInput struct {
	Ticket    string
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

// FromSpec creates a worktree seeded from a Ticket,
// writes PROMPT.md into the worktree, and hands off to a configured agent.
// Returns the agent's exit code (0 when no agent_command is configured).
func FromSpec(ctx context.Context, d deps.Deps, in FromSpecInput) (int, error) {
	p := profile.OrDisabled(d.Profiler)

	if in.Ticket == "" {
		return 0, errors.New("ticket is required")
	}

	// Resolved ahead of CreateWorktreeWithSync so an unresolvable ticket
	// leaves no worktree behind.
	resolveDone := p.Stage("ticket.resolve")
	ticketURL, err := resolveTicketFromRepo(ctx, d, in.OutputDir, in.Ticket)
	resolveDone()
	if err != nil {
		return 0, err
	}
	spec := "Read the ticket at:\n" + ticketURL

	res, err := lifecycle.CreateWorktreeWithSync(ctx, d, in.Branch, in.Base, in.OutputDir)
	if err != nil {
		return 0, err
	}

	promptDone := p.Stage("prompt.write")
	promptPath, rendered, err := resolveOrBuildPrompt(d, res, in.Branch, spec, in.Prompt)
	promptDone()
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
	cd.MaybeWarnStaleWrapper(d, len(res.Cfg.FromSpec.AgentCommand) > 0)
	agentDone := p.Stage("agent.run")
	code, err := runAgent(ctx, d, res.Cfg.FromSpec.AgentCommand, data)
	agentDone()
	if err != nil {
		return code, err
	}
	if !in.Current {
		cdDone := p.Stage("cd.emit")
		cd.EmitCD(d, res.WorktreePath)
		cdDone()
	}
	return code, nil
}

func resolveOrBuildPrompt(
	d deps.Deps,
	res lifecycle.CreateResult,
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

func writePromptFile(d deps.Deps, worktreePath, body string) (string, error) {
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

// resolveTicketFromRepo loads the repo's from_spec config and resolves ticket
// to a Ticket URL. Loaded independently of lifecycle.CreateWorktreeWithSync so
// resolution happens, and can fail, before any worktree is created.
func resolveTicketFromRepo(ctx context.Context, d deps.Deps, outputDir, ticket string) (string, error) {
	rc, err := repo.Load(ctx, d.Runner, outputDir)
	if err != nil {
		return "", err
	}
	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	ticketURL, _, err := resolveTicket(cfg.FromSpec, ticket)
	return ticketURL, err
}

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

// runAgent returns 0 with no error when agent_command is empty.
func runAgent(ctx context.Context, d deps.Deps, cmdTmpls []string, data promptData) (int, error) {
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
