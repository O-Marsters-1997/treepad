package hook

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"text/template"
)

// CommandRunner executes a system command.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner renders each hook command as a text/template and executes it via sh -c.
type ExecRunner struct {
	Runner CommandRunner
}

// Run executes each hook sequentially, stopping on the first error.
func (e ExecRunner) Run(ctx context.Context, hooks []string, data Data) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("hooks are not supported on Windows")
	}
	for _, tmpl := range hooks {
		rendered, err := renderCommand(tmpl, data)
		if err != nil {
			return fmt.Errorf("render hook %q: %w", tmpl, err)
		}
		if _, err := e.Runner.Run(ctx, "sh", "-c", rendered); err != nil {
			return fmt.Errorf("hook %q: %w", rendered, err)
		}
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
