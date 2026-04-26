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

// ExecRunner renders each hook command as a text/template and executes it via sh -c.
type ExecRunner struct {
	Runner CommandRunner
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
