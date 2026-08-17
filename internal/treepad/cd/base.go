package cd

import (
	"context"

	"github.com/O-Marsters-1997/treepad/internal/treepad/cdshell"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
)

type BaseInput struct {
	// Cwd overrides os.Getwd for testing.
	Cwd string
}

func Base(ctx context.Context, d deps.Deps, in BaseInput) error {
	return cdshell.Base(ctx, cdshellDeps(d), cdshell.BaseInput{Cwd: in.Cwd})
}
