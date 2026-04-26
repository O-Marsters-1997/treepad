package cd

import (
	"context"

	"treepad/internal/treepad/cdshell"
	"treepad/internal/treepad/deps"
)

type BaseInput struct {
	// Cwd overrides os.Getwd for testing.
	Cwd string
}

func Base(ctx context.Context, d deps.Deps, in BaseInput) error {
	return cdshell.Base(ctx, cdshellDeps(d), cdshell.BaseInput{Cwd: in.Cwd})
}
