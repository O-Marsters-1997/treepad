package cd

import (
	"context"

	"github.com/O-Marsters-1997/treepad/internal/treepad/cdshell"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
)

type CDInput struct {
	Branch string
}

func CD(ctx context.Context, d deps.Deps, in CDInput) error {
	return cdshell.CD(ctx, cdshellDeps(d), cdshell.CDInput{Branch: in.Branch})
}
