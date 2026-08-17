package fromspec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"text/template"

	"github.com/O-Marsters-1997/treepad/batch"
	"github.com/O-Marsters-1997/treepad/internal/config"
	"github.com/O-Marsters-1997/treepad/internal/profile"
	"github.com/O-Marsters-1997/treepad/internal/treepad/cd"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/lifecycle"
	"github.com/O-Marsters-1997/treepad/internal/treepad/repo"
)

// FromSpecInput parameterises a tp new --ticket invocation.
// Ticket must be set to a Ticket URL or, when the repo configures ticket_url, a bare Ref.
type FromSpecInput struct {
	Ticket    string
	Branch    string
	Base      string
	Current   bool
	OutputDir string
}

// agentData is the template context for each agent_command element.
type agentData struct {
	Branch       string
	Slug         string
	WorktreePath string
	TicketURL    string
}

// FromSpec creates a worktree seeded from a Ticket and hands off to a
// configured agent.
// Returns the agent's exit code (0 when no agent_command is configured).
func FromSpec(ctx context.Context, d deps.Deps, in FromSpecInput) (int, error) {
	p := profile.OrDisabled(d.Profiler)

	if in.Ticket == "" {
		return 0, errors.New("ticket is required")
	}

	// Resolved ahead of CreateWorktreeWithSync so an unresolvable ticket
	// leaves no worktree behind.
	resolveDone := p.Stage("ticket.resolve")
	fsCfg, err := loadFromSpecConfig(ctx, d, in.OutputDir)
	if err != nil {
		resolveDone()
		return 0, err
	}
	ticketURL, _, err := batch.ResolveTicket(fsCfg.TicketURL, in.Ticket)
	resolveDone()
	if err != nil {
		return 0, err
	}

	res, err := lifecycle.CreateWorktreeWithSync(ctx, d, in.Branch, in.Base, in.OutputDir)
	if err != nil {
		return 0, err
	}

	data := agentData{
		Branch:       in.Branch,
		Slug:         res.RC.Slug,
		WorktreePath: res.WorktreePath,
		TicketURL:    ticketURL,
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

// loadFromSpecConfig loads the repo's [from_spec] config. Loaded independently
// of lifecycle.CreateWorktreeWithSync so ticket resolution happens, and can
// fail, before any worktree is created.
func loadFromSpecConfig(ctx context.Context, d deps.Deps, outputDir string) (config.FromSpecConfig, error) {
	rc, err := repo.Load(ctx, d.Runner, outputDir)
	if err != nil {
		return config.FromSpecConfig{}, err
	}
	cfg, err := config.Load(rc.Main.Path)
	if err != nil {
		return config.FromSpecConfig{}, fmt.Errorf("load config: %w", err)
	}
	return cfg.FromSpec, nil
}

func renderTemplate(tmpl string, data agentData) (string, error) {
	t, err := template.New("agent_command").Parse(tmpl)
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
func runAgent(ctx context.Context, d deps.Deps, cmdTmpls []string, data agentData) (int, error) {
	if len(cmdTmpls) == 0 {
		d.Log.Info("no agent_command configured; worktree ready at %s", data.WorktreePath)
		return 0, nil
	}
	rendered := make([]string, len(cmdTmpls))
	for i, t := range cmdTmpls {
		s, err := renderTemplate(t, data)
		if err != nil {
			return 0, fmt.Errorf("render agent_command[%d]: %w", i, err)
		}
		rendered[i] = s
	}
	return d.PTRunner.Run(ctx, data.WorktreePath, rendered[0], rendered[1:]...)
}
