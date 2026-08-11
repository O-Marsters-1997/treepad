package hook

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"text/template"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// TTYRunner executes a command with the caller's terminal inherited, returning
// the child's exit code. A non-zero exit is reported through the code, not the
// error. Satisfied by passthrough.Runner.
type TTYRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) (int, error)
}

// ExecRunner renders each hook command as a text/template and executes it via sh -c.
type ExecRunner struct {
	Runner CommandRunner
	TTY    TTYRunner
}

// Run executes each hook entry sequentially, skipping entries whose branch
// filters do not match data.Branch, stopping on the first error.
func (e ExecRunner) Run(ctx context.Context, hooks []HookEntry, data Data) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("hooks are not supported on Windows")
	}
	for _, entry := range hooks {
		if !shouldRun(entry, data.Branch) {
			continue
		}
		rendered, err := renderCommand(entry.Command, data)
		if err != nil {
			return fmt.Errorf("render hook %q: %w", entry.Command, err)
		}
		if err := e.run(ctx, entry, rendered); err != nil {
			return fmt.Errorf("hook %q: %w", rendered, err)
		}
	}
	return nil
}

// run dispatches one rendered command. Interactive entries go through the TTY
// runner with an empty dir so they inherit tp's working directory, matching
// non-interactive hooks.
func (e ExecRunner) run(ctx context.Context, entry HookEntry, rendered string) error {
	if !entry.Interactive {
		_, err := e.Runner.Run(ctx, "sh", "-c", rendered)
		return err
	}
	code, err := e.TTY.Run(ctx, "", "sh", "-c", rendered)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exit status %d", code)
	}
	return nil
}

func renderCommand(tmpl string, data Data) (string, error) {
	t, err := template.New("hook").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute: %w", err)
	}
	return buf.String(), nil
}
