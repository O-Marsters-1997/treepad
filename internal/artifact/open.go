package artifact

import (
	"context"
	"fmt"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// OpenSpec describes how to open an artifact or worktree path.
// Each element of Command is a text/template string rendered against OpenData.
type OpenSpec struct {
	Command []string
}

// IsZero reports whether no open command is configured.
func (o OpenSpec) IsZero() bool {
	return len(o.Command) == 0
}

// OpenData is the context available when rendering open command templates.
type OpenData struct {
	ArtifactPath string   // absolute path of the artifact; worktree path if no artifact was generated
	Worktree     Worktree // the specific worktree being opened
}

type Opener interface {
	Open(ctx context.Context, spec OpenSpec, data OpenData) error
}

type ExecOpener struct {
	Runner CommandRunner
}

func (e ExecOpener) Open(ctx context.Context, spec OpenSpec, data OpenData) error {
	if spec.IsZero() {
		return nil
	}
	rendered := make([]string, len(spec.Command))
	for i, tmpl := range spec.Command {
		s, err := renderString("open command", tmpl, data)
		if err != nil {
			return fmt.Errorf("render open command arg %d: %w", i, err)
		}
		rendered[i] = s
	}
	_, err := e.Runner.Run(ctx, rendered[0], rendered[1:]...)
	return err
}
